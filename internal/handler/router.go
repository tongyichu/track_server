package handler

import (
	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/service"

	"github.com/cloudwego/hertz/pkg/app/server"
)

// Deps groups all handler dependencies.
type Deps struct {
	TrackService   *service.TrackService
	UserService    *service.UserService
	LoginService   *service.LoginService
	OSSTokenService *service.OSSTokenService
	JWTSecret      string
	TokenBlacklist *middleware.TokenBlacklist
}

// RegisterRoutes registers all HTTP routes on the given Hertz server.
func RegisterRoutes(h *server.Hertz, deps Deps) {
	// global middleware
	h.Use(middleware.RequestMetaMiddleware())

	trackHandler := NewTrackHandler(deps.TrackService)
	userHandler := NewUserHandler(deps.UserService)
	loginHandler := NewLoginHandler(deps.LoginService, deps.TokenBlacklist)
	ossHandler := NewOSSHandler(deps.OSSTokenService)

	api := h.Group("/api/v1")

	// ping
	api.GET("/ping", Ping)

	// public: login (no auth required)
	api.GET("/captcha", loginHandler.GetCaptcha)
	api.POST("/sms/send", loginHandler.SendSMSCode)
	api.POST("/login/sms", loginHandler.LoginBySMS)
	api.POST("/login/wechat", loginHandler.LoginByWechat)
	api.POST("/login/apple", loginHandler.LoginByApple)

	// authenticated routes
	auth := api.Group("", middleware.JWTAuthMiddleware(deps.JWTSecret, deps.TokenBlacklist))

	auth.POST("/logout", loginHandler.Logout)
	auth.GET("/login/log", loginHandler.GetLoginLog)

	// oss upload
	auth.GET("/oss/sts-token", ossHandler.GetSTSToken)
	auth.GET("/oss/sts-token/read", ossHandler.GetSTSReadToken)

	// track related
	auth.POST("/track/create", trackHandler.CreateTrack)
	auth.PUT("/track/:track_id/update", trackHandler.UpdateTrackInfo)
	auth.GET("/track/recommend/list", trackHandler.ListRecommend)
	auth.GET("/track/search/list", trackHandler.SearchTracks)
	auth.POST("/track/:track_id/upload_cloud", trackHandler.UploadTrackCloud)
	auth.GET("/track/running", trackHandler.GetRunningTrack)
	auth.GET("/track/:track_id/map", trackHandler.GetTrackMap)
	auth.GET("/track/:track_id/detail", trackHandler.GetTrackDetail)
	auth.GET("/track/:track_id/summary", trackHandler.GetTrackDetail)

	// collection
	auth.GET("/user/:user_id/collect", trackHandler.GetCollectStatus)
	auth.POST("/track_collect", trackHandler.CollectTrack)
	auth.DELETE("/track_collect", trackHandler.UncollectTrack)

	// user profile
	auth.GET("/user/:user_id/detail", userHandler.GetUserDetail)
	auth.PUT("/user/profile/photo", userHandler.UpdateAvatar)
	auth.PUT("/user/profile/name", userHandler.UpdateName)
	auth.PUT("/user/profile/signature", userHandler.UpdateSignature)
	auth.PUT("/user/profile/phone", userHandler.UpdatePhone)
	auth.PUT("/user/profile/client_language", userHandler.UpdateClientLanguage)
}
