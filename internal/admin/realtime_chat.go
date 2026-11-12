package admin

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

func realtimeChatError(c *app.RequestContext, err error) {
	var invalid *service.InvalidArgumentError
	if errors.As(err, &invalid) {
		c.JSON(400, utils.H{"error": err.Error()})
	} else if errors.Is(err, repository.ErrNotFound) {
		c.JSON(404, utils.H{"error": "该对话暂无存储记录"})
	} else {
		c.JSON(503, utils.H{"error": "对话内容暂不可用，请重试"})
	}
}

func (h *Handler) RealtimeChat(ctx context.Context, c *app.RequestContext) {
	client, snapshot := h.realtimeClient(ctx, c)
	if client == nil {
		return
	}
	if client.Module != "messages" {
		c.JSON(400, utils.H{"error": "该连接不是消息提醒连接"})
		return
	}
	work, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	h.realtimeChatResponse(work, c, client, snapshot)
}

func (h *Handler) realtimeChatResponse(ctx context.Context, c *app.RequestContext, client *service.RealtimeClient, snapshot service.RealtimeClientsSnapshot) {
	if h.messages == nil {
		c.JSON(503, utils.H{"error": "消息服务暂不可用"})
		return
	}
	user, _ := strconv.ParseInt(client.UserID, 10, 64)
	path := c.FullPath()
	conversationsOnly := path == "/admin/api/realtime/clients/conversations"
	messagesOnly := path == "/admin/api/realtime/clients/messages"
	data := utils.H{"client": client, "status": snapshot.Status, "sampled_at": snapshot.SampledAt, "content_kind": "direct"}
	ids := []int64{user}
	var conversations *models.MessagePage[service.RealtimeConversation]
	var err error
	if !messagesOnly {
		conversations, err = h.messages.RealtimeConversations(ctx, user, string(c.Query("cursor")))
		if err != nil {
			realtimeChatError(c, err)
			return
		}
		data["conversations"] = conversations
		for _, conversation := range conversations.Items {
			peer, _ := strconv.ParseInt(conversation.PeerID, 10, 64)
			ids = append(ids, peer)
		}
	}
	if !conversationsOnly {
		peerText := string(c.Query("peer_id"))
		if !messagesOnly {
			peerText = ""
		}
		data["peer_id"] = peerText
		var page *service.RealtimeChatPage
		cursor := ""
		if messagesOnly {
			cursor = string(c.Query("cursor"))
		}
		if peerText == "" {
			if string(c.Query("after_seq")) != "" {
				c.JSON(400, utils.H{"error": "账号消息流请使用时间游标"})
				return
			}
			page, err = h.messages.RealtimeAccountMessages(ctx, user, cursor, string(c.Query("after")))
		} else {
			peer, parseErr := strconv.ParseInt(peerText, 10, 64)
			if parseErr != nil || peer <= 0 || peer == user {
				c.JSON(400, utils.H{"error": "对话用户无效"})
				return
			}
			ids = append(ids, peer)
			if string(c.Query("after")) != "" {
				c.JSON(400, utils.H{"error": "联系人筛选请使用序号游标"})
				return
			}
			page, err = h.messages.RealtimeDirectMessages(ctx, user, peer, cursor, string(c.Query("after_seq")))
		}
		if err != nil {
			realtimeChatError(c, err)
			return
		}
		for _, message := range page.Items {
			sender, _ := strconv.ParseInt(message.SenderID, 10, 64)
			receiver, _ := strconv.ParseInt(message.ReceiverID, 10, 64)
			ids = append(ids, sender, receiver)
		}
		data["messages"] = page
	}
	people, err := h.messages.RealtimePeople(ctx, ids)
	if err != nil {
		realtimeChatError(c, err)
		return
	}
	for id, person := range people {
		person.AvatarURL = rewriteAdminStaticURL(person.AvatarURL)
		people[id] = person
	}
	data["people"] = people
	c.JSON(200, utils.H{"code": 0, "data": data})
}
