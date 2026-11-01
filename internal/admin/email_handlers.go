package admin

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
)

const recoveryRequestMessage = "如果账号与已验证邮箱匹配，我们会发送验证码；请查看收件箱或垃圾邮件。"

func (a *Authenticator) emailAvailable(c *app.RequestContext) bool {
	c.Response.Header.Set("Cache-Control", "no-store")
	if a == nil || a.recovery == nil {
		c.JSON(503, utils.H{"error": "邮件找回暂不可用，请联系后台负责人配置发信服务"})
		return false
	}
	return true
}

func recoveryClientKey(c *app.RequestContext, operation string) string {
	// Use the socket peer, not client-controlled forwarded headers.
	remote := c.RemoteAddr().String()
	if host, _, err := net.SplitHostPort(remote); err == nil {
		remote = host
	}
	return operation + ":" + remote
}

func recoveryCodeValid(code string) bool {
	if len(code) != 8 {
		return false
	}
	for _, ch := range code {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func (a *Authenticator) HandleRecoveryStatus(_ context.Context, c *app.RequestContext) {
	c.Response.Header.Set("Cache-Control", "no-store")
	c.JSON(200, utils.H{"code": 0, "data": utils.H{"enabled": a != nil && a.recovery != nil}})
}

func (a *Authenticator) HandleSecurity(ctx context.Context, c *app.RequestContext) {
	sess := a.SessionFromRequest(c)
	account := sess.Account
	data := utils.H{"email": account.Email, "email_verified": account.recoveryReady(), "email_recovery_enabled": a.recovery != nil}
	if challenge := account.EmailBinding; challenge != nil && challenge.SecurityVersion == account.SecurityVersion && time.Now().Before(challenge.ExpiresAt) {
		data["pending_email"], data["verification_expires_at"] = challenge.Email, challenge.ExpiresAt
	}
	c.JSON(200, utils.H{"code": 0, "data": data})
}

func (a *Authenticator) HandleSendEmailCode(ctx context.Context, c *app.RequestContext) {
	if !a.emailAvailable(c) {
		return
	}
	if !a.recovery.allow(recoveryClientKey(c, "email-self"), 30, 10*time.Minute) {
		c.JSON(429, utils.H{"error": "操作过于频繁，请稍后重试"})
		return
	}
	var body struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		CurrentCode string `json:"current_code"`
	}
	if !readAccountBody(c, &body) {
		return
	}
	proveCurrent := strings.HasSuffix(c.FullPath(), "/current-code")
	var email string
	if !proveCurrent {
		var err error
		email, err = normalizeRecoveryEmail(body.Email)
		if err != nil {
			writeAccountError(c, err)
			return
		}
	}
	if len(body.Password) > 72 {
		c.JSON(400, utils.H{"error": "当前密码不正确"})
		return
	}
	sess := a.SessionFromRequest(c)
	ctx, cancel := context.WithTimeout(ctx, 22*time.Second)
	defer cancel()
	challenge, code, err := a.manager.startEmailBinding(ctx, sess, email, body.Password, strings.TrimSpace(body.CurrentCode), proveCurrent)
	if err == nil {
		purpose := emailBindPurpose
		if proveCurrent {
			purpose = emailChangePurpose
		}
		err = a.recovery.deliverCode(ctx, sess.Username, purpose, challenge, code)
	}
	if err != nil {
		writeAccountError(c, err)
		return
	}
	c.JSON(200, utils.H{"code": 0, "data": utils.H{"message": "验证码已提交发送，请查看邮箱", "expires_in": 600, "resend_after": 60}})
}

func (a *Authenticator) HandleVerifyEmail(ctx context.Context, c *app.RequestContext) {
	if !a.emailAvailable(c) {
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if !readAccountBody(c, &body) {
		return
	}
	body.Code = strings.TrimSpace(body.Code)
	if !recoveryCodeValid(body.Code) {
		c.JSON(400, utils.H{"error": "请输入邮件中的 8 位数字验证码"})
		return
	}
	sess := a.SessionFromRequest(c)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	previous, email, err := a.manager.confirmEmailBinding(ctx, sess, body.Code)
	if err != nil {
		writeAccountError(c, err)
		return
	}
	a.store.DeleteUsername(sess.Username)
	a.recovery.notify(sess.Username, email, "找回邮箱已绑定并验证。为保护账号，全部旧登录已退出，请重新登录。")
	if previous != "" && previous != email {
		a.recovery.notify(sess.Username, previous, "该账号的找回邮箱已更换，原邮箱不再用于密码找回。")
	}
	c.JSON(200, utils.H{"code": 0, "data": utils.H{"status": "ok", "login_required": true}})
}

func (a *Authenticator) HandleForgotPassword(ctx context.Context, c *app.RequestContext) {
	if !a.emailAvailable(c) {
		return
	}
	if !validAdminOrigin(c) {
		c.JSON(403, utils.H{"error": "请求来源无效"})
		return
	}
	if !a.recovery.allow(recoveryClientKey(c, "forgot"), 10, time.Hour) {
		c.JSON(429, utils.H{"error": "申请过于频繁，请稍后重试"})
		return
	}
	var body struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	}
	if !readAccountBody(c, &body) {
		return
	}
	email, err := normalizeRecoveryEmail(body.Email)
	if err != nil || !accountUsernamePattern.MatchString(strings.TrimSpace(body.Username)) {
		c.JSON(400, utils.H{"error": "请输入有效的登录名和邮箱"})
		return
	}
	// Queue before account lookup so matching and nonmatching accounts have the
	// same response, including SMTP failures. The queue and worker count are bounded.
	if !a.recovery.queue(recoveryMailJob{username: strings.TrimSpace(body.Username), email: email}) {
		c.JSON(429, utils.H{"error": "申请较多，请稍后重试"})
		return
	}
	c.JSON(202, utils.H{"code": 0, "data": utils.H{"message": recoveryRequestMessage, "resend_after": 60}})
}

func (a *Authenticator) HandleCompleteRecovery(ctx context.Context, c *app.RequestContext) {
	if !a.emailAvailable(c) {
		return
	}
	if !validAdminOrigin(c) {
		c.JSON(403, utils.H{"error": "请求来源无效"})
		return
	}
	if !a.recovery.allow(recoveryClientKey(c, "recover"), 30, time.Minute) {
		c.JSON(429, utils.H{"error": "尝试过于频繁，请稍后重试"})
		return
	}
	var body struct {
		Username     string `json:"username"`
		Email        string `json:"email"`
		Code         string `json:"code"`
		Password     string `json:"password"`
		Confirmation string `json:"confirmation"`
	}
	if !readAccountBody(c, &body) {
		return
	}
	body.Username, body.Code = strings.TrimSpace(body.Username), strings.TrimSpace(body.Code)
	email, err := normalizeRecoveryEmail(body.Email)
	if err != nil || !accountUsernamePattern.MatchString(body.Username) || !recoveryCodeValid(body.Code) {
		c.JSON(400, utils.H{"error": "账号、邮箱或验证码无效"})
		return
	}
	if body.Password != body.Confirmation {
		c.JSON(400, utils.H{"error": "两次输入的新密码不一致"})
		return
	}
	if err := validatePassword(body.Password); err != nil {
		writeAccountError(c, err)
		return
	}
	select {
	case a.recovery.completions <- struct{}{}:
		defer func() { <-a.recovery.completions }()
	default:
		c.JSON(429, utils.H{"error": "找回请求正在处理中，请稍后重试"})
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	id, err := randomToken(16)
	if err != nil {
		writeAccountError(c, errAccountStoreUnavailable)
		return
	}
	entry := AuditEntry{ID: id + ":attempt", ActorID: "email-recovery", Username: body.Username, Method: "POST", Path: string(c.Path()), CreatedAt: time.Now().UTC()}
	if err := a.manager.storage.AppendAudit(ctx, entry); err != nil {
		writeAccountError(c, errAccountStoreUnavailable)
		return
	}
	entry.ID = id + ":result"
	c.Set("admin.audit", entry)
	defer a.finishAudit(c)
	if err := a.manager.completePasswordRecovery(ctx, body.Username, email, body.Code, body.Password); err != nil {
		writeAccountError(c, err)
		return
	}
	a.store.DeleteUsername(body.Username)
	a.recovery.notify(body.Username, email, "后台登录密码已通过邮箱验证重新设置。全部旧登录及旧验证码已失效，请使用新密码正常登录。")
	c.JSON(200, utils.H{"code": 0, "data": utils.H{"status": "ok", "login_required": true}})
}
