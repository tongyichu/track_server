package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/repository"
)

func (h *CompanionHandler) RemoveHistory(ctx context.Context, c *app.RequestContext) {
	user, ok := h.authUserID(c)
	if !ok {
		return
	}
	result, err := h.svc.RemoveHistory(ctx, user, c.Param("session_id"), string(c.Query("scope")))
	if errors.Is(err, repository.ErrConflict) {
		c.JSON(http.StatusConflict, utils.H{"error": "companion history removal scope or session state changed", "error_code": "companion_history_scope_changed"})
		return
	}
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(result))
}
