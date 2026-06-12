package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
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
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "analytics service not configured"})
		return
	}
	body, err := c.Body()
	if err != nil || len(bytes.TrimSpace(body)) == 0 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	if int64(len(body)) > h.analyticsSvc.MaxBodyBytes() {
		c.JSON(http.StatusRequestEntityTooLarge, utils.H{"error": service.ErrAnalyticsTooLarge.Error()})
		return
	}
	var batch service.AnalyticsEventBatch
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&batch); err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
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
	if ingestMeta.UserID <= 0 {
		if headerUID := strings.TrimSpace(string(c.Request.Header.Peek(middleware.HeaderUserID))); headerUID != "" {
			if uid, err := strconv.ParseInt(headerUID, 10, 64); err == nil && uid > 0 {
				ingestMeta.UserID = uid
			}
		}
	}
	if auth := strings.TrimSpace(string(c.Request.Header.Peek("Authorization"))); auth != "" {
		ingestMeta.Authorization = true
	}
	result, err := h.analyticsSvc.Ingest(ctx, batch, ingestMeta)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAnalyticsDisabled):
			c.JSON(http.StatusServiceUnavailable, utils.H{"error": err.Error()})
		case errors.Is(err, service.ErrAnalyticsBadEvents):
			c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
		case errors.Is(err, service.ErrAnalyticsTooLarge):
			c.JSON(http.StatusRequestEntityTooLarge, utils.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, successResponse(result))
}
