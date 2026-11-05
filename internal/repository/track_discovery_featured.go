package repository

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// Featured discovery joins approval state before applying the public page cursor.
// It does not load submission images or issue a lookup for each candidate track.
func ListFeaturedDiscoveryTracks(ctx context.Context, tracks TrackRepository, submissions TrackSubmissionRepository, city, trackType string, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	if submissions == nil {
		return []*models.Track{}, nil
	}
	ids := make([]string, 0, limit)
	switch repo := tracks.(type) {
	case *MySQLTrackRepository:
		query := `SELECT t.id FROM track_records t WHERE t.status=? AND t.is_running=0 AND COALESCE(t.raw_track_url,'')<>''
		 AND EXISTS(SELECT 1 FROM track_submissions s WHERE s.track_id=t.id AND s.status=?)`
		args := []any{models.TrackStatusNormal, models.TrackSubmissionStatusApproved}
		codes := regionCodeCandidates([]string{city})
		query += ` AND t.city_code IN (` + strings.TrimRight(strings.Repeat("?,", len(codes)), ",") + `)`
		for _, code := range codes {
			args = append(args, code)
		}
		if trackType != "" {
			query += ` AND t.track_type=?`
			args = append(args, trackType)
		}
		if cursor != nil {
			query += ` AND (t.start_time<? OR (t.start_time=? AND t.id<?))`
			args = append(args, cursor.StartTime, cursor.StartTime, cursor.ID)
		}
		query += ` ORDER BY t.start_time DESC,t.id DESC LIMIT ?`
		args = append(args, limit)
		rows, err := repo.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
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
	case *MongoTrackRepository:
		subs, ok := submissions.(*MongoTrackSubmissionRepository)
		if !ok {
			return nil, errors.New("incompatible discovery repositories")
		}
		filter := bson.M{"status": models.TrackStatusNormal, "is_running": false,
			"raw_track_url": bson.M{"$exists": true, "$nin": bson.A{nil, ""}},
			"city_code":     bson.M{"$in": regionCodeCandidates([]string{city})}}
		if trackType != "" {
			filter["track_type"] = trackType
		}
		if cursor != nil {
			filter["$or"] = bson.A{bson.M{"start_time": bson.M{"$lt": cursor.StartTime}},
				bson.M{"start_time": cursor.StartTime, "_id": bson.M{"$lt": cursor.ID}}}
		}
		pipeline := mongo.Pipeline{
			{{Key: "$match", Value: filter}},
			{{Key: "$sort", Value: bson.D{{Key: "start_time", Value: -1}, {Key: "_id", Value: -1}}}},
			{{Key: "$lookup", Value: bson.M{"from": subs.collection.Name(), "localField": "_id", "foreignField": "track_id", "as": "discovery_submission"}}},
			{{Key: "$match", Value: bson.M{"discovery_submission": bson.M{"$elemMatch": bson.M{"status": models.TrackSubmissionStatusApproved}}}}},
			{{Key: "$limit", Value: int64(limit)}},
			{{Key: "$project", Value: bson.M{"_id": 1}}},
		}
		rows, err := repo.collection.Aggregate(ctx, pipeline)
		if err != nil {
			return nil, err
		}
		defer rows.Close(ctx)
		for rows.Next(ctx) {
			var row struct {
				ID string `bson:"_id"`
			}
			if err := rows.Decode(&row); err != nil {
				return nil, err
			}
			ids = append(ids, row.ID)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	case *InMemoryTrackRepository:
		subs, ok := submissions.(*InMemoryTrackSubmissionRepository)
		if !ok {
			return nil, errors.New("incompatible discovery repositories")
		}
		approved := make(map[string]bool)
		subs.mu.RLock()
		for _, sub := range subs.byID {
			if sub != nil && sub.Status == models.TrackSubmissionStatusApproved {
				approved[sub.TrackID] = true
			}
		}
		subs.mu.RUnlock()
		repo.mu.RLock()
		defer repo.mu.RUnlock()
		items := make([]*models.Track, 0)
		for _, track := range repo.tracks {
			if track == nil || !approved[track.ID] || track.Status != models.TrackStatusNormal || track.IsRunning ||
				track.RawTrackURL == "" || !sameRegionCode(track.CityCode, city) || trackType != "" && track.TrackType != trackType {
				continue
			}
			if cursor != nil && (track.StartTime.After(cursor.StartTime) || track.StartTime.Equal(cursor.StartTime) && track.ID >= cursor.ID) {
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
		if len(items) > limit {
			items = items[:limit]
		}
		return items, nil
	default:
		return nil, errors.New("discovery repository unavailable")
	}
	byID, err := tracks.FindByIDs(ctx, ids)
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
