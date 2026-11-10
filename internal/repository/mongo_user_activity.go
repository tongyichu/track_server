package repository

import (
	"context"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var _ UserActivityRepository = (*MongoUserRepository)(nil)

func (r *MongoUserRepository) TouchUserActivity(ctx context.Context, userID int64, at time.Time) error {
	first, day := userActivityDateWindow(at)
	dates := bson.M{"$setUnion": bson.A{bson.M{"$ifNull": bson.A{"$activity_days", bson.A{}}}, bson.A{day}}}
	_, err := r.collection.UpdateOne(ctx, bson.M{"_id": userID}, mongo.Pipeline{
		{{Key: "$set", Value: bson.M{
			"last_active_at": bson.M{"$max": bson.A{"$last_active_at", at}},
			"activity_days": bson.M{"$filter": bson.M{
				"input": dates, "as": "day", "cond": bson.M{"$gte": bson.A{"$$day", first}},
			}},
		}}},
	})
	return err
}

func (r *MongoUserRepository) GetUserActivityStats(ctx context.Context, userIDs []int64, now time.Time) (map[int64]models.UserActivityStats, error) {
	result := make(map[int64]models.UserActivityStats, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}
	first, today := userActivityDateWindow(now)
	cursor, err := r.collection.Find(ctx, bson.M{"_id": bson.M{"$in": userIDs}}, options.Find().SetProjection(bson.M{"_id": 1, "last_active_at": 1, "activity_days": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var row struct {
			ID   int64      `bson:"_id"`
			At   *time.Time `bson:"last_active_at"`
			Days []string   `bson:"activity_days"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		stats := models.UserActivityStats{LastActiveAt: row.At}
		if stats.LastActiveAt != nil && stats.LastActiveAt.IsZero() {
			stats.LastActiveAt = nil
		}
		dates := make(map[string]struct{}, len(row.Days))
		for _, day := range row.Days {
			if day >= first && day <= today {
				dates[day] = struct{}{}
			}
		}
		stats.ActiveDays7D = len(dates)
		result[row.ID] = stats
	}
	return result, cursor.Err()
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
