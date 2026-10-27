package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

func (h *Handler) GetFeedbackCorrectionConfig(ctx context.Context, c *app.RequestContext) {
	config, err := h.feedbackSvc.CorrectionConfig(ctx, false)
	if err != nil {
		writeAdminFeedbackError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": config})
}

func (h *Handler) UpdateFeedbackCorrectionConfig(ctx context.Context, c *app.RequestContext) {
	var input models.FeedbackCorrectionConfig
	body, err := c.Body()
	if err != nil || json.Unmarshal(body, &input) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	config, err := h.feedbackSvc.UpdateCorrectionConfig(ctx, input)
	if errors.Is(err, repository.ErrAlreadyExists) {
		c.JSON(http.StatusConflict, utils.H{"error": "配置已被其他管理员更新，请重新加载后修改"})
		return
	}
	if err != nil {
		writeAdminFeedbackError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": config})
}
