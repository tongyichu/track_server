package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

// UpdateLocation handles PUT /api/v1/user/profile/location.
func (h *UserHandler) UpdateLocation(ctx context.Context, c *app.RequestContext) {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	var body struct {
		CityCode string `json:"city_code"`
	}
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &body) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	if err := h.userSvc.UpdateLocation(ctx, meta.AuthUserID, body.CityCode); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidUserLocation):
			c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
		case errors.Is(err, repository.ErrNotFound):
			c.JSON(http.StatusNotFound, utils.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, successResponse(StatusResult{Status: "ok"}))
}
