package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const trackPhotosDDL = `CREATE TABLE IF NOT EXISTS track_photos (
  id CHAR(36) NOT NULL PRIMARY KEY,
  track_id VARCHAR(64) NOT NULL,
  slot TINYINT UNSIGNED NULL,
  status VARCHAR(12) NOT NULL,
  revision BIGINT NOT NULL,
  metadata JSON NOT NULL,
  jpeg MEDIUMBLOB NULL,
  UNIQUE KEY uq_track_photo_slot(track_id,slot),
  KEY idx_photo_review(status,id),
  KEY idx_photo_review_track(status,track_id),
  CONSTRAINT fk_track_photo_track FOREIGN KEY(track_id) REFERENCES track_records(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

const photoSourceAvailable = `EXISTS (SELECT 1 FROM track_records t WHERE t.id=track_photos.track_id AND t.status<>0 AND NOT EXISTS (SELECT 1 FROM user_account_deletions d WHERE d.user_id=t.user_id))`

type PhotoFilter struct {
	TrackIDs      []string
	Status, After string
	Limit         int
	Spot          string
	IncludedOnly  bool
}
type TrackPhotoRepository interface {
	PendingPhotos(context.Context) (int64, error)
	ReviewTrackIDs(context.Context, string, string, int) ([]string, error)
	InsertPhoto(context.Context, models.TrackPhoto, []byte) error
	ReadPhoto(context.Context, string, bool) (models.TrackPhoto, []byte, error)
	PhotoPage(context.Context, PhotoFilter) ([]models.TrackPhoto, error)
	UpdatePhoto(context.Context, models.TrackPhoto, int64) error
}

func (r *MySQLTrackRepository) PendingPhotos(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM track_photos WHERE status='pending' AND "+photoSourceAvailable).Scan(&n)
	return n, err
}
func (r *MongoTrackRepository) PendingPhotos(ctx context.Context) (int64, error) {
	pipeline := r.photoPipeline(bson.M{"status": "pending"}, PhotoFilter{})
	pipeline = append(pipeline, bson.M{"$count": "n"})
	cur, err := r.photosCollection().Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cur.Close(ctx)
	if !cur.Next(ctx) {
		return 0, cur.Err()
	}
	var result struct{ N int64 }
	err = cur.Decode(&result)
	return result.N, err
}
func (r *InMemoryTrackRepository) PendingPhotos(ctx context.Context) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var n int64
	for _, v := range r.photos {
		if v.meta.Status == "pending" && r.photoTrackAvailable(v.meta.TrackID) {
			n++
		}
	}
	return n, nil
}

// Caller holds the track repository's lock.
func (r *InMemoryTrackRepository) photoTrackAvailable(id string) bool {
	t := r.tracks[id]
	return t != nil && t.Status != models.TrackStatusDeleted
}
func (r *MySQLTrackRepository) ReviewTrackIDs(ctx context.Context, status, after string, limit int) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT DISTINCT track_id FROM track_photos WHERE status=? AND track_id>? AND "+photoSourceAvailable+" ORDER BY track_id LIMIT ?", status, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
func (r *MongoTrackRepository) ReviewTrackIDs(ctx context.Context, status, after string, limit int) ([]string, error) {
	stages := r.photoPipeline(bson.M{"status": status, "track_id": bson.M{"$gt": after}}, PhotoFilter{})
	stages = append(stages, bson.M{"$group": bson.M{"_id": "$track_id"}}, bson.M{"$sort": bson.M{"_id": 1}}, bson.M{"$limit": limit})
	cur, err := r.photosCollection().Aggregate(ctx, stages)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	ids := []string{}
	for cur.Next(ctx) {
		var v struct {
			ID string `bson:"_id"`
		}
		if err := cur.Decode(&v); err != nil {
			return nil, err
		}
		ids = append(ids, v.ID)
	}
	return ids, cur.Err()
}
func (r *InMemoryTrackRepository) ReviewTrackIDs(ctx context.Context, status, after string, limit int) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := map[string]bool{}
	for _, v := range r.photos {
		p := v.meta
		if p.Status == status && p.TrackID > after && r.photoTrackAvailable(p.TrackID) {
			seen[p.TrackID] = true
		}
	}
	ids := []string{}
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) > limit {
		ids = ids[:limit]
	}
	return ids, nil
}

type storedPhoto struct {
	meta models.TrackPhoto
	jpeg []byte
}

func (r *InMemoryTrackRepository) InsertPhoto(_ context.Context, p models.TrackPhoto, data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.photos == nil {
		r.photos = make(map[string]storedPhoto)
	}
	if _, ok := r.photos[p.ID]; ok {
		return ErrAlreadyExists
	}
	for _, v := range r.photos {
		if v.meta.TrackID == p.TrackID && v.meta.Slot == p.Slot {
			return ErrAlreadyExists
		}
	}
	r.photos[p.ID] = storedPhoto{p, data}
	return nil
}
func (r *InMemoryTrackRepository) ReadPhoto(_ context.Context, id string, media bool) (models.TrackPhoto, []byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.photos[id]
	if !ok || !r.photoTrackAvailable(v.meta.TrackID) {
		return v.meta, nil, ErrNotFound
	}
	if !media {
		return v.meta, nil, nil
	}
	return v.meta, v.jpeg, nil
}
func (r *InMemoryTrackRepository) PhotoPage(_ context.Context, f PhotoFilter) ([]models.TrackPhoto, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []models.TrackPhoto{}
	tracks := map[string]bool{}
	for _, id := range f.TrackIDs {
		tracks[id] = true
	}
	for _, v := range r.photos {
		p := v.meta
		if !r.photoTrackAvailable(p.TrackID) {
			continue
		}
		if f.Spot != "" {
			t := r.tracks[p.TrackID]
			if t.IsRunning || t.Status != models.TrackStatusNormal || t.RawTrackURL == "" || r.adminScenicSpots == nil {
				continue
			}
			r.adminScenicSpots.mu.RLock()
			link := r.adminScenicSpots.links[t.ID]
			matches := link != nil && link.SpotID == f.Spot
			r.adminScenicSpots.mu.RUnlock()
			if !matches || (f.IncludedOnly && p.ScenicChoice != f.Spot) {
				continue
			}
		}
		if p.ID <= f.After || p.Status == "removed" || f.TrackIDs != nil && !tracks[p.TrackID] || f.Status != "" && p.Status != f.Status {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}
func (r *InMemoryTrackRepository) UpdatePhoto(_ context.Context, p models.TrackPhoto, old int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.photos[p.ID]
	if !ok || v.meta.Revision != old {
		return ErrConflict
	}
	v.meta = p
	if p.Status == "removed" {
		v.jpeg = nil
	}
	r.photos[p.ID] = v
	return nil
}
func (r *MySQLTrackRepository) InsertPhoto(ctx context.Context, p models.TrackPhoto, data []byte) error {
	meta, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, "INSERT INTO track_photos(id,track_id,slot,status,revision,metadata,jpeg) VALUES (?,?,?,?,?,?,?)", p.ID, p.TrackID, p.Slot, p.Status, p.Revision, meta, data)
	var duplicate *mysqlDriver.MySQLError
	if errors.As(err, &duplicate) && duplicate.Number == 1062 {
		return ErrAlreadyExists
	}
	return err
}
func (r *MySQLTrackRepository) ReadPhoto(ctx context.Context, id string, media bool) (models.TrackPhoto, []byte, error) {
	var p models.TrackPhoto
	var meta, data []byte
	col := "NULL"
	if media {
		col = "jpeg"
	}
	err := r.db.QueryRowContext(ctx, "SELECT metadata,"+col+" FROM track_photos WHERE id=? AND "+photoSourceAvailable, id).Scan(&meta, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return p, nil, ErrNotFound
	}
	if err != nil {
		return p, nil, err
	}
	err = json.Unmarshal(meta, &p)
	return p, data, err
}
func (r *MySQLTrackRepository) PhotoPage(ctx context.Context, f PhotoFilter) ([]models.TrackPhoto, error) {
	out := []models.TrackPhoto{}
	q := "SELECT metadata FROM track_photos WHERE id>? AND slot IS NOT NULL AND " + photoSourceAvailable
	args := []any{f.After}
	if f.Status != "" {
		q += " AND status=?"
		args = append(args, f.Status)
	}
	if f.TrackIDs != nil {
		if len(f.TrackIDs) == 0 {
			return out, nil
		}
		q += " AND track_id IN (" + strings.TrimRight(strings.Repeat("?,", len(f.TrackIDs)), ",") + ")"
		for _, id := range f.TrackIDs {
			args = append(args, id)
		}
	}
	if f.Spot != "" {
		q += " AND EXISTS (SELECT 1 FROM track_scenic_spot_links l JOIN track_records t ON t.id=l.track_id WHERE l.track_id=track_photos.track_id AND l.spot_id=? AND t.status=1 AND t.is_running=0 AND t.raw_track_url<>'')"
		args = append(args, f.Spot)
		if f.IncludedOnly {
			q += " AND JSON_UNQUOTE(JSON_EXTRACT(metadata,'$.scenic_choice'))=?"
			args = append(args, f.Spot)
		}
	}
	rows, err := r.db.QueryContext(ctx, q+" ORDER BY id LIMIT ?", append(args, f.Limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var meta []byte
		var p models.TrackPhoto
		if err = rows.Scan(&meta); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(meta, &p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (r *MySQLTrackRepository) UpdatePhoto(ctx context.Context, p models.TrackPhoto, old int64) error {
	meta, err := json.Marshal(p)
	if err != nil {
		return err
	}
	var slot any = p.Slot
	if p.Status == "removed" {
		slot = nil
	}
	res, err := r.db.ExecContext(ctx, "UPDATE track_photos SET slot=?,status=?,revision=?,metadata=?,jpeg=IF(?='removed',NULL,jpeg) WHERE id=? AND revision=?", slot, p.Status, p.Revision, meta, p.Status, p.ID, old)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		return ErrConflict
	}
	return err
}
func (r *MongoTrackRepository) photosCollection() *mongo.Collection {
	return r.collection.Database().Collection("track_photos")
}
func (r *MongoTrackRepository) EnsurePhotoIndexes(ctx context.Context) error {
	_, err := r.photosCollection().Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "track_id", Value: 1}, {Key: "slot", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"slot": bson.M{"$exists": true}})},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "_id", Value: 1}}}})
	return err
}

type mongoPhoto struct {
	models.TrackPhoto `bson:",inline"`
	JPEG              []byte `bson:"jpeg,omitempty"`
}

func (r *MongoTrackRepository) InsertPhoto(ctx context.Context, p models.TrackPhoto, data []byte) error {
	_, err := r.photosCollection().InsertOne(ctx, mongoPhoto{p, data})
	if mongo.IsDuplicateKeyError(err) {
		return ErrAlreadyExists
	}
	return err
}
func (r *MongoTrackRepository) ReadPhoto(ctx context.Context, id string, media bool) (models.TrackPhoto, []byte, error) {
	opts := options.FindOne()
	if !media {
		opts.SetProjection(bson.M{"jpeg": 0})
	}
	var v mongoPhoto
	err := r.photosCollection().FindOne(ctx, bson.M{"_id": id}, opts).Decode(&v)
	if errors.Is(err, mongo.ErrNoDocuments) {
		err = ErrNotFound
	}
	return v.TrackPhoto, v.JPEG, err
}
func (r *MongoTrackRepository) photoPipeline(filter bson.M, f PhotoFilter) []bson.M {
	pipeline := []bson.M{{"$match": filter}, {"$project": bson.M{"jpeg": 0}}, {"$sort": bson.M{"_id": 1}},
		{"$lookup": bson.M{"from": r.collection.Name(), "localField": "track_id", "foreignField": "_id", "as": "parent"}}}
	parent := bson.M{"parent.0": bson.M{"$exists": true}, "parent.status": bson.M{"$ne": models.TrackStatusDeleted}}
	if f.Spot != "" {
		parent["parent.status"] = models.TrackStatusNormal
		parent["parent.is_running"] = false
		parent["parent.raw_track_url"] = bson.M{"$exists": true, "$nin": bson.A{"", nil}}
	}
	pipeline = append(pipeline, bson.M{"$match": parent})
	if f.Spot != "" {
		pipeline = append(pipeline, bson.M{"$lookup": bson.M{"from": "track_scenic_spot_links", "localField": "track_id", "foreignField": "_id", "as": "link"}}, bson.M{"$match": bson.M{"link.scenic_spot_id": f.Spot}})
		if f.IncludedOnly {
			pipeline = append(pipeline, bson.M{"$match": bson.M{"scenic_choice": f.Spot}})
		}
	}
	return pipeline
}
func (r *MongoTrackRepository) PhotoPage(ctx context.Context, f PhotoFilter) ([]models.TrackPhoto, error) {
	q := bson.M{"_id": bson.M{"$gt": f.After}, "slot": bson.M{"$exists": true}}
	if f.Status != "" {
		q["status"] = f.Status
	}
	if f.TrackIDs != nil {
		q["track_id"] = bson.M{"$in": f.TrackIDs}
	}
	pipeline := append(r.photoPipeline(q, f), bson.M{"$limit": f.Limit}, bson.M{"$project": bson.M{"parent": 0, "link": 0}})
	cur, err := r.photosCollection().Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []models.TrackPhoto{}
	err = cur.All(ctx, &out)
	return out, err
}
func (r *MongoTrackRepository) UpdatePhoto(ctx context.Context, p models.TrackPhoto, old int64) error {
	fields := bson.M{"status": p.Status, "reason": p.Reason, "revision": p.Revision, "scenic_choice": p.ScenicChoice}
	update := bson.M{"$set": fields}
	if p.Status == "removed" {
		update["$unset"] = bson.M{"slot": "", "jpeg": "", "latitude": "", "longitude": "", "digest": "", "description": ""}
		fields["taken_at_ms"] = int64(0)
	}
	res, err := r.photosCollection().UpdateOne(ctx, bson.M{"_id": p.ID, "revision": old}, update)
	if err == nil && res.MatchedCount != 1 {
		return ErrConflict
	}
	return err
}
