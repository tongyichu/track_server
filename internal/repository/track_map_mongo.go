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
	jobs          *mongo.Collection
	index         *mongo.Collection
	tracks        *mongo.Collection
	groups        *mongo.Collection
	members       *mongo.Collection
	introductions *mongo.Collection
}

func NewMongoTrackMapRepository(jobs, index, tracks, groups, members *mongo.Collection) *MongoTrackMapRepository {
	return &MongoTrackMapRepository{jobs: jobs, index: index, tracks: tracks, groups: groups, members: members, introductions: groups.Database().Collection("track_route_introductions")}
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
	if err == nil && existing.Status == models.TrackMapIndexJobPending {
		set := bson.M{
			"status":     models.TrackMapIndexJobPending,
			"updated_at": now,
		}
		if existing.NextRunAt.IsZero() || runAt.Before(existing.NextRunAt) {
			set["next_run_at"] = runAt
		}
		_, err = r.jobs.UpdateOne(ctx,
			bson.M{"track_id": trackID},
			bson.M{"$set": set},
		)
		return err
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

// CleanupDeletedTrack removes map-index side records for a deleted track.
func (r *MongoTrackMapRepository) CleanupDeletedTrack(ctx context.Context, trackID string) error {
	trackID = strings.TrimSpace(trackID)
	if trackID == "" {
		return nil
	}
	cur, err := r.members.Find(ctx, bson.M{"track_id": trackID})
	if err != nil {
		return err
	}
	defer cur.Close(ctx)
	var members []models.TrackRouteGroupMember
	if err := cur.All(ctx, &members); err != nil {
		return err
	}
	if _, err := r.jobs.DeleteOne(ctx, bson.M{"track_id": trackID}); err != nil {
		return err
	}
	if _, err := r.members.DeleteMany(ctx, bson.M{"track_id": trackID}); err != nil {
		return err
	}
	if _, err := r.index.DeleteOne(ctx, bson.M{"track_id": trackID}); err != nil {
		return err
	}
	now := time.Now()
	for _, member := range members {
		if member.GroupID == "" {
			continue
		}
		var group models.TrackRouteGroup
		err := r.groups.FindOne(ctx, bson.M{"group_id": member.GroupID}).Decode(&group)
		if errors.Is(err, mongo.ErrNoDocuments) {
			continue
		}
		if err != nil {
			return err
		}
		if group.RepresentativeTrackID == trackID {
			if _, err := r.members.DeleteMany(ctx, bson.M{"group_id": member.GroupID}); err != nil {
				return err
			}
			_, err = r.groups.UpdateOne(ctx, bson.M{"group_id": member.GroupID}, bson.M{"$set": bson.M{
				"status":       models.TrackRouteGroupStatusArchived,
				"member_count": 0,
				"updated_at":   now,
			}})
			if err != nil {
				return err
			}
			continue
		}
		count, err := r.members.CountDocuments(ctx, bson.M{"group_id": member.GroupID})
		if err != nil {
			return err
		}
		if _, err := r.groups.UpdateOne(ctx, bson.M{"group_id": member.GroupID}, bson.M{"$set": bson.M{"member_count": count, "updated_at": now}}); err != nil {
			return err
		}
	}
	return nil
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

func (r *MongoTrackMapRepository) ListAllTrackGeoIndexes(ctx context.Context) ([]*models.TrackGeoIndex, error) {
	cur, err := r.index.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "track_type", Value: 1}, {Key: "center_lat", Value: 1}, {Key: "center_lng", Value: 1}, {Key: "track_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	items := make([]*models.TrackGeoIndex, 0)
	for cur.Next(ctx) {
		var index models.TrackGeoIndex
		if err := cur.Decode(&index); err != nil {
			return nil, err
		}
		items = append(items, &index)
	}
	return items, cur.Err()
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

func (r *MongoTrackMapRepository) FindRouteGroup(ctx context.Context, groupID string) (*models.TrackRouteGroup, error) {
	var group models.TrackRouteGroup
	err := r.groups.FindOne(ctx, bson.M{"_id": groupID, "status": models.TrackRouteGroupStatusActive}).Decode(&group)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if group.GroupID == "" {
		group.GroupID = groupID
	}
	return &group, nil
}

func (r *MongoTrackMapRepository) ListRouteGroups(ctx context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackRouteGroup, error) {
	limit := normalizeTrackMapQueryLimit(filter.Limit)
	cur, err := r.groups.Find(ctx, mongoTrackRouteGroupFilter(filter), options.Find().SetSort(bson.D{{Key: "member_count", Value: -1}, {Key: "updated_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	items := make([]*models.TrackRouteGroup, 0, limit)
	for cur.Next(ctx) {
		var group models.TrackRouteGroup
		if err := cur.Decode(&group); err != nil {
			return nil, err
		}
		items = append(items, &group)
	}
	return items, cur.Err()
}

func (r *MongoTrackMapRepository) ListRouteGroupSummaries(ctx context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackRouteGroup, error) {
	limit := normalizeTrackMapQueryLimit(filter.Limit)
	opts := options.Find().
		SetSort(bson.D{{Key: "member_count", Value: -1}, {Key: "updated_at", Value: -1}}).
		SetLimit(int64(limit))
	cur, err := r.groups.Find(ctx, mongoTrackRouteGroupFilter(filter), opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	items := make([]*models.TrackRouteGroup, 0, limit)
	for cur.Next(ctx) {
		var group models.TrackRouteGroup
		if err := cur.Decode(&group); err != nil {
			return nil, err
		}
		items = append(items, &group)
	}
	return items, cur.Err()
}

func (r *MongoTrackMapRepository) ListAllRouteGroups(ctx context.Context) ([]*models.TrackRouteGroup, error) {
	cur, err := r.groups.Find(ctx, bson.M{"status": models.TrackRouteGroupStatusActive}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	items := make([]*models.TrackRouteGroup, 0)
	for cur.Next(ctx) {
		var group models.TrackRouteGroup
		if err := cur.Decode(&group); err != nil {
			return nil, err
		}
		items = append(items, &group)
	}
	return items, cur.Err()
}

func (r *MongoTrackMapRepository) ListAllRouteGroupMembers(ctx context.Context) ([]*models.TrackRouteGroupMember, error) {
	cur, err := r.members.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "group_id", Value: 1}, {Key: "track_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	items := make([]*models.TrackRouteGroupMember, 0)
	for cur.Next(ctx) {
		var member models.TrackRouteGroupMember
		if err := cur.Decode(&member); err != nil {
			return nil, err
		}
		items = append(items, &member)
	}
	return items, cur.Err()
}

func (r *MongoTrackMapRepository) ListRouteGroupCandidates(ctx context.Context, index *models.TrackGeoIndex, limit int) ([]*models.TrackRouteGroupCandidate, error) {
	if index == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	filter := mongoTrackRouteGroupFilter(models.TrackMapQueryFilter{
		TrackType: index.TrackType,
		BBox: &models.TrackMapBBox{
			MinLatitude:  index.MinLat - 0.02,
			MinLongitude: index.MinLng - 0.02,
			MaxLatitude:  index.MaxLat + 0.02,
			MaxLongitude: index.MaxLng + 0.02,
		},
	})
	cur, err := r.groups.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "member_count", Value: -1}, {Key: "updated_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	items := make([]*models.TrackRouteGroupCandidate, 0, limit)
	for cur.Next(ctx) {
		var group models.TrackRouteGroup
		if err := cur.Decode(&group); err != nil {
			return nil, err
		}
		geoIndex, err := r.FindTrackGeoIndex(ctx, group.RepresentativeTrackID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		items = append(items, &models.TrackRouteGroupCandidate{Group: &group, Index: geoIndex})
	}
	return items, cur.Err()
}

func (r *MongoTrackMapRepository) ListGeoIndexesWithoutRouteGroup(ctx context.Context, limit int) ([]*models.TrackGeoIndex, error) {
	if limit <= 0 {
		limit = 100
	}
	cur, err := r.index.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}).SetLimit(int64(limit*2)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	items := make([]*models.TrackGeoIndex, 0, limit)
	for cur.Next(ctx) {
		if len(items) >= limit {
			break
		}
		var index models.TrackGeoIndex
		if err := cur.Decode(&index); err != nil {
			return nil, err
		}
		err := r.members.FindOne(ctx, bson.M{"track_id": index.TrackID}).Err()
		if err == nil {
			continue
		}
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		items = append(items, &index)
	}
	return items, cur.Err()
}

func (r *MongoTrackMapRepository) UpsertRouteGroup(ctx context.Context, group *models.TrackRouteGroup) error {
	if group == nil || group.GroupID == "" {
		return errors.New("route group is required")
	}
	now := time.Now()
	if group.CreatedAt.IsZero() {
		group.CreatedAt = now
	}
	group.UpdatedAt = now
	_, err := r.groups.ReplaceOne(ctx,
		bson.M{"_id": group.GroupID},
		group,
		options.Replace().SetUpsert(true),
	)
	return err
}

func (r *MongoTrackMapRepository) UpsertRouteGroupMember(ctx context.Context, member *models.TrackRouteGroupMember) error {
	if member == nil || member.GroupID == "" || member.TrackID == "" {
		return errors.New("route group member is required")
	}
	now := time.Now()
	if member.CreatedAt.IsZero() {
		member.CreatedAt = now
	}
	member.UpdatedAt = now
	_, err := r.members.UpdateOne(ctx,
		bson.M{"track_id": member.TrackID},
		bson.M{"$set": member},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return err
	}
	count, err := r.members.CountDocuments(ctx, bson.M{"group_id": member.GroupID})
	if err != nil {
		return err
	}
	_, err = r.groups.UpdateOne(ctx, bson.M{"_id": member.GroupID}, bson.M{"$set": bson.M{"member_count": count, "updated_at": now}})
	return err
}

func (r *MongoTrackMapRepository) ReplaceRouteGroups(ctx context.Context, groups []*models.TrackRouteGroup, members []*models.TrackRouteGroupMember) error {
	if _, err := r.members.DeleteMany(ctx, bson.M{}); err != nil {
		return err
	}
	if _, err := r.groups.DeleteMany(ctx, bson.M{}); err != nil {
		return err
	}
	if len(groups) > 0 {
		docs := make([]interface{}, 0, len(groups))
		now := time.Now()
		for _, group := range groups {
			if group == nil || group.GroupID == "" {
				continue
			}
			if group.CreatedAt.IsZero() {
				group.CreatedAt = now
			}
			if group.UpdatedAt.IsZero() {
				group.UpdatedAt = now
			}
			docs = append(docs, group)
		}
		if len(docs) > 0 {
			if _, err := r.groups.InsertMany(ctx, docs); err != nil {
				return err
			}
		}
	}
	if len(members) > 0 {
		docs := make([]interface{}, 0, len(members))
		now := time.Now()
		for _, member := range members {
			if member == nil || member.GroupID == "" || member.TrackID == "" {
				continue
			}
			if member.CreatedAt.IsZero() {
				member.CreatedAt = now
			}
			if member.UpdatedAt.IsZero() {
				member.UpdatedAt = now
			}
			docs = append(docs, member)
		}
		if len(docs) > 0 {
			if _, err := r.members.InsertMany(ctx, docs); err != nil {
				return err
			}
		}
	}
	cursor, err := r.introductions.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "id", Value: 1}}))
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)
	boundGroups := make(map[string]struct{})
	for cursor.Next(ctx) {
		var introduction models.TrackRouteIntroduction
		if err := cursor.Decode(&introduction); err != nil {
			return err
		}
		var member models.TrackRouteGroupMember
		groupID := ""
		if err := r.members.FindOne(ctx, bson.M{"track_id": introduction.AnchorTrackID}).Decode(&member); err == nil {
			groupID = member.GroupID
		} else if !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
		status := introduction.Status
		if groupID != "" {
			if _, exists := boundGroups[groupID]; exists {
				groupID, status = "", models.TrackRouteIntroductionStatusArchived
			} else {
				boundGroups[groupID] = struct{}{}
			}
		}
		if _, err := r.introductions.UpdateOne(ctx, bson.M{"anchor_track_id": introduction.AnchorTrackID}, bson.M{"$set": bson.M{"current_group_id": groupID, "status": status, "updated_at": time.Now()}}); err != nil {
			return err
		}
	}
	if err := cursor.Err(); err != nil {
		return err
	}
	return nil
}

func (r *MongoTrackMapRepository) FindRouteIntroductionByGroupID(ctx context.Context, groupID string) (*models.TrackRouteIntroduction, error) {
	var introduction models.TrackRouteIntroduction
	err := r.introductions.FindOne(ctx, bson.M{"current_group_id": strings.TrimSpace(groupID)}).Decode(&introduction)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	return &introduction, err
}

func (r *MongoTrackMapRepository) ListPublishedRouteIntroductions(ctx context.Context, groupIDs []string) (map[string]*models.TrackRouteIntroduction, error) {
	result := make(map[string]*models.TrackRouteIntroduction)
	if len(groupIDs) == 0 {
		return result, nil
	}
	cursor, err := r.introductions.Find(ctx, bson.M{"status": models.TrackRouteIntroductionStatusPublished, "current_group_id": bson.M{"$in": groupIDs}})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var introduction models.TrackRouteIntroduction
		if err := cursor.Decode(&introduction); err != nil {
			return nil, err
		}
		clone := introduction
		result[introduction.CurrentGroupID] = &clone
	}
	return result, cursor.Err()
}

func (r *MongoTrackMapRepository) UpsertRouteIntroduction(ctx context.Context, introduction *models.TrackRouteIntroduction) error {
	if introduction == nil || strings.TrimSpace(introduction.AnchorTrackID) == "" {
		return errors.New("route introduction is required")
	}
	now := time.Now()
	if introduction.ID == 0 {
		introduction.ID = now.UnixNano()
	}
	if introduction.CreatedAt.IsZero() {
		introduction.CreatedAt = now
	}
	introduction.UpdatedAt = now
	filter := bson.M{"anchor_track_id": introduction.AnchorTrackID}
	if introduction.ID != 0 {
		filter = bson.M{"id": introduction.ID}
	}
	_, err := r.introductions.ReplaceOne(ctx, filter, introduction, options.Replace().SetUpsert(true))
	return err
}

func (r *MongoTrackMapRepository) DeleteRouteGroupMember(ctx context.Context, groupID, trackID string) error {
	res, err := r.members.DeleteOne(ctx, bson.M{"group_id": groupID, "track_id": trackID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	now := time.Now()
	count, err := r.members.CountDocuments(ctx, bson.M{"group_id": groupID})
	if err != nil {
		return err
	}
	_, err = r.groups.UpdateOne(ctx, bson.M{"_id": groupID}, bson.M{"$set": bson.M{"member_count": count, "updated_at": now}})
	return err
}

func (r *MongoTrackMapRepository) ArchiveRouteGroup(ctx context.Context, groupID string, now time.Time) error {
	res, err := r.groups.UpdateOne(ctx, bson.M{"_id": groupID}, bson.M{"$set": bson.M{"status": models.TrackRouteGroupStatusArchived, "updated_at": now}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MongoTrackMapRepository) ListRouteGroupMembers(ctx context.Context, groupID string, limit int) ([]*models.TrackRouteGroupMember, error) {
	if limit <= 0 {
		limit = 20
	}
	cur, err := r.members.Find(ctx, bson.M{"group_id": groupID}, options.Find().SetSort(bson.D{{Key: "role", Value: -1}, {Key: "created_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	items := make([]*models.TrackRouteGroupMember, 0, limit)
	for cur.Next(ctx) {
		var member models.TrackRouteGroupMember
		if err := cur.Decode(&member); err != nil {
			return nil, err
		}
		items = append(items, &member)
	}
	return items, cur.Err()
}

func (r *MongoTrackMapRepository) FindRouteGroupByTrackID(ctx context.Context, trackID string) (*models.TrackRouteGroup, error) {
	var member models.TrackRouteGroupMember
	err := r.members.FindOne(ctx, bson.M{"track_id": trackID}).Decode(&member)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return r.FindRouteGroup(ctx, member.GroupID)
}

func (r *MongoTrackMapRepository) CountRouteGroupsByCity(ctx context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackMapClusterItem, error) {
	groups, err := r.ListRouteGroups(ctx, filter)
	if err != nil {
		return nil, err
	}
	accs := make(map[string]*trackMapClusterAcc)
	for _, group := range groups {
		codes := group.CityCodes
		if len(codes) == 0 {
			codes = []string{""}
		}
		for _, code := range codes {
			key := code
			if key == "" {
				key = "unknown"
			}
			a := accs[key]
			if a == nil {
				a = &trackMapClusterAcc{}
				accs[key] = a
			}
			a.addRouteGroup(group)
		}
	}
	return sortedClusterItems(accs, "city_cluster", filter.TrackType, filter.Limit), nil
}

func (r *MongoTrackMapRepository) CountRouteGroupsByArea(ctx context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackMapClusterItem, error) {
	groups, err := r.ListRouteGroups(ctx, filter)
	if err != nil {
		return nil, err
	}
	accs := make(map[string]*trackMapClusterAcc)
	for _, group := range groups {
		key, areaID := trackMapRouteGroupAreaClusterKey(group)
		a := accs[key]
		if a == nil {
			a = &trackMapClusterAcc{}
			accs[key] = a
		}
		a.areaID = areaID
		a.addRouteGroup(group)
	}
	return sortedClusterItems(accs, "area_cluster", filter.TrackType, filter.Limit), nil
}

func mongoTrackRouteGroupFilter(filter models.TrackMapQueryFilter) bson.M {
	conds := []bson.M{{"status": models.TrackRouteGroupStatusActive}}
	if filter.TrackType != "" {
		conds = append(conds, bson.M{"track_type": filter.TrackType})
	}
	if filter.CityCode != "" {
		conds = append(conds, bson.M{"city_codes": filter.CityCode})
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
	if len(conds) == 1 {
		return conds[0]
	}
	return bson.M{"$and": conds}
}
