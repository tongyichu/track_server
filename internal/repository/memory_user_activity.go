package repository

import (
	"context"
	"time"
)

var _ UserActivityRepository = (*InMemoryUserRepository)(nil)

func (r *InMemoryUserRepository) TouchUserActivity(ctx context.Context, userID int64, at time.Time) error {
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
	}
	return nil
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
