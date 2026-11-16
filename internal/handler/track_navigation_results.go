package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

func (h *TrackHandler) ListNavigationResultTracks(ctx context.Context, c *app.RequestContext) {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	limit := 0
	if raw := string(c.Query("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			c.JSON(http.StatusBadRequest, utils.H{"error": "invalid limit"})
			return
		}
		limit = value
	}
	page, err := h.trackSvc.ListNavigationResultTracks(ctx, meta.AuthUserID, c.Param("track_id"), string(c.Query("cursor")), limit)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, repository.ErrNotFound):
			status = http.StatusNotFound
		case errors.Is(err, repository.ErrForbidden):
			status = http.StatusForbidden
		case errors.As(err, new(*service.InvalidArgumentError)):
			status = http.StatusBadRequest
		}
		c.JSON(status, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, successResponse(page))
}
