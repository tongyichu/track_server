package handler

import (
	"trackapp-server/internal/middleware"
	"trackapp-server/internal/service"

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

	api := h.Group("/api")

	// ping
	api.GET("/ping", Ping)

	// track related
	api.POST("/track/create", trackHandler.CreateTrack)
	api.GET("/track/recommend/list", trackHandler.ListRecommend)
	api.GET("/track/search/list", trackHandler.SearchTracks)
	api.POST("/track/:track_id/upload_cloud", trackHandler.UploadTrackCloud)
	api.GET("/track/running", trackHandler.GetRunningTrack)
	api.GET("/track/:track_id/map", trackHandler.GetTrackMap)
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
	api.PUT("/user/profile/client_language", userHandler.UpdateClientLanguage)
}
