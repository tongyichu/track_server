package handler

import (
	"context"
	"math"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/models"
	"net/http"
)

func (h *TrackHandler) SearchOptions(_ context.Context, c *app.RequestContext) {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	c.JSON(http.StatusOK, successResponse(h.trackSvc.SearchOptions()))
}

func searchDistanceQuery(c *app.RequestContext) (models.TrackDistanceRange, bool) {
	r := models.TrackDistanceRange{}
	for key, target := range map[string]**float64{"min_distance_m": &r.MinMeters, "max_distance_m": &r.MaxMeters} {
		raw := string(c.Query(key))
		if raw == "" {
			continue
		}
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			c.JSON(http.StatusBadRequest, utils.H{"error": "invalid distance range"})
			return r, false
		}
		*target = &value
	}
	return r, true
}
