package admin

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/cloudwego/hertz/pkg/protocol"
	"golang.org/x/crypto/bcrypt"
)

// 管理后台 cookie 名称。
const (
	sessionCookieName = "admin_session"
)

// 恒定用于"用户名未命中"分支的占位哈希，用来让认证耗时接近命中分支，
// 降低通过时间差枚举用户名的可能性。该哈希对应明文为空且不会被写入账号表。
//
// 生成方式：bcrypt.GenerateFromPassword([]byte("placeholder"), 10)。
var placeholderPasswordHash = []byte("$2a$10$CwTycUXWue0Thq9StjUM0uJ8h3zP5g6Xs4bE8x1bQp7qNt6hX5mqW")

// Authenticator 使用独立的持久化后台账号与内存会话，不回退到环境变量直接鉴权。
type Authenticator struct {
	store    *SessionStore
	manager  *AccountManager
	recovery *emailRecovery
}

// AccountCount 返回已加载的有效管理员账号数量。
func (a *Authenticator) AccountCount() int {
	if a == nil {
		return 0
	}
	if a.manager != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		items, err := a.manager.list(ctx)
		if err != nil {
			return 0
		}
		return len(items)
	}
	return 0
}

// Verify 校验用户名+密码，正确则创建会话并返回。
//
// 侧信道加固：未命中用户名时也执行一次 bcrypt 占位比较，
// 让失败分支的总耗时接近命中分支，降低通过时间差枚举用户名的可能性。
func (a *Authenticator) Verify(username, password string) (*Session, error) {
	if a == nil {
		return nil, errAccountStoreUnavailable
	}
	if a.manager != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		account, err := a.manager.get(ctx, username)
		if err != nil {
			return nil, err
		}
		if account == nil {
			_ = bcrypt.CompareHashAndPassword(placeholderPasswordHash, []byte(password))
			return nil, accountFailure(401, "用户名或密码错误")
		}
		if bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(password)) != nil || !account.Enabled {
			return nil, accountFailure(401, "用户名或密码错误")
		}
		err = a.manager.mutate(ctx, "", func(state *accountState) error {
			current := state.Accounts[username]
			if current == nil || !current.Enabled || current.SecurityVersion != account.SecurityVersion || current.PasswordHash != account.PasswordHash {
				return accountFailure(401, "用户名或密码错误")
			}
			now := time.Now().UTC()
			current.LastLoginAt = &now
			account = current
			return nil
		})
		if err != nil {
			return nil, err
		}
		sess, err := a.store.Create(username)
		if err != nil {
			return nil, err
		}
		sess.AccountID, sess.SecurityVersion = account.ID, account.SecurityVersion
		sess.Account = account
		return sess, nil
	}
	return nil, errAccountStoreUnavailable
}

// HandleLogin 处理 POST /admin/api/login
func (a *Authenticator) HandleLogin(ctx context.Context, c *app.RequestContext) {
	c.Response.Header.Set("Cache-Control", "no-store")
	if !validAdminOrigin(c) {
		c.JSON(http.StatusForbidden, utils.H{"error": "请求来源无效"})
		return
	}
	if a == nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "admin disabled"})
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !readAccountBody(c, &body) {
		return
	}
	sess, err := a.Verify(body.Username, body.Password)
	if err != nil {
		status, message := http.StatusUnauthorized, "用户名或密码错误"
		if a.manager != nil {
			status, message = accountErrorStatus(err)
		}
		c.JSON(status, utils.H{"error": message})
		return
	}
	maxAge := int(time.Until(sess.ExpiresAt).Seconds())
	c.SetCookie(sessionCookieName, sess.Token, maxAge, "/", "", protocol.CookieSameSiteLaxMode, false, true)
	c.JSON(http.StatusOK, utils.H{
		"code": 0,
		"data": sessionView(sess),
	})
}

// HandleLogout 处理 POST /admin/api/logout
func (a *Authenticator) HandleLogout(ctx context.Context, c *app.RequestContext) {
	c.Response.Header.Set("Cache-Control", "no-store")
	if !validAdminOrigin(c) {
		c.JSON(http.StatusForbidden, utils.H{"error": "请求来源无效"})
		return
	}
	if a == nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "admin disabled"})
		return
	}
	if token := readSessionToken(c); token != "" {
		a.store.Delete(token)
	}
	c.SetCookie(sessionCookieName, "", -1, "/", "", protocol.CookieSameSiteLaxMode, false, true)
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"status": "ok"}})
}

// HandleMe 处理 GET /admin/api/me
func (a *Authenticator) HandleMe(ctx context.Context, c *app.RequestContext) {
	c.Response.Header.Set("Cache-Control", "no-store")
	sess, err := a.session(ctx, c)
	if err != nil {
		status, message := accountErrorStatus(err)
		c.JSON(status, utils.H{"error": message})
		return
	}
	c.JSON(http.StatusOK, utils.H{
		"code": 0,
		"data": sessionView(sess),
	})
}

// SessionFromRequest 解析 cookie 并返回有效会话，否则返回 nil。
func (a *Authenticator) SessionFromRequest(c *app.RequestContext) *Session {
	sess, _ := a.session(context.Background(), c)
	return sess
}

func (a *Authenticator) session(ctx context.Context, c *app.RequestContext) (*Session, error) {
	if cached, ok := c.Get("admin.current_session"); ok {
		return cached.(*Session), nil
	}
	if a == nil {
		return nil, accountFailure(401, "请先登录")
	}
	if a.manager == nil || a.manager.storage == nil {
		return nil, errAccountStoreUnavailable
	}
	sess := a.store.Get(readSessionToken(c))
	if sess == nil {
		return nil, accountFailure(401, "登录已失效，请重新登录")
	}
	copy := *sess
	if a.manager != nil {
		readCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		account, err := a.manager.get(readCtx, sess.Username)
		if err != nil {
			return nil, err
		}
		if account == nil || !account.Enabled || account.ID != sess.AccountID || account.SecurityVersion != sess.SecurityVersion {
			a.store.Delete(sess.Token)
			return nil, accountFailure(401, "登录已失效，请重新登录")
		}
		copy.Account = account
	}
	c.Set("admin.current_session", &copy)
	return &copy, nil
}

func sessionView(sess *Session) utils.H {
	view := utils.H{"username": sess.Username, "expires_at": sess.ExpiresAt.Format(time.RFC3339), "home": "/admin/dashboard.html"}
	if sess.Account != nil {
		view["account"] = sess.Account.view()
		view["role"], view["display_name"] = sess.Account.Role, sess.Account.DisplayName
		view["permissions"] = sess.Account.Permissions
		view["must_change_password"] = sess.Account.MustChangePassword
		view["home"] = sess.Account.home()
	}
	return view
}

func validAdminOrigin(c *app.RequestContext) bool {
	if strings.EqualFold(string(c.Request.Header.Peek("Sec-Fetch-Site")), "cross-site") {
		return false
	}
	origin := string(c.Request.Header.Peek("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && strings.EqualFold(parsed.Host, string(c.Request.Host()))
}

// AuthMiddleware 校验请求是否带有有效 admin 会话；未通过返回 401。
func (a *Authenticator) AuthMiddleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		c.Response.Header.Set("Cache-Control", "no-store")
		if a == nil {
			c.JSON(http.StatusServiceUnavailable, utils.H{"error": "admin disabled"})
			c.Abort()
			return
		}
		sess, err := a.session(ctx, c)
		if err != nil {
			status, message := accountErrorStatus(err)
			c.JSON(status, utils.H{"error": message})
			c.Abort()
			return
		}
		if a.manager != nil {
			if !a.authorize(ctx, c, sess) {
				c.Abort()
				return
			}
		}
		c.Next(ctx)
		c.Response.Header.Set("Cache-Control", "no-store")
		if a.manager != nil {
			a.finishAudit(c)
		}
	}
}

func readSessionToken(c *app.RequestContext) string {
	v := c.Cookie(sessionCookieName)
	if len(v) == 0 {
		return ""
	}
	return string(v)
}
