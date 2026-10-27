package repository

import (
	"context"
	"sort"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
)

func escapeAdminLike(value string) string {
	value = strings.ReplaceAll(value, "!", "!!")
	value = strings.ReplaceAll(value, "%", "!%")
	return strings.ReplaceAll(value, "_", "!_")
}

func (r *InMemoryTrackRepository) ListAdminTracksByTitle(_ context.Context, title string, cursor *models.TrackListCursor, limit int) ([]*models.Track, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	needle := strings.ToLower(title)
	items := make([]*models.Track, 0)
	var total int64
	for _, track := range r.tracks {
		if track == nil || track.Status == models.TrackStatusDeleted || !strings.Contains(strings.ToLower(track.Title), needle) {
			continue
		}
		total++
		if cursor != nil && !cursor.StartTime.IsZero() && (track.StartTime.After(cursor.StartTime) || (track.StartTime.Equal(cursor.StartTime) && track.ID >= cursor.ID)) {
			continue
		}
		clone := *track
		items = append(items, &clone)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].StartTime.Equal(items[j].StartTime) {
			return items[i].ID > items[j].ID
		}
		return items[i].StartTime.After(items[j].StartTime)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, total, nil
}

func (r *InMemoryUserRepository) ListAdminUsersByTitle(_ context.Context, title string, cursor *models.UserListCursor, limit int) ([]*models.User, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	needle := strings.ToLower(title)
	items := make([]*models.User, 0)
	var total int64
	for _, user := range r.users {
		if user == nil || !strings.Contains(strings.ToLower(user.Nickname), needle) {
			continue
		}
		total++
		if cursor != nil && !cursor.CreatedAt.IsZero() && (user.CreatedAt.After(cursor.CreatedAt) || (user.CreatedAt.Equal(cursor.CreatedAt) && user.ID >= cursor.ID)) {
			continue
		}
		clone := *user
		if clone.TokenVersion <= 0 {
			clone.TokenVersion = 1
		}
		items = append(items, &clone)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, total, nil
}

func (r *InMemoryCompanionRepository) ListAdminCompanionsByTitle(_ context.Context, title string, cursor *models.CompanionSessionListCursor, limit int) ([]*models.CompanionSession, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	needle := strings.ToLower(title)
	items := make([]*models.CompanionSession, 0)
	var total int64
	for _, session := range r.sessions {
		if session == nil || !strings.Contains(strings.ToLower(session.Title), needle) {
			continue
		}
		total++
		if cursor != nil && !cursor.StartedAt.IsZero() && (session.StartedAt.After(cursor.StartedAt) || (session.StartedAt.Equal(cursor.StartedAt) && session.SessionID >= cursor.SessionID)) {
			continue
		}
		clone := *session
		items = append(items, &clone)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].StartedAt.Equal(items[j].StartedAt) {
			return items[i].SessionID > items[j].SessionID
		}
		return items[i].StartedAt.After(items[j].StartedAt)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, total, nil
}
