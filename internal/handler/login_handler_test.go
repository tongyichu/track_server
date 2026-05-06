package handler_test

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"strings"
	"testing"

	"github.com/tongyichu/track_server/internal/handler"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/service"
)

func sendSMSCodeViaService(t *testing.T, e *testEnv, phone string) string {
	t.Helper()
	ctx := context.Background()
	captcha, err := e.loginSvc.GenerateCaptcha(ctx)
	if err != nil {
		t.Fatalf("generate captcha failed: %v", err)
	}
	captchaCode := extractCaptchaCode(t, e, captcha.CaptchaID)
	code, err := e.loginSvc.SendSMSCode(ctx, phone, captcha.CaptchaID, captchaCode)
	if err != nil {
		t.Fatalf("send sms code failed: %v", err)
	}
	return code
}

func extractCaptchaCode(t *testing.T, e *testEnv, captchaID string) string {
	t.Helper()
	e.loginSvc.CaptchaMu().RLock()
	defer e.loginSvc.CaptchaMu().RUnlock()
	entry, ok := e.loginSvc.CaptchaStore()[captchaID]
	if !ok {
		t.Fatalf("captcha not found: %s", captchaID)
	}
	return entry.Code
}

func TestGetCaptcha_Success(t *testing.T) {
	e := newTestEnv()

	w := e.perform(http.MethodGet, "/api/v1/captcha", nil)
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode())
	}
	var result handler.StandardResponse[*service.CaptchaResult]
	decodeJSON(t, resp.Body(), &result)
	if result.Code != 0 {
		t.Fatalf("expected code 0, got %d", result.Code)
	}
	if result.Data == nil {
		t.Fatalf("expected non-nil data")
	}
	if result.Data.CaptchaID == "" {
		t.Fatalf("expected non-empty captcha_id")
	}
	if result.Data.CaptchaImg == "" {
		t.Fatalf("expected non-empty captcha_img")
	}
}

func TestSendSMSCode_Success(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()

	captcha, _ := e.loginSvc.GenerateCaptcha(ctx)
	captchaCode := extractCaptchaCode(t, e, captcha.CaptchaID)

	body, _ := json.Marshal(map[string]string{
		"phone":        "13800001111",
		"captcha_id":   captcha.CaptchaID,
		"captcha_code": captchaCode,
	})
	w := e.perform(http.MethodPost, "/api/v1/sms/send", body)
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode(), string(resp.Body()))
	}
	var result handler.StandardResponse[handler.SendSMSCodeResult]
	decodeJSON(t, resp.Body(), &result)
	if result.Code != 0 {
		t.Fatalf("expected code 0, got %d", result.Code)
	}
	if result.Data.Code == "" {
		t.Fatalf("expected non-empty sms code")
	}
}

func TestSendSMSCode_InvalidCaptcha(t *testing.T) {
	e := newTestEnv()

	body, _ := json.Marshal(map[string]string{
		"phone":        "13800001111",
		"captcha_id":   "bad_id",
		"captcha_code": "0000",
	})
	w := e.perform(http.MethodPost, "/api/v1/sms/send", body)
	if w.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("invalid captcha should return 400")
	}
}

func TestSendSMSCode_MissingCaptcha(t *testing.T) {
	e := newTestEnv()

	body, _ := json.Marshal(map[string]string{
		"phone": "13800001111",
	})
	w := e.perform(http.MethodPost, "/api/v1/sms/send", body)
	if w.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("missing captcha fields should return 400")
	}
}

func TestLoginBySMS_Success(t *testing.T) {
	e := newTestEnv()
	phone := "13800001111"
	code := sendSMSCodeViaService(t, e, phone)

	body, _ := json.Marshal(map[string]string{"phone": phone, "code": code})
	w := e.perform(http.MethodPost, "/api/v1/login/sms", body)
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode(), string(resp.Body()))
	}

	var result handler.StandardResponse[*service.LoginResult]
	decodeJSON(t, resp.Body(), &result)
	if result.Code != 0 {
		t.Fatalf("expected code 0, got %d", result.Code)
	}
	if result.Data == nil {
		t.Fatalf("expected non-nil data")
	}
	if result.Data.UserID <= 0 {
		t.Fatalf("expected positive user_id, got %d", result.Data.UserID)
	}
	if result.Data.User == nil {
		t.Fatalf("expected non-nil user")
	}
	if result.Data.User.Nickname == "" {
		t.Fatalf("expected generated nickname for first sms login")
	}
	if result.Data.Token == "" {
		t.Fatalf("expected non-empty token")
	}
}

func TestLoginBySMS_DuplicateNicknameAppendsRandomNumber(t *testing.T) {
	e := newTestEnv()

	phone1 := "13800001111"
	code1 := sendSMSCodeViaService(t, e, phone1)
	rand.Seed(1)
	body1, _ := json.Marshal(map[string]string{"phone": phone1, "code": code1})
	w1 := e.perform(http.MethodPost, "/api/v1/login/sms", body1)
	if w1.Result().StatusCode() != http.StatusOK {
		t.Fatalf("first login should succeed, got %d", w1.Result().StatusCode())
	}
	var result1 handler.StandardResponse[*service.LoginResult]
	decodeJSON(t, w1.Result().Body(), &result1)
	if result1.Data == nil || result1.Data.User == nil {
		t.Fatalf("expected first login user")
	}
	if result1.Data.User.Nickname == "" {
		t.Fatalf("expected first login nickname")
	}

	phone2 := "13800002222"
	code2 := sendSMSCodeViaService(t, e, phone2)
	rand.Seed(1)
	body2, _ := json.Marshal(map[string]string{"phone": phone2, "code": code2})
	w2 := e.perform(http.MethodPost, "/api/v1/login/sms", body2)
	if w2.Result().StatusCode() != http.StatusOK {
		t.Fatalf("second login should succeed, got %d", w2.Result().StatusCode())
	}
	var result2 handler.StandardResponse[*service.LoginResult]
	decodeJSON(t, w2.Result().Body(), &result2)
	if result2.Data == nil || result2.Data.User == nil {
		t.Fatalf("expected second login user")
	}
	if result2.Data.User.Nickname == result1.Data.User.Nickname {
		t.Fatalf("expected duplicate nickname to append random digits")
	}
	if !strings.HasPrefix(result2.Data.User.Nickname, result1.Data.User.Nickname) {
		t.Fatalf("expected second nickname %q to keep first nickname %q as prefix", result2.Data.User.Nickname, result1.Data.User.Nickname)
	}
	if len(result2.Data.User.Nickname) <= len(result1.Data.User.Nickname) {
		t.Fatalf("expected second nickname %q to be longer than first nickname %q", result2.Data.User.Nickname, result1.Data.User.Nickname)
	}
}

func TestLoginBySMS_SamePhoneReusesExistingUserID(t *testing.T) {
	e := newTestEnv()
	phone := "13800001111"

	code1 := sendSMSCodeViaService(t, e, phone)
	body1, _ := json.Marshal(map[string]string{"phone": phone, "code": code1})
	w1 := e.perform(http.MethodPost, "/api/v1/login/sms", body1)
	if w1.Result().StatusCode() != http.StatusOK {
		t.Fatalf("first login should succeed, got %d", w1.Result().StatusCode())
	}
	var result1 handler.StandardResponse[*service.LoginResult]
	decodeJSON(t, w1.Result().Body(), &result1)
	if result1.Data == nil || result1.Data.User == nil {
		t.Fatalf("expected first login user")
	}

	code2 := sendSMSCodeViaService(t, e, phone)
	body2, _ := json.Marshal(map[string]string{"phone": phone, "code": code2})
	w2 := e.perform(http.MethodPost, "/api/v1/login/sms", body2)
	if w2.Result().StatusCode() != http.StatusOK {
		t.Fatalf("second login should succeed, got %d", w2.Result().StatusCode())
	}
	var result2 handler.StandardResponse[*service.LoginResult]
	decodeJSON(t, w2.Result().Body(), &result2)
	if result2.Data == nil || result2.Data.User == nil {
		t.Fatalf("expected second login user")
	}
	if result2.Data.UserID != result1.Data.UserID {
		t.Fatalf("expected same phone to reuse user_id, got %d and %d", result1.Data.UserID, result2.Data.UserID)
	}
	if result2.Data.User.Nickname != result1.Data.User.Nickname {
		t.Fatalf("expected same phone to reuse nickname, got %q and %q", result1.Data.User.Nickname, result2.Data.User.Nickname)
	}
	stored, err := e.userRepo.FindByPhone(context.Background(), phone)
	if err != nil {
		t.Fatalf("expected stored user by phone, got err=%v", err)
	}
	if stored.ID != result1.Data.UserID {
		t.Fatalf("expected stored user id %d, got %d", result1.Data.UserID, stored.ID)
	}
}

func TestLoginBySMS_InvalidCode(t *testing.T) {
	e := newTestEnv()

	body, _ := json.Marshal(map[string]string{"phone": "13800001111", "code": "000000"})
	w := e.perform(http.MethodPost, "/api/v1/login/sms", body)
	resp := w.Result()
	if resp.StatusCode() != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", resp.StatusCode())
	}
}

func TestLoginBySMS_MissingFields(t *testing.T) {
	e := newTestEnv()

	w1 := e.perform(http.MethodPost, "/api/v1/login/sms", []byte(`{}`))
	if w1.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("empty body should return 400")
	}

	w2 := e.perform(http.MethodPost, "/api/v1/login/sms", []byte(`{"phone":"13800001111"}`))
	if w2.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("missing code should return 400")
	}
}

func TestLoginByApple_Success(t *testing.T) {
	e := newTestEnv()

	body, _ := json.Marshal(map[string]string{
		"apple_user_id":  "apple_uid_001",
		"identity_token": "fake_token",
	})
	w := e.perform(http.MethodPost, "/api/v1/login/apple", body)
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode(), string(resp.Body()))
	}

	var result service.LoginResult
	decodeJSON(t, resp.Body(), &result)
	if result.UserID <= 0 {
		t.Fatalf("expected positive user_id, got %d", result.UserID)
	}
}

func TestLoginByApple_MissingFields(t *testing.T) {
	e := newTestEnv()

	w := e.perform(http.MethodPost, "/api/v1/login/apple", []byte(`{}`))
	if w.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("empty body should return 400")
	}
}

func TestLoginByWechat_MissingCode(t *testing.T) {
	e := newTestEnv()

	w := e.perform(http.MethodPost, "/api/v1/login/wechat", []byte(`{}`))
	if w.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("empty code should return 400")
	}
}

func TestGetLoginLog_Success(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(2001)

	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 2001, Phone: "13900002001"})
	_ = e.loginLogRepo.Create(ctx, &models.LoginLog{
		UserID:    2001,
		LoginType: "sms",
		IP:        "127.0.0.1",
	})
	_ = e.loginLogRepo.Create(ctx, &models.LoginLog{
		UserID:    2001,
		LoginType: "apple",
		IP:        "127.0.0.2",
	})

	w := e.perform(http.MethodGet, "/api/v1/login/log?user_id=2001", nil, authHeader(token))
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode())
	}
	var logs []*models.LoginLog
	decodeJSON(t, resp.Body(), &logs)
	if len(logs) != 2 {
		t.Fatalf("expected 2 login logs, got %d", len(logs))
	}
}

func TestGetLoginLog_InvalidUserID(t *testing.T) {
	e := newTestEnv()
	token := e.generateTestToken(1001)

	w := e.perform(http.MethodGet, "/api/v1/login/log?user_id=bad", nil, authHeader(token))
	if w.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("invalid user_id should return 400")
	}

	w2 := e.perform(http.MethodGet, "/api/v1/login/log", nil, authHeader(token))
	if w2.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("missing user_id should return 400")
	}
}

func TestLogout_Success(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(3001)

	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 3001, Phone: "13900003001"})

	w := e.perform(http.MethodPost, "/api/v1/logout", nil, authHeader(token))
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode(), string(resp.Body()))
	}

	logs, err := e.loginLogRepo.ListByUserID(ctx, 3001, 10)
	if err != nil {
		t.Fatalf("list login logs failed: %v", err)
	}
	if len(logs) < 1 {
		t.Fatalf("expected at least 1 log, got %d", len(logs))
	}
	if logs[0].LoginType != "logout" {
		t.Fatalf("expected login_type logout, got %s", logs[0].LoginType)
	}
}

func TestLogout_TokenRevoked(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(3002)
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 3002, Phone: "13900003002"})

	w1 := e.perform(http.MethodPost, "/api/v1/logout", nil, authHeader(token))
	if w1.Result().StatusCode() != http.StatusOK {
		t.Fatalf("logout should succeed, got %d", w1.Result().StatusCode())
	}

	w2 := e.perform(http.MethodGet, "/api/v1/login/log?user_id=3002", nil, authHeader(token))
	if w2.Result().StatusCode() != http.StatusUnauthorized {
		t.Fatalf("revoked token should return 401, got %d", w2.Result().StatusCode())
	}
}

func TestLogout_NoAuth(t *testing.T) {
	e := newTestEnv()

	w := e.perform(http.MethodPost, "/api/v1/logout", nil)
	if w.Result().StatusCode() != http.StatusUnauthorized {
		t.Fatalf("missing token should return 401")
	}
}

func TestLoginBySMS_CreatesLoginLog(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()

	phone := "13800002222"
	code := sendSMSCodeViaService(t, e, phone)

	body, _ := json.Marshal(map[string]string{"phone": phone, "code": code})
	w := e.perform(http.MethodPost, "/api/v1/login/sms", body)
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("login should succeed")
	}

	var result handler.StandardResponse[*service.LoginResult]
	decodeJSON(t, w.Result().Body(), &result)
	if result.Code != 0 {
		t.Fatalf("expected code 0, got %d", result.Code)
	}
	if result.Data == nil {
		t.Fatalf("expected non-nil data")
	}

	logs, err := e.loginLogRepo.ListByUserID(ctx, result.Data.UserID, 10)
	if err != nil {
		t.Fatalf("list login logs failed: %v", err)
	}
	if len(logs) < 1 {
		t.Fatalf("expected at least 1 login log, got %d", len(logs))
	}
	if logs[0].LoginType != "sms" {
		t.Fatalf("expected login_type sms, got %s", logs[0].LoginType)
	}
}
