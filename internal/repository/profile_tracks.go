package repository

import (
	"context"
	"sort"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (r *InMemoryTrackRepository) ListPublicByUserID(_ context.Context, userID int64, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tracks := make([]*models.Track, 0)
	for _, t := range r.tracks {
		if t == nil || t.UserID != userID || t.Status != models.TrackStatusNormal || t.IsRunning || strings.TrimSpace(t.RawTrackURL) == "" {
			continue
		}
		if cursor != nil && (t.StartTime.After(cursor.StartTime) || (t.StartTime.Equal(cursor.StartTime) && t.ID >= cursor.ID)) {
			continue
		}
		tracks = append(tracks, t)
	}
	sort.Slice(tracks, func(i, j int) bool {
		if tracks[i].StartTime.Equal(tracks[j].StartTime) {
			return tracks[i].ID > tracks[j].ID
		}
		return tracks[i].StartTime.After(tracks[j].StartTime)
	})
	if limit > 0 && len(tracks) > limit {
		tracks = tracks[:limit]
	}
	return tracks, nil
}

func (r *InMemoryTrackRepository) CountPublicByUserID(ctx context.Context, userID int64) (int64, error) {
	tracks, err := r.ListPublicByUserID(ctx, userID, nil, 0)
	return int64(len(tracks)), err
}

func (r *MySQLTrackRepository) ListPublicByUserID(ctx context.Context, userID int64, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `SELECT id FROM track_records WHERE user_id=? AND status=? AND is_running=0 AND TRIM(COALESCE(raw_track_url, '')) <> ''`
	args := []interface{}{userID, models.TrackStatusNormal}
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
	tracks := make([]*models.Track, 0, len(ids))
	for _, id := range ids {
		if t := byID[id]; t != nil {
			tracks = append(tracks, t)
		}
	}
	return tracks, nil
}

func (r *MySQLTrackRepository) CountPublicByUserID(ctx context.Context, userID int64) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM track_records WHERE user_id=? AND status=? AND is_running=0 AND TRIM(COALESCE(raw_track_url, '')) <> ''`, userID, models.TrackStatusNormal).Scan(&count)
	return count, err
}

func publicProfileFilter(userID int64) bson.M {
	return bson.M{"user_id": userID, "status": models.TrackStatusNormal, "is_running": false, "raw_track_url": bson.M{"$type": "string", "$regex": `\S`}}
}

func (r *MongoTrackRepository) ListPublicByUserID(ctx context.Context, userID int64, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	filter := publicProfileFilter(userID)
	if cursor != nil {
		filter["$or"] = bson.A{bson.M{"start_time": bson.M{"$lt": cursor.StartTime}}, bson.M{"start_time": cursor.StartTime, "_id": bson.M{"$lt": cursor.ID}}}
	}
	return r.listTracks(ctx, filter, options.Find().SetSort(bson.D{{Key: "start_time", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(int64(limit)))
}

func (r *MongoTrackRepository) CountPublicByUserID(ctx context.Context, userID int64) (int64, error) {
	return r.collection.CountDocuments(ctx, publicProfileFilter(userID))
}
