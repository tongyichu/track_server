package repository

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

type TrackLikeRepository interface {
	SetLike(context.Context, int64, string, bool) error
	Stats(context.Context, int64, []string) (map[string]models.TrackLikeStats, error)
	CountReceived(context.Context, int64) (int64, error)
	// ListLikedByUser 按 created_at desc, track_id desc 返回用户点赞记录，cursor 之后的严格更小项，最多 limit 条。
	ListLikedByUser(ctx context.Context, userID int64, cursor *models.TrackLikeCursor, limit int) ([]*models.TrackLike, error)
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

func (r *InMemoryTrackLikeRepository) ListLikedByUser(_ context.Context, userID int64, cursor *models.TrackLikeCursor, limit int) ([]*models.TrackLike, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]*models.TrackLike, 0)
	for trackID, users := range r.likes {
		like, ok := users[userID]
		if !ok || like == nil {
			continue
		}
		if cursor != nil {
			if like.CreatedAt.After(cursor.CreatedAt) {
				continue
			}
			if like.CreatedAt.Equal(cursor.CreatedAt) && trackID >= cursor.TrackID {
				continue
			}
		}
		copied := *like
		copied.TrackID = trackID
		items = append(items, &copied)
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].TrackID > items[j].TrackID
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
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
