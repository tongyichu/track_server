package admin

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
)

func writeAccountError(c *app.RequestContext, err error) {
	status, message := accountErrorStatus(err)
	c.JSON(status, utils.H{"error": message})
}

func readAccountBody(c *app.RequestContext, body interface{}) bool {
	data, err := c.Body()
	if err != nil || len(data) > 32*1024 || json.Unmarshal(data, body) != nil {
		c.JSON(400, utils.H{"error": "请求内容无效"})
		return false
	}
	return true
}

func (a *Authenticator) HandleAccounts(ctx context.Context, c *app.RequestContext) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	items, err := a.manager.list(ctx)
	if err != nil {
		writeAccountError(c, err)
		return
	}
	c.JSON(200, utils.H{"code": 0, "data": utils.H{"items": items, "modules": permissionModules}})
}

func (a *Authenticator) HandleSaveAccount(ctx context.Context, c *app.RequestContext) {
	var body accountInput
	if !readAccountBody(c, &body) {
		return
	}
	create := string(c.Method()) == "POST"
	username := c.Param("username")
	if create {
		username = body.Username
	}
	sess := a.SessionFromRequest(c)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := a.manager.saveAccount(ctx, sess.Username, username, body, create); err != nil {
		writeAccountError(c, err)
		return
	}
	if !body.Enabled {
		a.store.DeleteUsername(username)
	}
	if value, ok := c.Get("admin.audit"); ok && create {
		entry := value.(AuditEntry)
		entry.Path += "/" + username
		c.Set("admin.audit", entry)
	}
	c.JSON(200, utils.H{"code": 0, "data": utils.H{"status": "ok"}})
}

func (a *Authenticator) HandleResetPassword(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Password string `json:"password"`
		Revision int64  `json:"revision"`
	}
	if !readAccountBody(c, &body) {
		return
	}
	sess := a.SessionFromRequest(c)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := a.manager.resetPassword(ctx, sess.Username, c.Param("username"), body.Password, body.Revision); err != nil {
		writeAccountError(c, err)
		return
	}
	a.store.DeleteUsername(c.Param("username"))
	c.JSON(200, utils.H{"code": 0, "data": utils.H{"status": "ok"}})
}

func (a *Authenticator) HandleChangePassword(ctx context.Context, c *app.RequestContext) {
	var body struct {
		OldPassword string `json:"old_password"`
		Password    string `json:"password"`
	}
	if !readAccountBody(c, &body) {
		return
	}
	sess := a.SessionFromRequest(c)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := a.manager.changePassword(ctx, sess, body.OldPassword, body.Password); err != nil {
		writeAccountError(c, err)
		return
	}
	a.store.DeleteUsername(sess.Username)
	c.JSON(200, utils.H{"code": 0, "data": utils.H{"status": "ok"}})
}

func (a *Authenticator) HandleAuditLogs(ctx context.Context, c *app.RequestContext) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	username := strings.TrimSpace(string(c.Query("username")))
	items, err := a.manager.storage.ListAudit(ctx, username, 100)
	if err != nil {
		writeAccountError(c, err)
		return
	}
	c.JSON(200, utils.H{"code": 0, "data": utils.H{"items": items}})
}
