package repository

import (
	"context"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

func (r *MySQLUserRepository) UpdateLocation(ctx context.Context, userID int64, location models.UserLocation) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE users SET province_code=?, province_name=?, city_code=?, city_name=?, updated_at=? WHERE id=?`,
		location.ProvinceCode, location.ProvinceName, location.CityCode, location.CityName, time.Now(), userID,
	)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		_, err = r.FindByID(ctx, userID)
		return err
	}
	return nil
}
