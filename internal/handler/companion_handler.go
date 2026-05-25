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
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

// CompanionHandler handles HTTP requests related to companion control plane.
type CompanionHandler struct {
	svc           *service.CompanionService
	internalToken string
}

// NewCompanionHandler creates a new CompanionHandler.
func NewCompanionHandler(svc *service.CompanionService, internalToken string) *CompanionHandler {
	return &CompanionHandler{svc: svc, internalToken: strings.TrimSpace(internalToken)}
}

func (h *CompanionHandler) authUserID(c *app.RequestContext) (int64, bool) {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return 0, false
	}
	return meta.AuthUserID, true
}

func (h *CompanionHandler) authorizeInternal(c *app.RequestContext) bool {
	if strings.TrimSpace(h.internalToken) == "" {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "companion mqtt internal auth not configured"})
		return false
	}
	token := strings.TrimSpace(string(c.Request.Header.Peek("X-Internal-Token")))
	if token == "" {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "missing internal token"})
		return false
	}
	if token != h.internalToken {
		c.JSON(http.StatusForbidden, utils.H{"error": "forbidden"})
		return false
	}
	return true
}

// CreateSession handles POST /api/v1/companion/session/create.
func (h *CompanionHandler) CreateSession(ctx context.Context, c *app.RequestContext) {
	userID, ok := h.authUserID(c)
	if !ok {
		return
	}
	var req service.CreateCompanionSessionInput
	data, err := c.Body()
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &req); err != nil {
			c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
			return
		}
	}
	result, err := h.svc.CreateSession(ctx, userID, req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(result))
}

// JoinSession handles POST /api/v1/companion/session/join.
func (h *CompanionHandler) JoinSession(ctx context.Context, c *app.RequestContext) {
	userID, ok := h.authUserID(c)
	if !ok {
		return
	}
	var req service.JoinCompanionSessionInput
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &req) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	result, err := h.svc.JoinSession(ctx, userID, req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(result))
}

// GetCurrentSession handles GET /api/v1/companion/session/current.
func (h *CompanionHandler) GetCurrentSession(ctx context.Context, c *app.RequestContext) {
	userID, ok := h.authUserID(c)
	if !ok {
		return
	}
	result, err := h.svc.GetCurrentSession(ctx, userID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(result))
}

// GetSnapshot handles GET /api/v1/companion/session/:session_id/snapshot.
func (h *CompanionHandler) GetSnapshot(ctx context.Context, c *app.RequestContext) {
	userID, ok := h.authUserID(c)
	if !ok {
		return
	}
	snapshot, err := h.svc.GetSnapshot(ctx, userID, c.Param("session_id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(snapshot))
}

// LeaveSession handles POST /api/v1/companion/session/:session_id/leave.
func (h *CompanionHandler) LeaveSession(ctx context.Context, c *app.RequestContext) {
	userID, ok := h.authUserID(c)
	if !ok {
		return
	}
	if err := h.svc.LeaveSession(ctx, userID, c.Param("session_id")); err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(StatusResult{Status: "ok"}))
}

// EndSession handles POST /api/v1/companion/session/:session_id/end.
func (h *CompanionHandler) EndSession(ctx context.Context, c *app.RequestContext) {
	userID, ok := h.authUserID(c)
	if !ok {
		return
	}
	if err := h.svc.EndSession(ctx, userID, c.Param("session_id")); err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(StatusResult{Status: "ok"}))
}

// IssueMQTTCredentials handles POST /api/v1/companion/session/:session_id/mqtt/credentials.
func (h *CompanionHandler) IssueMQTTCredentials(ctx context.Context, c *app.RequestContext) {
	userID, ok := h.authUserID(c)
	if !ok {
		return
	}
	result, err := h.svc.IssueMQTTCredentials(ctx, userID, c.Param("session_id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(result))
}

// MQTTAuth handles POST /api/v1/internal/mqtt/auth.
func (h *CompanionHandler) MQTTAuth(ctx context.Context, c *app.RequestContext) {
	if !h.authorizeInternal(c) {
		return
	}
	var req service.CompanionMQTTAuthInput
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &req) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"result": "deny"})
		return
	}
	c.JSON(http.StatusOK, h.svc.AuthenticateMQTTConnection(ctx, req))
}

// MQTTACL handles POST /api/v1/internal/mqtt/acl.
func (h *CompanionHandler) MQTTACL(ctx context.Context, c *app.RequestContext) {
	if !h.authorizeInternal(c) {
		return
	}
	var req service.CompanionMQTTACLInput
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &req) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"result": "deny"})
		return
	}
	c.JSON(http.StatusOK, h.svc.AuthorizeMQTTOperation(ctx, req))
}

// IngestMQTTLocation handles POST /api/v1/internal/companion/mqtt/location-ingest.
func (h *CompanionHandler) IngestMQTTLocation(ctx context.Context, c *app.RequestContext) {
	if !h.authorizeInternal(c) {
		return
	}
	var req service.CompanionMQTTLocationIngestInput
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &req) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	if err := h.svc.IngestLocationFromMQTT(ctx, req); err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"result": "ok"})
}

// IngestMQTTPresence handles POST /api/v1/internal/companion/mqtt/presence-ingest.
func (h *CompanionHandler) IngestMQTTPresence(ctx context.Context, c *app.RequestContext) {
	if !h.authorizeInternal(c) {
		return
	}
	var req service.CompanionMQTTPresenceIngestInput
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &req) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	if err := h.svc.IngestPresenceFromMQTT(ctx, req); err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"result": "ok"})
}

func (h *CompanionHandler) writeError(c *app.RequestContext, err error) {
	var iae *service.InvalidArgumentError
	switch {
	case errors.As(err, &iae):
		c.JSON(http.StatusBadRequest, utils.H{"error": iae.Error()})
	case errors.Is(err, repository.ErrNotFound):
		c.JSON(http.StatusNotFound, utils.H{"error": err.Error()})
	case errors.Is(err, repository.ErrForbidden):
		c.JSON(http.StatusForbidden, utils.H{"error": "forbidden"})
	case err != nil && err.Error() == "companion mqtt not configured":
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
	}
}
