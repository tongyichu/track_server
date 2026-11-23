package admin

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/ai"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

func (h *Handler) GenerateRouteGroupIntroduction(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.routeGroupSvc == nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "路线组服务暂不可用"})
		return
	}
	groupID := c.Param("group_id")
	summary, err := h.routeGroupSvc.GenerateRouteIntroductionDraft(ai.WithActor(ctx, h.adminOperator(c)), groupID)
	if err == nil {
		c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"summary": summary}})
		return
	}
	var invalid *service.InvalidArgumentError
	switch {
	case errors.As(err, &invalid), errors.Is(err, repository.ErrNotFound):
		writeAdminRouteGroupError(c, err)
	case errors.Is(err, ai.ErrBudget), errors.Is(err, ai.ErrBusy):
		c.JSON(http.StatusTooManyRequests, utils.H{"error": err.Error()})
	case ai.IsControlError(err), errors.Is(err, service.ErrRouteGroupIntroductionAIUnavailable):
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": err.Error()})
	default:
		log.Printf("route introduction draft failed group_id=%q: %v", groupID, err)
		c.JSON(http.StatusBadGateway, utils.H{"error": "AI 草稿生成失败，请稍后重试"})
	}
}
