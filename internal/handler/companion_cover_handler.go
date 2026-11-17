package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/repository"
)

func (h *CompanionHandler) CoverPreview(ctx context.Context, c *app.RequestContext) {
	viewer, ok := h.authUserID(c)
	if !ok {
		return
	}
	file, err := h.svc.CompanionCoverFile(ctx, viewer, c.Param("session_id"), c.Param("variant"), c.Query("v"))
	if err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, repository.ErrNotFound) {
			status = http.StatusNotFound
		}
		if errors.Is(err, repository.ErrForbidden) {
			status = http.StatusForbidden
		}
		c.JSON(status, utils.H{"error": "companion preview unavailable"})
		return
	}
	c.Header("Cache-Control", "private, max-age=86400")
	c.Header("X-Content-Type-Options", "nosniff")
	c.File(file)
}
