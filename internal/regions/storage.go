package regions

import (
	"context"
	"database/sql"
	"errors"
	"sync"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type mysqlStore struct{ db *sql.DB }

func NewMySQLStore(ctx context.Context, db *sql.DB) (Store, error) {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS region_catalog_state (
 id TINYINT UNSIGNED NOT NULL PRIMARY KEY,
 version BIGINT NOT NULL,
 payload JSON NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`)
	if err != nil {
		return nil, err
	}
	return &mysqlStore{db: db}, nil
}

func (s *mysqlStore) Load(ctx context.Context) (int64, []byte, error) {
	var version int64
	var payload []byte
	err := s.db.QueryRowContext(ctx, "SELECT version,payload FROM region_catalog_state WHERE id=1").Scan(&version, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, nil
	}
	return version, payload, err
}

func (s *mysqlStore) Save(ctx context.Context, version int64, payload []byte) (bool, error) {
	var result sql.Result
	var err error
	if version == 0 {
		result, err = s.db.ExecContext(ctx, "INSERT IGNORE INTO region_catalog_state (id,version,payload) VALUES (1,1,?)", string(payload))
	} else {
		result, err = s.db.ExecContext(ctx, "UPDATE region_catalog_state SET version=version+1,payload=? WHERE id=1 AND version=?", string(payload), version)
	}
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

type mongoStore struct{ collection *mongo.Collection }

func NewMongoStore(db *mongo.Database) Store {
	return &mongoStore{collection: db.Collection("region_catalog_state")}
}

func (s *mongoStore) Load(ctx context.Context) (int64, []byte, error) {
	var row struct {
		Version int64  `bson:"version"`
		Payload []byte `bson:"payload"`
	}
	err := s.collection.FindOne(ctx, bson.M{"_id": "catalog"}).Decode(&row)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return 0, nil, nil
	}
	return row.Version, row.Payload, err
}

func (s *mongoStore) Save(ctx context.Context, version int64, payload []byte) (bool, error) {
	if version == 0 {
		_, err := s.collection.InsertOne(ctx, bson.M{"_id": "catalog", "version": int64(1), "payload": payload})
		if mongo.IsDuplicateKeyError(err) {
			return false, nil
		}
		return err == nil, err
	}
	result, err := s.collection.UpdateOne(ctx, bson.M{"_id": "catalog", "version": version},
		bson.M{"$set": bson.M{"version": version + 1, "payload": payload}})
	return err == nil && result.MatchedCount == 1, err
}

type memoryStore struct {
	mu      sync.Mutex
	version int64
	payload []byte
}

func NewMemoryStore() Store { return &memoryStore{} }

func (s *memoryStore) Load(_ context.Context) (int64, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.version, append([]byte(nil), s.payload...), nil
}

func (s *memoryStore) Save(_ context.Context, version int64, payload []byte) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.version != version {
		return false, nil
	}
	s.version++
	s.payload = append([]byte(nil), payload...)
	return true, nil
}
