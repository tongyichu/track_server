package admin

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/service"
	"strconv"
)

func (h *Handler) SetPermissionSnapshotService(svc *service.PermissionSnapshotService) {
	h.permissionSnapshotSvc = svc
}
func (h *Handler) ListUserPermissionSnapshots(ctx context.Context, c *app.RequestContext) {
	id, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(400, utils.H{"error": "invalid user_id"})
		return
	}
	if h.permissionSnapshotSvc == nil {
		c.JSON(503, utils.H{"error": "权限快照服务暂不可用"})
		return
	}
	if _, err := h.userRepo.FindByID(ctx, id); err != nil {
		handleAdminRepoError(c, err)
		return
	}
	items, err := h.permissionSnapshotSvc.List(ctx, id)
	if err != nil {
		c.JSON(500, utils.H{"error": "读取权限快照失败"})
		return
	}
	c.JSON(200, utils.H{"code": 0, "data": utils.H{"items": items, "limit": 100}})
}
