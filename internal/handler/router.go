package handler

import (
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/service"

	"github.com/cloudwego/hertz/pkg/app/server"
)

// Deps groups all handler dependencies.
type Deps struct {
	TrackService               *service.TrackService
	UserService                *service.UserService
	LoginService               *service.LoginService
	OSSTokenService            *service.OSSTokenService
	AppReleaseService          *service.AppReleaseService
	CompanionService           *service.CompanionService
	AchievementService         *service.AchievementService
	JWTSecret                  string
	TokenBlacklist             *middleware.TokenBlacklist
	CompanionMQTTInternalToken string

	// StaticRoot 是服务端本地资源缓存根目录；若非空，会挂载到 /static
	// 路由下统一提供下载（下层按 screenshots/raw_tracks 等子目录组织）。
	StaticRoot string
}

// RegisterRoutes registers all HTTP routes on the given Hertz server.
func RegisterRoutes(h *server.Hertz, deps Deps) {
	// global middleware
	h.Use(middleware.RequestMetaMiddleware())

	// 静态文件服务：对外暴露服务端本地缓存的资源（轨迹截图 / 原始轨迹文件等），供客户端下载。
	// 路径与各 AssetCacheService 的 urlPrefix 保持一致（/api/v1/static/...）。
	// 注意：静态资源下载必须走 auth group，保证鉴权逻辑与其他接口一致。

	trackHandler := NewTrackHandler(deps.TrackService)
	userHandler := NewUserHandler(deps.UserService)
	loginHandler := NewLoginHandler(deps.LoginService, deps.TokenBlacklist, deps.AchievementService)
	ossHandler := NewOSSHandler(deps.OSSTokenService)
	appReleaseHandler := NewAppReleaseHandler(deps.AppReleaseService)
	companionHandler := NewCompanionHandler(deps.CompanionService, deps.CompanionMQTTInternalToken)
	achievementHandler := NewAchievementHandler(deps.AchievementService)

	api := h.Group("/api/v1")

	// ping
	api.GET("/ping", Ping)

	// public: login (no auth required)
	api.GET("/captcha", loginHandler.GetCaptcha)
	api.POST("/sms/send", loginHandler.SendSMSCode)
	api.POST("/login/sms", loginHandler.LoginBySMS)
	api.POST("/login/wechat", loginHandler.LoginByWechat)
	api.POST("/login/apple", loginHandler.LoginByApple)
	api.POST("/internal/mqtt/auth", companionHandler.MQTTAuth)
	api.POST("/internal/mqtt/acl", companionHandler.MQTTACL)
	api.POST("/internal/companion/mqtt/location-ingest", companionHandler.IngestMQTTLocation)
	api.POST("/internal/companion/mqtt/presence-ingest", companionHandler.IngestMQTTPresence)
	api.POST("/internal/companion/mqtt/danmaku-ingest", companionHandler.IngestMQTTDanmaku)

	// public: upgrade check (客户端启动/切前台时调用，无需登录)
	api.GET("/upgrade/check", appReleaseHandler.CheckUpgrade)
	// public: achievement level rule H5 page for app WebView.
	api.GET("/achievement/level-rules.html", achievementHandler.LevelRulesPage)
	// public: app release package download.
	// 升级检查接口本身无需登录，因此其返回的安装包 URL 也必须能被系统下载器直接访问。
	// 只公开 static/release 子目录；轨迹截图、头像、原始轨迹文件仍在下方 auth.StaticFS 里鉴权访问。
	if deps.StaticRoot != "" {
		releaseFS := &app.FS{
			Root: deps.StaticRoot,
			PathRewrite: func(ctx *app.RequestContext) []byte {
				fp := ctx.Param("filepath")
				if fp == "" {
					return []byte("/release/")
				}
				fp = strings.TrimPrefix(fp, "/")
				return []byte("/release/" + fp)
			},
		}
		api.StaticFS("/static/release", releaseFS)
	}

	// authenticated routes
	auth := api.Group("", middleware.JWTAuthMiddleware(deps.LoginService, deps.TokenBlacklist))

	// 静态资源下载（需要登录）
	if deps.StaticRoot != "" {
		// 重要：这里不要直接用 auth.Static("/static", deps.StaticRoot)。
		//
		// Hertz 静态文件（app.FS）默认使用 ctx.Path() 作为“相对 Root 的路径”，再拼出本地路径：
		//   local = <Root> + <ctx.Path()>
		//
		// 而本服务的 URL 设计为：
		//   GET /api/v1/static/<category>/<file>
		// Root 设计为：
		//   /var/log/track_server/static
		//
		// 若不做重写，ctx.Path() 会是 "/api/v1/static/screenshots/NO.00000005.png"，
		// 最终去打开的将是：
		//   /var/log/track_server/static/api/v1/static/screenshots/NO.00000005.png
		// 这与实际落盘路径：
		//   /var/log/track_server/static/screenshots/NO.00000005.png
		// 不一致，从而导致“鉴权通过但始终 404”。
		//
		// 因此这里用 StaticFS + PathRewrite，将通配参数 filepath 重写为 "/"+filepath，
		// 使其正确映射到 Root 下。
		fs := &app.FS{
			Root: deps.StaticRoot,
			PathRewrite: func(ctx *app.RequestContext) []byte {
				fp := ctx.Param("filepath")
				if fp == "" {
					return []byte("/")
				}
				// 通配参数一般不带前导 '/'
				if strings.HasPrefix(fp, "/") {
					return []byte(fp)
				}
				return []byte("/" + fp)
			},
		}
		auth.StaticFS("/static", fs)
	}

	auth.POST("/logout", loginHandler.Logout)
	auth.GET("/login/log", loginHandler.GetLoginLog)
	auth.POST("/companion/session/create", companionHandler.CreateSession)
	auth.POST("/companion/session/join", companionHandler.JoinSession)
	auth.GET("/companion/session/preview", companionHandler.PreviewSession)
	auth.GET("/companion/session/current", companionHandler.GetCurrentSession)
	auth.GET("/companion/session/history", companionHandler.ListHistory)
	auth.GET("/companion/session/nearby", companionHandler.ListNearby)
	auth.GET("/companion/session/:session_id/snapshot", companionHandler.GetSnapshot)
	auth.POST("/companion/session/:session_id/leave", companionHandler.LeaveSession)
	auth.POST("/companion/session/:session_id/end", companionHandler.EndSession)
	auth.POST("/companion/session/:session_id/members/:user_id/kick", companionHandler.KickSessionMember)
	auth.POST("/companion/session/:session_id/danmaku/toggle", companionHandler.ToggleSessionDanmaku)
	auth.POST("/companion/session/:session_id/mqtt/credentials", companionHandler.IssueMQTTCredentials)

	// oss upload
	auth.GET("/oss/sts-token", ossHandler.GetSTSToken)
	auth.GET("/oss/sts-token/read", ossHandler.GetSTSReadToken)

	// track related
	auth.GET("/track/types", trackHandler.ListTrackTypes)
	auth.POST("/track/create", trackHandler.CreateTrack)
	auth.PUT("/track/:track_id/update", trackHandler.UpdateTrackInfo)
	auth.DELETE("/track/:track_id", trackHandler.DeleteTrack)
	auth.GET("/track/recommend/list", trackHandler.ListRecommend)
	auth.GET("/track/my/list", trackHandler.ListMyTracks)
	auth.GET("/track/collected/list", trackHandler.ListCollectedTracks)
	auth.GET("/track/search/list", trackHandler.SearchTracks)
	auth.POST("/track/:track_id/upload_cloud", trackHandler.UploadTrackCloud)
	auth.GET("/track/running", trackHandler.GetRunningTrack)
	auth.GET("/track/:track_id/map", trackHandler.GetTrackMap)
	auth.POST("/track/:track_id/navigation/report", trackHandler.ReportTrackNavigation)
	auth.GET("/track/:track_id/detail", trackHandler.GetTrackDetail)
	auth.GET("/track/:track_id/summary", trackHandler.GetTrackDetail)

	// achievement center
	auth.GET("/achievement/summary", achievementHandler.Summary)
	auth.GET("/achievement/rewards", achievementHandler.Rewards)

	// collection
	auth.GET("/user/:user_id/collect", trackHandler.GetCollectStatus)
	auth.POST("/track_collect", trackHandler.CollectTrack)
	auth.DELETE("/track_collect", trackHandler.UncollectTrack)

	// user profile
	auth.GET("/user/:user_id/detail", userHandler.GetUserDetail)
	auth.PUT("/user/profile/update", userHandler.UpdateProfile)
	auth.PUT("/user/profile/phone", userHandler.UpdatePhone)
	auth.PUT("/user/profile/client_language", userHandler.UpdateClientLanguage)
}
