package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

const (
	AccountRestrictionMessageUpload    = "账号已被限制，禁止上传内容"
	AccountRestrictionMessageCompanion = "账号已被限制，禁止发起同行"
	AccountRestrictionMessageFollow    = "账号已被限制，禁止关注用户"
	AccountRestrictionMessageCollect   = "账号已被限制，禁止收藏轨迹"
	AccountRestrictionMessageProfile   = "账号已被限制，禁止修改个人信息"
	AccountRestrictionMessageOperation = "账号已被限制，禁止执行该操作"
	defaultAccountRestrictionListLimit = 50
	maxAccountRestrictionReasonRunes   = 255
	maxAccountRestrictionOperatorRunes = 64
)

type AccountRestrictionService struct {
	repo repository.AccountRestrictionRepository
}

type CreateAccountRestrictionInput struct {
	UserID    int64      `json:"user_id"`
	Reason    string     `json:"reason"`
	Operator  string     `json:"operator"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type AccountRestrictionBlockedError struct {
	Restriction *models.AccountRestriction `json:"restriction"`
	Message     string                     `json:"message"`
}

func (e *AccountRestrictionBlockedError) Error() string {
	if e == nil || e.Message == "" {
		return "account restricted"
	}
	return e.Message
}

func NewAccountRestrictionService(repo repository.AccountRestrictionRepository) *AccountRestrictionService {
	return &AccountRestrictionService{repo: repo}
}

func (s *AccountRestrictionService) Create(ctx context.Context, in CreateAccountRestrictionInput) (*models.AccountRestriction, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("account restriction service not configured")
	}
	if in.UserID <= 0 {
		return nil, invalidArg("user_id is required")
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		return nil, invalidArg("reason is required")
	}
	if utf8.RuneCountInString(reason) > maxAccountRestrictionReasonRunes {
		return nil, invalidArg("reason exceeds 255 characters")
	}
	operator := strings.TrimSpace(in.Operator)
	if operator == "" {
		operator = "ops"
	}
	if utf8.RuneCountInString(operator) > maxAccountRestrictionOperatorRunes {
		return nil, invalidArg("operator exceeds 64 characters")
	}
	now := time.Now()
	if in.ExpiresAt != nil && !in.ExpiresAt.After(now) {
		return nil, invalidArg("expires_at must be in the future")
	}
	if _, err := s.repo.RevokeActiveAccountRestrictions(ctx, in.UserID, operator, now); err != nil {
		return nil, err
	}
	item := &models.AccountRestriction{
		UserID:    in.UserID,
		Status:    models.AccountRestrictionStatusActive,
		Reason:    reason,
		Operator:  operator,
		ExpiresAt: in.ExpiresAt,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repo.CreateAccountRestriction(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *AccountRestrictionService) FindActive(ctx context.Context, userID int64) (*models.AccountRestriction, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("account restriction service not configured")
	}
	if userID <= 0 {
		return nil, invalidArg("user_id is required")
	}
	return s.repo.FindActiveAccountRestriction(ctx, userID, time.Now())
}

func (s *AccountRestrictionService) ListByUserID(ctx context.Context, userID int64, limit int) ([]*models.AccountRestriction, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("account restriction service not configured")
	}
	if userID <= 0 {
		return nil, invalidArg("user_id is required")
	}
	if limit <= 0 {
		limit = defaultAccountRestrictionListLimit
	}
	return s.repo.ListAccountRestrictionsByUserID(ctx, userID, limit)
}

func (s *AccountRestrictionService) RevokeActive(ctx context.Context, userID int64, operator string) (int64, error) {
	if s == nil || s.repo == nil {
		return 0, errors.New("account restriction service not configured")
	}
	if userID <= 0 {
		return 0, invalidArg("user_id is required")
	}
	operator = strings.TrimSpace(operator)
	if operator == "" {
		operator = "ops"
	}
	return s.repo.RevokeActiveAccountRestrictions(ctx, userID, operator, time.Now())
}

func (s *AccountRestrictionService) EnsureAllowed(ctx context.Context, userID int64, message string) error {
	item, err := s.FindActive(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return err
	}
	if message == "" {
		message = AccountRestrictionMessageOperation
	}
	return &AccountRestrictionBlockedError{Restriction: item, Message: message}
}
