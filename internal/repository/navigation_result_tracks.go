package repository

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func navigationResultTrackVisible(track *models.Track, viewer int64) bool {
	return track != nil && !track.IsRunning && track.DeletedAt.IsZero() && strings.TrimSpace(track.RawTrackURL) != "" &&
		(track.Status == models.TrackStatusNormal || track.Status == models.TrackStatusPrivate && track.UserID == viewer)
}

func (r *MySQLNavigationRepository) ListResultTracks(ctx context.Context, trackID string, viewer int64, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `SELECT DISTINCT t.id,t.user_id,t.session_id,t.city_code,t.locate_addr,t.track_type,t.source_tag,t.coordinate_system,t.title,t.route_description,t.description_source,t.description_rule_version,t.shape_label,t.start_time,t.end_time,
 t.distance,t.duration,t.calories_burned,t.elevation_gain,t.step_count,t.raw_track_url,t.track_screenshot_url,t.track_no_map_bg_screenshot_url,t.preview_images,t.is_running,t.status,t.avg_speed_kmh,t.created_at,t.updated_at,t.deleted_at,t.recording_timeline
 FROM track_navigations n JOIN track_records t ON t.user_id=n.navigator_user_id AND t.session_id=n.result_session_id
 WHERE n.track_id=? AND n.result_deleted=0 AND n.result_session_id IS NOT NULL AND n.result_session_id<>''
 AND t.is_running=0 AND t.deleted_at IS NULL AND t.raw_track_url IS NOT NULL AND TRIM(t.raw_track_url)<>''
 AND (t.status=1 OR (t.status=2 AND t.user_id=?))`
	args := []any{trackID, viewer}
	if cursor != nil {
		query += ` AND (t.start_time < ? OR (t.start_time = ? AND t.id < ?))`
		args = append(args, cursor.StartTime, cursor.StartTime, cursor.ID)
	}
	query += ` ORDER BY t.start_time DESC,t.id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tracks := make([]*models.Track, 0, limit)
	for rows.Next() {
		track, err := scanMySQLTrack(rows)
		if err != nil {
			return nil, err
		}
		tracks = append(tracks, track)
	}
	return tracks, rows.Err()
}

func (r *MongoNavigationRepository) ListResultTracks(ctx context.Context, trackID string, viewer int64, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	visible := bson.M{"is_running": false, "raw_track_url": bson.M{"$type": "string", "$regex": `\S`}, "$and": bson.A{
		bson.M{"$or": bson.A{bson.M{"status": models.TrackStatusNormal}, bson.M{"status": models.TrackStatusPrivate, "user_id": viewer}}},
		bson.M{"$or": bson.A{bson.M{"deleted_at": nil}, bson.M{"deleted_at": time.Time{}}}},
	}}
	if cursor != nil {
		visible["$and"] = append(visible["$and"].(bson.A), bson.M{"$or": bson.A{
			bson.M{"start_time": bson.M{"$lt": cursor.StartTime}}, bson.M{"start_time": cursor.StartTime, "_id": bson.M{"$lt": cursor.ID}},
		}})
	}
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{"track_id": trackID, "result_deleted": bson.M{"$ne": true}, "result_session_id": bson.M{"$type": "string", "$ne": ""}}}},
		bson.D{{Key: "$lookup", Value: bson.M{"from": "tracks", "let": bson.M{"user": "$navigator_user_id", "session": "$result_session_id"}, "pipeline": bson.A{
			bson.M{"$match": bson.M{"$expr": bson.M{"$and": bson.A{bson.M{"$eq": bson.A{"$user_id", "$$user"}}, bson.M{"$eq": bson.A{"$session_id", "$$session"}}}}}},
			bson.M{"$match": visible},
		}, "as": "_result"}}},
		bson.D{{Key: "$unwind", Value: "$_result"}},
		bson.D{{Key: "$replaceRoot", Value: bson.M{"newRoot": "$_result"}}},
		bson.D{{Key: "$group", Value: bson.M{"_id": "$_id", "track": bson.M{"$first": "$$ROOT"}}}},
		bson.D{{Key: "$replaceRoot", Value: bson.M{"newRoot": "$track"}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "start_time", Value: -1}, {Key: "_id", Value: -1}}}},
		bson.D{{Key: "$limit", Value: int64(limit)}},
	}
	cur, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	tracks := make([]*models.Track, 0, limit)
	if err := cur.All(ctx, &tracks); err != nil {
		return nil, err
	}
	for _, track := range tracks {
		normalizeTrackDescriptionMetadata(track)
	}
	return tracks, nil
}

func (r *InMemoryNavigationRepository) ListResultTracks(_ context.Context, trackID string, viewer int64, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	tracks := make([]*models.Track, 0, limit)
	if r.resultTracks == nil {
		return tracks, nil
	}
	results := map[string]bool{}
	for _, row := range r.byTrack[trackID] {
		if row != nil && !row.ResultDeleted && row.ResultSessionID != "" {
			results[navigationResultKey(row.NavigatorUserID, row.ResultSessionID)] = true
		}
	}
	r.resultTracks.mu.RLock()
	defer r.resultTracks.mu.RUnlock()
	for _, track := range r.resultTracks.tracks {
		if track == nil {
			continue
		}
		key := navigationResultKey(track.UserID, track.SessionID)
		if !results[key] || r.resultTracks.deletedResultSessions[key] || !navigationResultTrackVisible(track, viewer) {
			continue
		}
		if cursor != nil && (track.StartTime.After(cursor.StartTime) || track.StartTime.Equal(cursor.StartTime) && track.ID >= cursor.ID) {
			continue
		}
		copy := *track
		copy.RecordingTimeline = track.RecordingTimeline.Clone()
		tracks = append(tracks, &copy)
	}
	sort.Slice(tracks, func(i, j int) bool {
		if tracks[i].StartTime.Equal(tracks[j].StartTime) {
			return tracks[i].ID > tracks[j].ID
		}
		return tracks[i].StartTime.After(tracks[j].StartTime)
	})
	if len(tracks) > limit {
		tracks = tracks[:limit]
	}
	return tracks, nil
}

var _ NavigationResultTrackRepository = (*MySQLNavigationRepository)(nil)
var _ NavigationResultTrackRepository = (*MongoNavigationRepository)(nil)
var _ NavigationResultTrackRepository = (*InMemoryNavigationRepository)(nil)
