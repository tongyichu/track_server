package repository

import (
	"context"
	"regexp"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoScenicSpotRepository struct{ spots, links, audits, usage *mongo.Collection }

func NewMongoScenicSpotRepository(db *mongo.Database) *MongoScenicSpotRepository {
	return &MongoScenicSpotRepository{spots: db.Collection("scenic_spots"), links: db.Collection("track_scenic_spot_links"), audits: db.Collection("scenic_spot_audit_logs"), usage: db.Collection("scenic_spot_provider_usage")}
}
func (r *MongoScenicSpotRepository) EnsureIndexes(ctx context.Context) error {
	for _, item := range []struct {
		collection *mongo.Collection
		model      mongo.IndexModel
	}{
		{r.spots, mongo.IndexModel{Keys: bson.D{{Key: "status", Value: 1}, {Key: "name", Value: 1}, {Key: "_id", Value: 1}}, Options: options.Index().SetName("idx_scenic_spot_status_name")}},
		{r.spots, mongo.IndexModel{Keys: bson.D{{Key: "sources.provider", Value: 1}, {Key: "sources.provider_id", Value: 1}}, Options: options.Index().SetName("idx_scenic_spot_provider_source")}},
		{r.links, mongo.IndexModel{Keys: bson.D{{Key: "scenic_spot_id", Value: 1}, {Key: "_id", Value: 1}}, Options: options.Index().SetName("idx_track_scenic_spot_spot_track")}},
		{r.audits, mongo.IndexModel{Keys: bson.D{{Key: "scenic_spot_id", Value: 1}, {Key: "_id", Value: -1}}, Options: options.Index().SetName("idx_scenic_spot_audit_spot_id")}},
		{r.usage, mongo.IndexModel{Keys: bson.D{{Key: "usage_day", Value: 1}, {Key: "provider", Value: 1}, {Key: "scenic_spot_id", Value: 1}}, Options: options.Index().SetName("idx_scenic_provider_usage_day")}},
	} {
		if _, err := item.collection.Indexes().CreateOne(ctx, item.model); err != nil {
			return err
		}
	}
	return nil
}
func (r *MongoScenicSpotRepository) CreateScenicSpot(ctx context.Context, v *models.ScenicSpot) error {
	_, err := r.spots.InsertOne(ctx, v)
	if mongo.IsDuplicateKeyError(err) {
		return ErrAlreadyExists
	}
	return err
}
func (r *MongoScenicSpotRepository) GetScenicSpot(ctx context.Context, id string) (*models.ScenicSpot, error) {
	v := &models.ScenicSpot{}
	err := r.spots.FindOne(ctx, bson.M{"_id": id}).Decode(v)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return v, err
}
func (r *MongoScenicSpotRepository) GetScenicSpots(ctx context.Context, ids []string) (map[string]*models.ScenicSpot, error) {
	out := map[string]*models.ScenicSpot{}
	if len(ids) == 0 {
		return out, nil
	}
	cur, err := r.spots.Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		v := &models.ScenicSpot{}
		if err := cur.Decode(v); err != nil {
			return nil, err
		}
		out[v.ID] = v
	}
	return out, cur.Err()
}
func (r *MongoScenicSpotRepository) FindScenicSpotBySource(ctx context.Context, provider, providerID string) (*models.ScenicSpot, error) {
	v := &models.ScenicSpot{}
	err := r.spots.FindOne(ctx, bson.M{"status": bson.M{"$ne": models.ScenicSpotMerged}, "sources": bson.M{"$elemMatch": bson.M{"provider": provider, "provider_id": providerID}}}).Decode(v)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return v, err
}
func (r *MongoScenicSpotRepository) ListScenicSpots(ctx context.Context, f ScenicSpotListFilter) ([]*models.ScenicSpot, int64, error) {
	filter := bson.M{}
	if f.Status != "" {
		filter["status"] = f.Status
	}
	if f.Query != "" {
		filter["name"] = bson.M{"$regex": regexp.QuoteMeta(f.Query), "$options": "i"}
	}
	if f.Cursor != "" {
		filter["_id"] = bson.M{"$gt": f.Cursor}
	}
	base := bson.M{}
	if f.Status != "" {
		base["status"] = f.Status
	}
	if f.Query != "" {
		base["name"] = bson.M{"$regex": regexp.QuoteMeta(f.Query), "$options": "i"}
	}
	total, err := r.spots.CountDocuments(ctx, base)
	if err != nil {
		return nil, 0, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	cur, err := r.spots.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, 0, err
	}
	defer cur.Close(ctx)
	out := []*models.ScenicSpot{}
	for cur.Next(ctx) {
		v := &models.ScenicSpot{}
		if err := cur.Decode(v); err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, cur.Err()
}
func (r *MongoScenicSpotRepository) UpdateScenicSpot(ctx context.Context, v *models.ScenicSpot, expected int64) error {
	res, err := r.spots.ReplaceOne(ctx, bson.M{"_id": v.ID, "version": expected}, v)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrConflict
	}
	return nil
}
func (r *MongoScenicSpotRepository) GetTrackScenicSpotLink(ctx context.Context, id string) (*models.TrackScenicSpotLink, error) {
	v := &models.TrackScenicSpotLink{}
	err := r.links.FindOne(ctx, bson.M{"_id": id}).Decode(v)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return v, err
}
func (r *MongoScenicSpotRepository) GetTrackScenicSpotLinks(ctx context.Context, ids []string) (map[string]*models.TrackScenicSpotLink, error) {
	out := map[string]*models.TrackScenicSpotLink{}
	if len(ids) == 0 {
		return out, nil
	}
	cur, err := r.links.Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		v := &models.TrackScenicSpotLink{}
		if err := cur.Decode(v); err != nil {
			return nil, err
		}
		out[v.TrackID] = v
	}
	return out, cur.Err()
}
func (r *MongoScenicSpotRepository) SetTrackScenicSpotLink(ctx context.Context, v *models.TrackScenicSpotLink, expected string, absent bool) error {
	filter := bson.M{"_id": v.TrackID}
	if absent {
		_, err := r.links.InsertOne(ctx, v)
		if mongo.IsDuplicateKeyError(err) {
			return ErrConflict
		}
		return err
	}
	filter["scenic_spot_id"] = expected
	res, err := r.links.ReplaceOne(ctx, filter, v)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrConflict
	}
	return nil
}
func (r *MongoScenicSpotRepository) DeleteTrackScenicSpotLink(ctx context.Context, id, expected string) error {
	res, err := r.links.DeleteOne(ctx, bson.M{"_id": id, "scenic_spot_id": expected})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrConflict
	}
	return nil
}
func (r *MongoScenicSpotRepository) ListTrackScenicSpotLinks(ctx context.Context, spotID, cursor string, limit int) ([]*models.TrackScenicSpotLink, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	f := bson.M{"scenic_spot_id": spotID}
	if cursor != "" {
		f["_id"] = bson.M{"$gt": cursor}
	}
	cur, err := r.links.Find(ctx, f, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []*models.TrackScenicSpotLink{}
	for cur.Next(ctx) {
		v := &models.TrackScenicSpotLink{}
		if err := cur.Decode(v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, cur.Err()
}
func (r *MongoScenicSpotRepository) AppendScenicSpotAudit(ctx context.Context, v *models.ScenicSpotAudit) error {
	_, err := r.audits.InsertOne(ctx, v)
	return err
}
func (r *MongoScenicSpotRepository) ListScenicSpotAudit(ctx context.Context, spotID, cursor string, limit int) ([]*models.ScenicSpotAudit, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	f := bson.M{"scenic_spot_id": spotID}
	if cursor != "" {
		f["_id"] = bson.M{"$lt": cursor}
	}
	cur, err := r.audits.Find(ctx, f, options.Find().SetSort(bson.D{{Key: "_id", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []*models.ScenicSpotAudit{}
	for cur.Next(ctx) {
		v := &models.ScenicSpotAudit{}
		if err := cur.Decode(v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, cur.Err()
}

func (r *MongoScenicSpotRepository) IncrementScenicSpotProviderUsage(ctx context.Context, spotID, provider, day, operation string, succeeded bool) error {
	id := scenicUsageKey(spotID, provider, day, operation)
	inc := bson.M{"requests": 1, "successes": 0, "failures": 0}
	if succeeded {
		inc["successes"] = 1
	} else {
		inc["failures"] = 1
	}
	_, err := r.usage.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$inc": inc, "$setOnInsert": bson.M{"scenic_spot_id": spotID, "provider": provider, "usage_day": day, "operation": operation}}, options.Update().SetUpsert(true))
	return err
}
func (r *MongoScenicSpotRepository) ListScenicSpotProviderUsage(ctx context.Context, fromDay, toDay string) ([]*models.ScenicSpotProviderUsage, error) {
	cur, err := r.usage.Find(ctx, bson.M{"usage_day": bson.M{"$gte": fromDay, "$lte": toDay}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []*models.ScenicSpotProviderUsage{}
	for cur.Next(ctx) {
		v := &models.ScenicSpotProviderUsage{}
		if err := cur.Decode(v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, cur.Err()
}
func (r *MongoScenicSpotRepository) CountScenicSpotLinksBySpotIDs(ctx context.Context, spotIDs []string) (map[string]int64, error) {
	ids := uniqueNonEmptyStrings(spotIDs)
	out := make(map[string]int64, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"scenic_spot_id": bson.M{"$in": ids}}}},
		{{Key: "$group", Value: bson.M{"_id": "$scenic_spot_id", "count": bson.M{"$sum": 1}}}},
	}
	cur, err := r.links.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var item struct {
			ID    string `bson:"_id"`
			Count int64  `bson:"count"`
		}
		if err := cur.Decode(&item); err != nil {
			return nil, err
		}
		out[item.ID] = item.Count
	}
	return out, cur.Err()
}

var _ ScenicSpotRepository = (*MongoScenicSpotRepository)(nil)
