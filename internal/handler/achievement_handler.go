package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/service"
)

// AchievementHandler handles achievement center APIs.
type AchievementHandler struct {
	achievementSvc *service.AchievementService
}

func NewAchievementHandler(achievementSvc *service.AchievementService) *AchievementHandler {
	return &AchievementHandler{achievementSvc: achievementSvc}
}

func (h *AchievementHandler) Summary(ctx context.Context, c *app.RequestContext) {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	if h == nil || h.achievementSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "achievement service not configured"})
		return
	}
	resp, err := h.achievementSvc.GetSummary(ctx, meta.AuthUserID)
	if err != nil {
		var iae *service.InvalidArgumentError
		if errors.As(err, &iae) {
			c.JSON(http.StatusBadRequest, utils.H{"error": iae.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, successResponse(resp))
}

func (h *AchievementHandler) Rewards(ctx context.Context, c *app.RequestContext) {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	if h == nil || h.achievementSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "achievement service not configured"})
		return
	}
	resp, err := h.achievementSvc.ListRewards(ctx, meta.AuthUserID)
	if err != nil {
		var iae *service.InvalidArgumentError
		if errors.As(err, &iae) {
			c.JSON(http.StatusBadRequest, utils.H{"error": iae.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, successResponse(resp))
}
