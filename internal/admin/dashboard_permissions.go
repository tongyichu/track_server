package admin

import (
	"encoding/json"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
)

func writeDashboardResponse(auth *Authenticator, c *app.RequestContext, source utils.H) {
	data := source
	if auth != nil && auth.manager != nil {
		sess := auth.SessionFromRequest(c)
		if sess == nil || sess.Account == nil {
			c.JSON(403, utils.H{"error": "没有查看统计的权限"})
			return
		}
		// 独立页面复用统计时只返回自身模块，不能借工作台接口读取其他模块。
		scope := string(c.Query("scope"))
		if scope != "" {
			key := map[string]string{"tracks": "content", "track_submissions": "review", "feedbacks": "feedback"}[scope]
			data = utils.H{"period": source["period"], "range": source["range"], "updated_at": source["updated_at"], key: source[key]}
		}
		encoded, err := json.Marshal(data)
		if err != nil {
			c.JSON(500, utils.H{"error": "统计数据暂不可用"})
			return
		}
		if err := json.Unmarshal(encoded, &data); err != nil {
			c.JSON(500, utils.H{"error": "统计数据暂不可用"})
			return
		}
		can := func(module string) bool { return sess.Account.allows(module, false) }
		removeMetrics := func(keys ...string) {
			if metrics, ok := data["metrics"].(map[string]interface{}); ok {
				for _, key := range keys {
					delete(metrics, key)
				}
			}
		}
		removeTrend := func(keys ...string) {
			if items, ok := data["trend"].([]interface{}); ok {
				for _, item := range items {
					if row, ok := item.(map[string]interface{}); ok {
						for _, key := range keys {
							delete(row, key)
						}
					}
				}
			}
		}
		if !can("users") {
			removeMetrics("active_users", "new_users")
			removeTrend("active_users", "new_users")
		}
		if !can("tracks") {
			removeMetrics("uploaded_tracks", "upload_users")
			removeTrend("uploaded_tracks", "collections", "navigations", "likes", "comments")
			delete(data, "content")
			delete(data, "engagement")
		}
		if !can("track_submissions") {
			removeMetrics("submissions", "pending_submissions")
			delete(data, "review")
		}
		if !can("feedbacks") {
			removeMetrics("pending_feedback")
			delete(data, "feedback")
		}
		if !can("companions") {
			removeMetrics("active_companions")
			removeTrend("companion_sessions")
			delete(data, "companion")
		}
		if !can("users") || !can("tracks") || !can("track_submissions") {
			delete(data, "funnel")
		}
		if !can("route_groups") {
			if content, ok := data["content"].(map[string]interface{}); ok {
				delete(content, "active_route_groups")
				delete(content, "failed_map_jobs")
				delete(content, "pending_map_jobs")
			}
		}
	}
	c.JSON(200, utils.H{"code": 0, "data": data})
}
