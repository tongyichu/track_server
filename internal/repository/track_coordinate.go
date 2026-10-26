package repository

import (
	"context"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
)

type TrackRawCoordinateWriter interface {
	RepairRawCoordinateSystem(ctx context.Context, id, raw, old, source, next string) (bool, error)
}

var (
	_ TrackRawCoordinateWriter = (*MySQLTrackRepository)(nil)
	_ TrackRawCoordinateWriter = (*MongoTrackRepository)(nil)
	_ TrackRawCoordinateWriter = (*InMemoryTrackRepository)(nil)
)

func (r *MySQLTrackRepository) RepairRawCoordinateSystem(ctx context.Context, id, raw, old, source, next string) (bool, error) {
	result, err := r.db.ExecContext(ctx, `UPDATE track_records SET coordinate_system=?,updated_at=? WHERE id=? AND raw_track_url=? AND coordinate_system=? AND source_tag=? AND status=1 AND is_running=0`, next, time.Now(), id, raw, old, source)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n > 0, err
}

func (r *MongoTrackRepository) RepairRawCoordinateSystem(ctx context.Context, id, raw, old, source, next string) (bool, error) {
	filter := bson.M{"_id": id, "raw_track_url": raw, "coordinate_system": old, "status": models.TrackStatusNormal, "is_running": false}
	if source == "" {
		filter["$or"] = bson.A{bson.M{"source_tag": ""}, bson.M{"source_tag": nil}}
	} else {
		filter["source_tag"] = source
	}
	result, err := r.collection.UpdateOne(ctx, filter, bson.M{"$set": bson.M{"coordinate_system": next, "updated_at": time.Now()}})
	if err != nil {
		return false, err
	}
	return result.ModifiedCount > 0, nil
}

func (r *InMemoryTrackRepository) RepairRawCoordinateSystem(_ context.Context, id, raw, old, source, next string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.tracks[id]
	if t == nil || t.RawTrackURL != raw || t.CoordinateSystem != old || t.SourceTag != source || t.Status != models.TrackStatusNormal || t.IsRunning {
		return false, nil
	}
	copy := *t
	copy.CoordinateSystem = next
	copy.UpdatedAt = time.Now()
	r.tracks[id] = &copy
	return true, nil
}
