package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

func discoveryLimit(c *app.RequestContext) (int, bool) {
	raw := string(c.Query("limit"))
	if raw == "" {
		return 0, true
	}
	limit, err := strconv.Atoi(raw)
	return limit, err == nil && limit > 0
}

func discoveryUserID(c *app.RequestContext) (int64, bool) {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		return 0, false
	}
	return meta.AuthUserID, true
}

func discoveryError(c *app.RequestContext, err error) {
	if errors.Is(err, repository.ErrNotFound) {
		c.JSON(http.StatusNotFound, utils.H{"error": "scenic spot not found"})
		return
	}
	var invalid *service.InvalidArgumentError
	if errors.As(err, &invalid) {
		c.JSON(http.StatusBadRequest, utils.H{"error": invalid.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
}

func (h *TrackHandler) DiscoverSpotTracks(ctx context.Context, c *app.RequestContext) {
	userID, ok := discoveryUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	limit, ok := discoveryLimit(c)
	if !ok {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid limit"})
		return
	}
	page, err := h.trackSvc.DiscoverSpotTracks(ctx, userID, string(c.Param("spot_id")), string(c.Query("cursor")), limit)
	if err != nil {
		discoveryError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(page))
}

func (h *TrackHandler) DiscoverSpots(ctx context.Context, c *app.RequestContext) {
	if _, ok := discoveryUserID(c); !ok {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	limit, ok := discoveryLimit(c)
	if !ok {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid limit"})
		return
	}
	page, err := h.trackSvc.DiscoverSpots(ctx, string(c.Query("city_code")), string(c.Query("cursor")), limit)
	if err != nil {
		discoveryError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(page))
}

func (h *TrackHandler) DiscoverShapes(ctx context.Context, c *app.RequestContext) {
	userID, ok := discoveryUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	limit, ok := discoveryLimit(c)
	if !ok {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid limit"})
		return
	}
	page, err := h.trackSvc.DiscoverShapes(ctx, userID, string(c.Query("city_code")), string(c.Query("cursor")), limit)
	if err != nil {
		discoveryError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(page))
}

func (h *TrackHandler) DiscoverRoutes(ctx context.Context, c *app.RequestContext) {
	userID, ok := discoveryUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	limit, ok := discoveryLimit(c)
	if !ok {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid limit"})
		return
	}
	page, err := h.trackSvc.DiscoverRoutes(ctx, userID, string(c.Query("city_code")), string(c.Query("label_type")), string(c.Query("track_type")), string(c.Query("cursor")), limit, string(c.Query("shape_label")))
	if err != nil {
		discoveryError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(page))
}

func (h *TrackHandler) DiscoveryShapeLabels(ctx context.Context, c *app.RequestContext) {
	if _, ok := discoveryUserID(c); !ok {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	result, err := h.trackSvc.DiscoveryShapeLabels(ctx, string(c.Query("city_code")), string(c.Query("track_type")))
	if err != nil {
		discoveryError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(result))
}

func (h *TrackHandler) ActivityCalendar(ctx context.Context, c *app.RequestContext) {
	userID, ok := discoveryUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	timezone := strings.TrimSpace(string(c.Query("timezone")))
	if timezone == "" {
		timezone = "Asia/Shanghai"
	}
	if len(timezone) > 64 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid timezone"})
		return
	}
	result, err := h.trackSvc.ActivityCalendar(ctx, userID, timezone, string(c.Query("track_type")))
	if err != nil {
		discoveryError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(result))
}

func (h *TrackHandler) ActivityTypeDistribution(ctx context.Context, c *app.RequestContext) {
	userID, ok := discoveryUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	result, err := h.trackSvc.ActivityTypeDistribution(ctx, userID)
	if err != nil {
		discoveryError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(result))
}
