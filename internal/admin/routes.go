package admin

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
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
// - GET /admin/index.html             管理首页（公开静态文件，前端会拉 /admin/api/me）
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
// - POST /admin/api/users/:user_id/restrictions 创建账号限制（鉴权）
// - DELETE /admin/api/users/:user_id/restrictions/current 解除当前账号限制（鉴权）
// - GET  /admin/api/tracks                 轨迹列表（鉴权，cursor 翻页）
// - PUT  /admin/api/tracks/:track_id       更新轨迹标题与城市 Code（鉴权）
// - DELETE /admin/api/tracks/:track_id     删除轨迹（鉴权，软删除并清理关联索引/收藏）
// - GET  /admin/api/companions             同行会话列表（鉴权，cursor 翻页）
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
	g.GET("/index.html", serveEmbedded("static/index.html", "text/html; charset=utf-8"))
	g.GET("/users.html", serveEmbedded("static/users.html", "text/html; charset=utf-8"))
	g.GET("/tracks.html", serveEmbedded("static/tracks.html", "text/html; charset=utf-8"))
	g.GET("/route_groups.html", serveEmbedded("static/route_groups.html", "text/html; charset=utf-8"))
	g.GET("/companions.html", serveEmbedded("static/companions.html", "text/html; charset=utf-8"))
	g.GET("/companion_detail.html", serveEmbedded("static/companion_detail.html", "text/html; charset=utf-8"))
	g.GET("/feedbacks.html", serveEmbedded("static/feedbacks.html", "text/html; charset=utf-8"))
	g.GET("/analytics.html", serveEmbedded("static/analytics.html", "text/html; charset=utf-8"))
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

	// 用户 / 轨迹 / 同行 列表（仅查询，提供基础翻页）
	api.GET("/static/*filepath", m.Handler.GetStaticAsset)
	api.GET("/users", m.Handler.ListUsers)
	api.GET("/users/:user_id/restrictions/current", m.Handler.GetCurrentAccountRestriction)
	api.POST("/users/:user_id/restrictions", m.Handler.CreateAccountRestriction)
	api.DELETE("/users/:user_id/restrictions/current", m.Handler.RevokeCurrentAccountRestriction)
	api.GET("/tracks", m.Handler.ListTracks)
	api.PUT("/tracks/:track_id", m.Handler.UpdateTrack)
	api.DELETE("/tracks/:track_id", m.Handler.DeleteTrack)
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
		c.Data(http.StatusOK, contentType, data)
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
		c.Data(http.StatusOK, contentTypeFor(raw), data)
	}
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
