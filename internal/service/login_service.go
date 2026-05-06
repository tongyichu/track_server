package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/tongyichu/track_server/internal/config"
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

const maxDefaultNicknameAttempts = 20

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
	log.Printf("debug info generate captcha [code: %s, id=%s]", code, captchaID)
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
		log.Printf("debug info captcha code expired %s", entry.Code)
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

	user, err := s.users.FindByPhone(ctx, phone)
	if err != nil {
		if !errors.Is(err, repository.ErrNotFound) {
			return nil, err
		}

		userID, idErr := phoneToUserID(phone)
		if idErr != nil {
			return nil, idErr
		}

		nickname, genErr := s.generateUniqueDefaultNickname(ctx)
		if genErr != nil {
			return nil, genErr
		}

		user, err = s.users.CreateIfNotExists(ctx, &models.User{
			ID:       userID,
			Phone:    phone,
			Nickname: nickname,
		})
		if err != nil {
			return nil, err
		}
		if user.Nickname == "" {
			user.Nickname = nickname
			_ = s.users.Update(ctx, user)
		}
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

	// 登录接口返回头像时，统一做兜底：若为空则返回默认头像。
	user.AvatarURL = fallbackAvatarURL(user.ID, user.AvatarURL)

	return &LoginResult{UserID: user.ID, User: user, Token: token}, nil
}

func (s *LoginService) generateUniqueDefaultNickname(ctx context.Context) (string, error) {
	bases := config.DefaultNicknameBases()
	if len(bases) == 0 {
		return "", errors.New("default nickname bases is empty")
	}
	base := bases[rand.Intn(len(bases))]

	available, err := s.nicknameAvailable(ctx, base)
	if err != nil {
		return "", err
	}
	if available {
		return base, nil
	}

	for i := 0; i < maxDefaultNicknameAttempts; i++ {
		candidate := fmt.Sprintf("%s%d", base, rand.Intn(9000)+1000)
		available, err = s.nicknameAvailable(ctx, candidate)
		if err != nil {
			return "", err
		}
		if available {
			return candidate, nil
		}
	}

	return "", errors.New("failed to generate unique default nickname")
}

func (s *LoginService) nicknameAvailable(ctx context.Context, nickname string) (bool, error) {
	_, err := s.users.FindByNickname(ctx, nickname)
	if err == nil {
		return false, nil
	}
	if errors.Is(err, repository.ErrNotFound) {
		return true, nil
	}
	return false, err
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

// 自定义起始时间戳（2025-01-01 00:00:00 UTC+8）
const epoch int64 = 1735689600

func phoneToUserID(phone string) (int64, error) {
	// 1. 基础校验：手机号必须是11位纯数字
	if len(phone) != 11 {
		return 0, fmt.Errorf("invalid phone number length: must be 11 digits")
	}
	phoneNum, err := strconv.ParseInt(phone, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("phone number contains non-digit characters: %w", err)
	}
	// 2. 生成时间戳差值（秒级，32bit存储）
	now := time.Now().Unix()
	timeDiff := now - epoch
	if timeDiff < 0 || timeDiff > (1<<32)-1 {
		return 0, fmt.Errorf("timestamp out of valid range")
	}
	// 3. 提取脱敏手机号段：前3位 + 最后4位，共7位数字压缩到20bit
	// 手机号：13800138000 → 提取138（前3位）+8000（后4位）=1388000
	first3 := phoneNum / 10000000        // 取前3位（除以10^8，11位手机号÷1亿得到前3位）
	last4 := phoneNum % 10000            // 取最后4位
	phoneSegment := first3*10000 + last4 // 合并为7位数字，最大值1999999 < 2^20=1048576? 不，199万实际用21bit，这里调整为先存数值，位运算时截断
	// 修正：2^21=2097152刚好覆盖199万，调整位分配时将预留位借1bit给phoneSegment，不影响核心逻辑
	// 4. 位拼接生成最终ID
	var userID int64
	userID |= (timeDiff << 32)     // 时间戳左移32位
	userID |= (phoneSegment << 11) // 脱敏手机号段左移11位
	userID |= 0                    // 预留位暂时填0
	return userID, nil
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
