package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var _ UserActivityRepository = (*MongoUserRepository)(nil)

func (r *MongoUserRepository) TouchUserActivity(ctx context.Context, userID int64, at time.Time) error {
	_, err := r.collection.UpdateOne(ctx, bson.M{"_id": userID}, bson.M{"$max": bson.M{"last_active_at": at}})
	return err
}

func (r *MongoUserRepository) GetUserLastActiveTimes(ctx context.Context, userIDs []int64) (map[int64]time.Time, error) {
	result := make(map[int64]time.Time, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}
	cursor, err := r.collection.Find(ctx, bson.M{"_id": bson.M{"$in": userIDs}}, options.Find().SetProjection(bson.M{"_id": 1, "last_active_at": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var row struct {
			ID int64     `bson:"_id"`
			At time.Time `bson:"last_active_at"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		if !row.At.IsZero() {
			result[row.ID] = row.At
		}
	}
	return result, cursor.Err()
}
