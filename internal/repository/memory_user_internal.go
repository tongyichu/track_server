package repository

import (
	"context"
	"time"
)

func (r *InMemoryUserRepository) UpdateInternal(_ context.Context, userID int64, isInternal bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	user, ok := r.users[userID]
	if !ok {
		return ErrNotFound
	}
	clone := *user
	clone.IsInternal = isInternal
	clone.UpdatedAt = time.Now()
	r.users[userID] = &clone
	return nil
}
