package repository

import (
	"context"
	"time"
)

// UserActivityRepository stores authenticated App activity independently of profile updates.
type UserActivityRepository interface {
	TouchUserActivity(ctx context.Context, userID int64, at time.Time) error
	GetUserLastActiveTimes(ctx context.Context, userIDs []int64) (map[int64]time.Time, error)
}
