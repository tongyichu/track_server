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

func (h *TrackHandler) ListProfileTracks(ctx context.Context, c *app.RequestContext) {
	h.listProfileTracks(ctx, c, false)
}

func (h *TrackHandler) ListProfileCollections(ctx context.Context, c *app.RequestContext) {
	h.listProfileTracks(ctx, c, true)
}

func (h *TrackHandler) listProfileTracks(ctx context.Context, c *app.RequestContext, collections bool) {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	targetID, err := parseRequiredUserID(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
		return
	}
	limit := 0
	if raw := string(c.Query("limit")); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			c.JSON(http.StatusBadRequest, utils.H{"error": "invalid limit"})
			return
		}
	}
	input := service.ListMyTracksInput{Cursor: string(c.Query("cursor")), Limit: limit}
	var page *service.ProfileTrackPage
	if collections {
		page, err = h.trackSvc.ListProfileCollections(ctx, meta.AuthUserID, targetID, input)
	} else {
		page, err = h.trackSvc.ListProfileTracks(ctx, meta.AuthUserID, targetID, input)
	}
	if err != nil {
		status := http.StatusInternalServerError
		var invalid *service.InvalidArgumentError
		if errors.As(err, &invalid) {
			status = http.StatusBadRequest
		}
		if errors.Is(err, repository.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, successResponse(page))
}
