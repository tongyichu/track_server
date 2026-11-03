package repository

import (
	"context"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
)

// TrackNarrativeWriter updates only generated fields, conditional on the snapshot read before generation.
type TrackNarrativeWriter interface {
	ApplyTrackNarrative(context.Context, *models.Track, string, string, bool, bool) (bool, error)
}

var (
	_ TrackNarrativeWriter = (*MySQLTrackRepository)(nil)
	_ TrackNarrativeWriter = (*MongoTrackRepository)(nil)
	_ TrackNarrativeWriter = (*InMemoryTrackRepository)(nil)
)

func (r *MySQLTrackRepository) ApplyTrackNarrative(ctx context.Context, expected *models.Track, description, shape string, writeDescription, writeShape bool) (bool, error) {
	if !writeDescription && !writeShape {
		return true, nil
	}
	query := "UPDATE track_records SET updated_at=?"
	args := []interface{}{time.Now()}
	if writeDescription {
		query += ",route_description=?,description_source='ai'"
		args = append(args, description)
	}
	if writeShape {
		query += ",shape_label=?"
		args = append(args, shape)
	}
	query += " WHERE id=? AND updated_at=? AND status=1 AND is_running=0 AND deleted_at IS NULL AND COALESCE(route_description,'')=? AND COALESCE(NULLIF(description_source,''),'auto')=? AND shape_label=? AND raw_track_url=?"
	args = append(args, expected.ID, expected.UpdatedAt, expected.Description, descriptionSourceValue(expected.DescriptionSource), expected.ShapeLabel, expected.RawTrackURL)
	if writeDescription {
		query += " AND COALESCE(description_source,'auto') IN ('','auto','ai')"
	}
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n > 0, err
}

func (r *MongoTrackRepository) ApplyTrackNarrative(ctx context.Context, expected *models.Track, description, shape string, writeDescription, writeShape bool) (bool, error) {
	if !writeDescription && !writeShape {
		return true, nil
	}
	filter := bson.M{"_id": expected.ID, "updated_at": expected.UpdatedAt, "status": models.TrackStatusNormal, "is_running": false, "raw_track_url": expected.RawTrackURL}
	filter["description"] = bson.M{"$in": bson.A{expected.Description}}
	if expected.Description == "" {
		filter["description"] = bson.M{"$in": bson.A{"", nil}}
	}
	filter["shape_label"] = expected.ShapeLabel
	if expected.ShapeLabel == "" {
		filter["shape_label"] = bson.M{"$in": bson.A{"", nil}}
	}
	filter["description_source"] = expected.DescriptionSource
	if descriptionSourceValue(expected.DescriptionSource) == "auto" {
		filter["description_source"] = bson.M{"$in": bson.A{"auto", "", nil}}
	}
	if writeDescription && expected.DescriptionSource != "" && expected.DescriptionSource != "auto" && expected.DescriptionSource != "ai" {
		return false, nil
	}
	set := bson.M{"updated_at": time.Now()}
	if writeDescription {
		set["description"] = description
		set["description_source"] = "ai"
	}
	if writeShape {
		set["shape_label"] = shape
	}
	result, err := r.collection.UpdateOne(ctx, filter, bson.M{"$set": set})
	if err != nil {
		return false, err
	}
	return result.MatchedCount > 0, nil
}

func (r *InMemoryTrackRepository) ApplyTrackNarrative(_ context.Context, expected *models.Track, description, shape string, writeDescription, writeShape bool) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.tracks[expected.ID]
	if t == nil || t.Status != models.TrackStatusNormal || t.IsRunning || !t.UpdatedAt.Equal(expected.UpdatedAt) || t.Description != expected.Description || t.ShapeLabel != expected.ShapeLabel || t.RawTrackURL != expected.RawTrackURL || descriptionSourceValue(t.DescriptionSource) != descriptionSourceValue(expected.DescriptionSource) {
		return false, nil
	}
	if writeDescription {
		if t.DescriptionSource != "" && t.DescriptionSource != "auto" && t.DescriptionSource != "ai" {
			return false, nil
		}
		t.Description = description
		t.DescriptionSource = "ai"
	}
	if writeShape {
		t.ShapeLabel = shape
	}
	if writeDescription || writeShape {
		t.UpdatedAt = time.Now()
	}
	return true, nil
}
