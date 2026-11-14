package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/service"
)

func (h *CompanionHandler) UpdateTrackPreviews(ctx context.Context, c *app.RequestContext) {
	user, ok := h.authUserID(c)
	if !ok {
		return
	}
	body, err := c.Body()
	if err != nil || len(body) > 64*1024 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid previews"})
		return
	}
	var images models.TrackPreviewImages
	if err = json.Unmarshal(body, &images); err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid previews"})
		return
	}
	if err = h.svc.UpdateMemberTrackPreviews(ctx, user, c.Param("session_id"), images); err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(StatusResult{Status: "ok"}))
}

func (h *CompanionHandler) UploadTrackBatch(ctx context.Context, c *app.RequestContext) {
	user, ok := h.authUserID(c)
	if !ok {
		return
	}
	body, err := c.Body()
	if err != nil || len(body) > 128*1024 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid point batch"})
		return
	}
	var input service.CompanionPointBatch
	if err = json.Unmarshal(body, &input); err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid point batch"})
		return
	}
	if err = h.svc.UploadTrackBatch(ctx, user, c.Param("session_id"), input); err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(StatusResult{Status: "ok"}))
}
func (h *CompanionHandler) CompleteMemberTrack(ctx context.Context, c *app.RequestContext) {
	user, ok := h.authUserID(c)
	if !ok {
		return
	}
	body, err := c.Body()
	if err != nil || len(body) > 64*1024 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid completion"})
		return
	}
	var input service.CompleteCompanionTrackInput
	if err = json.Unmarshal(body, &input); err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid completion"})
		return
	}
	track, err := h.svc.CompleteMemberTrack(ctx, user, c.Param("session_id"), input)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(utils.H{"track": track, "complete": true}))
}
func (h *CompanionHandler) Result(ctx context.Context, c *app.RequestContext) {
	user, ok := h.authUserID(c)
	if !ok {
		return
	}
	result, err := h.svc.Result(ctx, user, c.Param("session_id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, successResponse(result))
}
func (h *CompanionHandler) ResultPoints(ctx context.Context, c *app.RequestContext) {
	user, ok := h.authUserID(c)
	if !ok {
		return
	}
	member, e := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if e != nil || member <= 0 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid user_id"})
		return
	}
	after := 0
	if value := string(c.Query("after")); value != "" {
		after, e = strconv.Atoi(value)
		if e != nil {
			c.JSON(http.StatusBadRequest, utils.H{"error": "invalid cursor"})
			return
		}
	}
	page, err := h.svc.ResultPoints(ctx, user, c.Param("session_id"), member, after)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, successResponse(page))
}
func (h *CompanionHandler) ResultTrack(ctx context.Context, c *app.RequestContext) {
	user, ok := h.authUserID(c)
	if !ok {
		return
	}
	member, e := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if e != nil || member <= 0 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid user_id"})
		return
	}
	track, err := h.svc.MemberResultTrack(ctx, user, c.Param("session_id"), member)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, successResponse(track))
}
