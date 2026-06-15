package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoTrackMapRepository implements TrackMapRepository using MongoDB.
type MongoTrackMapRepository struct {
	jobs   *mongo.Collection
	index  *mongo.Collection
	tracks *mongo.Collection
}

func NewMongoTrackMapRepository(jobs, index, tracks *mongo.Collection) *MongoTrackMapRepository {
	return &MongoTrackMapRepository{jobs: jobs, index: index, tracks: tracks}
}

func (r *MongoTrackMapRepository) EnqueueIndexJob(ctx context.Context, trackID string, runAt time.Time) error {
	trackID = strings.TrimSpace(trackID)
	if trackID == "" {
		return errors.New("track id is required")
	}
	now := time.Now()
	var existing models.TrackMapIndexJob
	err := r.jobs.FindOne(ctx, bson.M{"track_id": trackID}).Decode(&existing)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return err
	}
	if err == nil && (existing.Status == models.TrackMapIndexJobSucceeded || existing.Status == models.TrackMapIndexJobProcessing) {
		return nil
	}
	update := bson.M{
		"$setOnInsert": bson.M{
			"track_id":   trackID,
			"attempts":   0,
			"created_at": now,
		},
		"$set": bson.M{
			"status":      models.TrackMapIndexJobPending,
			"next_run_at": runAt,
			"updated_at":  now,
		},
	}
	_, err = r.jobs.UpdateOne(ctx,
		bson.M{"track_id": trackID},
		update,
		options.Update().SetUpsert(true),
	)
	return err
}

func (r *MongoTrackMapRepository) ClaimPendingIndexJobs(ctx context.Context, workerID string, now time.Time, limit int) ([]*models.TrackMapIndexJob, error) {
	if limit <= 0 {
		limit = 10
	}
	cur, err := r.jobs.Find(ctx,
		bson.M{"$or": []bson.M{
			{"status": models.TrackMapIndexJobPending, "next_run_at": bson.M{"$lte": now}},
			{"status": models.TrackMapIndexJobProcessing, "locked_at": bson.M{"$lt": now.Add(-30 * time.Minute)}},
		}},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}).SetLimit(int64(limit)),
	)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	jobs := make([]*models.TrackMapIndexJob, 0, limit)
	for cur.Next(ctx) {
		var job models.TrackMapIndexJob
		if err := cur.Decode(&job); err != nil {
			return nil, err
		}
		res, err := r.jobs.UpdateOne(ctx,
			bson.M{"track_id": job.TrackID, "$or": []bson.M{
				{"status": models.TrackMapIndexJobPending},
				{"status": models.TrackMapIndexJobProcessing, "locked_at": bson.M{"$lt": now.Add(-30 * time.Minute)}},
			}},
			bson.M{"$set": bson.M{
				"status":     models.TrackMapIndexJobProcessing,
				"locked_at":  now,
				"locked_by":  workerID,
				"updated_at": now,
			}},
		)
		if err != nil {
			return nil, err
		}
		if res.ModifiedCount == 0 {
			continue
		}
		job.Status = models.TrackMapIndexJobProcessing
		job.LockedAt = now
		job.LockedBy = workerID
		job.UpdatedAt = now
		jobs = append(jobs, &job)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (r *MongoTrackMapRepository) MarkIndexJobSucceeded(ctx context.Context, trackID string, now time.Time) error {
	res, err := r.jobs.UpdateOne(ctx,
		bson.M{"track_id": trackID},
		bson.M{"$set": bson.M{
			"status":       models.TrackMapIndexJobSucceeded,
			"last_error":   "",
			"succeeded_at": now,
			"updated_at":   now,
		}},
	)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MongoTrackMapRepository) MarkIndexJobFailed(ctx context.Context, trackID, errMsg string, nextRunAt time.Time, now time.Time) error {
	if len(errMsg) > 512 {
		errMsg = errMsg[:512]
	}
	res, err := r.jobs.UpdateOne(ctx,
		bson.M{"track_id": trackID},
		bson.M{
			"$set": bson.M{
				"status":         models.TrackMapIndexJobPending,
				"last_error":     errMsg,
				"last_failed_at": now,
				"next_run_at":    nextRunAt,
				"updated_at":     now,
			},
			"$inc": bson.M{"attempts": 1},
		},
	)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MongoTrackMapRepository) UpsertTrackGeoIndex(ctx context.Context, index *models.TrackGeoIndex) error {
	if index == nil || index.TrackID == "" {
		return errors.New("track geo index is required")
	}
	now := time.Now()
	if index.CreatedAt.IsZero() {
		index.CreatedAt = now
	}
	index.UpdatedAt = now
	_, err := r.index.UpdateOne(ctx,
		bson.M{"track_id": index.TrackID},
		bson.M{"$set": index},
		options.Update().SetUpsert(true),
	)
	return err
}

func (r *MongoTrackMapRepository) HasTrackGeoIndex(ctx context.Context, trackID string) (bool, error) {
	err := r.index.FindOne(ctx, bson.M{"track_id": trackID}).Err()
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *MongoTrackMapRepository) ListCompletedTracksMissingGeoIndex(ctx context.Context, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 100
	}
	cur, err := r.tracks.Find(ctx,
		bson.M{
			"is_running":    false,
			"status":        models.TrackStatusNormal,
			"raw_track_url": bson.M{"$ne": ""},
		},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}).SetLimit(int64(limit*2)),
	)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	items := make([]*models.Track, 0, limit)
	for cur.Next(ctx) {
		if len(items) >= limit {
			break
		}
		var track models.Track
		if err := cur.Decode(&track); err != nil {
			return nil, err
		}
		ok, err := r.HasTrackGeoIndex(ctx, track.ID)
		if err != nil {
			return nil, err
		}
		if ok {
			continue
		}
		items = append(items, &track)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *MongoTrackMapRepository) FindTrackGeoIndex(ctx context.Context, trackID string) (*models.TrackGeoIndex, error) {
	var index models.TrackGeoIndex
	err := r.index.FindOne(ctx, bson.M{"track_id": trackID}).Decode(&index)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &index, nil
}

func (r *MongoTrackMapRepository) ListTrackGeoIndexes(ctx context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackGeoIndex, error) {
	limit := normalizeTrackMapQueryLimit(filter.Limit)
	cur, err := r.index.Find(ctx, mongoTrackGeoIndexFilter(filter), options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	items := make([]*models.TrackGeoIndex, 0, limit)
	for cur.Next(ctx) {
		var index models.TrackGeoIndex
		if err := cur.Decode(&index); err != nil {
			return nil, err
		}
		items = append(items, &index)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *MongoTrackMapRepository) CountTrackGeoIndexesByCity(ctx context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackMapClusterItem, error) {
	indexes, err := r.ListTrackGeoIndexes(ctx, filter)
	if err != nil {
		return nil, err
	}
	groups := make(map[string]*trackMapClusterAcc)
	for _, index := range indexes {
		key := index.CityCode
		if key == "" {
			key = "unknown"
		}
		a := groups[key]
		if a == nil {
			a = &trackMapClusterAcc{}
			groups[key] = a
		}
		a.add(index)
	}
	items := make([]*models.TrackMapClusterItem, 0, len(groups))
	for cityCode, a := range groups {
		items = append(items, a.item("city_cluster", "", cityCode, filter.TrackType))
	}
	return items, nil
}

func (r *MongoTrackMapRepository) CountTrackGeoIndexesByArea(ctx context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackMapClusterItem, error) {
	indexes, err := r.ListTrackGeoIndexes(ctx, filter)
	if err != nil {
		return nil, err
	}
	groups := make(map[string]*trackMapClusterAcc)
	for _, index := range indexes {
		key := trackMapAreaClusterKey(index.CenterLat, index.CenterLng)
		a := groups[key]
		if a == nil {
			a = &trackMapClusterAcc{}
			groups[key] = a
		}
		a.add(index)
	}
	items := make([]*models.TrackMapClusterItem, 0, len(groups))
	for key, a := range groups {
		items = append(items, a.item("area_cluster", key, "", filter.TrackType))
	}
	return items, nil
}

func mongoTrackGeoIndexFilter(filter models.TrackMapQueryFilter) bson.M {
	conds := make([]bson.M, 0, 4)
	if filter.TrackType != "" {
		conds = append(conds, bson.M{"track_type": filter.TrackType})
	}
	if filter.CityCode != "" {
		conds = append(conds, bson.M{"city_code": filter.CityCode})
	}
	if filter.BBox != nil {
		conds = append(conds, bson.M{
			"max_lat": bson.M{"$gte": filter.BBox.MinLatitude},
			"min_lat": bson.M{"$lte": filter.BBox.MaxLatitude},
			"max_lng": bson.M{"$gte": filter.BBox.MinLongitude},
			"min_lng": bson.M{"$lte": filter.BBox.MaxLongitude},
		})
	}
	if filter.Center != nil && filter.RadiusM > 0 {
		deg := float64(filter.RadiusM) / 111000.0
		conds = append(conds, bson.M{
			"center_lat": bson.M{"$gte": filter.Center.Latitude - deg, "$lte": filter.Center.Latitude + deg},
			"center_lng": bson.M{"$gte": filter.Center.Longitude - deg, "$lte": filter.Center.Longitude + deg},
		})
	}
	if len(conds) == 0 {
		return bson.M{}
	}
	if len(conds) == 1 {
		return conds[0]
	}
	return bson.M{"$and": conds}
}
