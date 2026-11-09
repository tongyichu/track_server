package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	roleAdmin    = "admin"
	roleOperator = "operator"
	accessNone   = "none"
	accessRead   = "read"
	accessWrite  = "write"
)

type PermissionModule struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Page string `json:"page"`
}

var permissionModules = []PermissionModule{
	{"dashboard", "工作台", "/admin/dashboard.html"},
	{"track_submissions", "投稿审核", "/admin/track_submissions.html"},
	{"tracks", "轨迹管理", "/admin/tracks.html"},
	{"scenic_spots", "景点管理", "/admin/scenic_spots.html"},
	{"regions", "城市管理", "/admin/regions.html"},
	{"users", "用户管理", "/admin/users.html"},
	{"route_groups", "地图模式", "/admin/route_groups.html"},
	{"companions", "与友同行", "/admin/companions.html"},
	{"feedbacks", "反馈与纠错", "/admin/feedbacks.html"},
	{"releases", "App 发布", "/admin/index.html"},
	{"analytics", "埋点同步", "/admin/analytics.html"},
	{"ai_usage", "AI 使用", "/admin/ai_usage.html"},
	{"ai_tasks", "AI 任务中断与重试", ""},
	{"ai_settings", "AI 预算与价格配置", ""},
}

type accountRecord struct {
	ID                 string            `json:"id"`
	Username           string            `json:"username"`
	DisplayName        string            `json:"display_name"`
	Role               string            `json:"role"`
	Enabled            bool              `json:"enabled"`
	Permissions        map[string]string `json:"permissions"`
	PasswordHash       string            `json:"password_hash"`
	MustChangePassword bool              `json:"must_change_password"`
	SecurityVersion    int64             `json:"security_version"`
	Revision           int64             `json:"revision"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
	LastLoginAt        *time.Time        `json:"last_login_at,omitempty"`
	Email              string            `json:"email,omitempty"`
	EmailVerifiedAt    *time.Time        `json:"email_verified_at,omitempty"`
	EmailBinding       *emailChallenge   `json:"email_binding,omitempty"`
	EmailChangeProof   *emailChallenge   `json:"email_change_proof,omitempty"`
	PasswordRecovery   *emailChallenge   `json:"password_recovery,omitempty"`
	MailWindow         emailSendWindow   `json:"mail_window,omitempty"`
}

type accountState struct {
	Accounts                    map[string]*accountRecord `json:"accounts"`
	LegacyPasswordResetDisabled bool                      `json:"legacy_password_reset_disabled,omitempty"`
}

// accountView 是唯一可以下发浏览器的账号结构，不包含密码哈希和会话失效版本。
type accountView struct {
	ID                 string            `json:"id"`
	Username           string            `json:"username"`
	DisplayName        string            `json:"display_name"`
	Role               string            `json:"role"`
	Enabled            bool              `json:"enabled"`
	Permissions        map[string]string `json:"permissions"`
	MustChangePassword bool              `json:"must_change_password"`
	Revision           int64             `json:"revision"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
	LastLoginAt        *time.Time        `json:"last_login_at,omitempty"`
	RecoveryEmailBound bool              `json:"recovery_email_bound"`
	CanResetPassword   bool              `json:"can_reset_password"`
}

func (a *accountRecord) view() accountView {
	return accountView{a.ID, a.Username, a.DisplayName, a.Role, a.Enabled, a.Permissions, a.MustChangePassword, a.Revision, a.CreatedAt, a.UpdatedAt, a.LastLoginAt, a.recoveryReady(), false}
}

func (a *accountRecord) allows(module string, write bool) bool {
	if a.Role == roleAdmin {
		return true
	}
	if module == "accounts" {
		return false
	}
	level := a.Permissions[module]
	return level == accessWrite || (!write && level == accessRead)
}

func (a *accountRecord) home() string {
	if a.MustChangePassword {
		return "/admin/password.html"
	}
	for _, module := range permissionModules {
		if module.Page != "" && a.allows(module.ID, false) {
			return module.Page
		}
	}
	return "/admin/access.html"
}

type accountError struct {
	Status  int
	Message string
}

func (e *accountError) Error() string { return e.Message }

func accountFailure(status int, message string) error { return &accountError{status, message} }

type AccountManager struct {
	storage      AccountStorage
	bootstrap    map[string]string
	emailCodeKey []byte
}

func (m *AccountManager) load(ctx context.Context) (*accountState, int64, error) {
	if m == nil || m.storage == nil {
		return nil, 0, errAccountStoreUnavailable
	}
	for attempt := 0; attempt < 5; attempt++ {
		version, data, err := m.storage.Load(ctx)
		if err != nil {
			return nil, 0, errAccountStoreUnavailable
		}
		if version != 0 {
			var state accountState
			if version < 1 || json.Unmarshal(data, &state) != nil || len(state.Accounts) == 0 {
				return nil, 0, errAccountStoreUnavailable
			}
			admins := 0
			for username, account := range state.Accounts {
				if account == nil || account.Username != username || account.ID == "" || account.PasswordHash == "" || account.SecurityVersion < 1 || account.Revision < 1 || (account.Role != roleAdmin && account.Role != roleOperator) {
					return nil, 0, errAccountStoreUnavailable
				}
				if account.Enabled && account.Role == roleAdmin {
					admins++
				}
			}
			if admins == 0 {
				return nil, 0, errAccountStoreUnavailable
			}
			return &state, version, nil
		}
		if len(m.bootstrap) == 0 {
			return nil, 0, accountFailure(503, "后台尚未初始化，请配置初始 admin 账号")
		}
		if m.bootstrap["admin"] == "" {
			return nil, 0, accountFailure(503, "首次初始化必须配置名为 admin 的管理员账号")
		}
		state := &accountState{Accounts: make(map[string]*accountRecord)}
		for username, hash := range m.bootstrap {
			if _, err := bcrypt.Cost([]byte(hash)); err != nil {
				return nil, 0, accountFailure(503, "初始账号密码哈希无效")
			}
			id, err := randomToken(16)
			if err != nil {
				return nil, 0, errAccountStoreUnavailable
			}
			permissions := make(map[string]string)
			for _, module := range permissionModules {
				permissions[module.ID] = accessRead
			}
			role := roleOperator
			if username == "admin" {
				role = roleAdmin
			}
			now := time.Now().UTC()
			state.Accounts[username] = &accountRecord{ID: id, Username: username, DisplayName: username, Role: role, Enabled: true, Permissions: permissions, PasswordHash: hash, SecurityVersion: 1, Revision: 1, CreatedAt: now, UpdatedAt: now}
		}
		data, err = json.Marshal(state)
		if err != nil {
			return nil, 0, errAccountStoreUnavailable
		}
		saved, err := m.storage.Save(ctx, 0, data)
		if err != nil {
			return nil, 0, errAccountStoreUnavailable
		}
		if saved {
			return state, 1, nil
		}
	}
	return nil, 0, accountFailure(409, "账号正在被其他管理员修改，请重试")
}

func (m *AccountManager) mutate(ctx context.Context, actor string, change func(*accountState) error) error {
	for attempt := 0; attempt < 5; attempt++ {
		state, version, err := m.load(ctx)
		if err != nil {
			return err
		}
		if actor != "" {
			a := state.Accounts[actor]
			if a == nil || !a.Enabled || a.Role != roleAdmin || a.MustChangePassword {
				return accountFailure(403, "只有管理员可以管理账号")
			}
		}
		if err := change(state); err != nil {
			return err
		}
		activeAdmins := 0
		for _, account := range state.Accounts {
			if account.Enabled && account.Role == roleAdmin {
				activeAdmins++
			}
		}
		if activeAdmins == 0 {
			return accountFailure(409, "必须保留至少一个启用的管理员")
		}
		if allEnabledAccountsHaveEmail(state) {
			state.LegacyPasswordResetDisabled = true
		}
		data, err := json.Marshal(state)
		if err != nil {
			return errAccountStoreUnavailable
		}
		saved, err := m.storage.Save(ctx, version, data)
		if err != nil {
			return errAccountStoreUnavailable
		}
		if saved {
			return nil
		}
	}
	return accountFailure(409, "账号正在被其他管理员修改，请重新加载")
}

func (m *AccountManager) get(ctx context.Context, username string) (*accountRecord, error) {
	state, _, err := m.load(ctx)
	if err != nil {
		return nil, err
	}
	return state.Accounts[username], nil
}

func validatePermissions(permissions map[string]string) error {
	known := make(map[string]bool)
	for _, module := range permissionModules {
		known[module.ID] = true
	}
	for module, level := range permissions {
		if !known[module] || (level != accessNone && level != accessRead && level != accessWrite) {
			return accountFailure(400, "页面权限配置无效")
		}
	}
	return nil
}

func validatePassword(password string) error {
	if utf8.RuneCountInString(password) < 8 || len(password) > 72 {
		return accountFailure(400, "密码至少 8 个字符，且不能超过 72 字节")
	}
	return nil
}

type accountInput struct {
	Username    string            `json:"username"`
	DisplayName string            `json:"display_name"`
	Role        string            `json:"role"`
	Enabled     bool              `json:"enabled"`
	Permissions map[string]string `json:"permissions"`
	Password    string            `json:"password"`
	Revision    int64             `json:"revision"`
}

var accountUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]{0,63}$`)

func (m *AccountManager) saveAccount(ctx context.Context, actor, username string, body accountInput, create bool) error {
	if create && !accountUsernamePattern.MatchString(username) {
		return accountFailure(400, "登录名为 1–64 位字母、数字或 _.@-，且以字母或数字开头")
	}
	if body.Role != roleAdmin && body.Role != roleOperator {
		return accountFailure(400, "身份无效")
	}
	if utf8.RuneCountInString(strings.TrimSpace(body.DisplayName)) > 64 {
		return accountFailure(400, "显示名称不能超过 64 个字符")
	}
	if err := validatePermissions(body.Permissions); err != nil {
		return err
	}
	var hash []byte
	var err error
	if create {
		if err := validatePassword(body.Password); err != nil {
			return err
		}
		hash, err = bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
	}
	id, err := randomToken(16)
	if err != nil {
		return err
	}
	return m.mutate(ctx, actor, func(state *accountState) error {
		a := state.Accounts[username]
		if create {
			if a != nil {
				return accountFailure(409, "登录名已存在")
			}
			a = &accountRecord{ID: id, Username: username, PasswordHash: string(hash), SecurityVersion: 1, CreatedAt: time.Now().UTC(), MustChangePassword: true}
			state.Accounts[username] = a
		} else if a == nil {
			return accountFailure(404, "账号不存在")
		} else if a.Revision != body.Revision {
			return accountFailure(409, "该账号已被修改，请重新加载后再保存")
		}
		if !body.Enabled && a.Enabled {
			a.SecurityVersion++
			a.clearEmailChallenges()
		}
		a.DisplayName = strings.TrimSpace(body.DisplayName)
		if a.DisplayName == "" {
			a.DisplayName = username
		}
		a.Role, a.Enabled, a.Permissions = body.Role, body.Enabled, body.Permissions
		a.Revision++
		a.UpdatedAt = time.Now().UTC()
		return nil
	})
}

func (m *AccountManager) list(ctx context.Context) ([]accountView, error) {
	state, _, err := m.load(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]accountView, 0, len(state.Accounts))
	for _, account := range state.Accounts {
		item := account.view()
		item.CanResetPassword = !state.LegacyPasswordResetDisabled && !account.recoveryReady()
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Username < items[j].Username })
	return items, nil
}

func (m *AccountManager) resetPassword(ctx context.Context, actor, username, password string, revision int64) error {
	if err := validatePassword(password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return m.mutate(ctx, actor, func(state *accountState) error {
		a := state.Accounts[username]
		if a == nil {
			return accountFailure(404, "账号不存在")
		}
		if state.LegacyPasswordResetDisabled || a.recoveryReady() {
			return accountFailure(403, "该账号密码由本人管理，请使用已验证邮箱找回")
		}
		if a.Revision != revision {
			return accountFailure(409, "该账号已被修改，请重新加载")
		}
		a.PasswordHash = string(hash)
		a.MustChangePassword = true
		a.SecurityVersion++
		a.clearEmailChallenges()
		a.Revision++
		a.UpdatedAt = time.Now().UTC()
		return nil
	})
}

func (m *AccountManager) changePassword(ctx context.Context, sess *Session, oldPassword, password string) error {
	if err := validatePassword(password); err != nil {
		return err
	}
	if oldPassword == password {
		return accountFailure(400, "新密码不能与原密码相同")
	}
	a, err := m.get(ctx, sess.Username)
	if err != nil {
		return err
	}
	if a == nil || bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(oldPassword)) != nil {
		return accountFailure(400, "原密码不正确")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return m.mutate(ctx, "", func(state *accountState) error {
		current := state.Accounts[sess.Username]
		if current == nil || !current.Enabled || current.SecurityVersion != sess.SecurityVersion || current.PasswordHash != a.PasswordHash {
			return accountFailure(401, "登录已失效，请重新登录")
		}
		current.PasswordHash = string(hash)
		current.SecurityVersion++
		current.clearEmailChallenges()
		current.MustChangePassword = false
		current.Revision++
		current.UpdatedAt = time.Now().UTC()
		return nil
	})
}

func accountErrorStatus(err error) (int, string) {
	var expected *accountError
	if errors.As(err, &expected) {
		return expected.Status, expected.Message
	}
	return 503, fmt.Sprint(errAccountStoreUnavailable)
}
