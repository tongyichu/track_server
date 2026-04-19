package handler

import (
	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/service"

	"github.com/cloudwego/hertz/pkg/app/server"
)

// Deps groups all handler dependencies.
type Deps struct {
	TrackService *service.TrackService
	UserService  *service.UserService
}

// RegisterRoutes registers all HTTP routes on the given Hertz server.
func RegisterRoutes(h *server.Hertz, deps Deps) {
	// global middleware
	h.Use(middleware.RequestMetaMiddleware())

	trackHandler := NewTrackHandler(deps.TrackService)
	userHandler := NewUserHandler(deps.UserService)

	api := h.Group("/api/v1")

	// ping
	api.GET("/ping", Ping)

	// track related
	api.POST("/track/create", trackHandler.CreateTrack)                      // 创建轨迹
	api.GET("/track/recommend/list", trackHandler.ListRecommend)             // 轨迹推荐列表
	api.GET("/track/search/list", trackHandler.SearchTracks)                 // 轨迹搜索
	api.POST("/track/:track_id/upload_cloud", trackHandler.UploadTrackCloud) // 轨迹上传云端
	api.GET("/track/running", trackHandler.GetRunningTrack)                  // 获取处于“正在记录”的轨迹信息
	api.GET("/track/:track_id/map", trackHandler.GetTrackMap)                // 根据轨迹ID获取轨迹地图
	api.GET("/track/:track_id/detail", trackHandler.GetTrackDetail)
	api.GET("/track/:track_id/summary", trackHandler.GetTrackDetail)

	// collection
	api.GET("/user/:user_id/collect", trackHandler.GetCollectStatus)
	api.POST("/track_collect", trackHandler.CollectTrack)
	api.DELETE("/track_collect", trackHandler.UncollectTrack)

	// user profile
	api.GET("/user/:user_id/detail", userHandler.GetUserDetail)
	api.PUT("/user/profile/photo", userHandler.UpdateAvatar)
	api.PUT("/user/profile/name", userHandler.UpdateName)
	api.PUT("/user/profile/signature", userHandler.UpdateSignature)
	api.PUT("/user/profile/phone", userHandler.UpdatePhone)
	api.PUT("/user/profile/client_language", userHandler.UpdateClientLanguage)
}
