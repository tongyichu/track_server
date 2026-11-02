package admin

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/utils"

	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

// staticFS 内置管理后台前端静态资源。
// 目录布局：internal/admin/static/<*.html|*.js|*.css>
//
//go:embed static
var staticFS embed.FS

var embeddedGzipCache sync.Map

// Module 是管理后台对外暴露的注入入口，封装了 auth + handler + session store。
type Module struct {
	Auth    *Authenticator
	Handler *Handler
	Store   *SessionStore
}

// NewModule 根据配置构造 Module。
//
// accounts 仅用于首次导入；已有持久化账号不依赖环境变量继续存在。
// 无初始账号且数据库未初始化时 Auth 为 nil；账号库不可用则拒绝登录与受保护请求。
//
// staticRoot 为服务端本地静态资源根目录（通常是 <LogDir>/static）。
// 管理后台上传的安装包会落盘到 <staticRoot>/release/<platform>/，
// 并通过 /api/v1/static/release/<platform>/<file> 对外下发。
// 留空表示禁用本地上传接口（接口会返回 500）。
func NewModule(accounts map[string]string, releaseSvc *service.AppReleaseService, stsSvc *service.OSSTokenService, staticRoot string, userRepo repository.UserRepository, trackRepo repository.TrackRepository, collectRepo repository.CollectRepository, trackMapRepo repository.TrackMapRepository, companionRepo repository.CompanionRepository, analyticsRepo repository.AnalyticsRepository, userSvc *service.UserService, feedbackSvc *service.FeedbackService, restrictionSvc *service.AccountRestrictionService, routeGroupSvc *service.TrackRouteGroupService, storage AccountStorage) *Module {
	store := NewSessionStore(12 * time.Hour)
	var auth *Authenticator
	if len(accounts) > 0 || storage != nil {
		manager := &AccountManager{storage: storage, bootstrap: accounts}
		auth = &Authenticator{store: store, manager: manager}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if storage != nil {
			version, _, err := storage.Load(ctx)
			if err == nil && version == 0 && len(accounts) == 0 {
				auth = nil
			}
		}
		if auth != nil {
			if _, _, err := manager.load(ctx); err != nil {
				log.Printf("admin accounts unavailable; login and protected requests will be rejected")
			}
		}
		cancel()
	}
	handler := NewHandler(releaseSvc, stsSvc, auth, staticRoot, userRepo, trackRepo, collectRepo, trackMapRepo, companionRepo, analyticsRepo, userSvc, feedbackSvc, restrictionSvc, routeGroupSvc)
	return &Module{
		Auth:    auth,
		Handler: handler,
		Store:   store,
	}
}

// Close 释放 Module 持有的资源（停止 GC 协程）。
func (m *Module) Close() {
	if m == nil || m.Store == nil {
		return
	}
	if m.Auth != nil && m.Auth.recovery != nil {
		m.Auth.recovery.close()
	}
	m.Store.Close()
}

// RegisterRoutes 在 Hertz 上注册管理后台所有路由（前端页面 + API）。
//
// - GET /admin/                       重定向到 /admin/login.html
// - GET /admin/login.html             登录页（公开）
// - GET /admin/index.html             管理首页（返回 HTML 前校验会话）
// - GET /admin/dashboard.html         即时运营工作台（返回 HTML 前校验会话）
// - GET /admin/static/*               其它静态资源（公开）
// - POST /admin/api/login             登录
// - POST /admin/api/logout            退出
// - GET  /admin/api/me                查询当前会话
// - GET  /admin/api/releases          列出版本（鉴权）
// - POST /admin/api/releases          发布版本（鉴权）
// - DELETE /admin/api/releases/:id    删除版本并尽力删除本机安装包文件，changelog 留存（鉴权）
// - GET  /admin/api/releases/changelogs 独立留存的版本更新日志（鉴权）
// - POST /admin/api/releases/upload-package 上传安装包到本机静态目录（鉴权）
// - GET  /admin/api/releases/upload-token  旧 OSS 直传凭证接口（鉴权，前端不再使用）
// - GET  /admin/api/users                  用户列表（鉴权，cursor 翻页）
// - GET  /admin/api/users/statistics       用户资料分布与轨迹城市统计（鉴权）
// - POST /admin/api/users/:user_id/restrictions 创建账号限制（鉴权）
// - DELETE /admin/api/users/:user_id/restrictions/current 解除当前账号限制（鉴权）
// - GET  /admin/api/tracks                 轨迹列表（鉴权，cursor 翻页）
// - GET  /admin/api/cities                 城市 Code 与名称选项（鉴权）
// - GET  /admin/api/tracks/:track_id       轨迹详情（鉴权）
// - PUT  /admin/api/tracks/:track_id       更新轨迹业务字段（鉴权）
// - DELETE /admin/api/tracks/:track_id     删除轨迹（鉴权，软删除并清理关联索引/收藏）
// - POST /admin/api/tracks/:track_id/restore 恢复已删除轨迹（鉴权，还原公开并重新排队地图索引）
// - POST /admin/api/tracks/:track_id/purge  彻底删除已下架轨迹（鉴权，物理删除 DB 关联数据并尽力清 OSS 文件）
// - GET  /admin/api/companions             同行会话列表（鉴权，cursor 翻页）
// - GET  /admin/api/companions/statistics  同行会话运营统计（鉴权）
// - GET  /admin/api/companions/usage-statistics  同行趋势与实际人数分布（鉴权）
// - GET  /admin/api/route-groups/statistics  全量有效路线组覆盖与规模分布（鉴权）
// - GET  /admin/api/companions/:session_id  同行会话详情：会话/成员/位置/弹幕（鉴权）
// - GET  /admin/api/feedbacks              意见反馈列表（鉴权，cursor 翻页）
// - GET  /admin/api/feedbacks/:feedback_id 意见反馈详情（鉴权）
// - PUT  /admin/api/feedbacks/:feedback_id/status 处理意见反馈（鉴权）
// - GET  /admin/api/feedbacks/:feedback_id/images/:image_id 读取反馈图片（鉴权）
// - GET  /admin/api/analytics/sync-summaries       埋点 OSS 同步摘要列表（鉴权）
// - GET  /admin/api/static/*filepath               后台静态资源代理（鉴权）
//
// 当 m == nil 或 m.Auth == nil 时，整个 /admin 不会被挂载。
func (m *Module) RegisterRoutes(h *server.Hertz) {
	if m == nil || m.Auth == nil {
		return
	}

	g := h.Group("/admin")

	// 入口跳转
	g.GET("/", redirectToLogin)
	g.GET("", redirectToLogin)

	// 静态资源：直接从 embed.FS 读出来返回
	g.GET("/login.html", serveEmbedded("static/login.html", "text/html; charset=utf-8"))
	g.GET("/forgot-password.html", serveEmbedded("static/forgot_password.html", "text/html; charset=utf-8"))
	g.GET("/index.html", serveAdminPage(m.Auth, "static/index.html"))
	g.GET("/dashboard.html", serveAdminPage(m.Auth, "static/dashboard.html"))
	g.GET("/more.html", serveAdminPage(m.Auth, "static/more.html"))
	g.GET("/users.html", serveAdminPage(m.Auth, "static/users.html"))
	g.GET("/tracks.html", serveAdminPage(m.Auth, "static/tracks.html"))
	g.GET("/scenic_spots.html", serveAdminPage(m.Auth, "static/scenic_spots.html"))
	g.GET("/route_groups.html", serveAdminPage(m.Auth, "static/route_groups.html"))
	g.GET("/track_submissions.html", serveAdminPage(m.Auth, "static/track_submissions.html"))
	g.GET("/companions.html", serveAdminPage(m.Auth, "static/companions.html"))
	g.GET("/companion_detail.html", serveAdminPage(m.Auth, "static/companion_detail.html"))
	g.GET("/feedbacks.html", serveAdminPage(m.Auth, "static/feedbacks.html"))
	g.GET("/analytics.html", serveAdminPage(m.Auth, "static/analytics.html"))
	g.GET("/accounts.html", serveAdminPage(m.Auth, "static/accounts.html"))
	g.GET("/password.html", serveAdminPage(m.Auth, "static/password.html"))
	g.GET("/access.html", serveAdminPage(m.Auth, "static/access.html"))
	g.GET("/static/*filepath", serveEmbeddedDir())

	// 公开 API
	g.POST("/api/login", m.Auth.HandleLogin)
	g.POST("/api/logout", m.Auth.HandleLogout)
	g.GET("/api/me", m.Auth.HandleMe)
	g.GET("/api/password-recovery/status", m.Auth.HandleRecoveryStatus)
	g.POST("/api/password-recovery/request", m.Auth.HandleForgotPassword)
	g.POST("/api/password-recovery/complete", m.Auth.HandleCompleteRecovery)

	// 鉴权 API
	api := g.Group("/api", m.Auth.AuthMiddleware())
	api.GET("/accounts", m.Auth.HandleAccounts)
	api.POST("/accounts", m.Auth.HandleSaveAccount)
	api.PUT("/accounts/:username", m.Auth.HandleSaveAccount)
	api.POST("/accounts/:username/reset-password", m.Auth.HandleResetPassword)
	api.POST("/password", m.Auth.HandleChangePassword)
	api.GET("/security", m.Auth.HandleSecurity)
	api.POST("/security/email/current-code", m.Auth.HandleSendEmailCode)
	api.POST("/security/email/send-code", m.Auth.HandleSendEmailCode)
	api.POST("/security/email/verify", m.Auth.HandleVerifyEmail)
	api.GET("/audit-logs", m.Auth.HandleAuditLogs)
	api.GET("/releases", m.Handler.ListReleases)
	api.POST("/releases", m.Handler.PublishRelease)
	api.DELETE("/releases/:id", m.Handler.DeleteRelease)
	api.GET("/releases/changelogs", m.Handler.ListReleaseChangelogs)
	api.GET("/releases/upload-token", m.Handler.GetReleaseUploadCredential)
	api.GET("/releases/client-statistics", m.Handler.GetReleaseClientStatistics)
	api.POST("/releases/upload-package", m.Handler.UploadPackage)
	api.GET("/dashboard", m.Handler.GetDashboard)
	api.GET("/users/statistics", m.Handler.GetAdminUserStatistics)
	api.GET("/companions/statistics", m.Handler.GetAdminCompanionStatistics)
	api.GET("/companions/usage-statistics", m.Handler.GetAdminCompanionUsageStatistics)
	api.GET("/route-groups/statistics", m.Handler.GetAdminMapModeStatistics)

	// 用户 / 轨迹 / 同行 列表（仅查询，提供基础翻页）
	api.GET("/static/*filepath", m.Handler.GetStaticAsset)
	api.GET("/scenic-spots", m.Handler.ListScenicSpots)
	api.GET("/scenic-spots/statistics", m.Handler.GetScenicSpotStatistics)
	api.GET("/scenic-spots/map-config", m.Handler.GetScenicSpotMapConfig)
	api.GET("/scenic-spots/amap-service/*filepath", m.Handler.ProxyScenicSpotMapService)
	api.POST("/scenic-spots/provider-content/backfill", m.Handler.BackfillScenicSpotProviderContent)
	api.POST("/scenic-spots/description/generate", m.Handler.GenerateScenicSpotDescription)
	api.POST("/scenic-spots", m.Handler.CreateScenicSpot)
	api.POST("/scenic-spots/backfill", m.Handler.BackfillScenicSpots)
	api.GET("/scenic-spots/track-search", m.Handler.SearchScenicSpotTracks)
	api.GET("/scenic-spots/:spot_id/map", m.Handler.GetScenicSpotMap)
	api.GET("/scenic-spots/:spot_id", m.Handler.GetScenicSpot)
	api.PUT("/scenic-spots/:spot_id", m.Handler.UpdateScenicSpot)
	api.POST("/scenic-spots/:spot_id/status", m.Handler.SetScenicSpotStatus)
	api.POST("/scenic-spots/:spot_id/provider-sources/verify", m.Handler.VerifyScenicSpotSource)
	api.POST("/scenic-spots/:spot_id/provider-sources/preview", m.Handler.PreviewScenicSpotSource)
	api.POST("/scenic-spots/:spot_id/photos", m.Handler.UploadScenicSpotPhoto)
	api.GET("/scenic-spots/:spot_id/tracks", m.Handler.ListScenicSpotTracks)
	api.POST("/scenic-spots/:spot_id/tracks", m.Handler.AssignScenicSpotTrack)
	api.DELETE("/scenic-spots/:spot_id/tracks/:track_id", m.Handler.UnassignScenicSpotTrack)
	api.POST("/scenic-spots/:spot_id/merge", m.Handler.MergeScenicSpots)
	api.POST("/scenic-spots/:spot_id/split", m.Handler.SplitScenicSpot)
	api.GET("/scenic-spots/:spot_id/audit", m.Handler.ListScenicSpotAudit)
	api.GET("/place-candidates", m.Handler.ListPlaceCandidates)
	api.POST("/place-candidates/:track_id/confirm", m.Handler.ConfirmPlaceCandidate)
	api.POST("/place-candidates/:track_id/dismiss", m.Handler.DismissPlaceCandidate)
	api.GET("/users", m.Handler.ListUsers)
	api.GET("/users/:user_id", m.Handler.GetUserProfile)
	api.GET("/users/:user_id/permission-snapshots", m.Handler.ListUserPermissionSnapshots)
	api.PUT("/users/:user_id", m.Handler.UpdateUserProfile)
	api.PUT("/users/:user_id/internal", m.Handler.UpdateUserInternal)
	api.GET("/users/:user_id/restrictions/current", m.Handler.GetCurrentAccountRestriction)
	api.POST("/users/:user_id/restrictions", m.Handler.CreateAccountRestriction)
	api.DELETE("/users/:user_id/restrictions/current", m.Handler.RevokeCurrentAccountRestriction)
	api.GET("/tracks", m.Handler.ListTracks)
	api.POST("/tracks/import", m.Handler.ImportTracks)
	api.GET("/tracks/map-config", m.Handler.GetTrackMapConfig)
	api.GET("/tracks/amap-service", m.Handler.ProxyScenicSpotMapService)
	api.GET("/cities", m.Handler.ListAdminCities)
	api.GET("/tracks/:track_id", m.Handler.GetTrack)
	api.GET("/tracks/:track_id/map-preview", m.Handler.GetTrackMapPreview)
	api.POST("/tracks/:track_id/place-refresh", m.Handler.RefreshTrackPlaceCandidate)
	api.GET("/tracks/:track_id/place-candidates", m.Handler.GetPlaceCandidate)
	api.PUT("/tracks/:track_id", m.Handler.UpdateTrack)
	api.DELETE("/tracks/:track_id", m.Handler.DeleteTrack)
	api.POST("/tracks/:track_id/restore", m.Handler.RestoreTrack)
	api.POST("/tracks/:track_id/purge", m.Handler.PurgeTrack)
	api.GET("/track-submissions", m.Handler.ListTrackSubmissions)
	api.GET("/track-submissions/:submission_id", m.Handler.GetTrackSubmission)
	api.POST("/track-submissions/:submission_id/review", m.Handler.ReviewTrackSubmission)
	api.GET("/route-groups", m.Handler.ListRouteGroups)
	api.GET("/route-groups/:group_id", m.Handler.GetRouteGroup)
	api.PUT("/route-groups/:group_id/name", m.Handler.RenameRouteGroup)
	api.POST("/route-groups/:group_id/merge", m.Handler.MergeRouteGroup)
	api.DELETE("/route-groups/:group_id/members/:track_id", m.Handler.RemoveRouteGroupMember)
	api.PUT("/route-groups/:group_id/representative", m.Handler.SetRouteGroupRepresentative)
	api.GET("/route-groups/:group_id/introduction", m.Handler.GetRouteGroupIntroduction)
	api.PUT("/route-groups/:group_id/introduction", m.Handler.SaveRouteGroupIntroduction)
	api.POST("/route-groups/:group_id/introduction/publish", m.Handler.PublishRouteGroupIntroduction)
	api.POST("/route-groups/:group_id/introduction/unpublish", m.Handler.UnpublishRouteGroupIntroduction)
	api.GET("/companions", m.Handler.ListCompanions)
	api.GET("/companions/:session_id", m.Handler.GetCompanionDetail)
	api.GET("/feedbacks", m.Handler.ListFeedbacks)
	api.GET("/feedback-correction-config", m.Handler.GetFeedbackCorrectionConfig)
	api.PUT("/feedback-correction-config", m.Handler.UpdateFeedbackCorrectionConfig)
	api.GET("/feedbacks/:feedback_id", m.Handler.GetFeedback)
	api.PUT("/feedbacks/:feedback_id/status", m.Handler.UpdateFeedbackStatus)
	api.GET("/feedbacks/:feedback_id/images/:image_id", m.Handler.GetFeedbackImage)
	api.GET("/analytics/sync-summaries", m.Handler.ListAnalyticsSyncSummaries)
}

func redirectToLogin(_ context.Context, c *app.RequestContext) {
	c.Redirect(http.StatusFound, []byte("/admin/login.html"))
}

func serveEmbedded(name, contentType string) app.HandlerFunc {
	return func(_ context.Context, c *app.RequestContext) {
		data, err := embeddedPage(name)
		if err != nil {
			c.JSON(http.StatusNotFound, utils.H{"error": "not found"})
			return
		}
		serveEmbeddedAsset(c, contentType, data, "no-cache")
	}
}

func serveAdminPage(auth *Authenticator, name string) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		c.Response.Header.Set("Cache-Control", "no-store")
		sess, err := auth.session(ctx, c)
		if err != nil {
			status, _ := accountErrorStatus(err)
			if status == 401 {
				c.Redirect(http.StatusFound, []byte("/admin/login.html"))
			} else {
				c.Data(status, "text/plain; charset=utf-8", []byte("后台账号库暂不可用，请稍后重试"))
			}
			return
		}
		if sess.Account != nil {
			if sess.Account.MustChangePassword && name != "static/password.html" {
				c.Redirect(http.StatusFound, []byte("/admin/password.html"))
				return
			}
			module := pageModule(name)
			commonPage := name == "static/password.html" || name == "static/access.html" || name == "static/more.html"
			if !commonPage && (module == "" || !sess.Account.allows(module, false)) {
				c.Data(http.StatusForbidden, "text/html; charset=utf-8", []byte(`<!doctype html><html lang="zh-CN"><meta charset="utf-8"><title>无权限</title><p>没有该页面的查看权限。</p><a href="/admin/access.html">返回后台</a></html>`))
				return
			}
		}
		data, err := embeddedPage(name)
		if err != nil {
			c.JSON(404, utils.H{"error": "not found"})
			return
		}
		identity, _ := json.Marshal(sessionView(sess))
		injection := append([]byte(`<script id="admin-session" type="application/json">`), identity...)
		injection = append(injection, []byte("</script></head>")...)
		data = bytes.Replace(data, []byte("</head>"), injection, 1)
		servePrivateHTML(c, data)
	}
}

// serveEmbeddedDir 处理 /admin/static/* 下的任意文件请求。
func serveEmbeddedDir() app.HandlerFunc {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// fallback：编译期就不会发生，写在这里只是为了消除空指针风险
		return func(_ context.Context, c *app.RequestContext) {
			c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		}
	}
	return func(_ context.Context, c *app.RequestContext) {
		raw := c.Param("filepath")
		raw = strings.TrimPrefix(raw, "/")
		if raw == "" || strings.Contains(raw, "..") {
			c.JSON(http.StatusBadRequest, utils.H{"error": "invalid path"})
			return
		}
		// 页面只能从经过鉴权和权限检查的具名路由访问。
		if strings.EqualFold(path.Ext(raw), ".html") {
			c.JSON(http.StatusNotFound, utils.H{"error": "not found"})
			return
		}
		data, err := fs.ReadFile(sub, raw)
		if err != nil {
			c.JSON(http.StatusNotFound, utils.H{"error": "not found"})
			return
		}
		cacheControl := "public, max-age=0, must-revalidate"
		if c.Query("v") == adminAssetVersion() {
			cacheControl = "public, max-age=31536000, immutable"
		}
		serveEmbeddedAsset(c, contentTypeFor(raw), data, cacheControl)
	}
}

func serveEmbeddedAsset(c *app.RequestContext, contentType string, data []byte, cacheControl string) {
	hash := sha256.Sum256(data)
	etag := `W/"` + hex.EncodeToString(hash[:]) + `"`
	compressible := isCompressibleContentType(contentType)
	if compressible {
		c.Response.Header.Set("Vary", "Accept-Encoding")
	}
	c.Response.Header.Set("Cache-Control", cacheControl)
	c.Response.Header.Set("ETag", etag)

	if matchesETag(string(c.Request.Header.Peek("If-None-Match")), etag) {
		c.Response.SetStatusCode(http.StatusNotModified)
		return
	}

	if compressible && acceptsGzip(c.Request.Header.Peek("Accept-Encoding")) {
		compressed, err := gzipEmbeddedAsset(etag, data)
		if err == nil && len(compressed) < len(data) {
			c.Response.Header.Set("Content-Encoding", "gzip")
			c.Data(http.StatusOK, contentType, compressed)
			return
		}
	}
	c.Data(http.StatusOK, contentType, data)
}

func isCompressibleContentType(contentType string) bool {
	contentType = strings.ToLower(contentType)
	return strings.HasPrefix(contentType, "text/") ||
		strings.Contains(contentType, "javascript") ||
		strings.Contains(contentType, "json") ||
		strings.Contains(contentType, "svg+xml")
}

func acceptsGzip(header []byte) bool {
	for _, encoding := range strings.Split(string(header), ",") {
		parts := strings.Split(strings.TrimSpace(encoding), ";")
		if !strings.EqualFold(strings.TrimSpace(parts[0]), "gzip") {
			continue
		}
		for _, parameter := range parts[1:] {
			keyValue := strings.SplitN(strings.TrimSpace(parameter), "=", 2)
			if len(keyValue) == 2 && strings.EqualFold(strings.TrimSpace(keyValue[0]), "q") {
				quality, err := strconv.ParseFloat(strings.TrimSpace(keyValue[1]), 64)
				if err == nil && quality <= 0 {
					return false
				}
			}
		}
		return true
	}
	return false
}

func matchesETag(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || candidate == etag || strings.TrimPrefix(candidate, "W/") == strings.TrimPrefix(etag, "W/") {
			return true
		}
	}
	return false
}

func gzipEmbeddedAsset(etag string, data []byte) ([]byte, error) {
	if cached, ok := embeddedGzipCache.Load(etag); ok {
		return cached.([]byte), nil
	}
	compressed, err := gzipBytes(data)
	if err != nil {
		return nil, err
	}
	cached, _ := embeddedGzipCache.LoadOrStore(etag, compressed)
	return cached.([]byte), nil
}

func gzipBytes(data []byte) ([]byte, error) {
	var output bytes.Buffer
	writer := gzip.NewWriter(&output)
	if _, err := writer.Write(data); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func contentTypeFor(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".ico":
		return "image/x-icon"
	default:
		return "application/octet-stream"
	}
}
