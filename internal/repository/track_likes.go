package repository

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

type TrackLikeRepository interface {
	SetLike(context.Context, int64, string, bool) error
	Stats(context.Context, int64, []string) (map[string]models.TrackLikeStats, error)
	CountReceived(context.Context, int64) (int64, error)
}

type InMemoryTrackLikeRepository struct {
	mu     sync.RWMutex
	likes  map[string]map[int64]*models.TrackLike
	tracks TrackRepository
}

func NewInMemoryTrackLikeRepository(tracks TrackRepository) *InMemoryTrackLikeRepository {
	return &InMemoryTrackLikeRepository{likes: make(map[string]map[int64]*models.TrackLike), tracks: tracks}
}

func (r *InMemoryTrackLikeRepository) SetLike(_ context.Context, userID int64, trackID string, liked bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !liked {
		delete(r.likes[trackID], userID)
		return nil
	}
	if r.likes[trackID] == nil {
		r.likes[trackID] = make(map[int64]*models.TrackLike)
	}
	if r.likes[trackID][userID] == nil {
		r.likes[trackID][userID] = &models.TrackLike{UserID: userID, TrackID: trackID, CreatedAt: time.Now().UTC().Truncate(time.Microsecond)}
	}
	return nil
}

func (r *InMemoryTrackLikeRepository) Stats(_ context.Context, userID int64, ids []string) (map[string]models.TrackLikeStats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]models.TrackLikeStats, len(ids))
	for _, id := range ids {
		result[id] = models.TrackLikeStats{Liked: r.likes[id][userID] != nil, LikeCount: int64(len(r.likes[id]))}
	}
	return result, nil
}

func (r *InMemoryTrackLikeRepository) CountReceived(ctx context.Context, ownerID int64) (int64, error) {
	r.mu.RLock()
	counts := make(map[string]int64, len(r.likes))
	for trackID, users := range r.likes {
		for userID := range users {
			if userID != ownerID {
				counts[trackID]++
			}
		}
	}
	r.mu.RUnlock()
	var total int64
	for trackID, count := range counts {
		track, err := r.tracks.FindByID(ctx, trackID)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if track.UserID == ownerID && track.Status == models.TrackStatusNormal && !track.IsRunning && strings.TrimSpace(track.RawTrackURL) != "" {
			total += count
		}
	}
	return total, nil
}
