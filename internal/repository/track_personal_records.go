package repository

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type PersonalRecordsRepository interface {
	FindPersonalRecordIDs(context.Context, int64) ([]string, error)
}

var personalRecordColumns = []struct{ metric, column string }{
	{"distance", "distance"}, {"duration", "duration"}, {"elevation", "elevation_gain"}, {"steps", "step_count"},
}

func (r *InMemoryTrackRepository) FindPersonalRecordIDs(_ context.Context, userID int64) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	winners := make(map[string]*models.Track)
	for _, t := range r.tracks {
		if t == nil || t.UserID != userID || t.IsRunning || strings.TrimSpace(t.RawTrackURL) == "" ||
			(t.Status != models.TrackStatusNormal && t.Status != models.TrackStatusPrivate) {
			continue
		}
		for _, m := range personalRecordColumns {
			value := models.PersonalRecordValue(t, m.metric)
			if value <= 0 {
				continue
			}
			key := t.TrackType + "\x00" + m.metric
			previous := winners[key]
			if previous == nil || value > models.PersonalRecordValue(previous, m.metric) ||
				(value == models.PersonalRecordValue(previous, m.metric) && (t.StartTime.After(previous.StartTime) ||
					(t.StartTime.Equal(previous.StartTime) && t.ID > previous.ID))) {
				winners[key] = t
			}
		}
	}
	ids := make(map[string]bool)
	for _, t := range winners {
		ids[t.ID] = true
	}
	return personalRecordIDs(ids), nil
}

func (r *MySQLTrackRepository) FindPersonalRecordIDs(ctx context.Context, userID int64) ([]string, error) {
	ids := make(map[string]bool)
	for _, m := range personalRecordColumns {
		// A grouped maximum followed by latest-date/ID tie-breaking works without MySQL window functions.
		query := fmt.Sprintf(`SELECT SUBSTRING(MAX(CONCAT(DATE_FORMAT(t.start_time,'%%Y%%m%%d%%H%%i%%s'),t.id)),15)
            FROM track_records t JOIN (
                SELECT BINARY COALESCE(track_type,'') AS type_code, MAX(%s) AS record_value
                FROM track_records WHERE user_id=? AND status IN (?,?) AND is_running=0
                AND raw_track_url IS NOT NULL AND TRIM(raw_track_url)<>'' AND %s>0
                GROUP BY BINARY COALESCE(track_type,'')
            ) best ON BINARY COALESCE(t.track_type,'')=best.type_code AND t.%s=best.record_value
            WHERE t.user_id=? AND t.status IN (?,?) AND t.is_running=0
            AND t.raw_track_url IS NOT NULL AND TRIM(t.raw_track_url)<>''
            GROUP BY BINARY COALESCE(t.track_type,'')`, m.column, m.column, m.column)
		rows, err := r.db.QueryContext(ctx, query, userID, models.TrackStatusNormal, models.TrackStatusPrivate, userID, models.TrackStatusNormal, models.TrackStatusPrivate)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			ids[id] = true
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return personalRecordIDs(ids), nil
}

func (r *MongoTrackRepository) FindPersonalRecordIDs(ctx context.Context, userID int64) ([]string, error) {
	ids := make(map[string]bool)
	for _, m := range personalRecordColumns {
		filter := bson.M{"user_id": userID, "status": bson.M{"$in": bson.A{models.TrackStatusNormal, models.TrackStatusPrivate}},
			"is_running": false, "raw_track_url": bson.M{"$type": "string", "$regex": `\S`}, m.column: bson.M{"$gt": 0}}
		pipeline := mongo.Pipeline{
			bson.D{{Key: "$match", Value: filter}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: m.column, Value: -1}, {Key: "start_time", Value: -1}, {Key: "_id", Value: -1}}}},
			bson.D{{Key: "$group", Value: bson.M{"_id": "$track_type", "track_id": bson.M{"$first": "$_id"}}}},
		}
		cur, err := r.collection.Aggregate(ctx, pipeline)
		if err != nil {
			return nil, err
		}
		var items []struct {
			ID string `bson:"track_id"`
		}
		err = cur.All(ctx, &items)
		cur.Close(ctx)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			ids[item.ID] = true
		}
	}
	return personalRecordIDs(ids), nil
}

func personalRecordIDs(ids map[string]bool) []string {
	result := make([]string, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

var _ PersonalRecordsRepository = (*InMemoryTrackRepository)(nil)
var _ PersonalRecordsRepository = (*MySQLTrackRepository)(nil)
var _ PersonalRecordsRepository = (*MongoTrackRepository)(nil)
