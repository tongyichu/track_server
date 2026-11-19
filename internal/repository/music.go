package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"sort"
	"sync"
)

const MusicSchema = `CREATE TABLE IF NOT EXISTS background_music (id VARCHAR(32) NOT NULL PRIMARY KEY, revision BIGINT NOT NULL, payload LONGTEXT NOT NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`

var ErrMusicConflict = errors.New("music revision conflict")

type MusicRepository interface {
	ListMusic(context.Context) ([]models.MusicTrack, error)
	SaveMusic(context.Context, models.MusicTrack, int64) error
	DeleteMusic(context.Context, string, int64) error
}
type MemoryMusicRepository struct {
	mu    sync.Mutex
	items map[string]models.MusicTrack
}

func NewMemoryMusicRepository() MusicRepository {
	return &MemoryMusicRepository{items: map[string]models.MusicTrack{}}
}
func (r *MemoryMusicRepository) ListMusic(ctx context.Context) ([]models.MusicTrack, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]models.MusicTrack, 0, len(r.items))
	for _, v := range r.items {
		out = append(out, v)
	}
	sortMusic(out)
	return out, nil
}
func (r *MemoryMusicRepository) SaveMusic(ctx context.Context, v models.MusicTrack, previous int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	old, ok := r.items[v.ID]
	if (!ok && previous != 0) || (ok && old.Revision != previous) {
		return ErrMusicConflict
	}
	r.items[v.ID] = v
	return nil
}
func sortMusic(items []models.MusicTrack) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].SortOrder != items[j].SortOrder {
			return items[i].SortOrder < items[j].SortOrder
		}
		return items[i].ID < items[j].ID
	})
}

type MySQLMusicRepository struct{ db *sql.DB }

func NewMySQLMusicRepository(ctx context.Context, db *sql.DB) (MusicRepository, error) {
	if _, err := db.ExecContext(ctx, MusicSchema); err != nil {
		return nil, err
	}
	return &MySQLMusicRepository{db}, nil
}
func (r *MySQLMusicRepository) ListMusic(ctx context.Context) ([]models.MusicTrack, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT payload FROM background_music LIMIT 501")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.MusicTrack{}
	for rows.Next() {
		var raw []byte
		var v models.MusicTrack
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	sortMusic(out)
	return out, rows.Err()
}
func (r *MySQLMusicRepository) SaveMusic(ctx context.Context, v models.MusicTrack, previous int64) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if previous == 0 {
		_, err = r.db.ExecContext(ctx, "INSERT INTO background_music(id,revision,payload) VALUES(?,?,?)", v.ID, v.Revision, raw)
		return err
	}
	res, err := r.db.ExecContext(ctx, "UPDATE background_music SET revision=?,payload=? WHERE id=? AND revision=?", v.Revision, raw, v.ID, previous)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		return ErrMusicConflict
	}
	return err
}

type MongoMusicRepository struct{ collection *mongo.Collection }

func NewMongoMusicRepository(db *mongo.Database) MusicRepository {
	return &MongoMusicRepository{db.Collection("background_music")}
}
func (r *MongoMusicRepository) ListMusic(ctx context.Context) ([]models.MusicTrack, error) {
	cur, err := r.collection.Find(ctx, bson.M{}, options.Find().SetLimit(501))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []models.MusicTrack{}
	err = cur.All(ctx, &out)
	sortMusic(out)
	return out, err
}
func (r *MongoMusicRepository) SaveMusic(ctx context.Context, v models.MusicTrack, previous int64) error {
	if previous == 0 {
		_, err := r.collection.InsertOne(ctx, v)
		if mongo.IsDuplicateKeyError(err) {
			return ErrMusicConflict
		}
		return err
	}
	res, err := r.collection.ReplaceOne(ctx, bson.M{"_id": v.ID, "revision": previous}, v)
	if err == nil && res.MatchedCount != 1 {
		return ErrMusicConflict
	}
	return err
}

func (r *MemoryMusicRepository) DeleteMusic(ctx context.Context, id string, previous int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	old, ok := r.items[id]
	if !ok {
		return ErrMusicConflict
	}
	if old.Revision != previous {
		return ErrMusicConflict
	}
	delete(r.items, id)
	return nil
}

func (r *MySQLMusicRepository) DeleteMusic(ctx context.Context, id string, previous int64) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM background_music WHERE id=? AND revision=?", id, previous)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		return ErrMusicConflict
	}
	return err
}

func (r *MongoMusicRepository) DeleteMusic(ctx context.Context, id string, previous int64) error {
	res, err := r.collection.DeleteOne(ctx, bson.M{"_id": id, "revision": previous})
	if err == nil && res.DeletedCount != 1 {
		return ErrMusicConflict
	}
	return err
}
