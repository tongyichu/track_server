package service

import (
	"context"
	"errors"
	"sync"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type UserProfileOverview struct {
	FollowingCount    int64                   `json:"following_count"`
	FollowerCount     int64                   `json:"follower_count"`
	ReceivedLikeCount int64                   `json:"received_like_count"`
	Level             models.AchievementLevel `json:"level"`
}

// GetUserLevels uses the exact profile XP/level rules without settling rewards.
func (s *UserService) GetUserLevels(ctx context.Context, userIDs []int64) (map[int64]models.AchievementLevel, error) {
	if s == nil || s.achievements == nil {
		return nil, errors.New("achievement service unavailable")
	}
	reader, ok := s.achievements.tracks.(repository.AchievementTrackReader)
	if !ok {
		return nil, errors.New("achievement track reader unavailable")
	}
	return userLevelsFromTracks(userIDs, func(consume func(*models.Track)) error {
		return reader.ScanAchievementTracks(ctx, userIDs, consume)
	})
}

// UserLevelOptions returns a copy of the authoritative level definitions.
func UserLevelOptions() []models.AchievementLevel {
	return append([]models.AchievementLevel(nil), achievementLevels...)
}

// GetAllUserLevels streams existing tracks once; absent users have the base level.
// Only explicit admin level filters need this full scan; ordinary lists stay page-scoped.
func (s *UserService) GetAllUserLevels(ctx context.Context) (map[int64]models.AchievementLevel, error) {
	if s == nil || s.achievements == nil {
		return nil, errors.New("achievement service unavailable")
	}
	reader, ok := s.achievements.tracks.(repository.AchievementTrackScanner)
	if !ok {
		return nil, errors.New("achievement track scanner unavailable")
	}
	return userLevelsFromTracks(nil, func(consume func(*models.Track)) error {
		return reader.ScanAllAchievementTracks(ctx, consume)
	})
}

func userLevelsFromTracks(userIDs []int64, scan func(func(*models.Track)) error) (map[int64]models.AchievementLevel, error) {
	xp := make(map[int64]int64, len(userIDs))
	for _, id := range userIDs {
		xp[id] = 0
	}
	if err := scan(func(track *models.Track) {
		if isQualifiedAchievementTrack(track) {
			xp[track.UserID] += achievementTrackXP(track)
		}
	}); err != nil {
		return nil, err
	}
	levels := make(map[int64]models.AchievementLevel, len(xp))
	for id, total := range xp {
		level, _, _ := resolveAchievementLevel(total)
		levels[id] = level
	}
	return levels, nil
}

func (s *UserService) GetUserFollowerCounts(ctx context.Context, userIDs []int64) (map[int64]int64, error) {
	if s == nil || s.follows == nil {
		return nil, errors.New("follow repository unavailable")
	}
	return s.follows.CountFollowersByUserIDs(ctx, userIDs)
}

// GetUserProfileOverview is read-only: no media fetching or achievement settlement.
func (s *UserService) GetUserProfileOverview(ctx context.Context, userID int64) (*UserProfileOverview, error) {
	if s == nil || s.follows == nil || s.likes == nil || userID <= 0 {
		return nil, errors.New("user overview unavailable")
	}
	result := &UserProfileOverview{}
	var errs [4]error
	var reads sync.WaitGroup
	reads.Add(4)
	go func() { defer reads.Done(); result.FollowingCount, errs[0] = s.follows.CountFollowing(ctx, userID) }()
	go func() { defer reads.Done(); result.FollowerCount, errs[1] = s.follows.CountFollowers(ctx, userID) }()
	go func() { defer reads.Done(); result.ReceivedLikeCount, errs[2] = s.likes.CountReceived(ctx, userID) }()
	go func() {
		defer reads.Done()
		levels, err := s.GetUserLevels(ctx, []int64{userID})
		errs[3] = err
		if err == nil {
			result.Level = levels[userID]
		}
	}()
	reads.Wait()
	if err := errors.Join(errs[:]...); err != nil {
		return nil, err
	}
	return result, nil
}
