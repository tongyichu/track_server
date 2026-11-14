package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type CompanionTrackRepository interface {
	PutTrackChunk(context.Context, models.CompanionTrackChunk) error
	ListTrackChunks(context.Context, string, int64, int, int) ([]models.CompanionTrackChunk, error)
	ListTrackManifests(context.Context, string) ([]models.CompanionTrackChunk, error)
	TrackChunkCounts(context.Context, string, int64) (count, points, last int, err error)
	SetResultTotals(context.Context, string, float64, int64, int64) error
}

func (r *InMemoryCompanionRepository) SetResultTotals(_ context.Context, id string, distance float64, duration, count int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	session := r.sessions[id]
	if session == nil {
		return ErrNotFound
	}
	if session.Status != models.CompanionSessionStatusEnded {
		return ErrConflict
	}
	session.TotalDistance = distance
	session.TotalDuration = duration
	session.ActualParticipantCount = count
	session.UpdatedAt = time.Now()
	return nil
}

func (r *MySQLCompanionRepository) SetResultTotals(ctx context.Context, id string, distance float64, duration, count int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE companion_sessions SET total_distance=?,total_duration=?,actual_participant_count=?,updated_at=? WHERE session_id=? AND status='ended'`, distance, duration, count, time.Now(), id)
	return err
}

func (r *MongoCompanionRepository) SetResultTotals(ctx context.Context, id string, distance float64, duration, count int64) error {
	if r.sessionCollection == nil {
		return errors.New("companion track storage unavailable")
	}
	_, err := r.sessionCollection.UpdateOne(ctx, bson.M{"session_id": id, "status": "ended"}, bson.M{"$set": bson.M{"total_distance": distance, "total_duration": duration, "actual_participant_count": count, "updated_at": time.Now()}})
	return err
}

const companionTrackDDL = `CREATE TABLE IF NOT EXISTS companion_track_chunks (
 session_id VARCHAR(64) NOT NULL, user_id BIGINT UNSIGNED NOT NULL,
 chunk_index INT UNSIGNED NOT NULL, point_count INT UNSIGNED NOT NULL DEFAULT 0,
 digest CHAR(64) NOT NULL, data MEDIUMTEXT NOT NULL,
 PRIMARY KEY(session_id,user_id,chunk_index)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

func companionChunkKey(session string, user int64, index int) string {
	return fmt.Sprintf("%x:%d:%d", sha256.Sum256([]byte(session)), user, index)
}

func (r *InMemoryCompanionRepository) PutTrackChunk(_ context.Context, chunk models.CompanionTrackChunk) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.trackChunks == nil {
		r.trackChunks = make(map[string]models.CompanionTrackChunk)
	}
	key := companionChunkKey(chunk.SessionID, chunk.UserID, chunk.Index)
	if old, ok := r.trackChunks[key]; ok {
		if old.Digest != chunk.Digest {
			return ErrConflict
		}
		return nil
	}
	r.trackChunks[key] = chunk
	return nil
}

func (r *InMemoryCompanionRepository) ListTrackChunks(_ context.Context, session string, user int64, after, limit int) ([]models.CompanionTrackChunk, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := []models.CompanionTrackChunk{}
	for _, c := range r.trackChunks {
		if c.SessionID == session && c.UserID == user && c.Index > after {
			result = append(result, c)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Index < result[j].Index })
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (r *InMemoryCompanionRepository) ListTrackManifests(_ context.Context, session string) ([]models.CompanionTrackChunk, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := []models.CompanionTrackChunk{}
	for _, c := range r.trackChunks {
		if c.SessionID == session && c.Index == 0 {
			result = append(result, c)
		}
	}
	return result, nil
}

func (r *InMemoryCompanionRepository) TrackChunkCounts(_ context.Context, session string, user int64) (count, points, last int, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, c := range r.trackChunks {
		if c.SessionID == session && c.UserID == user && c.Index > 0 {
			count++
			points += c.PointCount
			if c.Index > last {
				last = c.Index
			}
		}
	}
	return
}

func (r *MySQLCompanionRepository) PutTrackChunk(ctx context.Context, c models.CompanionTrackChunk) error {
	_, err := r.db.ExecContext(ctx, `INSERT IGNORE INTO companion_track_chunks(session_id,user_id,chunk_index,point_count,digest,data) VALUES(?,?,?,?,?,?)`, c.SessionID, c.UserID, c.Index, c.PointCount, c.Digest, c.Data)
	if err != nil {
		return err
	}
	var digest string
	err = r.db.QueryRowContext(ctx, `SELECT digest FROM companion_track_chunks WHERE session_id=? AND user_id=? AND chunk_index=?`, c.SessionID, c.UserID, c.Index).Scan(&digest)
	if err != nil {
		return err
	}
	if digest != c.Digest {
		return ErrConflict
	}
	return nil
}

func scanCompanionChunks(rows *sql.Rows) ([]models.CompanionTrackChunk, error) {
	defer rows.Close()
	result := []models.CompanionTrackChunk{}
	for rows.Next() {
		var c models.CompanionTrackChunk
		if err := rows.Scan(&c.SessionID, &c.UserID, &c.Index, &c.PointCount, &c.Digest, &c.Data); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func (r *MySQLCompanionRepository) ListTrackChunks(ctx context.Context, session string, user int64, after, limit int) ([]models.CompanionTrackChunk, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT session_id,user_id,chunk_index,point_count,digest,data FROM companion_track_chunks WHERE session_id=? AND user_id=? AND chunk_index>? ORDER BY chunk_index LIMIT ?`, session, user, after, limit)
	if err != nil {
		return nil, err
	}
	return scanCompanionChunks(rows)
}

func (r *MySQLCompanionRepository) ListTrackManifests(ctx context.Context, session string) ([]models.CompanionTrackChunk, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT session_id,user_id,chunk_index,point_count,digest,data FROM companion_track_chunks WHERE session_id=? AND chunk_index=0`, session)
	if err != nil {
		return nil, err
	}
	return scanCompanionChunks(rows)
}

func (r *MySQLCompanionRepository) TrackChunkCounts(ctx context.Context, session string, user int64) (count, points, last int, err error) {
	err = r.db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(point_count),0),COALESCE(MAX(chunk_index),0) FROM companion_track_chunks WHERE session_id=? AND user_id=? AND chunk_index>0`, session, user).Scan(&count, &points, &last)
	return
}

func (r *MongoCompanionRepository) trackChunkCollection() *mongo.Collection {
	return r.sessionCollection.Database().Collection("companion_track_chunks")
}

func (r *MongoCompanionRepository) PutTrackChunk(ctx context.Context, c models.CompanionTrackChunk) error {
	if r.sessionCollection == nil {
		return errors.New("companion track storage unavailable")
	}
	doc := bson.M{"_id": companionChunkKey(c.SessionID, c.UserID, c.Index), "session_id": c.SessionID, "user_id": c.UserID, "index": c.Index, "point_count": c.PointCount, "digest": c.Digest, "data": c.Data}
	_, err := r.trackChunkCollection().InsertOne(ctx, doc)
	if err == nil {
		return nil
	}
	if !mongo.IsDuplicateKeyError(err) {
		return err
	}
	var existing models.CompanionTrackChunk
	if err = r.trackChunkCollection().FindOne(ctx, bson.M{"_id": doc["_id"]}).Decode(&existing); err != nil {
		return err
	}
	if existing.Digest != c.Digest {
		return ErrConflict
	}
	return nil
}

func (r *MongoCompanionRepository) ListTrackChunks(ctx context.Context, session string, user int64, after, limit int) ([]models.CompanionTrackChunk, error) {
	if r.sessionCollection == nil {
		return nil, errors.New("companion track storage unavailable")
	}
	cur, err := r.trackChunkCollection().Find(ctx, bson.M{"session_id": session, "user_id": user, "index": bson.M{"$gt": after}}, options.Find().SetSort(bson.D{{Key: "index", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	result := []models.CompanionTrackChunk{}
	err = cur.All(ctx, &result)
	return result, err
}

func (r *MongoCompanionRepository) ListTrackManifests(ctx context.Context, session string) ([]models.CompanionTrackChunk, error) {
	if r.sessionCollection == nil {
		return nil, errors.New("companion track storage unavailable")
	}
	cur, err := r.trackChunkCollection().Find(ctx, bson.M{"session_id": session, "index": 0})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	result := []models.CompanionTrackChunk{}
	err = cur.All(ctx, &result)
	return result, err
}

func (r *MongoCompanionRepository) TrackChunkCounts(ctx context.Context, session string, user int64) (count, points, last int, err error) {
	if r.sessionCollection == nil {
		return 0, 0, 0, errors.New("companion track storage unavailable")
	}
	cur, e := r.trackChunkCollection().Find(ctx, bson.M{"session_id": session, "user_id": user, "index": bson.M{"$gt": 0}}, options.Find().SetProjection(bson.M{"index": 1, "point_count": 1}))
	if e != nil {
		err = e
		return
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var c models.CompanionTrackChunk
		if e = cur.Decode(&c); e != nil {
			err = e
			return
		}
		count++
		points += c.PointCount
		if c.Index > last {
			last = c.Index
		}
	}
	err = cur.Err()
	return
}
