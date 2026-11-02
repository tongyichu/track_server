package admin

import (
	"context"
	"errors"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

func (h *Handler) SetTrackMapIndexService(s *service.TrackMapIndexService) { h.trackMapIndexSvc = s }

// GetTrackMapPreview serves read-only coordinate variants under track-view permission.
func (h *Handler) GetTrackMapPreview(ctx context.Context, c *app.RequestContext) {
	if h.trackMapIndexSvc == nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "轨迹预览服务未配置"})
		return
	}
	variants, err := h.trackMapIndexSvc.PreviewCoordinateSystems(ctx, c.Param("track_id"))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			handleAdminRepoError(c, err)
			return
		}
		c.JSON(http.StatusUnprocessableEntity, utils.H{"error": "无法读取原始轨迹，请稍后重试"})
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"coordinate_system": "GCJ02", "variants": variants}})
}
