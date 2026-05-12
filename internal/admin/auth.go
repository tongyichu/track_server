package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
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

// Authenticator 持有管理员账号配置与会话存储。
//
// 支持多账号：accounts 为 username -> bcrypt(passwordHash) 的映射。
// 当 accounts 为空时，NewAuthenticator 返回 nil，表示后台未启用。
type Authenticator struct {
	accounts map[string]string
	store    *SessionStore
}

// NewAuthenticator 根据 accounts 创建 Authenticator。
// accounts 为空（nil 或 0 条有效记录）时返回 nil，表示后台未启用。
//
// 约束：
// - username 两端空白会被裁剪；裁剪后为空的条目将被忽略；
// - passwordHash 必须是 bcrypt 哈希（调用方保证，本处不做格式校验）。
func NewAuthenticator(accounts map[string]string, store *SessionStore) *Authenticator {
	if len(accounts) == 0 {
		return nil
	}
	cleaned := make(map[string]string, len(accounts))
	for u, h := range accounts {
		u = strings.TrimSpace(u)
		h = strings.TrimSpace(h)
		if u == "" || h == "" {
			continue
		}
		cleaned[u] = h
	}
	if len(cleaned) == 0 {
		return nil
	}
	return &Authenticator{
		accounts: cleaned,
		store:    store,
	}
}

// AccountCount 返回已加载的有效管理员账号数量。
func (a *Authenticator) AccountCount() int {
	if a == nil {
		return 0
	}
	return len(a.accounts)
}

// Verify 校验用户名+密码，正确则创建会话并返回。
//
// 侧信道加固：未命中用户名时也执行一次 bcrypt 占位比较，
// 让失败分支的总耗时接近命中分支，降低通过时间差枚举用户名的可能性。
func (a *Authenticator) Verify(username, password string) (*Session, error) {
	if a == nil {
		return nil, errors.New("admin disabled")
	}
	hash, ok := a.accounts[username]
	if !ok {
		// 未命中时仍执行一次 bcrypt，抹平时间差；结果必然为错误。
		_ = bcrypt.CompareHashAndPassword(placeholderPasswordHash, []byte(password))
		return nil, errors.New("invalid credentials")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, errors.New("invalid credentials")
	}
	return a.store.Create(username)
}

// HandleLogin 处理 POST /admin/api/login
func (a *Authenticator) HandleLogin(ctx context.Context, c *app.RequestContext) {
	if a == nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "admin disabled"})
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if data, err := c.Body(); err != nil || json.Unmarshal(data, &body) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	sess, err := a.Verify(body.Username, body.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "invalid credentials"})
		return
	}
	maxAge := int(time.Until(sess.ExpiresAt).Seconds())
	c.SetCookie(sessionCookieName, sess.Token, maxAge, "/", "", protocol.CookieSameSiteLaxMode, false, true)
	c.JSON(http.StatusOK, utils.H{
		"code": 0,
		"data": utils.H{
			"username":   sess.Username,
			"expires_at": sess.ExpiresAt.Format(time.RFC3339),
		},
	})
}

// HandleLogout 处理 POST /admin/api/logout
func (a *Authenticator) HandleLogout(ctx context.Context, c *app.RequestContext) {
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
	sess := a.SessionFromRequest(c)
	if sess == nil {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	c.JSON(http.StatusOK, utils.H{
		"code": 0,
		"data": utils.H{
			"username":   sess.Username,
			"expires_at": sess.ExpiresAt.Format(time.RFC3339),
		},
	})
}

// SessionFromRequest 解析 cookie 并返回有效会话，否则返回 nil。
func (a *Authenticator) SessionFromRequest(c *app.RequestContext) *Session {
	if a == nil {
		return nil
	}
	token := readSessionToken(c)
	if token == "" {
		return nil
	}
	return a.store.Get(token)
}

// AuthMiddleware 校验请求是否带有有效 admin 会话；未通过返回 401。
func (a *Authenticator) AuthMiddleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if a == nil {
			c.JSON(http.StatusServiceUnavailable, utils.H{"error": "admin disabled"})
			c.Abort()
			return
		}
		if a.SessionFromRequest(c) == nil {
			c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		c.Next(ctx)
	}
}

func readSessionToken(c *app.RequestContext) string {
	v := c.Cookie(sessionCookieName)
	if len(v) == 0 {
		return ""
	}
	return string(v)
}
