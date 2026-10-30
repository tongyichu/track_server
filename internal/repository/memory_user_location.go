package repository

import (
	"context"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

func (r *InMemoryUserRepository) UpdateLocation(_ context.Context, userID int64, location models.UserLocation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	user, ok := r.users[userID]
	if !ok {
		return ErrNotFound
	}
	clone := *user
	clone.ProvinceCode = location.ProvinceCode
	clone.ProvinceName = location.ProvinceName
	clone.CityCode = location.CityCode
	clone.CityName = location.CityName
	clone.UpdatedAt = time.Now()
	r.users[userID] = &clone
	return nil
}
