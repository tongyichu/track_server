package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

type TrackLikeHandler struct{ service *service.TrackLikeService }

func NewTrackLikeHandler(s *service.TrackLikeService) *TrackLikeHandler {
	return &TrackLikeHandler{service: s}
}
func (h *TrackLikeHandler) Like(ctx context.Context, c *app.RequestContext) { h.track(ctx, c, "like") }
func (h *TrackLikeHandler) Unlike(ctx context.Context, c *app.RequestContext) {
	h.track(ctx, c, "unlike")
}
func (h *TrackLikeHandler) Status(ctx context.Context, c *app.RequestContext) {
	h.track(ctx, c, "status")
}
func (h *TrackLikeHandler) viewer(c *app.RequestContext) int64 {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(401, utils.H{"error": "unauthorized"})
		return 0
	}
	if h.service == nil {
		c.JSON(503, utils.H{"error": "likes not configured"})
		return 0
	}
	return meta.AuthUserID
}
func (h *TrackLikeHandler) track(ctx context.Context, c *app.RequestContext, action string) {
	viewer := h.viewer(c)
	if viewer == 0 {
		return
	}
	var result *service.TrackLikeStatus
	var err error
	if action == "status" {
		result, err = h.service.Status(ctx, viewer, c.Param("track_id"))
	} else {
		result, err = h.service.Set(ctx, viewer, c.Param("track_id"), action == "like")
	}
	h.respond(c, result, err)
}
func (h *TrackLikeHandler) respond(c *app.RequestContext, result interface{}, err error) {
	if err == nil {
		c.JSON(http.StatusOK, successResponse(result))
		return
	}
	status := 500
	var invalid *service.InvalidArgumentError
	if errors.Is(err, repository.ErrNotFound) {
		status = 404
	} else if errors.As(err, &invalid) {
		status = 400
	}
	c.JSON(status, utils.H{"error": err.Error()})
}
