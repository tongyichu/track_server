package handler

import (
	"context"
	"embed"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
)

//go:embed static/track_type_icon/*.svg
var trackTypeIcons embed.FS

// TrackTypeIcon serves versioned MingCute assets without depending on the disk cache.
func TrackTypeIcon(_ context.Context, c *app.RequestContext) {
	data, err := trackTypeIcons.ReadFile("static/track_type_icon/" + c.Param("icon_file"))
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "private, max-age=31536000, immutable")
	c.Data(http.StatusOK, "image/svg+xml", data)
	if c.IsHead() {
		c.Response.Header.SetContentLength(len(data))
		c.Response.SkipBody = true
	}
}
