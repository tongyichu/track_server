package repository

import (
	"context"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
)

func (r *MongoUserRepository) UpdateLocation(ctx context.Context, userID int64, location models.UserLocation) error {
	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": userID}, bson.M{"$set": bson.M{
		"province_code": location.ProvinceCode,
		"province_name": location.ProvinceName,
		"city_code":     location.CityCode,
		"city_name":     location.CityName,
		"updated_at":    time.Now(),
	}})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}
