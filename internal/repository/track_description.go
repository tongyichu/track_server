package repository

import (
	"context"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
)

// TrackDescriptionWriter saves an explicitly accepted description without resubmitting other fields.
type TrackDescriptionWriter interface {
	ApplyTrackDescription(context.Context, *models.Track, string) (bool, error)
}

var (
	_ TrackDescriptionWriter = (*MySQLTrackRepository)(nil)
	_ TrackDescriptionWriter = (*MongoTrackRepository)(nil)
	_ TrackDescriptionWriter = (*InMemoryTrackRepository)(nil)
)

func (r *MySQLTrackRepository) ApplyTrackDescription(ctx context.Context, expected *models.Track, description string) (bool, error) {
	result, err := r.db.ExecContext(ctx, "UPDATE track_records SET route_description=?,description_source='manual',description_rule_version=0,updated_at=? WHERE id=? AND updated_at=? AND status=1 AND is_running=0 AND deleted_at IS NULL AND COALESCE(route_description,'')=? AND COALESCE(NULLIF(description_source,''),'auto')=? AND COALESCE(description_rule_version,0)=? AND shape_label=? AND raw_track_url=?",
		description, time.Now(), expected.ID, expected.UpdatedAt, expected.Description, descriptionSourceValue(expected.DescriptionSource), expected.DescriptionRuleVersion, expected.ShapeLabel, expected.RawTrackURL)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n > 0, err
}

func (r *MongoTrackRepository) ApplyTrackDescription(ctx context.Context, expected *models.Track, description string) (bool, error) {
	filter := bson.M{"_id": expected.ID, "updated_at": expected.UpdatedAt, "status": models.TrackStatusNormal, "is_running": false, "raw_track_url": expected.RawTrackURL,
		"description": expected.Description, "description_source": expected.DescriptionSource, "description_rule_version": expected.DescriptionRuleVersion, "shape_label": expected.ShapeLabel}
	if expected.Description == "" {
		filter["description"] = bson.M{"$in": bson.A{"", nil}}
	}
	if expected.ShapeLabel == "" {
		filter["shape_label"] = bson.M{"$in": bson.A{"", nil}}
	}
	if descriptionSourceValue(expected.DescriptionSource) == "auto" {
		filter["description_source"] = bson.M{"$in": bson.A{"auto", "", nil}}
	}
	if expected.DescriptionRuleVersion == 0 {
		filter["description_rule_version"] = bson.M{"$in": bson.A{0, nil}}
	}
	result, err := r.collection.UpdateOne(ctx, filter, bson.M{"$set": bson.M{"description": description, "description_source": "manual", "description_rule_version": 0, "updated_at": time.Now()}})
	if err != nil {
		return false, err
	}
	return result.MatchedCount > 0, nil
}

func (r *InMemoryTrackRepository) ApplyTrackDescription(_ context.Context, expected *models.Track, description string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.tracks[expected.ID]
	if t == nil || t.Status != models.TrackStatusNormal || t.IsRunning || !t.UpdatedAt.Equal(expected.UpdatedAt) || t.Description != expected.Description || t.ShapeLabel != expected.ShapeLabel || t.RawTrackURL != expected.RawTrackURL || t.DescriptionRuleVersion != expected.DescriptionRuleVersion || descriptionSourceValue(t.DescriptionSource) != descriptionSourceValue(expected.DescriptionSource) {
		return false, nil
	}
	t.Description = description
	t.DescriptionSource = "manual"
	t.DescriptionRuleVersion = 0
	t.UpdatedAt = time.Now()
	return true, nil
}
