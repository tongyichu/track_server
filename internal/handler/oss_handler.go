package handler

import (
	"context"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"

	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/service"
)

type OSSHandler struct {
	stsSvc *service.OSSTokenService
}

func NewOSSHandler(stsSvc *service.OSSTokenService) *OSSHandler {
	return &OSSHandler{stsSvc: stsSvc}
}

// GetSTSToken handles GET /api/v1/oss/sts-token
func (h *OSSHandler) GetSTSToken(ctx context.Context, c *app.RequestContext) {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	if h.stsSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "oss sts not configured"})
		return
	}

	cred, err := h.stsSvc.GetUploadCredential(meta.AuthUserID)
	if err != nil {
		switch err.(type) {
		case *service.InvalidArgumentError:
			c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, successResponse(cred))
}
