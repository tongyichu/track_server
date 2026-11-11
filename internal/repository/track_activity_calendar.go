package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ActivityRecord struct {
	RecordingTimeline *models.RecordingTimeline `bson:"recording_timeline"`
	StartTime         time.Time                 `bson:"start_time"`
	TrackType         string                    `bson:"track_type"`
	Distance          float64                   `bson:"distance"`
	Duration          int64                     `bson:"duration"`
	ElevationGain     int64                     `bson:"elevation_gain"`
	StepCount         *int64                    `bson:"step_count"`
}

type ActivityCalendarRepository interface {
	ListActivityRecords(context.Context, int64, time.Time, time.Time) ([]ActivityRecord, error)
}

func (r *InMemoryTrackRepository) ListActivityRecords(_ context.Context, userID int64, from, until time.Time) ([]ActivityRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]ActivityRecord, 0)
	for _, t := range r.tracks {
		if t == nil || t.UserID != userID || t.IsRunning ||
			(t.Status != models.TrackStatusNormal && t.Status != models.TrackStatusPrivate) ||
			strings.TrimSpace(t.RawTrackURL) == "" ||
			(t.StartTime.Before(from) && (t.RecordingTimeline == nil || t.EndTime.Before(from))) || !t.StartTime.Before(until) {
			continue
		}
		item := ActivityRecord{RecordingTimeline: t.RecordingTimeline.Clone(), StartTime: t.StartTime, TrackType: t.TrackType, Distance: t.Distance, Duration: int64(t.Duration), ElevationGain: int64(t.ElevationGain)}
		if t.StepCount != nil {
			n := *t.StepCount
			item.StepCount = &n
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *MySQLTrackRepository) ListActivityRecords(ctx context.Context, userID int64, from, until time.Time) ([]ActivityRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT start_time, COALESCE(track_type,''), COALESCE(distance,0), COALESCE(duration,0), COALESCE(elevation_gain,0), step_count, recording_timeline
        FROM track_records WHERE user_id=? AND status IN (?,?) AND is_running=0
        AND raw_track_url IS NOT NULL AND TRIM(raw_track_url)<>'' AND (start_time>=? OR (recording_timeline IS NOT NULL AND end_time>=?)) AND start_time<?`, userID, models.TrackStatusNormal, models.TrackStatusPrivate, from, from, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ActivityRecord, 0)
	for rows.Next() {
		var item ActivityRecord
		var timelineJSON sql.NullString
		if err := rows.Scan(&item.StartTime, &item.TrackType, &item.Distance, &item.Duration, &item.ElevationGain, &item.StepCount, &timelineJSON); err != nil {
			return nil, err
		}
		if err := decodeRecordingTimeline(timelineJSON, &item.RecordingTimeline); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MongoTrackRepository) ListActivityRecords(ctx context.Context, userID int64, from, until time.Time) ([]ActivityRecord, error) {
	filter := bson.M{"user_id": userID, "status": bson.M{"$in": bson.A{models.TrackStatusNormal, models.TrackStatusPrivate}}, "is_running": false,
		"raw_track_url": bson.M{"$type": "string", "$regex": `\S`}, "start_time": bson.M{"$lt": until}, "$or": bson.A{
			bson.M{"recording_timeline": bson.M{"$ne": nil}, "end_time": bson.M{"$gte": from}}, bson.M{"start_time": bson.M{"$gte": from}},
		}}
	projection := bson.M{"start_time": 1, "track_type": 1, "distance": 1, "duration": 1, "elevation_gain": 1, "step_count": 1, "recording_timeline": 1}
	cur, err := r.collection.Find(ctx, filter, options.Find().SetProjection(projection))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	items := make([]ActivityRecord, 0)
	if err := cur.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}

var _ ActivityCalendarRepository = (*InMemoryTrackRepository)(nil)
var _ ActivityCalendarRepository = (*MySQLTrackRepository)(nil)
var _ ActivityCalendarRepository = (*MongoTrackRepository)(nil)
