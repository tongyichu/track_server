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

func (h *Handler) RealtimeRoom(ctx context.Context, c *app.RequestContext) {
	if !h.realtimeReady(c) || !h.realtimeCompanionAccess(c) {
		return
	}
	sessionID := strings.TrimSpace(string(c.Query("session_id")))
	if sessionID == "" || len(sessionID) > 128 {
		c.JSON(400, utils.H{"error": "房间ID无效"})
		return
	}
	if h.realtimeBroker == nil || h.companionRepo == nil {
		c.JSON(503, utils.H{"error": "房间网络暂不可用"})
		return
	}
	snapshot := h.realtimeBroker.Snapshot(ctx)
	online := map[string]int{}
	for _, client := range snapshot.Items {
		if client.Module == "companions" && client.SessionID == sessionID && client.Connected {
			online[client.UserID]++
		}
	}
	if len(online) == 0 {
		if snapshot.SampledAt == nil || !snapshot.Complete || snapshot.Status == "stale" {
			c.JSON(503, utils.H{"error": "房间在线状态暂未知，请重试"})
			return
		}
		c.JSON(409, utils.H{"error": "房间已不在当前在线网络中，请刷新关系图"})
		return
	}
	work, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	room, err := h.companionRepo.FindSessionByID(work, sessionID)
	if err != nil || room == nil {
		c.JSON(503, utils.H{"error": "房间资料暂不可用，请重试"})
		return
	}
	members, err := h.companionRepo.ListMembers(work, sessionID)
	if err != nil {
		c.JSON(503, utils.H{"error": "成员资料暂不可用，请重试"})
		return
	}
	positions, err := h.companionRepo.ListPositions(work, sessionID)
	if err != nil {
		c.JSON(503, utils.H{"error": "位置快照暂不可用，请重试"})
		return
	}
	danmakus, err := h.companionRepo.ListDanmakusBySessionID(work, sessionID, 201)
	if err != nil {
		c.JSON(503, utils.H{"error": "房间弹幕暂不可用，请重试"})
		return
	}
	messagesLimited := len(danmakus) > 200
	if messagesLimited {
		danmakus = danmakus[:200]
	}
	events, err := h.companionRepo.ListEventsBySessionID(work, sessionID, nil, 101, false)
	if err != nil {
		c.JSON(503, utils.H{"error": "房间事件暂不可用，请重试"})
		return
	}
	eventsLimited := len(events) > 100
	if eventsLimited {
		events = events[:100]
	}
	ids := []int64{room.OwnerUserID}
	items := []utils.H{}
	for _, member := range members {
		id := strconv.FormatInt(member.UserID, 10)
		ids = append(ids, member.UserID)
		items = append(items, utils.H{"user_id": id, "role": member.Role, "member_status": member.MemberStatus, "connections": online[id], "last_seen_at": member.LastSeenAt})
	}
	points := map[string]utils.H{}
	for _, position := range positions {
		points[strconv.FormatInt(position.UserID, 10)] = utils.H{"latitude": position.Latitude, "longitude": position.Longitude, "coordinate_system": position.CoordinateSystem, "recorded_at": position.RecordedAt}
	}
	messages := []service.RealtimeMessage{}
	for _, message := range danmakus {
		ids = append(ids, message.UserID)
		messages = append(messages, service.RealtimeMessage{ID: strconv.FormatInt(message.ID, 10), SenderID: strconv.FormatInt(message.UserID, 10), Content: message.Content, CreatedAt: message.CreatedAt})
	}
	eventItems := []utils.H{}
	for _, event := range events {
		ids = append(ids, event.OwnerUserID)
		targetID := ""
		if event.TargetUserID > 0 {
			targetID = strconv.FormatInt(event.TargetUserID, 10)
			ids = append(ids, event.TargetUserID)
		}
		eventItems = append(eventItems, utils.H{"id": strconv.FormatInt(event.ID, 10), "owner_user_id": strconv.FormatInt(event.OwnerUserID, 10), "target_user_id": targetID, "event_type": event.EventType, "title": event.Title, "content": event.Content, "event_time": event.EventTime, "created_at": event.CreatedAt})
	}
	if h.messages == nil {
		c.JSON(503, utils.H{"error": "成员用户资料暂不可用"})
		return
	}
	people, err := h.messages.RealtimePeople(work, ids)
	if err != nil {
		c.JSON(503, utils.H{"error": "成员用户资料暂不可用，请重试"})
		return
	}
	for id, person := range people {
		person.AvatarURL = rewriteAdminStaticURL(person.AvatarURL)
		people[id] = person
	}
	c.JSON(200, utils.H{"code": 0, "data": utils.H{"room": utils.H{"session_id": room.SessionID, "title": room.Title, "owner_id": strconv.FormatInt(room.OwnerUserID, 10), "status": room.Status, "started_at": room.StartedAt}, "members": items, "positions": points, "messages": messages, "events": eventItems, "messages_limited": messagesLimited, "events_limited": eventsLimited, "people": people, "sampled_at": snapshot.SampledAt, "status": snapshot.Status}})
}
