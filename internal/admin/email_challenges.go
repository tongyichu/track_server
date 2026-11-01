package admin

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	emailBindPurpose     = "bind"
	emailChangePurpose   = "change-email"
	emailRecoveryPurpose = "recover-password"
)

type emailChallenge struct {
	Email           string    `json:"email"`
	PreviousEmail   string    `json:"previous_email,omitempty"`
	Salt            string    `json:"salt"`
	Hash            string    `json:"hash"`
	ExpiresAt       time.Time `json:"expires_at"`
	SecurityVersion int64     `json:"security_version"`
	Attempts        int       `json:"attempts"`
}

type emailSendWindow struct {
	Start time.Time            `json:"start"`
	Last  map[string]time.Time `json:"last,omitempty"`
	Count int                  `json:"count"`
}

func normalizeRecoveryEmail(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	address, err := mail.ParseAddress(value)
	if err != nil || len(value) > 254 || address.Name != "" || address.Address != value || strings.ContainsAny(value, "\r\n") {
		return "", accountFailure(400, "请填写有效的邮箱地址")
	}
	for _, ch := range value {
		if ch > 127 {
			return "", accountFailure(400, "请使用标准英文邮箱地址")
		}
	}
	return value, nil
}

func (a *accountRecord) recoveryReady() bool {
	return a.Email != "" && a.EmailVerifiedAt != nil
}

func allEnabledAccountsHaveEmail(state *accountState) bool {
	count := 0
	for _, account := range state.Accounts {
		if account.Enabled {
			count++
			if !account.recoveryReady() {
				return false
			}
		}
	}
	return count > 0
}

func (a *accountRecord) clearEmailChallenges() {
	a.EmailBinding, a.EmailChangeProof, a.PasswordRecovery = nil, nil, nil
}

func (a *accountRecord) challenge(purpose string) **emailChallenge {
	switch purpose {
	case emailBindPurpose:
		return &a.EmailBinding
	case emailChangePurpose:
		return &a.EmailChangeProof
	default:
		return &a.PasswordRecovery
	}
}

func (m *AccountManager) emailCodeHash(salt, purpose, username, code string) string {
	mac := hmac.New(sha256.New, m.emailCodeKey)
	_, _ = mac.Write([]byte(salt + "\x00" + purpose + "\x00" + username + "\x00" + code))
	return hex.EncodeToString(mac.Sum(nil))
}

func (m *AccountManager) newEmailChallenge(a *accountRecord, email, purpose string) (*emailChallenge, string, error) {
	if len(m.emailCodeKey) != 32 {
		return nil, "", errAccountStoreUnavailable
	}
	number, err := rand.Int(rand.Reader, big.NewInt(100000000))
	if err != nil {
		return nil, "", errAccountStoreUnavailable
	}
	salt, err := randomToken(16)
	if err != nil {
		return nil, "", errAccountStoreUnavailable
	}
	code := fmt.Sprintf("%08d", number.Int64())
	return &emailChallenge{Email: email, PreviousEmail: a.Email, Salt: salt, Hash: m.emailCodeHash(salt, purpose, a.Username, code), ExpiresAt: time.Now().UTC().Add(10 * time.Minute), SecurityVersion: a.SecurityVersion}, code, nil
}

func reserveEmailSend(a *accountRecord, purpose string) error {
	now := time.Now().UTC()
	if now.Sub(a.MailWindow.Last[purpose]) < time.Minute {
		return accountFailure(429, "发送过于频繁，请一分钟后重试")
	}
	if now.Sub(a.MailWindow.Start) >= time.Hour {
		a.MailWindow.Start, a.MailWindow.Count = now, 0
	}
	if a.MailWindow.Count >= 10 {
		return accountFailure(429, "本小时邮件发送次数已达上限，请稍后重试")
	}
	if a.MailWindow.Last == nil {
		a.MailWindow.Last = make(map[string]time.Time)
	}
	a.MailWindow.Last[purpose] = now
	a.MailWindow.Count++
	return nil
}

// Invalid guesses must be committed through mutate as well as valid consumption.
func (m *AccountManager) consumeEmailCode(a *accountRecord, purpose, code string) bool {
	slot := a.challenge(purpose)
	c := *slot
	if c == nil {
		return false
	}
	if !time.Now().Before(c.ExpiresAt) || c.SecurityVersion != a.SecurityVersion || c.Attempts >= 5 {
		*slot = nil
		return false
	}
	if subtle.ConstantTimeCompare([]byte(c.Hash), []byte(m.emailCodeHash(c.Salt, purpose, a.Username, strings.TrimSpace(code)))) != 1 {
		c.Attempts++
		if c.Attempts >= 5 {
			*slot = nil
		}
		return false
	}
	*slot = nil
	return true
}

func currentEmailAccount(state *accountState, sess *Session) (*accountRecord, error) {
	a := state.Accounts[sess.Username]
	if a == nil || !a.Enabled || a.ID != sess.AccountID || a.SecurityVersion != sess.SecurityVersion || a.MustChangePassword {
		return nil, accountFailure(401, "请重新登录并先修改临时密码")
	}
	return a, nil
}

func emailAlreadyBound(state *accountState, username, email string) bool {
	for name, a := range state.Accounts {
		if name != username && a.recoveryReady() && a.Email == email {
			return true
		}
	}
	return false
}

func (m *AccountManager) startEmailBinding(ctx context.Context, sess *Session, email, password, previousCode string, proveCurrent bool) (*emailChallenge, string, error) {
	snapshot, err := m.get(ctx, sess.Username)
	if err != nil {
		return nil, "", err
	}
	if snapshot == nil || bcrypt.CompareHashAndPassword([]byte(snapshot.PasswordHash), []byte(password)) != nil {
		return nil, "", accountFailure(400, "当前密码不正确")
	}
	purpose := emailBindPurpose
	if proveCurrent {
		purpose, email = emailChangePurpose, snapshot.Email
	}
	challenge, code, err := m.newEmailChallenge(snapshot, email, purpose)
	if err != nil {
		return nil, "", err
	}
	var problem error
	err = m.mutate(ctx, "", func(state *accountState) error {
		problem = nil
		a, err := currentEmailAccount(state, sess)
		if err != nil {
			return err
		}
		if a.PasswordHash != snapshot.PasswordHash || a.Email != snapshot.Email {
			return accountFailure(409, "账号资料已变化，请刷新后重试")
		}
		if proveCurrent {
			if !a.recoveryReady() {
				return accountFailure(400, "尚未绑定邮箱")
			}
		} else {
			if email == a.Email && a.recoveryReady() {
				return accountFailure(400, "该邮箱已验证，无需重复绑定")
			}
			if emailAlreadyBound(state, a.Username, email) {
				return accountFailure(409, "该邮箱暂不能绑定，请使用其他邮箱")
			}
			if a.recoveryReady() && !m.consumeEmailCode(a, emailChangePurpose, previousCode) {
				problem = accountFailure(400, "原邮箱验证码无效、已过期或错误次数过多")
				return nil
			}
		}
		if err := reserveEmailSend(a, purpose); err != nil {
			return err
		}
		*a.challenge(purpose) = challenge
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	if problem != nil {
		return nil, "", problem
	}
	return challenge, code, nil
}

func (m *AccountManager) confirmEmailBinding(ctx context.Context, sess *Session, code string) (string, string, error) {
	var previous, email string
	var problem error
	err := m.mutate(ctx, "", func(state *accountState) error {
		problem = nil
		a, err := currentEmailAccount(state, sess)
		if err != nil {
			return err
		}
		challenge := a.EmailBinding
		if !m.consumeEmailCode(a, emailBindPurpose, code) {
			problem = accountFailure(400, "邮箱验证码无效、已过期或错误次数过多")
			return nil
		}
		if challenge.PreviousEmail != a.Email || emailAlreadyBound(state, a.Username, challenge.Email) {
			return accountFailure(409, "邮箱信息已变化，请重新验证")
		}
		previous, email = a.Email, challenge.Email
		now := time.Now().UTC()
		a.Email, a.EmailVerifiedAt = email, &now
		a.SecurityVersion++
		a.Revision++
		a.UpdatedAt = now
		a.clearEmailChallenges()
		return nil
	})
	if err != nil {
		return "", "", err
	}
	return previous, email, problem
}

func (m *AccountManager) startPasswordRecovery(ctx context.Context, username, email string) (*emailChallenge, string, error) {
	a, err := m.get(ctx, username)
	if err != nil {
		return nil, "", err
	}
	if a == nil || !a.Enabled || !a.recoveryReady() || a.Email != email {
		return nil, "", nil
	}
	challenge, code, err := m.newEmailChallenge(a, email, emailRecoveryPurpose)
	if err != nil {
		return nil, "", err
	}
	err = m.mutate(ctx, "", func(state *accountState) error {
		current := state.Accounts[username]
		if current == nil || !current.Enabled || !current.recoveryReady() || current.Email != email || current.SecurityVersion != a.SecurityVersion {
			return accountFailure(409, "账号信息已变化")
		}
		if err := reserveEmailSend(current, emailRecoveryPurpose); err != nil {
			return err
		}
		current.PasswordRecovery = challenge
		return nil
	})
	return challenge, code, err
}

func (m *AccountManager) completePasswordRecovery(ctx context.Context, username, email, code, password string) error {
	if err := validatePassword(password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return errAccountStoreUnavailable
	}
	var problem error
	err = m.mutate(ctx, "", func(state *accountState) error {
		problem = nil
		a := state.Accounts[username]
		if a == nil || !a.Enabled || !a.recoveryReady() || a.Email != email {
			problem = accountFailure(400, "验证码无效或已过期，请重新申请")
			return nil
		}
		if !m.consumeEmailCode(a, emailRecoveryPurpose, code) {
			problem = accountFailure(400, "验证码无效或已过期，请重新申请")
			return nil
		}
		if bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(password)) == nil {
			return accountFailure(400, "新密码不能与当前密码相同")
		}
		a.PasswordHash, a.MustChangePassword = string(hash), false
		a.SecurityVersion++
		a.Revision++
		a.UpdatedAt = time.Now().UTC()
		a.clearEmailChallenges()
		return nil
	})
	if err != nil {
		return err
	}
	return problem
}

func (m *AccountManager) discardEmailChallenge(ctx context.Context, username, purpose, hash string) {
	_ = m.mutate(ctx, "", func(state *accountState) error {
		if a := state.Accounts[username]; a != nil {
			if c := *a.challenge(purpose); c != nil && c.Hash == hash {
				*a.challenge(purpose) = nil
			}
		}
		return nil
	})
}
