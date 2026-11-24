package admin

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

func (h *Handler) SetTrackPhotoService(s *service.TrackPhotoService) { h.photos = s }
func adminPhotoResponse(c *app.RequestContext, data any, err error) {
	c.Header("Cache-Control", "private, no-store")
	if errors.Is(err, repository.ErrConflict) {
		c.JSON(409, utils.H{"error": "照片已变化，请刷新"})
		return
	}
	if err != nil {
		writeAdminTrackSubmissionError(c, err)
		return
	}
	c.JSON(200, utils.H{"code": 0, "data": data})
}
func (h *Handler) photoQueue(ctx context.Context, c *app.RequestContext) {
	if string(c.Query("counts")) == "1" {
		n, err := h.photos.PendingCount(ctx)
		adminPhotoResponse(c, utils.H{"pending": n}, err)
		return
	}
	items, next, err := h.photos.Queue(ctx, string(c.Query("status")), string(c.Query("cursor")))
	if err != nil {
		adminPhotoResponse(c, nil, err)
		return
	}
	pending, err := h.photos.PendingCount(ctx)
	adminPhotoResponse(c, utils.H{"items": items, "next_cursor": next, "pending": pending}, err)
}
func (h *Handler) photoImage(ctx context.Context, c *app.RequestContext) {
	data, err := h.photos.Media(ctx, "", c.Param("id"), 0, true)
	if err != nil {
		adminPhotoResponse(c, nil, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Data(200, "image/jpeg", data)
}
func (h *Handler) photoReview(ctx context.Context, c *app.RequestContext) {
	var in service.PhotoDecision
	raw, err := c.Body()
	if err != nil || json.Unmarshal(raw, &in) != nil || in.Scenic != nil {
		c.JSON(400, utils.H{"error": "审核参数无效"})
		return
	}
	adminPhotoResponse(c, true, h.photos.Review(ctx, c.Param("id"), in))
}
func (h *Handler) scenicUserPhotos(ctx context.Context, c *app.RequestContext) {
	v, err := h.photos.Scenic(ctx, c.Param("spot_id"), string(c.Query("cursor")), string(c.Query("included")) != "1")
	adminPhotoResponse(c, v, err)
}
func (h *Handler) scenicPhotoDecision(ctx context.Context, c *app.RequestContext) {
	var in service.PhotoDecision
	raw, err := c.Body()
	spot := c.Param("spot_id")
	if err != nil || json.Unmarshal(raw, &in) != nil || in.Scenic == nil || (*in.Scenic != "-" && *in.Scenic != spot) {
		c.JSON(400, utils.H{"error": "无效景点"})
		return
	}
	if *in.Scenic == "-" {
		hidden := "-" + spot
		in.Scenic = &hidden
	}
	err = h.photos.CheckScenicPhoto(ctx, spot, c.Param("id"))
	if err == nil {
		err = h.photos.Review(ctx, c.Param("id"), in)
	}
	adminPhotoResponse(c, true, err)
}
func (h *Handler) scenicUserPhotoImage(ctx context.Context, c *app.RequestContext) {
	data, err := h.photos.ScenicMedia(ctx, c.Param("spot_id"), c.Param("id"), true)
	if err != nil {
		adminPhotoResponse(c, nil, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Data(200, "image/jpeg", data)
}
