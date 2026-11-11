package admin

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/service"
	"strconv"
	"strings"
	"time"
)

func (h *Handler) SetRealtime(m *service.RealtimeMonitor, b *service.RealtimeBroker) {
	h.realtime = m
	h.realtimeBroker = b
}
func (h *Handler) realtimeReady(c *app.RequestContext) bool {
	c.Response.Header.Set("Cache-Control", "no-store")
	if h.realtime == nil {
		c.JSON(503, utils.H{"error": "实时通信监控尚未初始化"})
		return false
	}
	return true
}
func (h *Handler) RealtimeSummary(ctx context.Context, c *app.RequestContext) {
	if !h.realtimeReady(c) {
		return
	}
	state, _ := h.realtime.Snapshot()
	work, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	backlog, err := h.messages.RealtimeBacklog(work)
	if err != nil {
		backlog = service.RealtimeBacklog{Status: "unavailable"}
	}
	c.JSON(200, utils.H{"code": 0, "data": utils.H{"runtime": state, "backlog": backlog, "at": time.Now().UTC()}})
}
func realtimePage(c *app.RequestContext, total int) (int, int, int) {
	page, _ := strconv.Atoi(string(c.Query("page")))
	if page < 1 {
		page = 1
	}
	if page > 100000 {
		page = 100000
	}
	limit := 30
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	return page, start, end
}
func realtimeFilter(c *app.RequestContext) (module, q string, ok bool) {
	module = string(c.Query("module"))
	q = strings.TrimSpace(string(c.Query("q")))
	if (module != "" && module != "messages" && module != "companions" && module != "server" && module != "unknown") || len(q) > 200 {
		c.JSON(400, utils.H{"error": "筛选条件无效"})
		return "", "", false
	}
	return module, strings.ToLower(q), true
}
func (h *Handler) RealtimeClients(ctx context.Context, c *app.RequestContext) {
	if !h.realtimeReady(c) {
		return
	}
	module, q, ok := realtimeFilter(c)
	if !ok {
		return
	}
	if module != "" && module != "messages" && module != "companions" {
		c.JSON(400, utils.H{"error": "连接业务类型无效"})
		return
	}
	state := string(c.Query("state"))
	if state != "" && state != "connected" && state != "disconnected" {
		c.JSON(400, utils.H{"error": "连接状态无效"})
		return
	}
	if h.realtimeBroker == nil {
		c.JSON(503, utils.H{"error": "EMQX 管理接口未初始化"})
		return
	}
	snapshot := h.realtimeBroker.Snapshot(ctx)
	filtered := []service.RealtimeClient{}
	online, messages, companions := 0, 0, 0
	for _, item := range snapshot.Items {
		if item.Connected {
			online++
			if item.Module == "messages" {
				messages++
			} else {
				companions++
			}
		}
		if module != "" && item.Module != module || state == "connected" && !item.Connected || state == "disconnected" && item.Connected {
			continue
		}
		if q != "" && item.UserID != q && !strings.Contains(strings.ToLower(item.ClientID), q) && !strings.Contains(strings.ToLower(item.SessionID), q) {
			continue
		}
		filtered = append(filtered, item)
	}
	page, start, end := realtimePage(c, len(filtered))
	items := append([]service.RealtimeClient{}, filtered[start:end]...)
	ids := []int64{}
	seen := map[int64]bool{}
	for _, item := range items {
		id, _ := strconv.ParseInt(item.UserID, 10, 64)
		if id > 0 && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) > 0 {
		work, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		people, err := h.userRepo.FindByIDs(work, ids)
		if err != nil {
			c.JSON(503, utils.H{"error": "连接用户信息暂不可用，请重试"})
			return
		}
		for i := range items {
			id, _ := strconv.ParseInt(items[i].UserID, 10, 64)
			if p := people[id]; p != nil {
				items[i].Nickname = p.Nickname
			} else {
				items[i].Nickname = "用户已注销"
			}
		}
	}
	var onlineCount, messageCount, companionCount, totalCount any
	if snapshot.SampledAt != nil {
		onlineCount, messageCount, companionCount, totalCount = online, messages, companions, len(filtered)
	}
	c.JSON(200, utils.H{"code": 0, "data": utils.H{"status": snapshot.Status, "message": snapshot.Message, "sampled_at": snapshot.SampledAt, "complete": snapshot.Complete,
		"online": onlineCount, "messages_online": messageCount, "companions_online": companionCount, "total": totalCount, "page": page, "has_more": end < len(filtered), "items": items}})
}
func (h *Handler) RealtimeEvents(_ context.Context, c *app.RequestContext) {
	if !h.realtimeReady(c) {
		return
	}
	module, q, ok := realtimeFilter(c)
	if !ok {
		return
	}
	_, events := h.realtime.Snapshot()
	filtered := []service.RealtimeEvent{}
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if module != "" && module != e.Module {
			continue
		}
		if q != "" && e.UserID != q && !strings.Contains(strings.ToLower(e.ClientID), q) && !strings.Contains(strings.ToLower(e.SessionID), q) {
			continue
		}
		filtered = append(filtered, e)
	}
	page, start, end := realtimePage(c, len(filtered))
	c.JSON(200, utils.H{"code": 0, "data": utils.H{"items": filtered[start:end], "total": len(filtered), "page": page, "has_more": end < len(filtered)}})
}
