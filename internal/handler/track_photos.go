package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

type photoHandler struct{ s *service.TrackPhotoService }

func photoResponse(c *app.RequestContext, result any, err error) {
	c.Header("Cache-Control", "private, no-store")
	if errors.Is(err, repository.ErrConflict) {
		respondSubmissionError(c, 409, "photo changed")
		return
	}
	if err != nil {
		writeTrackSubmissionError(c, err)
		return
	}
	c.JSON(200, successResponse(result))
}
func (h photoHandler) list(ctx context.Context, c *app.RequestContext) {
	user, ok := submissionUserID(c)
	if !ok {
		return
	}
	v, err := h.s.List(ctx, c.Param("track_id"), user)
	photoResponse(c, v, err)
}
func (h photoHandler) upload(ctx context.Context, c *app.RequestContext) {
	user, ok := submissionUserID(c)
	if !ok {
		return
	}
	var in service.PhotoUpload
	if c.Request.Header.ContentLength() > service.PhotoBytesLimit+8192 {
		respondSubmissionError(c, 413, "photo too large")
		return
	}
	form, err := c.MultipartForm()
	if err != nil || len(form.Value["metadata"]) != 1 || len(form.Value["metadata"][0]) > 2048 || len(form.File["image"]) != 1 || json.Unmarshal([]byte(form.Value["metadata"][0]), &in) != nil {
		respondSubmissionError(c, 400, "invalid photo metadata")
		return
	}
	header := form.File["image"][0]
	if header.Size <= 0 || header.Size > service.PhotoBytesLimit {
		respondSubmissionError(c, 400, "invalid photo size")
		return
	}
	file, err := header.Open()
	if err != nil {
		respondSubmissionError(c, 400, "invalid photo")
		return
	}
	defer file.Close()
	in.JPEG, err = io.ReadAll(io.LimitReader(file, service.PhotoBytesLimit+1))
	if err != nil || len(in.JPEG) > service.PhotoBytesLimit {
		respondSubmissionError(c, 400, "invalid photo")
		return
	}
	v, err := h.s.Upload(ctx, c.Param("track_id"), user, in)
	photoResponse(c, v, err)
}
func (h photoHandler) remove(ctx context.Context, c *app.RequestContext) {
	user, ok := submissionUserID(c)
	if !ok {
		return
	}
	photoResponse(c, true, h.s.Remove(ctx, c.Param("track_id"), c.Param("id"), user))
}
func (h photoHandler) media(ctx context.Context, c *app.RequestContext) {
	user, ok := submissionUserID(c)
	if !ok {
		return
	}
	v, err := h.s.Media(ctx, c.Param("track_id"), c.Param("id"), user, false)
	if err != nil {
		photoResponse(c, nil, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Data(200, "image/jpeg", v)
}
func (h photoHandler) scenicMedia(ctx context.Context, c *app.RequestContext) {
	data, err := h.s.ScenicMedia(ctx, c.Param("spot_id"), c.Param("id"), false)
	if err != nil {
		photoResponse(c, nil, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Data(200, "image/jpeg", data)
}
func (h photoHandler) scenic(ctx context.Context, c *app.RequestContext) {
	v, err := h.s.Scenic(ctx, c.Param("spot_id"), string(c.Query("cursor")), false)
	photoResponse(c, v, err)
}
