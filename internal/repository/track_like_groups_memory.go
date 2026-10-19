package repository

import (
	"context"
	"sort"

	"github.com/tongyichu/track_server/internal/models"
)

func (r *InMemoryTrackLikeRepository) ListReceivedGroups(ctx context.Context, owner int64, cursor *models.ReceivedLikeTrackCursor, limit int) ([]*models.ReceivedLikeTrack, error) {
	records, err := r.ListReceived(ctx, owner, nil, 0)
	if err != nil {
		return nil, err
	}
	groups := make(map[string]*models.ReceivedLikeTrack)
	for _, record := range records {
		group := groups[record.TrackID]
		if group == nil {
			group = &models.ReceivedLikeTrack{TrackID: record.TrackID, LastLikedAt: record.CreatedAt, RecentLikes: make([]*models.TrackLike, 0, 3)}
			groups[record.TrackID] = group
		}
		group.LikeCount++
		if len(group.RecentLikes) < 3 {
			group.RecentLikes = append(group.RecentLikes, record)
		}
	}
	result := make([]*models.ReceivedLikeTrack, 0, len(groups))
	for _, group := range groups {
		if cursor != nil && (group.LastLikedAt.After(cursor.LastLikedAt) || group.LastLikedAt.Equal(cursor.LastLikedAt) && group.TrackID >= cursor.TrackID) {
			continue
		}
		result = append(result, group)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].LastLikedAt.Equal(result[j].LastLikedAt) {
			return result[i].TrackID > result[j].TrackID
		}
		return result[i].LastLikedAt.After(result[j].LastLikedAt)
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
func (r *InMemoryTrackLikeRepository) CountReceivedGroups(ctx context.Context, owner int64) (int64, error) {
	groups, err := r.ListReceivedGroups(ctx, owner, nil, 0)
	return int64(len(groups)), err
}
func (r *InMemoryTrackLikeRepository) ListReceivedTrackLikers(ctx context.Context, owner int64, track string, cursor *models.TrackLikeCursor, limit int) ([]*models.TrackLike, error) {
	records, err := r.ListReceived(ctx, owner, cursor, 0)
	if err != nil {
		return nil, err
	}
	result := make([]*models.TrackLike, 0)
	for _, record := range records {
		if record.TrackID == track {
			result = append(result, record)
			if len(result) == limit {
				break
			}
		}
	}
	return result, nil
}
