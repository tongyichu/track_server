package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type LoginService struct {
	users           repository.UserRepository
	loginLogs       repository.LoginLogRepository
	wechatAppID     string
	wechatAppSecret string
	jwtSecret       string
	mu              sync.RWMutex
	smsCodes        map[string]smsCodeEntry
	captchas        map[string]CaptchaEntry
}

type smsCodeEntry struct {
	code      string
	expiresAt time.Time
}

type CaptchaEntry struct {
	Code      string
	ExpiresAt time.Time
}

type CaptchaResult struct {
	CaptchaID  string `json:"captcha_id"`
	CaptchaImg string `json:"captcha_img"`
}

type WechatSessionResponse struct {
	OpenID     string `json:"openid"`
	SessionKey string `json:"session_key"`
	UnionID    string `json:"unionid"`
	ErrCode    int    `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
}

type LoginResult struct {
	UserID int64        `json:"user_id"`
	User   *models.User `json:"user"`
	Token  string       `json:"token"`
}

func NewLoginService(users repository.UserRepository, loginLogs repository.LoginLogRepository, wechatAppID, wechatAppSecret, jwtSecret string) *LoginService {
	return &LoginService{
		users:           users,
		loginLogs:       loginLogs,
		wechatAppID:     wechatAppID,
		wechatAppSecret: wechatAppSecret,
		jwtSecret:       jwtSecret,
		smsCodes:        make(map[string]smsCodeEntry),
		captchas:        make(map[string]CaptchaEntry),
	}
}

const jwtTokenExpiry = 7 * 24 * time.Hour

func (s *LoginService) generateToken(userID int64) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(jwtTokenExpiry).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwtSecret))
}

func (s *LoginService) JWTSecret() string {
	return s.jwtSecret
}

func (s *LoginService) ParseTokenExpiry(tokenStr string) time.Time {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return []byte(s.jwtSecret), nil
	})
	if err != nil || !token.Valid {
		return time.Now().Add(jwtTokenExpiry)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return time.Now().Add(jwtTokenExpiry)
	}
	if exp, ok := claims["exp"].(float64); ok {
		return time.Unix(int64(exp), 0)
	}
	return time.Now().Add(jwtTokenExpiry)
}

func (s *LoginService) CaptchaMu() *sync.RWMutex {
	return &s.mu
}

func (s *LoginService) CaptchaStore() map[string]CaptchaEntry {
	return s.captchas
}

func (s *LoginService) GenerateCaptcha(ctx context.Context) (*CaptchaResult, error) {
	captchaID := fmt.Sprintf("cap_%d_%d", time.Now().UnixNano(), rand.Intn(100000))
	code := fmt.Sprintf("%04d", rand.Intn(10000))

	s.mu.Lock()
	s.captchas[captchaID] = CaptchaEntry{
		Code:      code,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	s.mu.Unlock()

	captchaImg := generateCaptchaBase64(code)
	return &CaptchaResult{
		CaptchaID:  captchaID,
		CaptchaImg: captchaImg,
	}, nil
}

func (s *LoginService) ValidateCaptcha(captchaID, captchaCode string) bool {
	s.mu.RLock()
	entry, ok := s.captchas[captchaID]
	s.mu.RUnlock()
	if !ok || time.Now().After(entry.ExpiresAt) {
		return false
	}
	if entry.Code != captchaCode {
		return false
	}
	s.mu.Lock()
	delete(s.captchas, captchaID)
	s.mu.Unlock()
	return true
}

func (s *LoginService) SendSMSCode(ctx context.Context, phone, captchaID, captchaCode string) (string, error) {
	if phone == "" {
		return "", errors.New("phone is required")
	}
	if captchaID == "" || captchaCode == "" {
		return "", errors.New("captcha_id and captcha_code are required")
	}
	if !s.ValidateCaptcha(captchaID, captchaCode) {
		return "", errors.New("invalid or expired captcha")
	}

	code := fmt.Sprintf("%06d", rand.Intn(1000000))
	s.mu.Lock()
	s.smsCodes[phone] = smsCodeEntry{
		code:      code,
		expiresAt: time.Now().Add(5 * time.Minute),
	}
	s.mu.Unlock()

	// TODO: 接入短信服务商（如阿里云短信、腾讯云短信等）实现真实短信下发。
	// 当前实现仅将验证码存储在内存中并直接返回给调用方，适用于开发和测试环境。
	// 接入时需要：
	//   1. 在 config 中添加短信服务商的 AccessKey、签名、模板 ID 等配置项
	//   2. 在此处调用短信 SDK 将 code 发送到 phone 对应的手机
	//   3. 发送成功后不再将 code 返回给客户端，改为返回发送状态
	//   4. 建议增加发送频率限制（如同一手机号 60 秒内仅允许发送一次）

	return code, nil
}

func (s *LoginService) LoginBySMS(ctx context.Context, phone, code, ip, deviceID, platform string) (*LoginResult, error) {
	if phone == "" || code == "" {
		return nil, errors.New("phone and code are required")
	}

	s.mu.RLock()
	entry, ok := s.smsCodes[phone]
	s.mu.RUnlock()
	if !ok || entry.code != code || time.Now().After(entry.expiresAt) {
		return nil, errors.New("invalid or expired verification code")
	}

	s.mu.Lock()
	delete(s.smsCodes, phone)
	s.mu.Unlock()

	userID := phoneToUserID(phone)
	user, err := s.users.CreateIfNotExists(ctx, &models.User{
		ID:    userID,
		Phone: phone,
	})
	if err != nil {
		return nil, err
	}
	if user.Phone != phone {
		user.Phone = phone
		_ = s.users.Update(ctx, user)
	}

	_ = s.loginLogs.Create(ctx, &models.LoginLog{
		UserID:    user.ID,
		LoginType: "sms",
		IP:        ip,
		DeviceID:  deviceID,
		Platform:  platform,
	})

	token, err := s.generateToken(user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return &LoginResult{UserID: user.ID, User: user, Token: token}, nil
}

func (s *LoginService) LoginByWechat(ctx context.Context, code, ip, deviceID, platform string) (*LoginResult, error) {
	if code == "" {
		return nil, errors.New("code is required")
	}

	session, err := s.fetchWechatSession(code)
	if err != nil {
		return nil, fmt.Errorf("wechat login failed: %w", err)
	}
	if session.OpenID == "" {
		return nil, errors.New("wechat login failed: empty openid")
	}

	userID := wechatOpenIDToUserID(session.OpenID)
	user, err := s.users.CreateIfNotExists(ctx, &models.User{
		ID: userID,
	})
	if err != nil {
		return nil, err
	}

	_ = s.loginLogs.Create(ctx, &models.LoginLog{
		UserID:    user.ID,
		LoginType: "wechat",
		IP:        ip,
		DeviceID:  deviceID,
		Platform:  platform,
	})

	token, err := s.generateToken(user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return &LoginResult{UserID: user.ID, User: user, Token: token}, nil
}

func (s *LoginService) LoginByApple(ctx context.Context, appleUserID, identityToken, ip, deviceID, platform string) (*LoginResult, error) {
	if appleUserID == "" {
		return nil, errors.New("apple_user_id is required")
	}

	userID := appleUserIDToUserID(appleUserID)
	user, err := s.users.CreateIfNotExists(ctx, &models.User{
		ID: userID,
	})
	if err != nil {
		return nil, err
	}

	_ = s.loginLogs.Create(ctx, &models.LoginLog{
		UserID:    user.ID,
		LoginType: "apple",
		IP:        ip,
		DeviceID:  deviceID,
		Platform:  platform,
	})

	token, err := s.generateToken(user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return &LoginResult{UserID: user.ID, User: user, Token: token}, nil
}

func (s *LoginService) Logout(ctx context.Context, userID int64, ip, deviceID, platform string) error {
	if userID <= 0 {
		return errors.New("user_id is required")
	}
	return s.loginLogs.Create(ctx, &models.LoginLog{
		UserID:    userID,
		LoginType: "logout",
		IP:        ip,
		DeviceID:  deviceID,
		Platform:  platform,
	})
}

func (s *LoginService) GetLoginLog(ctx context.Context, userID int64, limit int) ([]*models.LoginLog, error) {
	if userID <= 0 {
		return nil, errors.New("user_id is required")
	}
	if limit <= 0 {
		limit = 20
	}
	return s.loginLogs.ListByUserID(ctx, userID, limit)
}

func (s *LoginService) fetchWechatSession(code string) (*WechatSessionResponse, error) {
	url := fmt.Sprintf(
		"https://api.weixin.qq.com/sns/jscode2session?appid=%s&secret=%s&js_code=%s&grant_type=authorization_code",
		s.wechatAppID, s.wechatAppSecret, code,
	)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var session WechatSessionResponse
	if err := json.Unmarshal(body, &session); err != nil {
		return nil, err
	}
	if session.ErrCode != 0 {
		return nil, fmt.Errorf("wechat error %d: %s", session.ErrCode, session.ErrMsg)
	}
	return &session, nil
}

func phoneToUserID(phone string) int64 {
	var id int64
	for _, c := range phone {
		id = id*31 + int64(c)
	}
	if id < 0 {
		id = -id
	}
	if id == 0 {
		id = 1
	}
	return id
}

func wechatOpenIDToUserID(openID string) int64 {
	var id int64
	for _, c := range openID {
		id = id*31 + int64(c)
	}
	if id < 0 {
		id = -id
	}
	if id == 0 {
		id = 1
	}
	return id
}

func appleUserIDToUserID(appleUID string) int64 {
	var id int64
	for _, c := range appleUID {
		id = id*31 + int64(c)
	}
	if id < 0 {
		id = -id
	}
	if id == 0 {
		id = 1
	}
	return id
}

func generateCaptchaBase64(code string) string {
	width := 120
	height := 40
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d">`, width, height)
	svg += fmt.Sprintf(`<rect width="%d" height="%d" fill="#f0f0f0"/>`, width, height)
	for i := 0; i < 5; i++ {
		x1 := rand.Intn(width)
		y1 := rand.Intn(height)
		x2 := rand.Intn(width)
		y2 := rand.Intn(height)
		svg += fmt.Sprintf(`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#ccc" stroke-width="1"/>`, x1, y1, x2, y2)
	}
	for i, ch := range code {
		x := 15 + i*25
		y := 28 + rand.Intn(5) - 2
		colors := []string{"#333", "#c00", "#00c", "#0a0"}
		color := colors[rand.Intn(len(colors))]
		rotate := rand.Intn(30) - 15
		svg += fmt.Sprintf(`<text x="%d" y="%d" font-size="24" fill="%s" transform="rotate(%d %d %d)">%c</text>`,
			x, y, color, rotate, x, y, ch)
	}
	svg += `</svg>`
	return "data:image/svg+xml;base64," + base64Encode([]byte(svg))
}

func base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}
