package repository

import (
	"context"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AchievementTrackReader streams only the fields needed by the shared XP rule.
// User lists use one query for the current page, without per-user track reads.
type AchievementTrackReader interface {
	ScanAchievementTracks(context.Context, []int64, func(*models.Track)) error
}

// AchievementTrackScanner supplies a streaming snapshot for admin level filters.
type AchievementTrackScanner interface {
	ScanAllAchievementTracks(context.Context, func(*models.Track)) error
}

func (r *InMemoryTrackRepository) ScanAchievementTracks(ctx context.Context, userIDs []int64, consume func(*models.Track)) error {
	if len(userIDs) == 0 {
		return nil
	}
	return r.scanAchievementTracks(ctx, userIDs, consume)
}

func (r *InMemoryTrackRepository) ScanAllAchievementTracks(ctx context.Context, consume func(*models.Track)) error {
	return r.scanAchievementTracks(ctx, nil, consume)
}

func (r *InMemoryTrackRepository) scanAchievementTracks(ctx context.Context, userIDs []int64, consume func(*models.Track)) error {
	selected := make(map[int64]bool, len(userIDs))
	for _, id := range userIDs {
		selected[id] = true
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, track := range r.tracks {
		if err := ctx.Err(); err != nil {
			return err
		}
		if track != nil && (userIDs == nil || selected[track.UserID]) && track.Status == models.TrackStatusNormal && !track.IsRunning {
			copy := *track
			consume(&copy)
		}
	}
	return nil
}

func (r *MySQLTrackRepository) ScanAchievementTracks(ctx context.Context, userIDs []int64, consume func(*models.Track)) error {
	if len(userIDs) == 0 {
		return nil
	}
	return r.scanAchievementTracks(ctx, userIDs, consume)
}

func (r *MySQLTrackRepository) ScanAllAchievementTracks(ctx context.Context, consume func(*models.Track)) error {
	return r.scanAchievementTracks(ctx, nil, consume)
}

func (r *MySQLTrackRepository) scanAchievementTracks(ctx context.Context, userIDs []int64, consume func(*models.Track)) error {
	args := make([]interface{}, 0, len(userIDs))
	for _, id := range userIDs {
		args = append(args, id)
	}
	query := `SELECT user_id,track_type,distance,duration,elevation_gain,
		COALESCE(raw_track_url,''),COALESCE(track_screenshot_url,''),COALESCE(track_no_map_bg_screenshot_url,''),COALESCE(session_id,'')
		FROM track_records WHERE status=1 AND is_running=0`
	if userIDs != nil {
		query += ` AND user_id IN (` + strings.TrimSuffix(strings.Repeat("?,", len(args)), ",") + `)`
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		track := models.Track{Status: models.TrackStatusNormal}
		if err := rows.Scan(&track.UserID, &track.TrackType, &track.Distance, &track.Duration, &track.ElevationGain,
			&track.RawTrackURL, &track.TrackScreenshotURL, &track.TrackNoMapBgScreenshotURL, &track.SessionID); err != nil {
			return err
		}
		consume(&track)
	}
	return rows.Err()
}

func (r *MongoTrackRepository) ScanAchievementTracks(ctx context.Context, userIDs []int64, consume func(*models.Track)) error {
	if len(userIDs) == 0 {
		return nil
	}
	return r.scanAchievementTracks(ctx, userIDs, consume)
}

func (r *MongoTrackRepository) ScanAllAchievementTracks(ctx context.Context, consume func(*models.Track)) error {
	return r.scanAchievementTracks(ctx, nil, consume)
}

func (r *MongoTrackRepository) scanAchievementTracks(ctx context.Context, userIDs []int64, consume func(*models.Track)) error {
	filter := bson.M{"status": models.TrackStatusNormal, "is_running": false}
	if userIDs != nil {
		filter["user_id"] = bson.M{"$in": userIDs}
	}
	projection := bson.M{"user_id": 1, "track_type": 1, "distance": 1, "duration": 1, "elevation_gain": 1,
		"raw_track_url": 1, "track_screenshot_url": 1, "track_no_map_bg_screenshot_url": 1, "session_id": 1}
	cur, err := r.collection.Find(ctx, filter, options.Find().SetProjection(projection))
	if err != nil {
		return err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		track := models.Track{Status: models.TrackStatusNormal}
		if err := cur.Decode(&track); err != nil {
			return err
		}
		track.Status = models.TrackStatusNormal
		consume(&track)
	}
	return cur.Err()
}

var _ AchievementTrackReader = (*InMemoryTrackRepository)(nil)
var _ AchievementTrackReader = (*MySQLTrackRepository)(nil)
var _ AchievementTrackReader = (*MongoTrackRepository)(nil)

var _ AchievementTrackScanner = (*InMemoryTrackRepository)(nil)
var _ AchievementTrackScanner = (*MySQLTrackRepository)(nil)
var _ AchievementTrackScanner = (*MongoTrackRepository)(nil)
