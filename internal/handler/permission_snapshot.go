package handler

import (
	"context"
	"errors"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/service"
)

func permissionSnapshotHandler(svc *service.PermissionSnapshotService) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		meta := middleware.GetRequestMeta(c)
		if meta == nil || meta.AuthUserID <= 0 {
			c.JSON(401, utils.H{"error": "unauthorized"})
			return
		}
		if svc == nil {
			c.JSON(503, utils.H{"error": "permission snapshots unavailable"})
			return
		}
		if len(c.Request.Body()) > 128*1024 {
			c.JSON(413, utils.H{"error": "snapshot too large"})
			return
		}
		var snapshot models.PermissionSnapshot
		if err := c.BindJSON(&snapshot); err != nil {
			c.JSON(400, utils.H{"error": "invalid snapshot"})
			return
		}
		if err := svc.Save(ctx, meta.AuthUserID, &snapshot); err != nil {
			var invalid *service.InvalidArgumentError
			if errors.As(err, &invalid) {
				c.JSON(400, utils.H{"error": err.Error()})
			} else {
				c.JSON(500, utils.H{"error": "snapshot save failed"})
			}
			return
		}
		c.JSON(200, utils.H{"code": 0, "data": utils.H{"status": "ok"}})
	}
}
