package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"

	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/service"
)

type accountDeletionHandler struct {
	service *service.AccountDeletionService
}

func (h accountDeletionHandler) availability(ctx context.Context, c *app.RequestContext) {
	c.Header("Cache-Control", "no-store")
	meta := middleware.GetRequestMeta(c)
	available := false
	if h.service.Enabled() && meta != nil && meta.AuthUserID > 0 {
		available = h.service.AvailableFor(ctx, meta.AuthUserID, meta.AuthWechatVerified)
	}
	c.JSON(http.StatusOK, successResponse(utils.H{"available": available}))
}

func (h accountDeletionHandler) request(ctx context.Context, c *app.RequestContext) {
	c.Header("Cache-Control", "no-store")
	if !h.service.Enabled() {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "account deletion is unavailable"})
		return
	}
	var input struct {
		Confirm bool `json:"confirm"`
	}
	body, err := c.Body()
	if err != nil || len(body) > 128 || json.Unmarshal(body, &input) != nil || !input.Confirm {
		c.JSON(http.StatusBadRequest, utils.H{"error": "account deletion confirmation required"})
		return
	}
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "authorization required"})
		return
	}
	if !h.service.CanConfirm(meta.AuthWechatVerified) {
		c.JSON(http.StatusForbidden, utils.H{"error": "please sign in again with WeChat to delete this account", "error_code": "account_deletion_reauth_required"})
		return
	}
	if err := h.service.Request(ctx, meta.AuthUserID); err != nil {
		log.Printf("account deletion request: %v", err)
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "could not accept account deletion"})
		return
	}
	c.JSON(http.StatusAccepted, successResponse(utils.H{"status": "accepted"}))
}
