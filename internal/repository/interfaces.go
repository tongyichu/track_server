package repository

import (
	"context"

	"errors"

	"github.com/tongyichu/track_server/internal/models"
)

// Common repository errors.
var (
	// ErrNotFound indicates entity is not found in repository.
	ErrNotFound = errors.New("not found")
)

// TrackRepository defines persistence operations for Track entities.
type TrackRepository interface {
	Create(ctx context.Context, t *models.Track) error
	Update(ctx context.Context, t *models.Track) error
	FindByID(ctx context.Context, id string) (*models.Track, error)
	FindRunningByUserID(ctx context.Context, userID int64) (*models.Track, error)
	ListRecommend(ctx context.Context, userID int64, limit int) ([]*models.Track, error)
	Search(ctx context.Context, keyword string, limit int) ([]*models.Track, error)
}

// TrackWaypointRepository defines persistence operations for track multimedia waypoints.
type TrackWaypointRepository interface {
	Create(ctx context.Context, waypoint *models.TrackWaypoint) error
	ListByTrackID(ctx context.Context, trackID string) ([]*models.TrackWaypoint, error)
}

// UserRepository defines persistence operations for User entities.
type UserRepository interface {
	CreateIfNotExists(ctx context.Context, u *models.User) (*models.User, error)
	FindByID(ctx context.Context, id int64) (*models.User, error)
	Update(ctx context.Context, u *models.User) error
}

// CollectRepository defines persistence operations for track collections.
type CollectRepository interface {
	IsCollected(ctx context.Context, userID int64, trackID string) (bool, error)
	AddCollect(ctx context.Context, userID int64, trackID string) error
	RemoveCollect(ctx context.Context, userID int64, trackID string) error
}

// LoginLogRepository defines persistence operations for login logs.
type LoginLogRepository interface {
	Create(ctx context.Context, log *models.LoginLog) error
	ListByUserID(ctx context.Context, userID int64, limit int) ([]*models.LoginLog, error)
}
