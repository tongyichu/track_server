package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type CompanionTrackReference struct {
	TrackID   string
	SessionID string
	UserID    int64
}

// A session ID alone cannot distinguish a solo recording from a companion result.
type CompanionTrackOriginRepository interface {
	FindCompanionTrackIDs(context.Context, []CompanionTrackReference) (map[string]bool, error)
}

func companionOriginReferences(refs []CompanionTrackReference) []CompanionTrackReference {
	result := make([]CompanionTrackReference, 0, len(refs))
	seen := map[string]bool{}
	for _, ref := range refs {
		key := companionChunkKey(ref.SessionID, ref.UserID, 0)
		if ref.SessionID != "" && ref.UserID > 0 && !seen[key] {
			seen[key] = true
			result = append(result, ref)
		}
	}
	return result
}

func matchCompanionTrackOrigins(refs []CompanionTrackReference, rows []models.CompanionTrackChunk) (map[string]bool, error) {
	expected := make(map[string]map[string]bool, len(refs))
	for _, ref := range refs {
		key := companionChunkKey(ref.SessionID, ref.UserID, 0)
		if expected[key] == nil {
			expected[key] = map[string]bool{}
		}
		expected[key][ref.TrackID] = true
	}
	result := map[string]bool{}
	for _, row := range rows {
		var manifest struct {
			TrackID string `json:"track_id"`
		}
		if err := json.Unmarshal([]byte(row.Data), &manifest); err != nil {
			return nil, errors.New("invalid companion completion manifest")
		}
		if manifest.TrackID != "" && expected[companionChunkKey(row.SessionID, row.UserID, 0)][manifest.TrackID] {
			result[manifest.TrackID] = true
		}
	}
	return result, nil
}

func (r *InMemoryCompanionRepository) FindCompanionTrackIDs(_ context.Context, refs []CompanionTrackReference) (map[string]bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rows := []models.CompanionTrackChunk{}
	for _, ref := range companionOriginReferences(refs) {
		if row, ok := r.trackChunks[companionChunkKey(ref.SessionID, ref.UserID, 0)]; ok {
			rows = append(rows, row)
		}
	}
	return matchCompanionTrackOrigins(refs, rows)
}

func (r *MySQLCompanionRepository) FindCompanionTrackIDs(ctx context.Context, refs []CompanionTrackReference) (map[string]bool, error) {
	keys := companionOriginReferences(refs)
	rows := []models.CompanionTrackChunk{}
	for start := 0; start < len(keys); start += 200 {
		end := min(start+200, len(keys))
		clauses := make([]string, 0, end-start)
		args := make([]any, 0, (end-start)*2)
		for _, ref := range keys[start:end] {
			clauses = append(clauses, "(session_id=? AND user_id=?)")
			args = append(args, ref.SessionID, ref.UserID)
		}
		found, err := r.db.QueryContext(ctx, `SELECT session_id,user_id,chunk_index,point_count,digest,data FROM companion_track_chunks WHERE chunk_index=0 AND (`+strings.Join(clauses, " OR ")+`)`, args...)
		if err != nil {
			return nil, err
		}
		batch, err := scanCompanionChunks(found)
		if err != nil {
			return nil, err
		}
		rows = append(rows, batch...)
	}
	return matchCompanionTrackOrigins(refs, rows)
}

func (r *MongoCompanionRepository) FindCompanionTrackIDs(ctx context.Context, refs []CompanionTrackReference) (map[string]bool, error) {
	keys := companionOriginReferences(refs)
	rows := []models.CompanionTrackChunk{}
	for start := 0; start < len(keys); start += 200 {
		if r.sessionCollection == nil {
			return nil, errors.New("companion track storage unavailable")
		}
		end := min(start+200, len(keys))
		ids := make([]string, 0, end-start)
		for _, ref := range keys[start:end] {
			ids = append(ids, companionChunkKey(ref.SessionID, ref.UserID, 0))
		}
		cur, err := r.trackChunkCollection().Find(ctx, bson.M{"_id": bson.M{"$in": ids}}, options.Find().SetProjection(bson.M{"session_id": 1, "user_id": 1, "data": 1}))
		if err != nil {
			return nil, err
		}
		var batch []models.CompanionTrackChunk
		err = cur.All(ctx, &batch)
		cur.Close(ctx)
		if err != nil {
			return nil, err
		}
		rows = append(rows, batch...)
	}
	return matchCompanionTrackOrigins(refs, rows)
}
