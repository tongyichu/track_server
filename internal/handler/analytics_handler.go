package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"

	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/service"
)

type AnalyticsHandler struct {
	analyticsSvc *service.AnalyticsService
}

func NewAnalyticsHandler(analyticsSvc *service.AnalyticsService) *AnalyticsHandler {
	return &AnalyticsHandler{analyticsSvc: analyticsSvc}
}

// Ingest handles POST /api/v1/analytics/events.
func (h *AnalyticsHandler) Ingest(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.analyticsSvc == nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "analytics unavailable", "error_code": "analytics_unavailable"})
		return
	}
	body, err := c.Body()
	if err != nil || len(bytes.TrimSpace(body)) == 0 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload", "error_code": "analytics_invalid_payload"})
		return
	}
	if int64(len(body)) > h.analyticsSvc.MaxBodyBytes() {
		c.JSON(http.StatusRequestEntityTooLarge, utils.H{"error": service.ErrAnalyticsTooLarge.Error(), "error_code": "analytics_payload_too_large"})
		return
	}
	var batch service.AnalyticsEventBatch
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&batch); err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload", "error_code": "analytics_invalid_payload"})
		return
	}
	meta := middleware.GetRequestMeta(c)
	ingestMeta := service.AnalyticsIngestMeta{
		RemoteIP: string(c.ClientIP()),
	}
	if meta != nil {
		ingestMeta.UserID = meta.AuthUserID
		ingestMeta.Platform = strings.TrimSpace(meta.Platform)
		ingestMeta.AppVersion = strings.TrimSpace(meta.ClientVersion)
		ingestMeta.DeviceID = strings.TrimSpace(meta.DeviceID)
		ingestMeta.ClientLang = strings.TrimSpace(meta.ClientLanguage)
	}
	ingestMeta.Authorization = ingestMeta.UserID > 0
	result, err := h.analyticsSvc.Ingest(ctx, batch, ingestMeta)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAnalyticsDisabled):
			c.JSON(http.StatusServiceUnavailable, utils.H{"error": "analytics unavailable", "error_code": "analytics_unavailable"})
		case errors.Is(err, service.ErrAnalyticsAuthRequired):
			c.JSON(http.StatusUnauthorized, utils.H{"error": "analytics authorization required", "error_code": "analytics_auth_required"})
		case errors.Is(err, service.ErrAnalyticsUserMismatch):
			c.JSON(http.StatusForbidden, utils.H{"error": "analytics user mismatch", "error_code": "analytics_user_mismatch"})
		case errors.Is(err, service.ErrAnalyticsBadEvents):
			c.JSON(http.StatusBadRequest, utils.H{"error": err.Error(), "error_code": "analytics_invalid_payload"})
		case errors.Is(err, service.ErrAnalyticsTooLarge):
			c.JSON(http.StatusRequestEntityTooLarge, utils.H{"error": err.Error(), "error_code": "analytics_payload_too_large"})
		default:
			c.JSON(http.StatusInternalServerError, utils.H{"error": "analytics write failed", "error_code": "analytics_write_failed"})
		}
		return
	}
	c.JSON(http.StatusOK, successResponse(result))
}
