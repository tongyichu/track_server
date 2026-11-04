package repository

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (r *InMemoryTrackRepository) ListShaped(_ context.Context, cityCode string, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]*models.Track, 0)
	for _, track := range r.tracks {
		if track == nil || track.Status != models.TrackStatusNormal || track.IsRunning || track.RawTrackURL == "" || strings.TrimSpace(track.ShapeLabel) == "" {
			continue
		}
		if cityCode != "" && !sameRegionCode(track.CityCode, cityCode) {
			continue
		}
		if cursor != nil && (track.StartTime.After(cursor.StartTime) || (track.StartTime.Equal(cursor.StartTime) && track.ID >= cursor.ID)) {
			continue
		}
		items = append(items, track)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].StartTime.Equal(items[j].StartTime) {
			return items[i].ID > items[j].ID
		}
		return items[i].StartTime.After(items[j].StartTime)
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *MySQLTrackRepository) ListShaped(ctx context.Context, cityCode string, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `SELECT id FROM track_records WHERE status=? AND is_running=0 AND raw_track_url IS NOT NULL AND raw_track_url <> '' AND TRIM(COALESCE(shape_label,'')) <> ''`
	args := []any{models.TrackStatusNormal}
	if cityCode != "" {
		codes := regionCodeCandidates([]string{cityCode})
		query += ` AND city_code IN (` + strings.TrimRight(strings.Repeat("?,", len(codes)), ",") + `)`
		for _, code := range codes {
			args = append(args, code)
		}
	}
	if cursor != nil {
		query += ` AND (start_time < ? OR (start_time = ? AND id < ?))`
		args = append(args, cursor.StartTime, cursor.StartTime, cursor.ID)
	}
	query += ` ORDER BY start_time DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	byID, err := r.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	items := make([]*models.Track, 0, len(ids))
	for _, id := range ids {
		if track := byID[id]; track != nil {
			items = append(items, track)
		}
	}
	return items, nil
}

func (r *MongoTrackRepository) ListShaped(ctx context.Context, cityCode string, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	filter := bson.M{
		"status":        models.TrackStatusNormal,
		"is_running":    false,
		"raw_track_url": bson.M{"$exists": true, "$nin": bson.A{"", nil}},
		"shape_label":   bson.M{"$type": "string", "$regex": `\S`},
	}
	if cityCode != "" {
		filter["city_code"] = bson.M{"$in": regionCodeCandidates([]string{cityCode})}
	}
	if cursor != nil {
		filter["$or"] = bson.A{
			bson.M{"start_time": bson.M{"$lt": cursor.StartTime}},
			bson.M{"start_time": cursor.StartTime, "_id": bson.M{"$lt": cursor.ID}},
		}
	}
	return r.listTracks(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "start_time", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(int64(limit)))
}

func (r *InMemoryTrackRepository) ListActivityStartTimes(_ context.Context, userID int64, from, until time.Time) ([]time.Time, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	starts := make([]time.Time, 0)
	for _, track := range r.tracks {
		if track != nil && track.UserID == userID && (track.Status == models.TrackStatusNormal || track.Status == models.TrackStatusPrivate) && !track.IsRunning &&
			track.RawTrackURL != "" && !track.StartTime.Before(from) && track.StartTime.Before(until) {
			starts = append(starts, track.StartTime)
		}
	}
	return starts, nil
}

func (r *MySQLTrackRepository) ListActivityStartTimes(ctx context.Context, userID int64, from, until time.Time) ([]time.Time, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT start_time FROM track_records WHERE user_id=? AND status IN (?,?) AND is_running=0 AND raw_track_url IS NOT NULL AND raw_track_url <> '' AND start_time>=? AND start_time<?`,
		userID, models.TrackStatusNormal, models.TrackStatusPrivate, from, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	starts := make([]time.Time, 0)
	for rows.Next() {
		var start time.Time
		if err := rows.Scan(&start); err != nil {
			return nil, err
		}
		starts = append(starts, start)
	}
	return starts, rows.Err()
}

func (r *MongoTrackRepository) ListActivityStartTimes(ctx context.Context, userID int64, from, until time.Time) ([]time.Time, error) {
	filter := bson.M{
		"user_id":       userID,
		"status":        bson.M{"$in": bson.A{models.TrackStatusNormal, models.TrackStatusPrivate}},
		"is_running":    false,
		"raw_track_url": bson.M{"$exists": true, "$nin": bson.A{"", nil}},
		"start_time":    bson.M{"$gte": from, "$lt": until},
	}
	cur, err := r.collection.Find(ctx, filter, options.Find().SetProjection(bson.M{"start_time": 1}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	starts := make([]time.Time, 0)
	for cur.Next(ctx) {
		var item struct {
			StartTime time.Time `bson:"start_time"`
		}
		if err := cur.Decode(&item); err != nil {
			return nil, err
		}
		starts = append(starts, item.StartTime)
	}
	return starts, cur.Err()
}
