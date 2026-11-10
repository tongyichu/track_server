package handler

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
	"strconv"
	"time"
)

type MessageHandler struct{ svc *service.MessageService }

func (h *MessageHandler) Sent(ctx context.Context, c *app.RequestContext) {
	user := h.viewer(c)
	if user == 0 {
		return
	}
	peer := messagePeer(c)
	if peer == 0 {
		return
	}
	v, err := h.svc.FindSentDirect(ctx, user, peer, c.Param("client_id"))
	messageResponse(c, v, err)
}

func (h *MessageHandler) viewer(c *app.RequestContext) int64 {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(401, utils.H{"error": "unauthorized"})
		return 0
	}
	if !h.svc.Enabled() {
		c.JSON(503, utils.H{"error": "消息服务暂不可用"})
		return 0
	}
	c.Response.Header.Set("Cache-Control", "no-store")
	return meta.AuthUserID
}
func messageResponse(c *app.RequestContext, value any, err error) {
	if err == nil {
		c.JSON(200, successResponse(value))
		return
	}
	status := 500
	message := "消息操作未完成，请重试"
	var invalid *service.InvalidArgumentError
	var restricted *service.AccountRestrictionBlockedError
	switch {
	case errors.As(err, &invalid):
		status = 400
		message = err.Error()
	case errors.As(err, &restricted):
		status = 403
		message = restricted.Message
	case errors.Is(err, repository.ErrNotFound):
		status = 404
		message = "内容不存在或已不可用"
	case errors.Is(err, repository.ErrForbidden):
		status = 403
		message = "没有权限访问该消息"
	case errors.Is(err, repository.ErrConflict):
		status = 409
		message = "消息状态已变化，请重试"
	}
	c.JSON(status, utils.H{"error": message, "message": message})
}
func messageBody(c *app.RequestContext, v any) bool {
	if len(c.Request.Body()) > 64<<10 || json.Unmarshal(c.Request.Body(), v) != nil {
		c.JSON(400, utils.H{"error": "消息请求格式无效"})
		return false
	}
	return true
}
func messagePeer(c *app.RequestContext) int64 {
	peer, _ := strconv.ParseInt(c.Param("peer_id"), 10, 64)
	if peer <= 0 {
		c.JSON(400, utils.H{"error": "用户ID无效"})
	}
	return peer
}
func messageLimit(c *app.RequestContext) int {
	v, _ := strconv.Atoi(string(c.Query("limit")))
	return v
}
func (h *MessageHandler) Summary(ctx context.Context, c *app.RequestContext) {
	user := h.viewer(c)
	if user == 0 {
		return
	}
	v, err := h.svc.Summary(ctx, user)
	messageResponse(c, v, err)
}
func (h *MessageHandler) Notices(ctx context.Context, c *app.RequestContext) {
	user := h.viewer(c)
	if user == 0 {
		return
	}
	v, err := h.svc.ListNotices(ctx, user, string(c.Query("cursor")), messageLimit(c))
	messageResponse(c, v, err)
}
func (h *MessageHandler) ReadNotices(ctx context.Context, c *app.RequestContext) {
	user := h.viewer(c)
	if user == 0 {
		return
	}
	var in struct {
		IDs []string `json:"ids"`
	}
	if !messageBody(c, &in) {
		return
	}
	messageResponse(c, utils.H{"ok": true}, h.svc.ReadNotices(ctx, user, in.IDs))
}
func (h *MessageHandler) Interactions(ctx context.Context, c *app.RequestContext) {
	user := h.viewer(c)
	if user == 0 {
		return
	}
	v, err := h.svc.ListInteractions(ctx, user, string(c.Query("category")), string(c.Query("cursor")), messageLimit(c))
	messageResponse(c, v, err)
}
func (h *MessageHandler) ReadInteractions(ctx context.Context, c *app.RequestContext) {
	user := h.viewer(c)
	if user == 0 {
		return
	}
	var in struct {
		IDs []string `json:"ids"`
	}
	if !messageBody(c, &in) {
		return
	}
	messageResponse(c, utils.H{"ok": true}, h.svc.ReadInteractions(ctx, user, in.IDs))
}
func (h *MessageHandler) ReadAll(ctx context.Context, c *app.RequestContext) {
	user := h.viewer(c)
	if user == 0 {
		return
	}
	var in struct {
		Cutoff time.Time `json:"cutoff"`
	}
	if !messageBody(c, &in) {
		return
	}
	messageResponse(c, utils.H{"ok": true}, h.svc.ReadAll(ctx, user, in.Cutoff))
}
func (h *MessageHandler) Conversations(ctx context.Context, c *app.RequestContext) {
	user := h.viewer(c)
	if user == 0 {
		return
	}
	v, err := h.svc.ListConversations(ctx, user, string(c.Query("cursor")), messageLimit(c), false)
	messageResponse(c, v, err)
}
func (h *MessageHandler) Blacklist(ctx context.Context, c *app.RequestContext) {
	user := h.viewer(c)
	if user == 0 {
		return
	}
	v, err := h.svc.ListConversations(ctx, user, string(c.Query("cursor")), messageLimit(c), true)
	messageResponse(c, v, err)
}
func (h *MessageHandler) Conversation(ctx context.Context, c *app.RequestContext) {
	user := h.viewer(c)
	if user == 0 {
		return
	}
	peer := messagePeer(c)
	if peer == 0 {
		return
	}
	v, err := h.svc.GetConversation(ctx, user, peer)
	messageResponse(c, v, err)
}
func (h *MessageHandler) Messages(ctx context.Context, c *app.RequestContext) {
	user := h.viewer(c)
	if user == 0 {
		return
	}
	peer := messagePeer(c)
	if peer == 0 {
		return
	}
	after, err := strconv.ParseInt(string(c.Query("after_seq")), 10, 64)
	if len(c.Query("after_seq")) > 0 && (err != nil || after < 0) {
		c.JSON(400, utils.H{"error": "消息游标无效"})
		return
	}
	if len(c.Query("after_seq")) == 0 {
		after = -1
	}
	v, err := h.svc.ListDirect(ctx, user, peer, string(c.Query("cursor")), after, messageLimit(c))
	messageResponse(c, v, err)
}
func (h *MessageHandler) Send(ctx context.Context, c *app.RequestContext) {
	user := h.viewer(c)
	if user == 0 {
		return
	}
	peer := messagePeer(c)
	if peer == 0 {
		return
	}
	var in service.SendDirectMessageInput
	if !messageBody(c, &in) {
		return
	}
	v, err := h.svc.SendDirect(ctx, user, peer, in)
	messageResponse(c, v, err)
}
func (h *MessageHandler) ReadConversation(ctx context.Context, c *app.RequestContext) {
	user := h.viewer(c)
	if user == 0 {
		return
	}
	peer := messagePeer(c)
	if peer == 0 {
		return
	}
	var in struct {
		ThroughSeq int64 `json:"through_seq"`
	}
	if !messageBody(c, &in) {
		return
	}
	messageResponse(c, utils.H{"ok": true}, h.svc.ReadConversation(ctx, user, peer, in.ThroughSeq))
}
func (h *MessageHandler) Block(ctx context.Context, c *app.RequestContext) {
	user := h.viewer(c)
	if user == 0 {
		return
	}
	peer := messagePeer(c)
	if peer == 0 {
		return
	}
	var in struct {
		Blocked bool `json:"blocked"`
	}
	if !messageBody(c, &in) {
		return
	}
	messageResponse(c, utils.H{"ok": true}, h.svc.SetBlocked(ctx, user, peer, in.Blocked))
}
func (h *MessageHandler) Credentials(ctx context.Context, c *app.RequestContext) {
	user := h.viewer(c)
	if user == 0 {
		return
	}
	v, err := h.svc.IssueMessageCredentials(ctx, user)
	if err != nil {
		c.JSON(503, utils.H{"error": "实时连接暂不可用，消息仍可正常收发"})
		return
	}
	messageResponse(c, v, nil)
}
