package service

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

const defaultAvatarURLPrefix = "/api/v1/static/default_avatars/"

var defaultAvatarFiles = [...]string{"girl_01.png", "girl_2.png", "boy_01.png", "boy_02.png"}

// UserService provides business logic related to user profile and settings.
type UserService struct {
	users       repository.UserRepository
	avatarCache *AssetCacheService
}

// NewUserService constructs a new UserService.
func NewUserService(users repository.UserRepository) *UserService {
	return &UserService{users: users}
}

// SetAvatarCache 设置用户头像本地缓存服务。
// 未设置时，GetUserProfile 将直接返回仓储中的原始 AvatarURL。
func (s *UserService) SetAvatarCache(cache *AssetCacheService) {
	s.avatarCache = cache
}

// EnsureUser makes sure the user exists in persistence layer.
func (s *UserService) EnsureUser(ctx context.Context, userID int64, language string) (*models.User, error) {
	if userID <= 0 {
		return nil, errors.New("userID is required")
	}
	user := &models.User{ID: userID, ClientLanguage: language}
	return s.users.CreateIfNotExists(ctx, user)
}

// GetUserProfile returns the detail of a user.
func (s *UserService) GetUserProfile(ctx context.Context, userID int64) (*models.User, error) {
	if userID <= 0 {
		return nil, errors.New("userID is required")
	}
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	s.decorateUserAvatar(ctx, user)
	return user, nil
}

// UpdateAvatar updates user's avatar URL.
func (s *UserService) UpdateAvatar(ctx context.Context, userID int64, avatarURL string) (*models.User, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	cacheKey := strconv.FormatInt(userID, 10)
	user.AvatarURL = avatarURL
	if err := s.users.Update(ctx, user); err != nil {
		return nil, err
	}
	if s.avatarCache != nil {
		if err := s.avatarCache.RemoveCached(cacheKey); err != nil {
			return nil, err
		}
		s.avatarCache.PrefetchAsync(userID, cacheKey, avatarURL)
		user.AvatarURL = s.avatarCache.GuessLocalURL(cacheKey, avatarURL)
	}
	return user, nil
}

func (s *UserService) decorateUserAvatar(ctx context.Context, user *models.User) {
	if user == nil {
		return
	}
	if user.AvatarURL == "" {
		user.AvatarURL = defaultAvatarURL(user.ID)
		return
	}
	if s.avatarCache == nil {
		return
	}
	cacheCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	user.AvatarURL = fallbackAvatarURL(user.ID, s.avatarCache.EnsureCached(cacheCtx, user.ID, strconv.FormatInt(user.ID, 10), user.AvatarURL))
}

func fallbackAvatarURL(userID int64, avatarURL string) string {
	if avatarURL != "" {
		return avatarURL
	}
	return defaultAvatarURL(userID)
}

func defaultAvatarURL(userID int64) string {
	idx := 0
	if userID > 0 {
		idx = int((userID - 1) % int64(len(defaultAvatarFiles)))
	}
	return defaultAvatarURLPrefix + defaultAvatarFiles[idx]
}

// UpdateName updates user's nickname.
func (s *UserService) UpdateName(ctx context.Context, userID int64, name string) (*models.User, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	user.Nickname = name
	return user, s.users.Update(ctx, user)
}

// UpdateSignature updates user's signature.
func (s *UserService) UpdateSignature(ctx context.Context, userID int64, sig string) (*models.User, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	user.Signature = sig
	return user, s.users.Update(ctx, user)
}

// UpdatePhone updates user's phone.
func (s *UserService) UpdatePhone(ctx context.Context, userID int64, phone string) (*models.User, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	user.Phone = phone
	return user, s.users.Update(ctx, user)
}

// UpdateClientLanguage updates user's client language.
func (s *UserService) UpdateClientLanguage(ctx context.Context, userID int64, lang string) (*models.User, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	user.ClientLanguage = lang
	return user, s.users.Update(ctx, user)
}
