package admin

import (
	"context"
	"net/http"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/service"
)

func (h *Handler) SetTrackNarrativeService(svc *service.TrackNarrativeService) {
	h.trackNarrativeSvc = svc
}

func (h *Handler) GenerateTrackShape(ctx context.Context, c *app.RequestContext) {
	if h.trackNarrativeSvc == nil {
		c.JSON(503, utils.H{"error": "AI 标签服务未配置"})
		return
	}
	result, err := h.trackNarrativeSvc.PreviewShape(ctx, strings.TrimSpace(c.Param("track_id")))
	if err != nil {
		c.JSON(http.StatusConflict, utils.H{"error": err.Error()})
		return
	}
	c.JSON(200, utils.H{"code": 0, "data": utils.H{"shape_label": result.ShapeLabel, "saved": false, "diagnosis": result}})
}

func (h *Handler) GenerateTrackDescription(ctx context.Context, c *app.RequestContext) {
	if h.trackNarrativeSvc == nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "AI 路线说明服务未配置"})
		return
	}
	result, err := h.trackNarrativeSvc.PreviewDescription(ctx, strings.TrimSpace(c.Param("track_id")))
	if err != nil {
		c.JSON(http.StatusConflict, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"description": result.Description, "sources": result.Sources, "saved": false}})
}
