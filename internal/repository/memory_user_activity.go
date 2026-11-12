package repository

import (
	"context"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

var _ UserActivityRepository = (*InMemoryUserRepository)(nil)

func (r *InMemoryUserRepository) TouchUserActivity(ctx context.Context, userID int64, at time.Time, appVersion string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.users[userID] == nil {
		return nil
	}
	if r.lastActive == nil {
		r.lastActive = make(map[int64]time.Time)
	}
	if at.After(r.lastActive[userID]) {
		r.lastActive[userID] = at
		if r.lastActiveVersions == nil {
			r.lastActiveVersions = make(map[int64]string)
		}
		r.lastActiveVersions[userID] = appVersion
	}
	if r.activityDays == nil {
		r.activityDays = make(map[int64]map[string]struct{})
	}
	if r.activityDays[userID] == nil {
		r.activityDays[userID] = make(map[string]struct{})
	}
	first, day := userActivityDateWindow(at)
	r.activityDays[userID][day] = struct{}{}
	for recorded := range r.activityDays[userID] {
		if recorded < first {
			delete(r.activityDays[userID], recorded)
		}
	}
	return nil
}

func (r *InMemoryUserRepository) GetUserActivityStats(ctx context.Context, userIDs []int64, now time.Time) (map[int64]models.UserActivityStats, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	first, today := userActivityDateWindow(now)
	result := make(map[int64]models.UserActivityStats, len(userIDs))
	for _, id := range userIDs {
		if r.users[id] == nil {
			continue
		}
		stats := models.UserActivityStats{}
		if at, ok := r.lastActive[id]; ok && !at.IsZero() {
			stats.LastActiveAt = &at
			stats.LastActiveAppVersion = r.lastActiveVersions[id]
		}
		for day := range r.activityDays[id] {
			if day >= first && day <= today {
				stats.ActiveDays7D++
			}
		}
		result[id] = stats
	}
	return result, nil
}

func (r *InMemoryUserRepository) GetUserLastActiveTimes(ctx context.Context, userIDs []int64) (map[int64]time.Time, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[int64]time.Time, len(userIDs))
	for _, id := range userIDs {
		if at, ok := r.lastActive[id]; ok && r.users[id] != nil {
			result[id] = at
		}
	}
	return result, nil
}
