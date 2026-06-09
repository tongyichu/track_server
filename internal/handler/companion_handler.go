package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
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

// ListHistory handles GET /api/v1/companion/session/history.
func (h *CompanionHandler) ListHistory(ctx context.Context, c *app.RequestContext) {
	userID, ok := h.authUserID(c)
	if !ok {
		return
	}
	limit := 0
	if rawLimit := string(c.Query("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, utils.H{"error": "invalid limit"})
			return
		}
		limit = parsed
	}
	result, err := h.svc.ListHistory(ctx, userID, service.ListCompanionHistoryInput{
		Cursor: string(c.Query("cursor")),
		Limit:  limit,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(result))
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

// PreviewSession handles GET /api/v1/companion/session/preview?join_token=xxx.
func (h *CompanionHandler) PreviewSession(ctx context.Context, c *app.RequestContext) {
	userID, ok := h.authUserID(c)
	if !ok {
		return
	}
	result, err := h.svc.PreviewSessionByJoinToken(ctx, userID, string(c.Query("join_token")))
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

// UpdateSessionStats handles PUT /api/v1/companion/session/:session_id/update.
func (h *CompanionHandler) UpdateSessionStats(ctx context.Context, c *app.RequestContext) {
	userID, ok := h.authUserID(c)
	if !ok {
		return
	}
	var req service.UpdateCompanionSessionStatsInput
	body := c.Request.Body()
	if len(bytes.TrimSpace(body)) == 0 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	session, err := h.svc.UpdateSessionStats(ctx, userID, c.Param("session_id"), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(session))
}

// CreateEvent handles POST /api/v1/companion/session/:session_id/events.
func (h *CompanionHandler) CreateEvent(ctx context.Context, c *app.RequestContext) {
	userID, ok := h.authUserID(c)
	if !ok {
		return
	}
	var req service.CreateCompanionEventInput
	body := c.Request.Body()
	if len(bytes.TrimSpace(body)) == 0 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	event, err := h.svc.CreateEvent(ctx, userID, c.Param("session_id"), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(event))
}

// ListEvents handles GET /api/v1/companion/session/:session_id/events.
func (h *CompanionHandler) ListEvents(ctx context.Context, c *app.RequestContext) {
	userID, ok := h.authUserID(c)
	if !ok {
		return
	}
	in := service.ListCompanionEventsInput{
		Cursor: string(c.Query("cursor")),
		Order:  string(c.Query("order")),
	}
	if rawLimit := strings.TrimSpace(string(c.Query("limit"))); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit <= 0 {
			c.JSON(http.StatusBadRequest, utils.H{"error": "invalid limit"})
			return
		}
		in.Limit = limit
	}
	page, err := h.svc.ListEvents(ctx, userID, c.Param("session_id"), in)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(page))
}

// KickSessionMember handles POST /api/v1/companion/session/:session_id/members/:user_id/kick.
func (h *CompanionHandler) KickSessionMember(ctx context.Context, c *app.RequestContext) {
	userID, ok := h.authUserID(c)
	if !ok {
		return
	}
	targetUserID, err := strconv.ParseInt(strings.TrimSpace(c.Param("user_id")), 10, 64)
	if err != nil || targetUserID <= 0 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "user_id is required"})
		return
	}
	state, err := h.svc.KickSessionMember(ctx, userID, c.Param("session_id"), targetUserID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(state))
}

type toggleDanmakuRequest struct {
	Enabled *bool `json:"enabled"`
}

// ToggleSessionDanmaku handles POST /api/v1/companion/session/:session_id/danmaku/toggle.
//
// 仅会话 owner 可调用。请求体必须显式携带 `enabled` 字段（true/false），
// 服务端会持久化开关状态并通过 control topic 广播 `danmaku_toggled` 事件。
func (h *CompanionHandler) ToggleSessionDanmaku(ctx context.Context, c *app.RequestContext) {
	userID, ok := h.authUserID(c)
	if !ok {
		return
	}
	var req toggleDanmakuRequest
	body := c.Request.Body()
	if len(bytes.TrimSpace(body)) == 0 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "enabled is required"})
		return
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Enabled == nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "enabled is required"})
		return
	}
	state, err := h.svc.SetSessionDanmakuEnabled(ctx, userID, c.Param("session_id"), *req.Enabled)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(state))
}

// ListNearby handles GET /api/v1/companion/session/nearby.
//
// 查询参数：
//   - latitude / longitude（必填，WGS84，浮点）；
//   - radius_m（可选，米；默认 5000，最大 20000）；
//   - limit（可选；默认/最大 50）。
func (h *CompanionHandler) ListNearby(ctx context.Context, c *app.RequestContext) {
	userID, ok := h.authUserID(c)
	if !ok {
		return
	}
	latStr := strings.TrimSpace(string(c.Query("latitude")))
	lonStr := strings.TrimSpace(string(c.Query("longitude")))
	if latStr == "" || lonStr == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "latitude and longitude are required"})
		return
	}
	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid latitude"})
		return
	}
	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid longitude"})
		return
	}
	in := service.ListCompanionNearbyInput{Latitude: lat, Longitude: lon}
	if rawRadius := strings.TrimSpace(string(c.Query("radius_m"))); rawRadius != "" {
		radius, err := strconv.ParseFloat(rawRadius, 64)
		if err != nil || radius <= 0 {
			c.JSON(http.StatusBadRequest, utils.H{"error": "invalid radius_m"})
			return
		}
		in.RadiusMeters = radius
	}
	if rawLimit := strings.TrimSpace(string(c.Query("limit"))); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit <= 0 {
			c.JSON(http.StatusBadRequest, utils.H{"error": "invalid limit"})
			return
		}
		in.Limit = limit
	}
	result, err := h.svc.ListNearbySessions(ctx, userID, in)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(result))
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
	if err != nil {
		log.Printf("[location-ingest] read body failed: %v", err)
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	if jerr := json.Unmarshal(data, &req); jerr != nil {
		log.Printf("[location-ingest] unmarshal failed: %v body=%s", jerr, string(data))
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	if err := h.svc.IngestLocationFromMQTT(ctx, req); err != nil {
		log.Printf("[location-ingest] ingest failed: %v sid=%s uid=%d seq=%d", err, req.SessionID, req.UserID, req.Seq)
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
	if err != nil {
		log.Printf("[presence-ingest] read body failed: %v", err)
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	if jerr := json.Unmarshal(data, &req); jerr != nil {
		log.Printf("[presence-ingest] unmarshal failed: %v body=%s", jerr, string(data))
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	if err := h.svc.IngestPresenceFromMQTT(ctx, req); err != nil {
		log.Printf("[presence-ingest] ingest failed: %v sid=%s uid=%d status=%s", err, req.SessionID, req.UserID, req.Status)
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"result": "ok"})
}

// IngestMQTTDanmaku handles POST /api/v1/internal/companion/mqtt/danmaku-ingest.
//
// 由 EMQX Rule Engine 在收到上行弹幕消息（companion/{sid}/member/{uid}/danmaku）后回调，
// 服务端完成校验、限速、持久化，并通过 sessionDanmakuBroadcastTopic 广播给所有成员。
func (h *CompanionHandler) IngestMQTTDanmaku(ctx context.Context, c *app.RequestContext) {
	if !h.authorizeInternal(c) {
		return
	}
	var req service.CompanionMQTTDanmakuIngestInput
	data, err := c.Body()
	if err != nil {
		log.Printf("[danmaku-ingest] read body failed: %v", err)
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	if jerr := json.Unmarshal(data, &req); jerr != nil {
		log.Printf("[danmaku-ingest] unmarshal failed: %v body=%s", jerr, string(data))
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	if err := h.svc.IngestDanmakuFromMQTT(ctx, req); err != nil {
		log.Printf("[danmaku-ingest] ingest failed: %v sid=%s uid=%d", err, req.SessionID, req.UserID)
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
