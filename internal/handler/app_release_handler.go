package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"

	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/service"
)

// AppReleaseHandler 暴露 App 升级相关的“公开”接口（客户端使用，无需登录态）。
//
// 管理后台相关的发布、列表、删除接口在 internal/handler/admin/ 内独立处理，
// 走独立的 admin session 鉴权，不复用业务的 JWT 中间件。
type AppReleaseHandler struct {
	svc *service.AppReleaseService
}

// NewAppReleaseHandler 构造 AppReleaseHandler。
func NewAppReleaseHandler(svc *service.AppReleaseService) *AppReleaseHandler {
	return &AppReleaseHandler{svc: svc}
}

// CheckUpgrade handles GET /api/v1/upgrade/check?version_code=123
//
// 客户端在启动 / 切回前台时调用：根据当前平台与本地 version_code，
// 询问服务端是否有可升级的版本，以及是否需要强制升级。
// 平台从公共请求头 X-Platform 读取（由 RequestMetaMiddleware 提取）。
func (h *AppReleaseHandler) CheckUpgrade(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.svc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "app release service not configured"})
		return
	}

	var platformStr string
	if meta := middleware.GetRequestMeta(c); meta != nil {
		platformStr = meta.Platform
	}
	platform := models.AppReleasePlatform(platformStr)
	versionCodeStr := string(c.Query("version_code"))
	versionCode, err := strconv.ParseInt(versionCodeStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "version_code must be an integer"})
		return
	}

	result, err := h.svc.CheckUpgrade(ctx, platform, versionCode)
	if err != nil {
		var iae *service.InvalidArgumentError
		if errors.As(err, &iae) {
			c.JSON(http.StatusBadRequest, utils.H{"error": iae.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, successResponse(result))
}
