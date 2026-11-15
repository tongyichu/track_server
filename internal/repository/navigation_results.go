package repository

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const activeNavigationSQL = `n.result_deleted=0 AND (
 n.result_session_id IS NULL OR
 NOT EXISTS (SELECT 1 FROM track_records r WHERE r.user_id=n.navigator_user_id AND r.session_id=n.result_session_id) OR
 EXISTS (SELECT 1 FROM track_records r WHERE r.user_id=n.navigator_user_id AND r.session_id=n.result_session_id AND r.status IN (1,2) AND r.deleted_at IS NULL)
)`

func ensureMySQLNavigationResultColumns(ctx context.Context, db *sql.DB) error {
	for _, column := range []struct{ name, definition string }{
		{"result_session_id", "VARCHAR(64) NULL DEFAULT NULL"},
		{"result_deleted", "BOOLEAN NOT NULL DEFAULT FALSE"},
	} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='track_navigations' AND COLUMN_NAME=?`, column.name).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			if _, err := db.ExecContext(ctx, "ALTER TABLE track_navigations ADD COLUMN "+column.name+" "+column.definition); err != nil {
				return err
			}
		}
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='track_navigations' AND INDEX_NAME='uniq_nav_result'`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		_, err := db.ExecContext(ctx, `ALTER TABLE track_navigations ADD UNIQUE KEY uniq_nav_result (navigator_user_id,result_session_id)`)
		return err
	}
	return nil
}

func (r *MySQLNavigationRepository) AddNavigationWithResult(ctx context.Context, user int64, track, session string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO track_navigations (track_id,navigator_user_id,result_session_id,created_at) VALUES (?,?,?,?) ON DUPLICATE KEY UPDATE id=id`, track, user, session, time.Now())
	return err
}

func (r *MySQLNavigationRepository) DeleteNavigationResult(ctx context.Context, user int64, session string) error {
	// Keep a tombstone even if deletion races ahead of the completion report.
	_, err := r.db.ExecContext(ctx, `INSERT INTO track_navigations (track_id,navigator_user_id,result_session_id,result_deleted,created_at) VALUES ('',?,?,1,?) ON DUPLICATE KEY UPDATE result_deleted=1`, user, session, time.Now())
	return err
}

func mongoActiveNavigations(match bson.M) mongo.Pipeline {
	match["result_deleted"] = bson.M{"$ne": true}
	return mongo.Pipeline{
		bson.D{{Key: "$match", Value: match}},
		bson.D{{Key: "$lookup", Value: bson.M{
			"from": "tracks", "let": bson.M{"user": "$navigator_user_id", "session": bson.M{"$ifNull": bson.A{"$result_session_id", ""}}},
			"pipeline": bson.A{bson.M{"$match": bson.M{"$expr": bson.M{"$and": bson.A{
				bson.M{"$ne": bson.A{"$$session", ""}}, bson.M{"$eq": bson.A{"$user_id", "$$user"}}, bson.M{"$eq": bson.A{"$session_id", "$$session"}},
			}}}}, bson.M{"$project": bson.M{"status": 1, "deleted_at": 1}}}, "as": "_result_tracks",
		}}},
		bson.D{{Key: "$match", Value: bson.M{"$or": bson.A{
			bson.M{"_result_tracks.0": bson.M{"$exists": false}},
			bson.M{"_result_tracks": bson.M{"$elemMatch": bson.M{"status": bson.M{"$in": bson.A{models.TrackStatusNormal, models.TrackStatusPrivate}}, "$or": bson.A{bson.M{"deleted_at": bson.M{"$exists": false}}, bson.M{"deleted_at": time.Time{}}, bson.M{"deleted_at": nil}}}}},
		}}}},
	}
}

func (r *MongoNavigationRepository) ensureResultIndex(ctx context.Context) error {
	_, err := r.collection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "navigator_user_id", Value: 1}, {Key: "result_session_id", Value: 1}}, Options: options.Index().SetName("uniq_nav_result").SetUnique(true).SetPartialFilterExpression(bson.M{"result_session_id": bson.M{"$type": "string"}})})
	if err != nil {
		return err
	}
	_, err = r.collection.Database().Collection("tracks").Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "session_id", Value: 1}}, Options: options.Index().SetName("idx_track_nav_result_session")})
	return err
}

func (r *MongoNavigationRepository) AddNavigationWithResult(ctx context.Context, user int64, track, session string) error {
	if err := r.ensureResultIndex(ctx); err != nil {
		return err
	}
	_, err := r.collection.UpdateOne(ctx, bson.M{"navigator_user_id": user, "result_session_id": session}, bson.M{"$setOnInsert": bson.M{"_id": time.Now().UnixNano(), "track_id": track, "created_at": time.Now()}}, options.Update().SetUpsert(true))
	return err
}

func (r *MongoNavigationRepository) DeleteNavigationResult(ctx context.Context, user int64, session string) error {
	if err := r.ensureResultIndex(ctx); err != nil {
		return err
	}
	_, err := r.collection.UpdateOne(ctx, bson.M{"navigator_user_id": user, "result_session_id": session}, bson.M{"$set": bson.M{"result_deleted": true}, "$setOnInsert": bson.M{"_id": time.Now().UnixNano(), "track_id": "", "created_at": time.Now()}}, options.Update().SetUpsert(true))
	return err
}

func navigationResultKey(user int64, session string) string {
	return strconv.FormatInt(user, 10) + ":" + session
}

func (r *InMemoryNavigationRepository) SetResultTracks(tracks TrackRepository) {
	r.resultTracks, _ = tracks.(*InMemoryTrackRepository)
}

func (r *InMemoryNavigationRepository) AddNavigationWithResult(_ context.Context, user int64, track, session string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byResult == nil {
		r.byResult = make(map[string]*models.TrackNavigation)
	}
	key := navigationResultKey(user, session)
	if r.byResult[key] != nil {
		return nil
	}
	row := &models.TrackNavigation{ID: r.nextID, TrackID: track, NavigatorUserID: user, ResultSessionID: session, CreatedAt: time.Now()}
	r.nextID++
	r.byResult[key] = row
	r.byTrack[track] = append(r.byTrack[track], row)
	return nil
}

func (r *InMemoryNavigationRepository) DeleteNavigationResult(_ context.Context, user int64, session string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byResult == nil {
		r.byResult = make(map[string]*models.TrackNavigation)
	}
	key := navigationResultKey(user, session)
	if r.byResult[key] == nil {
		r.byResult[key] = &models.TrackNavigation{NavigatorUserID: user, ResultSessionID: session}
	}
	r.byResult[key].ResultDeleted = true
	return nil
}

func (r *InMemoryNavigationRepository) activeResultSessions() map[string]bool {
	states := make(map[string]bool)
	if r.resultTracks == nil {
		return states
	}
	r.resultTracks.mu.RLock()
	defer r.resultTracks.mu.RUnlock()
	for _, track := range r.resultTracks.tracks {
		if track.SessionID == "" {
			continue
		}
		key := navigationResultKey(track.UserID, track.SessionID)
		states[key] = states[key] || (track.Status == models.TrackStatusNormal || track.Status == models.TrackStatusPrivate) && track.DeletedAt.IsZero()
	}
	for key := range r.resultTracks.deletedResultSessions {
		states[key] = false
	}
	return states
}

func resultVisible(row *models.TrackNavigation, states map[string]bool) bool {
	if row == nil || row.ResultDeleted {
		return false
	}
	if row.ResultSessionID == "" {
		return true
	}
	visible, uploaded := states[navigationResultKey(row.NavigatorUserID, row.ResultSessionID)]
	return !uploaded || visible
}

var _ NavigationResultRepository = (*MySQLNavigationRepository)(nil)
var _ NavigationResultRepository = (*MongoNavigationRepository)(nil)
var _ NavigationResultRepository = (*InMemoryNavigationRepository)(nil)
