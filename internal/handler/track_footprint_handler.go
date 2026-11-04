package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
)

func (h *TrackHandler) ActivityFootprint(ctx context.Context, c *app.RequestContext) {
	c.Header("Cache-Control", "no-store")
	userID, ok := discoveryUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	result, err := h.trackSvc.ActivityFootprint(ctx, userID, string(c.Query("track_type")))
	if err != nil {
		discoveryError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(result))
}

func (h *TrackHandler) FootprintTracks(ctx context.Context, c *app.RequestContext) {
	c.Header("Cache-Control", "no-store")
	userID, ok := discoveryUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	limit, _ := strconv.Atoi(string(c.Query("limit")))
	result, err := h.trackSvc.FootprintTracks(ctx, userID, string(c.Query("region_id")), string(c.Query("track_type")), string(c.Query("cursor")), limit)
	if err != nil {
		discoveryError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(result))
}
