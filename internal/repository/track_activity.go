package repository

import (
	"context"
	"math"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type ActivityTypeStatisticsRepository interface {
	AggregateActivityTypes(context.Context, int64) ([]models.ActivityTypeTotal, error)
}

func (r *InMemoryTrackRepository) AggregateActivityTypes(_ context.Context, userID int64) ([]models.ActivityTypeTotal, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	groups := make(map[string]*models.ActivityTypeTotal)
	for _, track := range r.tracks {
		if track == nil || track.UserID != userID || track.IsRunning ||
			(track.Status != models.TrackStatusNormal && track.Status != models.TrackStatusPrivate) || strings.TrimSpace(track.RawTrackURL) == "" {
			continue
		}
		item := groups[track.TrackType]
		if item == nil {
			item = &models.ActivityTypeTotal{TrackType: track.TrackType}
			groups[track.TrackType] = item
		}
		item.TrackCount++
		item.DurationSeconds += int64(track.Duration)
		if track.Distance > 0 && !math.IsNaN(track.Distance) && !math.IsInf(track.Distance, 0) {
			item.DistanceMeters += track.Distance
		}
	}
	items := make([]models.ActivityTypeTotal, 0, len(groups))
	for _, item := range groups {
		items = append(items, *item)
	}
	return items, nil
}

func (r *MySQLTrackRepository) AggregateActivityTypes(ctx context.Context, userID int64) ([]models.ActivityTypeTotal, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT COALESCE(track_type,''), COUNT(*),
		COALESCE(SUM(GREATEST(COALESCE(duration,0),0)),0),
		COALESCE(SUM(GREATEST(COALESCE(distance,0),0)),0)
		FROM track_records WHERE user_id=? AND status IN (?,?) AND is_running=0
		AND raw_track_url IS NOT NULL AND TRIM(raw_track_url) <> '' GROUP BY track_type`,
		userID, models.TrackStatusNormal, models.TrackStatusPrivate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]models.ActivityTypeTotal, 0)
	for rows.Next() {
		var item models.ActivityTypeTotal
		if err := rows.Scan(&item.TrackType, &item.TrackCount, &item.DurationSeconds, &item.DistanceMeters); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MongoTrackRepository) AggregateActivityTypes(ctx context.Context, userID int64) ([]models.ActivityTypeTotal, error) {
	positive := func(field string) bson.M {
		return bson.M{"$cond": bson.A{bson.M{"$gt": bson.A{field, 0}}, field, 0}}
	}
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{
			"user_id": userID, "is_running": false,
			"status":        bson.M{"$in": bson.A{models.TrackStatusNormal, models.TrackStatusPrivate}},
			"raw_track_url": bson.M{"$type": "string", "$regex": `\S`},
		}}},
		bson.D{{Key: "$group", Value: bson.M{
			"_id":              bson.M{"$ifNull": bson.A{"$track_type", ""}},
			"track_count":      bson.M{"$sum": 1},
			"duration_seconds": bson.M{"$sum": positive("$duration")},
			"distance_meters":  bson.M{"$sum": positive("$distance")},
		}}},
	}
	cur, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	items := make([]models.ActivityTypeTotal, 0)
	if err := cur.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}

var _ ActivityTypeStatisticsRepository = (*InMemoryTrackRepository)(nil)
var _ ActivityTypeStatisticsRepository = (*MySQLTrackRepository)(nil)
var _ ActivityTypeStatisticsRepository = (*MongoTrackRepository)(nil)
