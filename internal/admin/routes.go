package admin

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
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
// 当 accounts 为空时，返回的 Module.Auth 为 nil，
// RegisterRoutes 会跳过路由注册。
//
// staticRoot 为服务端本地静态资源根目录（通常是 <LogDir>/static）。
// 管理后台上传的安装包会落盘到 <staticRoot>/release/<platform>/，
// 并通过 /api/v1/static/release/<platform>/<file> 对外下发。
// 留空表示禁用本地上传接口（接口会返回 500）。
func NewModule(accounts map[string]string, releaseSvc *service.AppReleaseService, stsSvc *service.OSSTokenService, staticRoot string, userRepo repository.UserRepository, trackRepo repository.TrackRepository, collectRepo repository.CollectRepository, trackMapRepo repository.TrackMapRepository, companionRepo repository.CompanionRepository, analyticsRepo repository.AnalyticsRepository, userSvc *service.UserService, feedbackSvc *service.FeedbackService, restrictionSvc *service.AccountRestrictionService, routeGroupSvc *service.TrackRouteGroupService) *Module {
	store := NewSessionStore(12 * time.Hour)
	auth := NewAuthenticator(accounts, store)
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
// - DELETE /admin/api/releases/:id    删除版本（鉴权）
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
// - GET  /admin/api/companions             同行会话列表（鉴权，cursor 翻页）
// - GET  /admin/api/companions/statistics  同行会话运营统计（鉴权）
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
	g.GET("/static/*filepath", serveEmbeddedDir())

	// 公开 API
	g.POST("/api/login", m.Auth.HandleLogin)
	g.POST("/api/logout", m.Auth.HandleLogout)
	g.GET("/api/me", m.Auth.HandleMe)

	// 鉴权 API
	api := g.Group("/api", m.Auth.AuthMiddleware())
	api.GET("/releases", m.Handler.ListReleases)
	api.POST("/releases", m.Handler.PublishRelease)
	api.DELETE("/releases/:id", m.Handler.DeleteRelease)
	api.GET("/releases/upload-token", m.Handler.GetReleaseUploadCredential)
	api.POST("/releases/upload-package", m.Handler.UploadPackage)
	api.GET("/dashboard", m.Handler.GetDashboard)
	api.GET("/users/statistics", m.Handler.GetAdminUserStatistics)
	api.GET("/companions/statistics", m.Handler.GetAdminCompanionStatistics)

	// 用户 / 轨迹 / 同行 列表（仅查询，提供基础翻页）
	api.GET("/static/*filepath", m.Handler.GetStaticAsset)
	api.GET("/scenic-spots", m.Handler.ListScenicSpots)
	api.GET("/scenic-spots/statistics", m.Handler.GetScenicSpotStatistics)
	api.POST("/scenic-spots/provider-content/backfill", m.Handler.BackfillScenicSpotProviderContent)
	api.POST("/scenic-spots", m.Handler.CreateScenicSpot)
	api.POST("/scenic-spots/backfill", m.Handler.BackfillScenicSpots)
	api.GET("/scenic-spots/track-search", m.Handler.SearchScenicSpotTracks)
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
	api.GET("/users", m.Handler.ListUsers)
	api.GET("/users/:user_id/restrictions/current", m.Handler.GetCurrentAccountRestriction)
	api.POST("/users/:user_id/restrictions", m.Handler.CreateAccountRestriction)
	api.DELETE("/users/:user_id/restrictions/current", m.Handler.RevokeCurrentAccountRestriction)
	api.GET("/tracks", m.Handler.ListTracks)
	api.GET("/cities", m.Handler.ListAdminCities)
	api.GET("/tracks/:track_id", m.Handler.GetTrack)
	api.PUT("/tracks/:track_id", m.Handler.UpdateTrack)
	api.DELETE("/tracks/:track_id", m.Handler.DeleteTrack)
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
		data, err := staticFS.ReadFile(name)
		if err != nil {
			c.JSON(http.StatusNotFound, utils.H{"error": "not found"})
			return
		}
		serveEmbeddedAsset(c, contentType, data, "no-cache")
	}
}

func serveAdminPage(auth *Authenticator, name string) app.HandlerFunc {
	servePage := serveEmbedded(name, "text/html; charset=utf-8")
	return func(ctx context.Context, c *app.RequestContext) {
		if auth == nil || auth.SessionFromRequest(c) == nil {
			c.Redirect(http.StatusFound, []byte("/admin/login.html"))
			return
		}
		servePage(ctx, c)
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
		data, err := fs.ReadFile(sub, raw)
		if err != nil {
			c.JSON(http.StatusNotFound, utils.H{"error": "not found"})
			return
		}
		serveEmbeddedAsset(c, contentTypeFor(raw), data, "public, max-age=0, must-revalidate")
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
	var output bytes.Buffer
	writer := gzip.NewWriter(&output)
	if _, err := writer.Write(data); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	compressed := output.Bytes()
	cached, _ := embeddedGzipCache.LoadOrStore(etag, compressed)
	return cached.([]byte), nil
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
