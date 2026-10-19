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
	ListReceivedGroups(context.Context, int64, *models.ReceivedLikeTrackCursor, int) ([]*models.ReceivedLikeTrack, error)
	CountReceivedGroups(context.Context, int64) (int64, error)
	ListReceivedTrackLikers(context.Context, int64, string, *models.TrackLikeCursor, int) ([]*models.TrackLike, error)
	SetLike(context.Context, int64, string, bool) error
	Stats(context.Context, int64, []string) (map[string]models.TrackLikeStats, error)
	CountReceived(context.Context, int64) (int64, error)
	ListReceived(context.Context, int64, *models.TrackLikeCursor, int) ([]*models.TrackLike, error)
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

func (r *InMemoryTrackLikeRepository) ListReceived(ctx context.Context, ownerID int64, cursor *models.TrackLikeCursor, limit int) ([]*models.TrackLike, error) {
	r.mu.RLock()
	records := make([]*models.TrackLike, 0)
	for _, users := range r.likes {
		for _, like := range users {
			copy := *like
			records = append(records, &copy)
		}
	}
	r.mu.RUnlock()
	result := make([]*models.TrackLike, 0)
	for _, like := range records {
		track, err := r.tracks.FindByID(ctx, like.TrackID)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if like.UserID == ownerID || track.UserID != ownerID || track.Status != models.TrackStatusNormal || track.IsRunning || strings.TrimSpace(track.RawTrackURL) == "" {
			continue
		}
		if cursor != nil && !likeBefore(like, cursor) {
			continue
		}
		result = append(result, like)
	}
	sort.Slice(result, func(i, j int) bool {
		return likeBefore(result[j], &models.TrackLikeCursor{CreatedAt: result[i].CreatedAt, TrackID: result[i].TrackID, UserID: result[i].UserID})
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func likeBefore(like *models.TrackLike, c *models.TrackLikeCursor) bool {
	if !like.CreatedAt.Equal(c.CreatedAt) {
		return like.CreatedAt.Before(c.CreatedAt)
	}
	if like.TrackID != c.TrackID {
		return like.TrackID < c.TrackID
	}
	return like.UserID < c.UserID
}

func (r *InMemoryTrackLikeRepository) CountReceived(ctx context.Context, ownerID int64) (int64, error) {
	records, err := r.ListReceived(ctx, ownerID, nil, 0)
	return int64(len(records)), err
}
