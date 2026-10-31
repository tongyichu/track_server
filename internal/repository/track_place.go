package repository

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// TrackPlaceRepository stores provider data separately from route groups.
// Leases and API budgets are shared by all workers, including after restarts.
type TrackPlaceRepository interface {
	GetPlaceRecord(context.Context, string) (*models.TrackPlaceRecord, error)
	GetPlaceRecords(context.Context, []string) (map[string]*models.TrackPlaceRecord, error)
	ListPlaceCandidateRecords(ctx context.Context, cursorTrackID string, limit int) ([]*models.TrackPlaceRecord, error)
	LeasePlaceRecord(ctx context.Context, key, owner string, now, until time.Time) (bool, error)
	SavePlaceRecord(ctx context.Context, record *models.TrackPlaceRecord, owner string) (bool, error)
	ReservePlaceAPICall(ctx context.Context, provider, day string, budget int) (bool, error)
	DeleteExpiredPlaceContent(context.Context, time.Time) error
}

var (
	_ TrackPlaceRepository  = (*MySQLTrackPlaceRepository)(nil)
	_ TrackPlaceRepository  = (*MongoTrackPlaceRepository)(nil)
	_ TrackPlaceRepository  = (*MemoryTrackPlaceRepository)(nil)
	_ TrackPlaceIndexReader = (*MySQLTrackMapRepository)(nil)
	_ TrackPlaceIndexReader = (*MongoTrackMapRepository)(nil)
	_ TrackPlaceIndexReader = (*InMemoryTrackMapRepository)(nil)
	_ TrackPlaceGroupWriter = (*MySQLTrackMapRepository)(nil)
	_ TrackPlaceGroupWriter = (*MongoTrackMapRepository)(nil)
	_ TrackPlaceGroupWriter = (*InMemoryTrackMapRepository)(nil)
)

type MySQLTrackPlaceRepository struct{ db *sql.DB }

func NewMySQLTrackPlaceRepository(db *sql.DB) *MySQLTrackPlaceRepository {
	return &MySQLTrackPlaceRepository{db: db}
}
func (r *MySQLTrackPlaceRepository) GetPlaceRecord(ctx context.Context, key string) (*models.TrackPlaceRecord, error) {
	items, err := r.GetPlaceRecords(ctx, []string{key})
	if err != nil {
		return nil, err
	}
	if items[key] == nil {
		return nil, ErrNotFound
	}
	return items[key], nil
}
func (r *MySQLTrackPlaceRepository) GetPlaceRecords(ctx context.Context, keys []string) (map[string]*models.TrackPlaceRecord, error) {
	out := map[string]*models.TrackPlaceRecord{}
	if len(keys) == 0 {
		return out, nil
	}
	keys = uniqueNonEmptyStrings(keys)
	placeholders := strings.TrimRight(strings.Repeat("?,", len(keys)), ",")
	args := make([]interface{}, len(keys))
	for i, key := range keys {
		args[i] = key
	}
	rows, err := r.db.QueryContext(ctx, `SELECT record_key,data_json,expires_at,lease_until,lease_owner FROM track_place_records WHERE record_key IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var v models.TrackPlaceRecord
		var expires, lease sql.NullTime
		if err := rows.Scan(&v.Key, &v.Data, &expires, &lease, &v.LeaseOwner); err != nil {
			return nil, err
		}
		v.ExpiresAt = expires.Time
		v.LeaseUntil = lease.Time
		out[v.Key] = &v
	}
	return out, rows.Err()
}
func (r *MySQLTrackPlaceRepository) LeasePlaceRecord(ctx context.Context, key, owner string, now, until time.Time) (bool, error) {
	if _, err := r.db.ExecContext(ctx, `INSERT IGNORE INTO track_place_records(record_key,data_json,lease_owner) VALUES (?, '{}','')`, key); err != nil {
		return false, err
	}
	res, err := r.db.ExecContext(ctx, `UPDATE track_place_records SET lease_owner=?,lease_until=? WHERE record_key=? AND (lease_until IS NULL OR lease_until<=?)`, owner, until, key, now)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
func (r *MySQLTrackPlaceRepository) SavePlaceRecord(ctx context.Context, v *models.TrackPlaceRecord, owner string) (bool, error) {
	if owner != "" {
		res, err := r.db.ExecContext(ctx, `UPDATE track_place_records SET data_json=?,expires_at=?,lease_until=NULL,lease_owner='' WHERE record_key=? AND lease_owner=?`, v.Data, nullableTimeValue(v.ExpiresAt), v.Key, owner)
		if err != nil {
			return false, err
		}
		n, err := res.RowsAffected()
		return n > 0, err
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO track_place_records(record_key,data_json,expires_at,lease_owner) VALUES(?,?,?,'') ON DUPLICATE KEY UPDATE data_json=VALUES(data_json),expires_at=VALUES(expires_at)`, v.Key, v.Data, nullableTimeValue(v.ExpiresAt))
	return err == nil, err
}
func (r *MySQLTrackPlaceRepository) ReservePlaceAPICall(ctx context.Context, provider, day string, budget int) (bool, error) {
	if _, err := r.db.ExecContext(ctx, `INSERT IGNORE INTO track_place_api_usage(provider,usage_day,used) VALUES(?,?,0)`, provider, day); err != nil {
		return false, err
	}
	query := `UPDATE track_place_api_usage SET used=used+1 WHERE provider=? AND usage_day=?`
	args := []interface{}{provider, day}
	if budget > 0 {
		query += ` AND used<?`
		args = append(args, budget)
	}
	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

type MongoTrackPlaceRepository struct{ records, usage *mongo.Collection }

func NewMongoTrackPlaceRepository(db *mongo.Database) *MongoTrackPlaceRepository {
	return &MongoTrackPlaceRepository{db.Collection("track_place_records"), db.Collection("track_place_api_usage")}
}

func (r *MongoTrackPlaceRepository) EnsureIndexes(ctx context.Context) error {
	if _, err := r.records.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetName("idx_track_place_expiry")}); err != nil {
		return err
	}
	_, err := r.records.Database().Collection("track_geo_indexes").Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "track_id", Value: 1}}, Options: options.Index().SetName("idx_track_place_scan")})
	return err
}
func (r *MongoTrackPlaceRepository) GetPlaceRecord(ctx context.Context, key string) (*models.TrackPlaceRecord, error) {
	var v models.TrackPlaceRecord
	err := r.records.FindOne(ctx, bson.M{"_id": key}).Decode(&v)
	if errors.Is(err, mongo.ErrNoDocuments) {
		err = ErrNotFound
	}
	return &v, err
}
func (r *MongoTrackPlaceRepository) GetPlaceRecords(ctx context.Context, keys []string) (map[string]*models.TrackPlaceRecord, error) {
	out := map[string]*models.TrackPlaceRecord{}
	if len(keys) == 0 {
		return out, nil
	}
	cur, err := r.records.Find(ctx, bson.M{"_id": bson.M{"$in": keys}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var v models.TrackPlaceRecord
		if err := cur.Decode(&v); err != nil {
			return nil, err
		}
		out[v.Key] = &v
	}
	return out, cur.Err()
}
func (r *MongoTrackPlaceRepository) LeasePlaceRecord(ctx context.Context, key, owner string, now, until time.Time) (bool, error) {
	_, err := r.records.UpdateOne(ctx, bson.M{"_id": key}, bson.M{"$setOnInsert": bson.M{"data": "{}", "lease_owner": "", "lease_until": time.Time{}}}, options.Update().SetUpsert(true))
	if err != nil && !mongo.IsDuplicateKeyError(err) {
		return false, err
	}
	res, err := r.records.UpdateOne(ctx, bson.M{"_id": key, "$or": bson.A{bson.M{"lease_until": bson.M{"$lte": now}}, bson.M{"lease_until": nil}}}, bson.M{"$set": bson.M{"lease_until": until, "lease_owner": owner}})
	if err != nil {
		return false, err
	}
	return res.ModifiedCount > 0, nil
}
func (r *MongoTrackPlaceRepository) SavePlaceRecord(ctx context.Context, v *models.TrackPlaceRecord, owner string) (bool, error) {
	filter := bson.M{"_id": v.Key}
	set := bson.M{"data": v.Data, "expires_at": v.ExpiresAt}
	if owner != "" {
		filter["lease_owner"] = owner
		set["lease_until"] = time.Time{}
		set["lease_owner"] = ""
	}
	res, err := r.records.UpdateOne(ctx, filter, bson.M{"$set": set}, options.Update().SetUpsert(owner == ""))
	if err != nil {
		return false, err
	}
	return res.MatchedCount > 0 || res.UpsertedCount > 0, nil
}
func (r *MongoTrackPlaceRepository) ReservePlaceAPICall(ctx context.Context, provider, day string, budget int) (bool, error) {
	key := provider + ":" + day
	filter := bson.M{"_id": key}
	if budget > 0 {
		filter["used"] = bson.M{"$lt": budget}
	}
	res, err := r.usage.UpdateOne(ctx, filter, bson.M{"$inc": bson.M{"used": 1}}, options.Update().SetUpsert(true))
	if mongo.IsDuplicateKeyError(err) {
		if budget > 0 {
			return false, nil
		}
		res, err = r.usage.UpdateOne(ctx, bson.M{"_id": key}, bson.M{"$inc": bson.M{"used": 1}})
	}
	if err != nil {
		return false, err
	}
	return res.ModifiedCount > 0 || res.UpsertedCount > 0, nil
}

type MemoryTrackPlaceRepository struct {
	mu      sync.Mutex
	records map[string]*models.TrackPlaceRecord
	usage   map[string]int
}

func NewMemoryTrackPlaceRepository() *MemoryTrackPlaceRepository {
	return &MemoryTrackPlaceRepository{records: map[string]*models.TrackPlaceRecord{}, usage: map[string]int{}}
}
func (r *MemoryTrackPlaceRepository) GetPlaceRecord(ctx context.Context, key string) (*models.TrackPlaceRecord, error) {
	items, _ := r.GetPlaceRecords(ctx, []string{key})
	if items[key] == nil {
		return nil, ErrNotFound
	}
	return items[key], nil
}
func (r *MemoryTrackPlaceRepository) GetPlaceRecords(_ context.Context, keys []string) (map[string]*models.TrackPlaceRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]*models.TrackPlaceRecord{}
	for _, key := range keys {
		if v := r.records[key]; v != nil {
			copy := *v
			out[key] = &copy
		}
	}
	return out, nil
}
func (r *MemoryTrackPlaceRepository) LeasePlaceRecord(_ context.Context, key, owner string, now, until time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v := r.records[key]
	if v == nil {
		v = &models.TrackPlaceRecord{Key: key, Data: "{}"}
		r.records[key] = v
	}
	if v.LeaseUntil.After(now) {
		return false, nil
	}
	v.LeaseOwner = owner
	v.LeaseUntil = until
	return true, nil
}
func (r *MemoryTrackPlaceRepository) SavePlaceRecord(_ context.Context, v *models.TrackPlaceRecord, owner string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if owner != "" && (r.records[v.Key] == nil || r.records[v.Key].LeaseOwner != owner) {
		return false, nil
	}
	copy := *v
	copy.LeaseOwner = ""
	copy.LeaseUntil = time.Time{}
	r.records[v.Key] = &copy
	return true, nil
}
func (r *MemoryTrackPlaceRepository) ReservePlaceAPICall(_ context.Context, provider, day string, budget int) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := provider + ":" + day
	if budget > 0 && r.usage[key] >= budget {
		return false, nil
	}
	r.usage[key]++
	return true, nil
}

// Bounded keyset scans also backfill old indexes without downloading raw GPS again.
type TrackPlaceIndexReader interface {
	ListPlaceGeoIndexes(context.Context, string, int) ([]*models.TrackGeoIndex, error)
	FindPlaceGeoIndexes(context.Context, []string) (map[string]*models.TrackGeoIndex, error)
}

func (r *MySQLTrackMapRepository) FindPlaceGeoIndexes(ctx context.Context, ids []string) (map[string]*models.TrackGeoIndex, error) {
	out := map[string]*models.TrackGeoIndex{}
	ids = uniqueNonEmptyStrings(ids)
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := r.db.QueryContext(ctx, trackGeoIndexSelectSQL()+` WHERE track_id IN (`+strings.TrimRight(strings.Repeat("?,", len(ids)), ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		index, err := scanTrackGeoIndex(rows)
		if err != nil {
			return nil, err
		}
		out[index.TrackID] = index
	}
	return out, rows.Err()
}

func (r *MongoTrackMapRepository) FindPlaceGeoIndexes(ctx context.Context, ids []string) (map[string]*models.TrackGeoIndex, error) {
	out := map[string]*models.TrackGeoIndex{}
	if len(ids) == 0 {
		return out, nil
	}
	cur, err := r.index.Find(ctx, bson.M{"track_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var index models.TrackGeoIndex
		if err := cur.Decode(&index); err != nil {
			return nil, err
		}
		out[index.TrackID] = &index
	}
	return out, cur.Err()
}

func (r *InMemoryTrackMapRepository) FindPlaceGeoIndexes(_ context.Context, ids []string) (map[string]*models.TrackGeoIndex, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := map[string]*models.TrackGeoIndex{}
	for _, id := range ids {
		if index := r.indexes[id]; index != nil {
			out[id] = cloneTrackGeoIndex(index)
		}
	}
	return out, nil
}

func (r *MySQLTrackMapRepository) ListPlaceGeoIndexes(ctx context.Context, after string, limit int) ([]*models.TrackGeoIndex, error) {
	rows, err := r.db.QueryContext(ctx, trackGeoIndexSelectSQL()+` WHERE track_id>? ORDER BY track_id LIMIT ?`, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*models.TrackGeoIndex{}
	for rows.Next() {
		v, err := scanTrackGeoIndex(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *MongoTrackMapRepository) ListPlaceGeoIndexes(ctx context.Context, after string, limit int) ([]*models.TrackGeoIndex, error) {
	cur, err := r.index.Find(ctx, bson.M{"track_id": bson.M{"$gt": after}}, options.Find().SetSort(bson.D{{Key: "track_id", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []*models.TrackGeoIndex{}
	for cur.Next(ctx) {
		var v models.TrackGeoIndex
		if err := cur.Decode(&v); err != nil {
			return nil, err
		}
		out = append(out, &v)
	}
	return out, cur.Err()
}
func (r *InMemoryTrackMapRepository) ListPlaceGeoIndexes(_ context.Context, after string, limit int) ([]*models.TrackGeoIndex, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []*models.TrackGeoIndex{}
	for id, v := range r.indexes {
		if id > after {
			out = append(out, cloneTrackGeoIndex(v))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TrackID < out[j].TrackID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// The background worker updates only the area, guarded by the current
// representative track. It cannot overwrite a concurrent rename or regroup.
type TrackPlaceGroupWriter interface {
	UpdateRouteGroupPlaceArea(context.Context, string, string, string) error
}

func (r *MySQLTrackMapRepository) UpdateRouteGroupPlaceArea(ctx context.Context, id, representative, area string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE track_route_groups SET area_id=? WHERE group_id=? AND representative_track_id=? AND status='active'`, area, id, representative)
	return err
}
func (r *MongoTrackMapRepository) UpdateRouteGroupPlaceArea(ctx context.Context, id, representative, area string) error {
	_, err := r.groups.UpdateOne(ctx, bson.M{"_id": id, "representative_track_id": representative, "status": models.TrackRouteGroupStatusActive}, bson.M{"$set": bson.M{"area_id": area}})
	return err
}
func (r *InMemoryTrackMapRepository) UpdateRouteGroupPlaceArea(_ context.Context, id, representative, area string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if g := r.routeGroups[id]; g != nil && g.RepresentativeTrackID == representative && g.Status == models.TrackRouteGroupStatusActive {
		g.AreaID = area
	}
	return nil
}

const trackPlaceRecordSchema = `CREATE TABLE IF NOT EXISTS track_place_records (
 record_key VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
 data_json LONGTEXT NOT NULL,
 expires_at DATETIME NULL,
 lease_until DATETIME NULL,
 lease_owner VARCHAR(64) NOT NULL DEFAULT '',
 KEY idx_track_place_expiry(expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`
const trackPlaceUsageSchema = `CREATE TABLE IF NOT EXISTS track_place_api_usage (
 provider VARCHAR(16) NOT NULL,
 usage_day DATE NOT NULL,
 used INT UNSIGNED NOT NULL DEFAULT 0,
 PRIMARY KEY(provider,usage_day)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`
