package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"

	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/service"
)

type LoginHandler struct {
	loginSvc   *service.LoginService
	blacklist  *middleware.TokenBlacklist
}

type StandardResponse[T any] struct {
	Code int `json:"code"`
	Data T   `json:"data"`
}

type SendSMSCodeResult struct {
	Code string `json:"code"`
}

func successResponse[T any](data T) StandardResponse[T] {
	return StandardResponse[T]{
		Code: 0,
		Data: data,
	}
}

func NewLoginHandler(loginSvc *service.LoginService, blacklist *middleware.TokenBlacklist) *LoginHandler {
	return &LoginHandler{loginSvc: loginSvc, blacklist: blacklist}
}

func (h *LoginHandler) GetCaptcha(ctx context.Context, c *app.RequestContext) {
	result, err := h.loginSvc.GenerateCaptcha(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, successResponse(result))
}

func (h *LoginHandler) SendSMSCode(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Phone       string `json:"phone"`
		CaptchaID   string `json:"captcha_id"`
		CaptchaCode string `json:"captcha_code"`
	}
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &body) != nil || body.Phone == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid phone payload"})
		return
	}

	code, err := h.loginSvc.SendSMSCode(ctx, body.Phone, body.CaptchaID, body.CaptchaCode)
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
		return
	}
	// TODO: 接入真实短信服务后，此处不应将验证码明文返回给客户端。
	// 应改为返回发送成功状态，例如：c.JSON(http.StatusOK, utils.H{"message": "sms sent"})
	c.JSON(http.StatusOK, successResponse(SendSMSCodeResult{Code: code}))
}

func (h *LoginHandler) LoginBySMS(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Phone string `json:"phone"`
		Code  string `json:"code"`
	}
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &body) != nil || body.Phone == "" || body.Code == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "phone and code are required"})
		return
	}
	meta := middleware.GetRequestMeta(c)
	ip := c.ClientIP()

	result, err := h.loginSvc.LoginBySMS(ctx, body.Phone, body.Code, ip, meta.DeviceID, meta.Platform)
	if err != nil {
		c.JSON(http.StatusUnauthorized, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, successResponse(result))
}

func (h *LoginHandler) LoginByWechat(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Code string `json:"code"`
	}
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &body) != nil || body.Code == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "code is required"})
		return
	}

	ip := c.ClientIP()
	deviceID := string(c.Request.Header.Peek("X-Device-ID"))
	platform := string(c.Request.Header.Peek("X-Platform"))

	result, err := h.loginSvc.LoginByWechat(ctx, body.Code, ip, deviceID, platform)
	if err != nil {
		c.JSON(http.StatusUnauthorized, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *LoginHandler) LoginByApple(ctx context.Context, c *app.RequestContext) {
	var body struct {
		AppleUserID   string `json:"apple_user_id"`
		IdentityToken string `json:"identity_token"`
	}
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &body) != nil || body.AppleUserID == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "apple_user_id is required"})
		return
	}

	ip := c.ClientIP()
	deviceID := string(c.Request.Header.Peek("X-Device-ID"))
	platform := string(c.Request.Header.Peek("X-Platform"))

	result, err := h.loginSvc.LoginByApple(ctx, body.AppleUserID, body.IdentityToken, ip, deviceID, platform)
	if err != nil {
		c.JSON(http.StatusUnauthorized, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *LoginHandler) Logout(ctx context.Context, c *app.RequestContext) {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.UserID <= 0 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "user_id is required (via X-User-ID header)"})
		return
	}

	ip := c.ClientIP()
	err := h.loginSvc.Logout(ctx, meta.UserID, ip, meta.DeviceID, meta.Platform)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}

	if h.blacklist != nil {
		tokenStr := strings.TrimPrefix(string(c.Request.Header.Peek("Authorization")), "Bearer ")
		exp := h.loginSvc.ParseTokenExpiry(tokenStr)
		h.blacklist.Add(tokenStr, exp)
	}

	c.JSON(http.StatusOK, successResponse(utils.H{"message": "logged out"}))
}

func (h *LoginHandler) GetLoginLog(ctx context.Context, c *app.RequestContext) {
	userID, err := parseRequiredUserID(string(c.Query("user_id")))
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
		return
	}

	logs, err := h.loginSvc.GetLoginLog(ctx, userID, 20)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, logs)
}
