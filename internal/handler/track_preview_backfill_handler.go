package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

func previewBackfillError(c *app.RequestContext, err error) {
	status := http.StatusServiceUnavailable
	var invalid *service.InvalidArgumentError
	switch {
	case errors.Is(err, service.ErrPreviewSourceChanged):
		c.JSON(http.StatusConflict, utils.H{"error": err.Error(), "error_code": "preview_source_changed"})
		return
	case errors.Is(err, service.ErrDescriptionSourceChanged):
		c.JSON(http.StatusConflict, utils.H{"error": err.Error(), "error_code": "description_source_changed"})
		return
	case errors.Is(err, repository.ErrNotFound):
		status = http.StatusNotFound
	case errors.As(err, &invalid):
		status = http.StatusBadRequest
	}
	c.JSON(status, utils.H{"error": err.Error()})
}

func (h *TrackHandler) ListPreviewBackfill(ctx context.Context, c *app.RequestContext) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	page, err := h.trackSvc.ListPreviewBackfill(ctx, c.Query("cursor"), limit)
	if err != nil {
		previewBackfillError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(page))
}

func (h *TrackHandler) ListDescriptionBackfill(ctx context.Context, c *app.RequestContext) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	ruleVersion, _ := strconv.Atoi(c.Query("rule_version"))
	if ruleVersion == 0 {
		ruleVersion = service.CurrentAutoDescriptionRuleVersion
	}
	page, err := h.trackSvc.ListDescriptionBackfill(ctx, c.Query("cursor"), limit, ruleVersion)
	if err != nil {
		previewBackfillError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(page))
}

func (h *TrackHandler) DescriptionBackfillData(ctx context.Context, c *app.RequestContext) {
	data, err := h.trackSvc.DescriptionBackfillData(ctx, c.Param("track_id"), c.Query("source_version"))
	if err != nil {
		previewBackfillError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, successResponse(data))
}

func (h *TrackHandler) CompleteDescriptionBackfill(ctx context.Context, c *app.RequestContext) {
	var input service.DescriptionBackfillInput
	if err := c.BindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	data, err := h.trackSvc.CompleteDescriptionBackfill(ctx, c.Param("track_id"), input)
	if err != nil {
		previewBackfillError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(data))
}

func (h *TrackHandler) ClearPreviewBackfill(ctx context.Context, c *app.RequestContext) {
	var input service.PreviewBackfillClearInput
	if err := c.BindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	data, err := h.trackSvc.ClearPreviewBackfill(ctx, input)
	if err != nil {
		previewBackfillError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, successResponse(data))
}

func (h *TrackHandler) PreviewBackfillData(ctx context.Context, c *app.RequestContext) {
	data, err := h.trackSvc.PreviewBackfillData(ctx, c.Param("track_id"), c.Query("source_version"))
	if err != nil {
		previewBackfillError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, successResponse(data))
}

func (h *TrackHandler) PreparePreviewBackfill(ctx context.Context, c *app.RequestContext) {
	var input service.PreviewBackfillInput
	if err := c.BindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	data, err := h.trackSvc.PreparePreviewBackfill(ctx, c.Param("track_id"), input)
	if err != nil {
		previewBackfillError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, successResponse(data))
}

func (h *TrackHandler) CompletePreviewBackfill(ctx context.Context, c *app.RequestContext) {
	var input service.PreviewBackfillInput
	if err := c.BindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	data, err := h.trackSvc.CompletePreviewBackfill(ctx, c.Param("track_id"), input)
	if err != nil {
		previewBackfillError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(data))
}
