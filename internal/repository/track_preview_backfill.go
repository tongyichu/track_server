package repository

import (
	"context"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var previewURLFields = []string{"square_day_url", "square_night_url", "landscape_day_url", "landscape_night_url"}

const previewBaseSQL = `deleted_at IS NULL AND status <> 0 AND is_running = 0`

func previewFieldSQL(field string) string {
	return `COALESCE(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(preview_images, '$.` + field + `')), 'null'), '')`
}

func previewMissingSQL() string {
	parts := make([]string, 0, len(previewURLFields))
	for _, field := range previewURLFields {
		parts = append(parts, previewFieldSQL(field)+` = ''`)
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

func (r *MySQLTrackRepository) ListPreviewBackfill(ctx context.Context, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	query := `SELECT id FROM track_records WHERE ` + previewBaseSQL + ` AND COALESCE(raw_track_url, '') <> '' AND ` + previewMissingSQL()
	args := []interface{}{}
	if cursor != nil {
		query += ` AND (start_time < ? OR (start_time = ? AND id < ?))`
		args = append(args, cursor.StartTime, cursor.StartTime, cursor.ID)
	}
	query += ` ORDER BY start_time DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	tracks, err := r.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make([]*models.Track, 0, len(ids))
	for _, id := range ids {
		if t := tracks[id]; t != nil {
			result = append(result, t)
		}
	}
	return result, nil
}

func (r *MySQLTrackRepository) PreviewBackfillCounts(ctx context.Context) (pending, withoutRaw int64, err error) {
	err = r.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(COALESCE(raw_track_url, '') <> ''), 0),
		COALESCE(SUM(COALESCE(raw_track_url, '') = ''), 0)
		FROM track_records WHERE `+previewBaseSQL+` AND `+previewMissingSQL()).Scan(&pending, &withoutRaw)
	return
}

func (r *MySQLTrackRepository) FillMissingPreviewImages(ctx context.Context, expected *models.Track, images models.TrackPreviewImages) (bool, error) {
	if expected.PreviewImages.Complete() {
		return false, nil
	}
	query := `UPDATE track_records SET preview_images = ?, updated_at = ? WHERE ` + previewBaseSQL +
		` AND id = ? AND raw_track_url = ? AND COALESCE(coordinate_system, '') = ? AND COALESCE(track_type, '') = ?`
	args := []interface{}{encodeTrackPreviewImages(images), time.Now(), expected.ID, expected.RawTrackURL, expected.CoordinateSystem, expected.TrackType}
	for _, field := range previewURLFields {
		query += ` AND ` + previewFieldSQL(field) + ` = ?`
		args = append(args, expected.PreviewImages.Variant(strings.TrimSuffix(field, "_url")))
	}
	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func previewMissingMongo() bson.M {
	or := bson.A{}
	for _, field := range previewURLFields {
		or = append(or, bson.M{"preview_images." + field: bson.M{"$in": bson.A{"", nil}}})
	}
	return bson.M{"status": bson.M{"$ne": models.TrackStatusDeleted}, "is_running": false, "$or": or}
}

func (r *MongoTrackRepository) ListPreviewBackfill(ctx context.Context, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	filter := previewMissingMongo()
	filter["raw_track_url"] = bson.M{"$nin": bson.A{"", nil}}
	if cursor != nil {
		filter["$and"] = bson.A{bson.M{"$or": bson.A{
			bson.M{"start_time": bson.M{"$lt": cursor.StartTime}},
			bson.M{"start_time": cursor.StartTime, "_id": bson.M{"$lt": cursor.ID}},
		}}}
	}
	return r.listTracks(ctx, filter, options.Find().SetSort(bson.D{{Key: "start_time", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(int64(limit)))
}

func (r *MongoTrackRepository) PreviewBackfillCounts(ctx context.Context) (pending, withoutRaw int64, err error) {
	filter := previewMissingMongo()
	filter["raw_track_url"] = bson.M{"$nin": bson.A{"", nil}}
	pending, err = r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return
	}
	filter["raw_track_url"] = bson.M{"$in": bson.A{"", nil}}
	withoutRaw, err = r.collection.CountDocuments(ctx, filter)
	return
}

func (r *MongoTrackRepository) FillMissingPreviewImages(ctx context.Context, expected *models.Track, images models.TrackPreviewImages) (bool, error) {
	if expected.PreviewImages.Complete() {
		return false, nil
	}
	filter := bson.M{"_id": expected.ID, "status": bson.M{"$ne": models.TrackStatusDeleted}, "is_running": false,
		"raw_track_url": expected.RawTrackURL, "coordinate_system": expected.CoordinateSystem, "track_type": expected.TrackType}
	for _, field := range previewURLFields {
		value := expected.PreviewImages.Variant(strings.TrimSuffix(field, "_url"))
		if value == "" {
			filter["preview_images."+field] = bson.M{"$in": bson.A{"", nil}}
		} else {
			filter["preview_images."+field] = value
		}
	}
	res, err := r.collection.UpdateOne(ctx, filter, bson.M{"$set": bson.M{"preview_images": images, "updated_at": time.Now()}})
	if err != nil {
		return false, err
	}
	return res.ModifiedCount > 0, nil
}

func previewBackfillEligible(t *models.Track) bool {
	return t != nil && t.Status != models.TrackStatusDeleted && !t.IsRunning && !t.PreviewImages.Complete()
}

func (r *InMemoryTrackRepository) ListPreviewBackfill(ctx context.Context, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	// Preserve the same ordering as ListAll, including when most of a page is already complete.
	result := make([]*models.Track, 0, limit)
	for len(result) < limit {
		page, err := r.ListAll(ctx, cursor, limit)
		if err != nil {
			return nil, err
		}
		for _, t := range page {
			if previewBackfillEligible(t) && t.RawTrackURL != "" {
				result = append(result, t)
				if len(result) == limit {
					return result, nil
				}
			}
		}
		if len(page) < limit {
			break
		}
		last := page[len(page)-1]
		cursor = &models.TrackListCursor{StartTime: last.StartTime, ID: last.ID}
	}
	return result, nil
}

func (r *InMemoryTrackRepository) PreviewBackfillCounts(_ context.Context) (pending, withoutRaw int64, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, t := range r.tracks {
		if !previewBackfillEligible(t) {
			continue
		}
		if t.RawTrackURL == "" {
			withoutRaw++
		} else {
			pending++
		}
	}
	return
}

func (r *InMemoryTrackRepository) FillMissingPreviewImages(_ context.Context, expected *models.Track, images models.TrackPreviewImages) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.tracks[expected.ID]
	if !previewBackfillEligible(t) || t.RawTrackURL != expected.RawTrackURL || t.CoordinateSystem != expected.CoordinateSystem ||
		t.TrackType != expected.TrackType || t.PreviewImages != expected.PreviewImages {
		return false, nil
	}
	t.PreviewImages = images
	t.UpdatedAt = time.Now()
	return true, nil
}
