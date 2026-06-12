package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"

	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

type FeedbackHandler struct {
	feedbackSvc   *service.FeedbackService
	internalToken string
}

func NewFeedbackHandler(feedbackSvc *service.FeedbackService, internalToken string) *FeedbackHandler {
	return &FeedbackHandler{feedbackSvc: feedbackSvc, internalToken: strings.TrimSpace(internalToken)}
}

func (h *FeedbackHandler) Submit(ctx context.Context, c *app.RequestContext) {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	if h == nil || h.feedbackSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "feedback service not configured"})
		return
	}
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid multipart payload"})
		return
	}
	files := form.File["images"]
	if len(files) == 0 {
		files = form.File["image"]
	}
	feedback, err := h.feedbackSvc.Submit(ctx, service.SubmitFeedbackInput{
		UserID:        meta.AuthUserID,
		Content:       formValue(form.Value, "content"),
		Images:        files,
		Contact:       formValue(form.Value, "contact"),
		AppVersion:    firstNonEmpty(formValue(form.Value, "app_version"), meta.ClientVersion),
		Platform:      firstNonEmpty(formValue(form.Value, "platform"), meta.Platform),
		DeviceModel:   formValue(form.Value, "device_model"),
		SystemVersion: formValue(form.Value, "system_version"),
	})
	if err != nil {
		h.writeFeedbackError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(feedback))
}

func (h *FeedbackHandler) ListMine(ctx context.Context, c *app.RequestContext) {
	userID, ok := h.authUserID(c)
	if !ok {
		return
	}
	cursor, err := service.ParseFeedbackCursor(string(c.Query("cursor")))
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
		return
	}
	page, err := h.feedbackSvc.ListMine(ctx, userID, cursor, parseIntQuery(c, "limit", 20))
	if err != nil {
		h.writeFeedbackError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(page))
}

func (h *FeedbackHandler) GetMine(ctx context.Context, c *app.RequestContext) {
	userID, ok := h.authUserID(c)
	if !ok {
		return
	}
	item, err := h.feedbackSvc.GetMine(ctx, userID, c.Param("feedback_id"))
	if err != nil {
		h.writeFeedbackError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(item))
}

func (h *FeedbackHandler) GetMineImage(ctx context.Context, c *app.RequestContext) {
	userID, ok := h.authUserID(c)
	if !ok {
		return
	}
	file, err := h.feedbackSvc.GetImageFile(ctx, userID, c.Param("feedback_id"), c.Param("image_id"), false)
	if err != nil {
		h.writeFeedbackError(c, err)
		return
	}
	h.writeImage(c, file)
}

func (h *FeedbackHandler) ListOps(ctx context.Context, c *app.RequestContext) {
	if !h.checkInternalToken(c) {
		return
	}
	cursor, err := service.ParseFeedbackCursor(string(c.Query("cursor")))
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
		return
	}
	page, err := h.feedbackSvc.ListOps(ctx, models.FeedbackStatus(strings.TrimSpace(string(c.Query("status")))), cursor, parseIntQuery(c, "limit", 20))
	if err != nil {
		h.writeFeedbackError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(page))
}

func (h *FeedbackHandler) GetOps(ctx context.Context, c *app.RequestContext) {
	if !h.checkInternalToken(c) {
		return
	}
	item, err := h.feedbackSvc.GetOps(ctx, c.Param("feedback_id"))
	if err != nil {
		h.writeFeedbackError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(item))
}

func (h *FeedbackHandler) UpdateOpsStatus(ctx context.Context, c *app.RequestContext) {
	if !h.checkInternalToken(c) {
		return
	}
	var body struct {
		Status models.FeedbackStatus `json:"status"`
		Reply  string                `json:"reply"`
	}
	if err := json.Unmarshal(c.Request.Body(), &body); err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	if err := h.feedbackSvc.UpdateStatus(ctx, service.UpdateFeedbackStatusInput{
		FeedbackID: c.Param("feedback_id"),
		Status:     body.Status,
		Reply:      body.Reply,
	}); err != nil {
		h.writeFeedbackError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(StatusResult{Status: "ok"}))
}

func (h *FeedbackHandler) GetOpsImage(ctx context.Context, c *app.RequestContext) {
	if !h.checkInternalToken(c) {
		return
	}
	file, err := h.feedbackSvc.GetImageFile(ctx, 0, c.Param("feedback_id"), c.Param("image_id"), true)
	if err != nil {
		h.writeFeedbackError(c, err)
		return
	}
	h.writeImage(c, file)
}

func (h *FeedbackHandler) authUserID(c *app.RequestContext) (int64, bool) {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return 0, false
	}
	if h == nil || h.feedbackSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "feedback service not configured"})
		return 0, false
	}
	return meta.AuthUserID, true
}

func (h *FeedbackHandler) checkInternalToken(c *app.RequestContext) bool {
	if h == nil || h.feedbackSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "feedback service not configured"})
		return false
	}
	if h.internalToken == "" {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "ops internal auth not configured"})
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

func (h *FeedbackHandler) writeFeedbackError(c *app.RequestContext, err error) {
	var iae *service.InvalidArgumentError
	switch {
	case errors.As(err, &iae),
		errors.Is(err, service.ErrFeedbackImageTooMany),
		errors.Is(err, service.ErrFeedbackImageType),
		errors.Is(err, service.ErrFeedbackReplyRequired):
		c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
	case errors.Is(err, service.ErrFeedbackImageTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, utils.H{"error": err.Error()})
	case errors.Is(err, service.ErrFeedbackOpenLimitExceeded):
		c.JSON(http.StatusTooManyRequests, utils.H{"error": err.Error()})
	case errors.Is(err, repository.ErrNotFound):
		c.JSON(http.StatusNotFound, utils.H{"error": "not found"})
	case errors.Is(err, repository.ErrForbidden):
		c.JSON(http.StatusForbidden, utils.H{"error": "forbidden"})
	default:
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
	}
}

func (h *FeedbackHandler) writeImage(c *app.RequestContext, file *service.FeedbackImageFile) {
	data, err := os.ReadFile(file.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.JSON(http.StatusNotFound, utils.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.Response.SetStatusCode(http.StatusOK)
	c.Response.Header.SetContentType(file.MimeType)
	c.Response.SetBody(data)
}

func formValue(values map[string][]string, key string) string {
	if len(values[key]) == 0 {
		return ""
	}
	return values[key][0]
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func parseIntQuery(c *app.RequestContext, key string, def int) int {
	raw := strings.TrimSpace(string(c.Query(key)))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}
