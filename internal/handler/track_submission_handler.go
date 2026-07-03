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

type TrackSubmissionHandler struct {
	service *service.TrackSubmissionService
}

func NewTrackSubmissionHandler(svc *service.TrackSubmissionService) *TrackSubmissionHandler {
	return &TrackSubmissionHandler{service: svc}
}

func submissionUserID(c *app.RequestContext) (int64, bool) {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return 0, false
	}
	return meta.AuthUserID, true
}

func writeTrackSubmissionError(c *app.RequestContext, err error) {
	var invalid *service.InvalidArgumentError
	switch {
	case errors.As(err, &invalid):
		c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
	case errors.Is(err, repository.ErrNotFound):
		c.JSON(http.StatusNotFound, utils.H{"error": "not found"})
	case errors.Is(err, repository.ErrForbidden), errors.Is(err, service.ErrForbidden):
		c.JSON(http.StatusForbidden, utils.H{"error": "forbidden"})
	case errors.Is(err, repository.ErrAlreadyExists):
		c.JSON(http.StatusConflict, utils.H{"error": "submission state conflict"})
	default:
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
	}
}

func (h *TrackSubmissionHandler) Options(_ context.Context, c *app.RequestContext) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "track submission service not configured"})
		return
	}
	c.JSON(http.StatusOK, successResponse(h.service.Options()))
}

func (h *TrackSubmissionHandler) save(ctx context.Context, c *app.RequestContext, editing bool) {
	userID, ok := submissionUserID(c)
	if !ok {
		return
	}
	var input service.TrackSubmissionInput
	body, err := c.Body()
	if err != nil || json.Unmarshal(body, &input) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	sub, err := h.service.Submit(ctx, userID, c.Param("track_id"), input, editing)
	if err != nil {
		writeTrackSubmissionError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(sub))
}

func (h *TrackSubmissionHandler) Submit(ctx context.Context, c *app.RequestContext) {
	h.save(ctx, c, false)
}

func (h *TrackSubmissionHandler) Update(ctx context.Context, c *app.RequestContext) {
	h.save(ctx, c, true)
}

func (h *TrackSubmissionHandler) Get(ctx context.Context, c *app.RequestContext) {
	userID, ok := submissionUserID(c)
	if !ok {
		return
	}
	sub, err := h.service.Get(ctx, userID, c.Param("track_id"))
	if err != nil {
		writeTrackSubmissionError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(sub))
}

func (h *TrackSubmissionHandler) Withdraw(ctx context.Context, c *app.RequestContext) {
	userID, ok := submissionUserID(c)
	if !ok {
		return
	}
	if err := h.service.Withdraw(ctx, userID, c.Param("track_id")); err != nil {
		writeTrackSubmissionError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(StatusResult{Status: "withdrawn"}))
}
