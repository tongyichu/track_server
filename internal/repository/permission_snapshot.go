package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"sync"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// PermissionSnapshotRepository replaces each account/installation snapshot atomically.
type PermissionSnapshotRepository interface {
	Save(context.Context, *models.PermissionSnapshot) error
	List(context.Context, int64) ([]models.PermissionSnapshot, error)
}

const permissionSnapshotDDL = `CREATE TABLE IF NOT EXISTS user_permission_snapshots (
 user_id BIGINT NOT NULL,
 installation_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 sequence BIGINT NOT NULL,
 received_at DATETIME(6) NOT NULL,
 data_json JSON NOT NULL,
 PRIMARY KEY(user_id,installation_id), KEY idx_permission_user_received(user_id,received_at)
)`

func NewPermissionSnapshotRepository(ctx context.Context, users UserRepository) (PermissionSnapshotRepository, error) {
	switch repo := users.(type) {
	case *MySQLUserRepository:
		if _, err := repo.db.ExecContext(ctx, permissionSnapshotDDL); err != nil {
			return nil, err
		}
		return &mysqlPermissionSnapshots{repo.db}, nil
	case *MongoUserRepository:
		collection := repo.collection.Database().Collection("user_permission_snapshots")
		_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "received_at", Value: -1}}})
		if err != nil {
			return nil, err
		}
		return &mongoPermissionSnapshots{collection}, nil
	case *InMemoryUserRepository:
		return &memoryPermissionSnapshots{items: map[string]models.PermissionSnapshot{}}, nil
	default:
		return nil, errors.New("permission snapshot repository unavailable")
	}
}

type mysqlPermissionSnapshots struct{ db *sql.DB }

func (r *mysqlPermissionSnapshots) Save(ctx context.Context, s *models.PermissionSnapshot) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO user_permission_snapshots(user_id,installation_id,sequence,received_at,data_json) VALUES(?,?,?,?,?) ON DUPLICATE KEY UPDATE data_json=IF(VALUES(sequence)>sequence,VALUES(data_json),data_json),received_at=IF(VALUES(sequence)>sequence,VALUES(received_at),received_at),sequence=GREATEST(sequence,VALUES(sequence))`, s.UserID, s.InstallationID, s.Sequence, s.ReceivedAt, raw)
	return err
}
func (r *mysqlPermissionSnapshots) List(ctx context.Context, userID int64) ([]models.PermissionSnapshot, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT data_json FROM user_permission_snapshots WHERE user_id=? ORDER BY received_at DESC LIMIT 100`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.PermissionSnapshot{}
	for rows.Next() {
		var raw []byte
		var s models.PermissionSnapshot
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

type mongoPermissionSnapshots struct{ collection *mongo.Collection }

func (r *mongoPermissionSnapshots) Save(ctx context.Context, s *models.PermissionSnapshot) error {
	id := strconv.FormatInt(s.UserID, 10) + ":" + s.InstallationID
	// A stale or simultaneous insert hits the same _id and must not overwrite a newer snapshot.
	_, err := r.collection.ReplaceOne(ctx, bson.M{"_id": id, "$or": bson.A{bson.M{"sequence": bson.M{"$lt": s.Sequence}}, bson.M{"sequence": bson.M{"$exists": false}}}}, s, options.Replace().SetUpsert(true))
	if mongo.IsDuplicateKeyError(err) {
		// Another insert may have won with an older sequence; retry without upsert.
		_, err = r.collection.ReplaceOne(ctx, bson.M{"_id": id, "sequence": bson.M{"$lt": s.Sequence}}, s)
	}
	return err
}
func (r *mongoPermissionSnapshots) List(ctx context.Context, userID int64) ([]models.PermissionSnapshot, error) {
	cur, err := r.collection.Find(ctx, bson.M{"user_id": userID}, options.Find().SetSort(bson.D{{Key: "received_at", Value: -1}}).SetLimit(100))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []models.PermissionSnapshot{}
	err = cur.All(ctx, &out)
	return out, err
}

type memoryPermissionSnapshots struct {
	mu    sync.RWMutex
	items map[string]models.PermissionSnapshot
}

func (r *memoryPermissionSnapshots) Save(_ context.Context, s *models.PermissionSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := strconv.FormatInt(s.UserID, 10) + ":" + s.InstallationID
	if old, ok := r.items[key]; ok && old.Sequence >= s.Sequence {
		return nil
	}
	copy := *s
	copy.Items = append([]models.PermissionState{}, s.Items...)
	r.items[key] = copy
	return nil
}
func (r *memoryPermissionSnapshots) List(_ context.Context, userID int64) ([]models.PermissionSnapshot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []models.PermissionSnapshot{}
	for _, s := range r.items {
		if s.UserID == userID {
			s.Items = append([]models.PermissionState{}, s.Items...)
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ReceivedAt.After(out[j].ReceivedAt) })
	if len(out) > 100 {
		out = out[:100]
	}
	return out, nil
}
