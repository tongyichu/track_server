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
	danmakus, err := h.companionRepo.ListDanmakusBySessionID(work, sessionID, 30)
	if err != nil {
		c.JSON(503, utils.H{"error": "房间弹幕暂不可用，请重试"})
		return
	}
	ids := []int64{}
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
	c.JSON(200, utils.H{"code": 0, "data": utils.H{"room": utils.H{"session_id": room.SessionID, "title": room.Title, "owner_id": strconv.FormatInt(room.OwnerUserID, 10), "status": room.Status, "started_at": room.StartedAt}, "members": items, "positions": points, "messages": messages, "people": people, "sampled_at": snapshot.SampledAt, "status": snapshot.Status}})
}
