package repository

import (
	"context"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ActivityFootprintRepository interface {
	ReadFootprintTracks(context.Context, int64) ([]models.FootprintTrack, error)
}

func (r *InMemoryTrackRepository) ReadFootprintTracks(ctx context.Context, userID int64) ([]models.FootprintTrack, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]models.FootprintTrack, 0)
	for _, t := range r.tracks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if t == nil || t.UserID != userID || t.IsRunning || strings.TrimSpace(t.RawTrackURL) == "" ||
			(t.Status != models.TrackStatusNormal && t.Status != models.TrackStatusPrivate) {
			continue
		}
		items = append(items, models.FootprintTrack{ID: t.ID, CityCode: t.CityCode, TrackType: t.TrackType, Distance: t.Distance, StartTime: t.StartTime})
	}
	return items, nil
}

func (r *MySQLTrackRepository) ReadFootprintTracks(ctx context.Context, userID int64) ([]models.FootprintTrack, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,COALESCE(city_code,''),COALESCE(track_type,''),COALESCE(distance,0),start_time
		FROM track_records WHERE user_id=? AND status IN (?,?) AND is_running=0
		AND raw_track_url IS NOT NULL AND TRIM(raw_track_url)<>''`, userID, models.TrackStatusNormal, models.TrackStatusPrivate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]models.FootprintTrack, 0)
	for rows.Next() {
		var item models.FootprintTrack
		if err := rows.Scan(&item.ID, &item.CityCode, &item.TrackType, &item.Distance, &item.StartTime); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MongoTrackRepository) ReadFootprintTracks(ctx context.Context, userID int64) ([]models.FootprintTrack, error) {
	filter := bson.M{"user_id": userID, "is_running": false, "status": bson.M{"$in": bson.A{models.TrackStatusNormal, models.TrackStatusPrivate}}, "raw_track_url": bson.M{"$type": "string", "$regex": `\S`}}
	cur, err := r.collection.Find(ctx, filter, options.Find().SetProjection(bson.M{"_id": 1, "city_code": 1, "track_type": 1, "distance": 1, "start_time": 1}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	items := make([]models.FootprintTrack, 0)
	if err := cur.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}

var _ ActivityFootprintRepository = (*InMemoryTrackRepository)(nil)
var _ ActivityFootprintRepository = (*MySQLTrackRepository)(nil)
var _ ActivityFootprintRepository = (*MongoTrackRepository)(nil)
