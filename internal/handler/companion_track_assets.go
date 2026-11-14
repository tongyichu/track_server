package handler

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
)

func (h *CompanionHandler) AuthorizeTrackAssets(ctx context.Context, c *app.RequestContext) {
	decoded, err := url.PathUnescape(c.Param("filepath"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusNotFound, utils.H{"error": "not found"})
		return
	}
	file := strings.TrimPrefix(path.Clean("/"+decoded), "/")
	if !strings.HasPrefix(file, "raw_tracks/") {
		c.Next(ctx)
		return
	}
	name := strings.TrimPrefix(file, "raw_tracks/")
	if strings.Contains(name, "/") || name == "" {
		c.AbortWithStatusJSON(http.StatusNotFound, utils.H{"error": "not found"})
		return
	}
	user, ok := h.authUserID(c)
	if !ok {
		c.Abort()
		return
	}
	trackID := strings.TrimSuffix(name, path.Ext(name))
	allowed, err := h.svc.CanReadTrackFile(ctx, user, trackID)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, utils.H{"error": "track storage unavailable"})
		return
	}
	if !allowed {
		c.AbortWithStatusJSON(http.StatusNotFound, utils.H{"error": "not found"})
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Next(ctx)
}
