package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

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
func (h *TrackLikeHandler) Received(ctx context.Context, c *app.RequestContext) {
	viewer := h.viewer(c)
	if viewer == 0 {
		return
	}
	target, err := parseRequiredUserID(c.Param("user_id"))
	if err != nil {
		c.JSON(400, utils.H{"error": err.Error()})
		return
	}
	limit := 0
	if raw := string(c.Query("limit")); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			c.JSON(400, utils.H{"error": "invalid limit"})
			return
		}
	}
	result, err := h.service.Received(ctx, viewer, target, string(c.Query("cursor")), limit)
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

func (h *TrackLikeHandler) ReceivedTracks(ctx context.Context, c *app.RequestContext) {
	h.grouped(ctx, c, false)
}
func (h *TrackLikeHandler) ReceivedTrackLikers(ctx context.Context, c *app.RequestContext) {
	h.grouped(ctx, c, true)
}
func (h *TrackLikeHandler) grouped(ctx context.Context, c *app.RequestContext, likers bool) {
	viewer := h.viewer(c)
	if viewer == 0 {
		return
	}
	target, err := parseRequiredUserID(c.Param("user_id"))
	if err != nil {
		c.JSON(400, utils.H{"error": err.Error()})
		return
	}
	limit := 0
	if raw := string(c.Query("limit")); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			c.JSON(400, utils.H{"error": "invalid limit"})
			return
		}
	}
	var result interface{}
	if likers {
		result, err = h.service.ReceivedTrackLikers(ctx, viewer, target, c.Param("track_id"), string(c.Query("cursor")), limit)
	} else {
		result, err = h.service.ReceivedTracks(ctx, viewer, target, string(c.Query("cursor")), limit)
	}
	h.respond(c, result, err)
}
