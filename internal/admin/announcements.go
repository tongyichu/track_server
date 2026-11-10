package admin

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

func (h *Handler) SetMessageService(s *service.MessageService) { h.messages = s }
func (h *Handler) messagesReady(c *app.RequestContext) bool {
	if h.messages == nil || !h.messages.Enabled() {
		c.JSON(503, utils.H{"error": "通知服务暂不可用，请检查持久化存储"})
		return false
	}
	return true
}
func announcementError(c *app.RequestContext, err error) {
	status := 500
	message := "通知操作未完成，请重试"
	var invalid *service.InvalidArgumentError
	if errors.As(err, &invalid) {
		status = 400
		message = err.Error()
	} else if errors.Is(err, repository.ErrConflict) {
		status = 409
		message = "通知已被其他操作修改，请重新打开核对"
	} else if errors.Is(err, repository.ErrNotFound) {
		status = 404
		message = "通知不存在"
	}
	c.JSON(status, utils.H{"error": message})
}
func (h *Handler) ListAnnouncements(ctx context.Context, c *app.RequestContext) {
	if !h.messagesReady(c) {
		return
	}
	v, err := h.messages.ListAdminAnnouncements(ctx, string(c.Query("cursor")), parseAdminListLimit(string(c.Query("limit"))))
	if err != nil {
		announcementError(c, err)
		return
	}
	c.JSON(200, utils.H{"code": 0, "data": v})
}
func (h *Handler) GetAnnouncement(ctx context.Context, c *app.RequestContext) {
	if !h.messagesReady(c) {
		return
	}
	v, err := h.messages.GetAdminAnnouncement(ctx, c.Param("notice_id"))
	if err != nil {
		announcementError(c, err)
		return
	}
	c.JSON(200, utils.H{"code": 0, "data": v})
}
func (h *Handler) SaveAnnouncement(ctx context.Context, c *app.RequestContext) {
	if !h.messagesReady(c) {
		return
	}
	var in struct {
		ExpectedVersion int64               `json:"expected_version"`
		Notice          models.Announcement `json:"notice"`
	}
	if len(c.Request.Body()) > 64<<10 || json.Unmarshal(c.Request.Body(), &in) != nil {
		c.JSON(400, utils.H{"error": "通知格式无效"})
		return
	}
	v, err := h.messages.SaveAnnouncement(ctx, h.adminOperator(c), c.Param("notice_id"), in.ExpectedVersion, in.Notice)
	if err != nil {
		announcementError(c, err)
		return
	}
	c.JSON(200, utils.H{"code": 0, "data": v})
}
