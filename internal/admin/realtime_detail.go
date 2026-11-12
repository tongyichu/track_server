package admin

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/service"
)

func (h *Handler) RealtimeClientDetail(ctx context.Context, c *app.RequestContext) {
	if !h.realtimeReady(c) {
		return
	}
	clientID := strings.TrimSpace(string(c.Query("client_id")))
	if clientID == "" || len(clientID) > 200 {
		c.JSON(400, utils.H{"error": "客户端ID无效"})
		return
	}
	if h.realtimeBroker == nil {
		c.JSON(503, utils.H{"error": "EMQX 管理接口未初始化"})
		return
	}
	snapshot := h.realtimeBroker.Snapshot(ctx)
	var client *service.RealtimeClient
	for i := range snapshot.Items {
		if snapshot.Items[i].ClientID == clientID {
			client = &snapshot.Items[i]
			break
		}
	}
	if client == nil {
		if snapshot.Status != "ok" || !snapshot.Complete {
			c.JSON(503, utils.H{"error": "连接采样暂不完整，请刷新列表后重试"})
		} else {
			c.JSON(404, utils.H{"error": "该连接已不在 Broker 保留的会话中，请刷新列表"})
		}
		return
	}
	work, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	items := []service.RealtimeMessage{}
	kind := "direct"
	if client.Module == "messages" {
		user, err := strconv.ParseInt(client.UserID, 10, 64)
		if err != nil || h.messages == nil {
			c.JSON(503, utils.H{"error": "消息内容暂不可用"})
			return
		}
		items, err = h.messages.RealtimeDirectMessages(work, user)
		if err != nil {
			c.JSON(503, utils.H{"error": "消息内容暂不可用，请重试"})
			return
		}
	} else {
		kind = "danmaku"
		// Room content keeps the same permission boundary as the companion detail page.
		sess := h.auth.SessionFromRequest(c)
		if sess == nil || sess.Account == nil || !sess.Account.allows("companions", false) {
			c.JSON(403, utils.H{"error": "查看房间弹幕需要与友同行查看权限"})
			return
		}
		if h.companionRepo == nil {
			c.JSON(503, utils.H{"error": "房间弹幕暂不可用"})
			return
		}
		rows, err := h.companionRepo.ListDanmakusBySessionID(work, client.SessionID, 30)
		if err != nil {
			c.JSON(503, utils.H{"error": "房间弹幕暂不可用，请重试"})
			return
		}
		for _, row := range rows {
			items = append(items, service.RealtimeMessage{ID: strconv.FormatInt(row.ID, 10), SenderID: strconv.FormatInt(row.UserID, 10), Content: row.Content, CreatedAt: row.CreatedAt})
		}
	}
	c.JSON(200, utils.H{"code": 0, "data": utils.H{"client": client, "status": snapshot.Status, "sampled_at": snapshot.SampledAt, "content_kind": kind, "items": items}})
}
