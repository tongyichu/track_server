package admin

import (
	"context"
	"sort"
	"strconv"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

func (h *Handler) realtimeCompanionAccess(c *app.RequestContext) bool {
	sess := h.auth.SessionFromRequest(c)
	if sess == nil || sess.Account == nil || !sess.Account.allows("companions", false) {
		c.JSON(403, utils.H{"error": "查看房间网络需要与友同行查看权限"})
		return false
	}
	return true
}

func (h *Handler) RealtimeNetwork(ctx context.Context, c *app.RequestContext) {
	if !h.realtimeReady(c) {
		return
	}
	module := string(c.Query("module"))
	if module == "" {
		module = "messages"
	}
	if module != "messages" && module != "companions" {
		c.JSON(400, utils.H{"error": "网络类型无效"})
		return
	}
	if module == "companions" && !h.realtimeCompanionAccess(c) {
		return
	}
	if h.realtimeBroker == nil {
		c.JSON(503, utils.H{"error": "连接采样尚未初始化"})
		return
	}
	snapshot := h.realtimeBroker.Snapshot(ctx)
	nodes := []service.RealtimeNetworkNode{}
	edges := []service.RealtimeNetworkEdge{}
	data := utils.H{"module": module, "status": snapshot.Status, "message": snapshot.Message, "sampled_at": snapshot.SampledAt, "complete": snapshot.Complete, "nodes": nodes, "edges": edges, "total_users": nil, "total_rooms": nil}
	if snapshot.SampledAt == nil {
		c.JSON(200, utils.H{"code": 0, "data": data})
		return
	}
	work, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	grouped := map[string]*service.RealtimeNetworkNode{}
	allUsers, allRooms := map[string]bool{}, map[string]bool{}
	for _, client := range snapshot.Items {
		if !client.Connected || client.Module != module {
			continue
		}
		key := client.UserID
		if module == "companions" {
			key = "member:" + client.SessionID + ":" + client.UserID
			allRooms[client.SessionID] = true
		}
		allUsers[client.UserID] = true
		n := grouped[key]
		if n == nil {
			n = &service.RealtimeNetworkNode{ID: key, Kind: "user", UserID: client.UserID, ClientID: client.ClientID, SessionID: client.SessionID}
			grouped[key] = n
		}
		n.Connections++
	}
	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	limited := len(keys) > 200
	ids := []int64{}
	rooms := map[string]bool{}
	for _, key := range keys {
		if len(nodes) >= 200 {
			limited = true
			break
		}
		n := grouped[key]
		if module == "companions" && !rooms[n.SessionID] && len(rooms) >= 50 {
			limited = true
			continue
		}
		rooms[n.SessionID] = true
		nodes = append(nodes, *n)
		id, _ := strconv.ParseInt(n.UserID, 10, 64)
		ids = append(ids, id)
	}
	if h.messages == nil {
		c.JSON(503, utils.H{"error": "网络用户资料暂不可用"})
		return
	}
	people, err := h.messages.RealtimePeople(work, ids)
	if err != nil {
		c.JSON(503, utils.H{"error": "网络用户资料暂不可用，请重试"})
		return
	}
	for i := range nodes {
		person := people[nodes[i].UserID]
		nodes[i].Label = person.Nickname
		nodes[i].AvatarURL = rewriteAdminStaticURL(person.AvatarURL)
	}
	if module == "messages" {
		var complete bool
		edges, complete, err = h.messages.RealtimeNetworkEdges(work, ids)
		if err != nil {
			c.JSON(503, utils.H{"error": "私信关系暂不可用，请重试"})
			return
		}
		limited = limited || !complete
	} else {
		titles := map[string]string{}
		roomIDs := make([]string, 0, len(rooms))
		for id := range rooms {
			roomIDs = append(roomIDs, id)
		}
		sort.Strings(roomIDs)
		if len(rooms) > 0 {
			roomRepo, ok := h.companionRepo.(repository.AdminCompanionListRepository)
			if !ok {
				c.JSON(503, utils.H{"error": "房间资料暂不可用"})
				return
			}
			sessions, _, err := roomRepo.ListAdminCompanions(work, repository.AdminCompanionListFilter{SessionIDs: roomIDs}, nil, 50)
			if err != nil {
				c.JSON(503, utils.H{"error": "房间资料暂不可用，请重试"})
				return
			}
			for _, s := range sessions {
				titles[s.SessionID] = s.Title
			}
		}
		for _, n := range nodes {
			edges = append(edges, service.RealtimeNetworkEdge{ID: "membership:" + n.ID, Source: "room:" + n.SessionID, Target: n.ID, Kind: "membership"})
		}
		for _, id := range roomIDs {
			title := titles[id]
			if title == "" {
				title = "同行房间"
			}
			nodes = append(nodes, service.RealtimeNetworkNode{ID: "room:" + id, Kind: "room", Label: title, SessionID: id})
		}
	}
	data["nodes"], data["edges"], data["total_users"], data["total_rooms"] = nodes, edges, len(allUsers), len(allRooms)
	data["complete"], data["limited"] = snapshot.Complete && !limited, limited
	c.JSON(200, utils.H{"code": 0, "data": data})
}
