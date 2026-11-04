package handler

import (
	"context"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
)

func (h *TrackHandler) PersonalRecords(ctx context.Context, c *app.RequestContext) {
	c.Header("Cache-Control", "no-store")
	userID, ok := discoveryUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	result, err := h.trackSvc.PersonalRecords(ctx, userID, string(c.Query("track_type")))
	if err != nil {
		discoveryError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(result))
}
