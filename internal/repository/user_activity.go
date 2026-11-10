package repository

import (
	"context"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

// UserActivityRepository stores authenticated App activity independently of profile updates.
type UserActivityRepository interface {
	TouchUserActivity(ctx context.Context, userID int64, at time.Time) error
	GetUserLastActiveTimes(ctx context.Context, userIDs []int64) (map[int64]time.Time, error)
	GetUserActivityStats(ctx context.Context, userIDs []int64, now time.Time) (map[int64]models.UserActivityStats, error)
}

var userActivityLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

func userActivityDateWindow(now time.Time) (first, today string) {
	local := now.In(userActivityLocation)
	return local.AddDate(0, 0, -6).Format(time.DateOnly), local.Format(time.DateOnly)
}
