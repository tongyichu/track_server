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

var (
	ErrNoFieldsToUpdate  = errors.New("no fields to update")
	ErrAvatarURLRequired = errors.New("avatar_url is required")
	ErrNameRequired      = errors.New("name is required")
)

// UserService provides business logic related to user profile and settings.
type UserService struct {
	users       repository.UserRepository
	tracks      repository.TrackRepository
	navigations repository.NavigationRepository
	avatarCache *AssetCacheService
}

// UserProfilePatch describes optional user profile fields that can be updated.
//
// Note:
// - AvatarURL/Name requires non-empty when provided.
// - Signature can be empty string to clear when provided.
type UserProfilePatch struct {
	AvatarURL *string
	Name      *string
	Signature *string
}

// NewUserService constructs a new UserService.
func NewUserService(users repository.UserRepository) *UserService {
	return &UserService{users: users}
}

// SetTrackRepository 设置轨迹仓储，用于用户统计信息（里程/轨迹数等）。
func (s *UserService) SetTrackRepository(repo repository.TrackRepository) {
	s.tracks = repo
}

// SetNavigationRepository 设置导航记录仓储，用于用户统计信息（轨迹被使用次数等）。
func (s *UserService) SetNavigationRepository(repo repository.NavigationRepository) {
	s.navigations = repo
}

type userTrackStatsProvider interface {
	StatsByUserID(ctx context.Context, userID int64) (trackCount int64, totalDistance float64, err error)
}

type userTrackUsedCounter interface {
	CountByTrackOwnerUserID(ctx context.Context, ownerUserID int64) (int64, error)
}

// UserStats 是 GetUserDetail 额外返回的统计信息。
type UserStats struct {
	TotalDistance  float64 `json:"total_distance"`
	TrackCount     int64   `json:"track_count"`
	TrackUsedCount int64   `json:"track_used_count"`
}

// GetUserStats 返回用户维度的统计信息。
// - 总里程：来自 track_records（按“我的轨迹”口径：排除删除与进行中）。
// - 轨迹总数：在上面基础上进一步过滤 raw_track_url 为空的记录。
// - 轨迹被使用总次数：来自 track_navigations（统计该用户轨迹被导航使用的记录数）。
func (s *UserService) GetUserStats(ctx context.Context, userID int64) (*UserStats, error) {
	stats := &UserStats{}
	if userID <= 0 {
		return stats, errors.New("userID is required")
	}

	// track_records 聚合
	if s.tracks != nil {
		if p, ok := s.tracks.(userTrackStatsProvider); ok {
			cnt, dist, err := p.StatsByUserID(ctx, userID)
			if err != nil {
				return nil, err
			}
			stats.TrackCount = cnt
			stats.TotalDistance = dist
		} else {
			// fallback：逐页扫描（用于非 SQL/测试实现）。
			var (
				cursor *models.TrackListCursor
				limit  = 200
			)
			for page := 0; page < 1000; page++ { // guard: 避免异常实现导致死循环
				items, err := s.tracks.ListByUserID(ctx, userID, cursor, limit)
				if err != nil {
					return nil, err
				}
				if len(items) == 0 {
					break
				}
				for _, t := range items {
					if t == nil {
						continue
					}
					if t.RawTrackURL != "" {
						stats.TrackCount++
					}
					stats.TotalDistance += t.Distance
				}
				last := items[len(items)-1]
				cursor = &models.TrackListCursor{StartTime: last.StartTime, ID: last.ID}
				if len(items) < limit {
					break
				}
			}
		}
	}

	// track_navigations 统计：优先走仓储侧聚合（MySQL join），否则 fallback 为基于 trackIDs 的累加。
	if s.navigations != nil {
		if c, ok := s.navigations.(userTrackUsedCounter); ok {
			used, err := c.CountByTrackOwnerUserID(ctx, userID)
			if err != nil {
				return nil, err
			}
			stats.TrackUsedCount = used
		} else if s.tracks != nil {
			// fallback：先拿到该用户所有轨迹 ID，再按 track_id 统计并求和。
			trackIDs := make([]string, 0, 64)
			var cursor *models.TrackListCursor
			limit := 200
			for page := 0; page < 1000; page++ {
				items, err := s.tracks.ListByUserID(ctx, userID, cursor, limit)
				if err != nil {
					return nil, err
				}
				if len(items) == 0 {
					break
				}
				for _, t := range items {
					if t != nil && t.ID != "" {
						trackIDs = append(trackIDs, t.ID)
					}
				}
				last := items[len(items)-1]
				cursor = &models.TrackListCursor{StartTime: last.StartTime, ID: last.ID}
				if len(items) < limit {
					break
				}
			}
			m, err := s.navigations.CountByTrackIDs(ctx, trackIDs)
			if err != nil {
				return nil, err
			}
			var total int64
			for _, v := range m {
				total += v
			}
			stats.TrackUsedCount = total
		}
	}

	return stats, nil
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

// UpdateProfile updates user's profile fields (avatar/name/signature) in one request.
// If avatar is updated and avatar cache is enabled, returned AvatarURL will be rewritten to local static URL.
func (s *UserService) UpdateProfile(ctx context.Context, userID int64, patch UserProfilePatch) (*models.User, error) {
	if userID <= 0 {
		return nil, errors.New("userID is required")
	}
	if patch.AvatarURL == nil && patch.Name == nil && patch.Signature == nil {
		return nil, ErrNoFieldsToUpdate
	}
	if patch.AvatarURL != nil && *patch.AvatarURL == "" {
		return nil, ErrAvatarURLRequired
	}
	if patch.Name != nil && *patch.Name == "" {
		return nil, ErrNameRequired
	}

	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Apply patch.
	avatarUpdated := false
	if patch.AvatarURL != nil {
		user.AvatarURL = *patch.AvatarURL
		avatarUpdated = true
	}
	if patch.Name != nil {
		user.Nickname = *patch.Name
	}
	if patch.Signature != nil {
		user.Signature = *patch.Signature
	}

	if err := s.users.Update(ctx, user); err != nil {
		return nil, err
	}

	// If avatar updated, clear cache and prefetch in background.
	if avatarUpdated && s.avatarCache != nil {
		cacheKey := strconv.FormatInt(userID, 10)
		if err := s.avatarCache.RemoveCached(cacheKey); err != nil {
			return nil, err
		}
		s.avatarCache.PrefetchAsync(userID, cacheKey, user.AvatarURL)
		user.AvatarURL = s.avatarCache.GuessLocalURL(cacheKey, user.AvatarURL)
	}
	return user, nil
}

// DecorateAvatar 把单个用户的头像 URL 改写为服务器本地缓存地址，
// 与 GetUserProfile 中的处理保持一致：未设置 avatarCache 时返回原值，
// 用户头像为空时回退到默认头像。供管理后台等批量场景按列表逐个调用。
func (s *UserService) DecorateAvatar(ctx context.Context, user *models.User) {
	s.decorateUserAvatar(ctx, user)
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
