package admin

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/config"
)

func (h *Handler) GetAdminTrackStatistics(ctx context.Context, c *app.RequestContext) {
	period := strings.TrimSpace(string(c.Query("period")))
	if period == "" {
		period = "month"
	}
	if period != "day" && period != "week" && period != "month" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "period must be day, week or month"})
		return
	}
	if h == nil || h.dashboardStatsRepo == nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "statistics not configured"})
		return
	}
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "statistics timezone unavailable"})
		return
	}
	now := time.Now().In(location)
	start, _, _, _, _ := dashboardPeriodBounds(now, period)
	data, err := h.dashboardStatsRepo.GetAdminTrackStatistics(ctx, start, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "轨迹统计暂不可用"})
		return
	}
	labels := make(map[string]string)
	for _, item := range config.DefaultTrackTypeConfigs {
		labels[item.Type] = item.Name
	}
	for i := range data.Types {
		item := &data.Types[i]
		item.Label = labels[item.Key]
		if item.Label == "" {
			item.Label = item.Key
		}
		if item.Key == "unknown" {
			item.Label = "未知类型"
		}
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{
		"period": period, "start": start, "end": now,
		"types": data.Types, "contributors": data.Contributors,
	}})
}
