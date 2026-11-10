package admin

import (
	"context"
	"log"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
)

func pageModule(name string) string {
	page := "/admin/" + path.Base(name)
	for _, module := range permissionModules {
		if module.Page == page {
			return module.ID
		}
	}
	if page == "/admin/companion_detail.html" {
		return "companions"
	}
	if page == "/admin/accounts.html" {
		return "accounts"
	}
	return ""
}

func requestPermission(c *app.RequestContext) (string, bool) {
	route := strings.TrimPrefix(c.FullPath(), "/admin/api/")
	write := string(c.Method()) != http.MethodGet && string(c.Method()) != http.MethodHead
	if route == "dashboard" {
		scope := string(c.Query("scope"))
		switch scope {
		case "tracks", "track_submissions", "feedbacks":
			return scope, false
		case "":
			return "dashboard", false
		default:
			return "", false
		}
	}
	parts := strings.Split(route, "/")
	switch parts[0] {
	case "announcements":
		return "announcements", write
	case "accounts", "audit-logs":
		return "accounts", write
	case "users":
		// Profile overview and its protected background image share users read permission.
		return "users", write
	// Asset GETs use tracks read permission; AI generation POSTs require write permission and consume quota.
	case "tracks", "place-candidates":
		return "tracks", write
	case "scenic-spots":
		return "scenic_spots", write
	case "regions":
		// Country DELETE and Pixabay POSTs require region write permission and audit.
		return "regions", write
	case "route-groups":
		return "route_groups", write
	case "track-submissions":
		return "track_submissions", write
	case "companions":
		return "companions", write
	case "feedbacks", "feedback-correction-config":
		return "feedbacks", write
	case "releases":
		return "releases", write || route == "releases/upload-token"
	case "ai":
		if !write {
			return "ai_usage", false
		}
		if len(parts) > 1 && (parts[1] == "policies" || parts[1] == "prices") {
			return "ai_settings", true
		}
		return "ai_tasks", true
	case "analytics":
		return "analytics", write
	}
	return "", write
}

func (a *Authenticator) authorize(ctx context.Context, c *app.RequestContext, sess *Session) bool {
	account := sess.Account
	route := c.FullPath()
	passwordRoute := route == "/admin/api/password"
	if account == nil || (account.MustChangePassword && !passwordRoute) {
		c.JSON(403, utils.H{"error": "请先修改初始密码", "password_change_required": true})
		return false
	}
	module, write := requestPermission(c)
	allowed := module != "" && account.allows(module, write)
	if passwordRoute {
		allowed = true
	}
	if route == "/admin/api/security" || strings.HasPrefix(route, "/admin/api/security/email/") {
		allowed = true
	}
	if route == "/admin/api/cities" {
		for _, module := range []string{"tracks", "scenic_spots", "users", "route_groups", "regions"} {
			allowed = allowed || account.allows(module, false)
		}
	}
	if route == "/admin/api/static/*filepath" {
		rel, valid := cleanAdminStaticPath(c.Param("filepath"))
		if !valid {
			c.JSON(400, utils.H{"error": "invalid path"})
			return false
		}
		category := strings.Split(rel, "/")[0]
		var modules []string
		switch category {
		case "avatars", "default_avatars":
			modules = []string{"users", "tracks", "track_submissions", "scenic_spots", "companions", "feedbacks", "route_groups"}
		case "screenshots":
			if strings.HasPrefix(path.Base(rel), "companion_") {
				modules = []string{"companions"}
			} else {
				modules = []string{"tracks"}
			}
		case "track_previews", "raw_tracks":
			modules = []string{"tracks"}
		case "scenic_spots":
			modules = []string{"scenic_spots"}
		case "regions":
			modules = []string{"regions"}
		case "release":
			modules = []string{"releases"}
		}
		allowed = account.Role == roleAdmin
		for _, module := range modules {
			allowed = allowed || account.allows(module, false)
		}
	}
	if strings.HasPrefix(route, "/admin/api/scenic-spots/") && (strings.Contains(route, "/tracks") || strings.HasSuffix(route, "/track-search")) {
		allowed = allowed && account.allows("tracks", write)
	}
	if route == "/admin/api/tracks/:track_id/scenic-spot" {
		allowed = allowed && account.allows("scenic_spots", true)
	}
	if strings.HasPrefix(route, "/admin/api/ai/") && write {
		allowed = allowed && account.allows("ai_usage", false)
	}
	if !allowed {
		c.JSON(http.StatusForbidden, utils.H{"error": "没有该页面的" + map[bool]string{false: "查看", true: "操作"}[write] + "权限"})
		return false
	}
	if write && !validAdminOrigin(c) {
		c.JSON(http.StatusForbidden, utils.H{"error": "请求来源无效"})
		return false
	}
	if write {
		id, err := randomToken(16)
		if err != nil {
			c.JSON(503, utils.H{"error": "无法记录操作，请稍后重试"})
			return false
		}
		entry := AuditEntry{ID: id + ":attempt", ActorID: account.ID, Username: account.Username, Method: string(c.Method()), Path: string(c.Path()), CreatedAt: time.Now().UTC()}
		auditCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = a.manager.storage.AppendAudit(auditCtx, entry)
		cancel()
		if err != nil {
			c.JSON(503, utils.H{"error": "操作日志暂不可用，请稍后重试"})
			return false
		}
		entry.ID = id + ":result"
		c.Set("admin.audit", entry)
	}
	return true
}

func (a *Authenticator) finishAudit(c *app.RequestContext) {
	value, ok := c.Get("admin.audit")
	if !ok {
		return
	}
	entry := value.(AuditEntry)
	entry.Status, entry.CreatedAt = c.Response.StatusCode(), time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := a.manager.storage.AppendAudit(ctx, entry); err != nil {
		log.Printf("admin audit result unavailable: id=%s status=%d", entry.ID, entry.Status)
	}
}
