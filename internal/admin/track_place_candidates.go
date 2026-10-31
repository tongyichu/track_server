package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

func (h *Handler) trackPlaceServiceReady(c *app.RequestContext) bool {
	if h == nil || h.trackPlaceSvc == nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "track place service unavailable"})
		return false
	}
	return true
}

func (h *Handler) trackPlaceServiceError(c *app.RequestContext, err error) {
	var invalid *service.InvalidArgumentError
	switch {
	case errors.As(err, &invalid):
		c.JSON(http.StatusBadRequest, utils.H{"error": invalid.Error()})
	case errors.Is(err, repository.ErrNotFound):
		c.JSON(http.StatusNotFound, utils.H{"error": "track place candidate not found"})
	case errors.Is(err, repository.ErrConflict):
		c.JSON(http.StatusConflict, utils.H{"error": "track source changed"})
	case errors.Is(err, repository.ErrForbidden):
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "track place service unavailable"})
	default:
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
	}
}

func (h *Handler) ListPlaceCandidates(ctx context.Context, c *app.RequestContext) {
	if !h.trackPlaceServiceReady(c) {
		return
	}
	result, err := h.trackPlaceSvc.ListPlaceCandidates(ctx, strings.TrimSpace(string(c.Query("cursor"))), parseAdminListLimit(string(c.Query("limit"))))
	if err != nil {
		h.trackPlaceServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": result})
}

func (h *Handler) ConfirmPlaceCandidate(ctx context.Context, c *app.RequestContext) {
	if !h.trackPlaceServiceReady(c) {
		return
	}
	var body struct {
		PlaceID string `json:"place_id"`
	}
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &body) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	if err := h.trackPlaceSvc.ConfirmPlaceCandidate(ctx, c.Param("track_id"), body.PlaceID, h.adminOperator(c)); err != nil {
		h.trackPlaceServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"confirmed": true}})
}

func (h *Handler) DismissPlaceCandidate(ctx context.Context, c *app.RequestContext) {
	if !h.trackPlaceServiceReady(c) {
		return
	}
	if err := h.trackPlaceSvc.DismissPlaceCandidate(ctx, c.Param("track_id")); err != nil {
		h.trackPlaceServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"dismissed": true}})
}
