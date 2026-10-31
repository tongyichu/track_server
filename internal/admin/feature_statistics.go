package admin

import (
	"context"
	"net/http"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
)

func adminStatisticsDays(c *app.RequestContext) (int, bool) {
	switch string(c.Query("days")) {
	case "1":
		return 1, true
	case "7":
		return 7, true
	case "", "30":
		return 30, true
	default:
		c.JSON(http.StatusBadRequest, utils.H{"error": "days must be 1, 7 or 30"})
		return 0, false
	}
}

func (h *Handler) GetAdminMapModeStatistics(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.dashboardStatsRepo == nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "statistics not configured"})
		return
	}
	data, err := h.dashboardStatsRepo.GetAdminMapModeStatistics(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": data})
}

func (h *Handler) GetAdminCompanionUsageStatistics(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.dashboardStatsRepo == nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "statistics not configured"})
		return
	}
	days, ok := adminStatisticsDays(c)
	if !ok {
		return
	}
	data, err := h.dashboardStatsRepo.GetAdminCompanionUsageStatistics(ctx, days, time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": data})
}
