package repository

import (
	"context"
	"database/sql"
	"sort"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (r *MySQLTrackPlaceRepository) ListPlaceCandidateRecords(ctx context.Context, cursorTrackID string, limit int) ([]*models.TrackPlaceRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `SELECT record_key,data_json,expires_at,lease_until,lease_owner FROM track_place_records WHERE record_key > ? AND record_key LIKE 'match:%' AND data_json LIKE '%"status":"candidate"%' ORDER BY record_key LIMIT ?`, "match:"+cursorTrackID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTrackPlaceRecords(rows)
}

func scanTrackPlaceRecords(rows *sql.Rows) ([]*models.TrackPlaceRecord, error) {
	items := make([]*models.TrackPlaceRecord, 0)
	for rows.Next() {
		var v models.TrackPlaceRecord
		var expires, lease sql.NullTime
		if err := rows.Scan(&v.Key, &v.Data, &expires, &lease, &v.LeaseOwner); err != nil {
			return nil, err
		}
		v.ExpiresAt = expires.Time
		v.LeaseUntil = lease.Time
		items = append(items, &v)
	}
	return items, rows.Err()
}

func (r *MongoTrackPlaceRepository) ListPlaceCandidateRecords(ctx context.Context, cursorTrackID string, limit int) ([]*models.TrackPlaceRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	filter := bson.M{
		"_id":  bson.M{"$gt": "match:" + cursorTrackID, "$regex": "^match:"},
		"data": primitive.Regex{Pattern: `"status":"candidate"`},
	}
	cur, err := r.records.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	items := make([]*models.TrackPlaceRecord, 0, limit)
	for cur.Next(ctx) {
		var v models.TrackPlaceRecord
		if err := cur.Decode(&v); err != nil {
			return nil, err
		}
		items = append(items, &v)
	}
	return items, cur.Err()
}

func (r *MemoryTrackPlaceRepository) ListPlaceCandidateRecords(_ context.Context, cursorTrackID string, limit int) ([]*models.TrackPlaceRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	keys := make([]string, 0, len(r.records))
	cursor := "match:" + cursorTrackID
	for key, record := range r.records {
		if strings.HasPrefix(key, "match:") && key > cursor && strings.Contains(record.Data, `"status":"candidate"`) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) > limit {
		keys = keys[:limit]
	}
	items := make([]*models.TrackPlaceRecord, 0, len(keys))
	for _, key := range keys {
		v := *r.records[key]
		items = append(items, &v)
	}
	return items, nil
}
