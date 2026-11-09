package repository

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoAIUsageRepository struct{ tasks, states *mongo.Collection }

func NewMongoAIUsageRepository(ctx context.Context, db *mongo.Database) (*MongoAIUsageRepository, error) {
	r := &MongoAIUsageRepository{db.Collection("ai_usage_tasks"), db.Collection("ai_usage_state")}
	_, err := r.tasks.Indexes().CreateMany(ctx, []mongo.IndexModel{{Keys: bson.D{{Key: "created_at", Value: 1}, {Key: "_id", Value: -1}}}, {Keys: bson.D{{Key: "status", Value: 1}, {Key: "_id", Value: -1}}}, {Keys: bson.D{{Key: "feature", Value: 1}, {Key: "created_at", Value: 1}}}, {Keys: bson.D{{Key: "provider", Value: 1}, {Key: "model", Value: 1}, {Key: "created_at", Value: 1}}}, {Keys: bson.D{{Key: "object_id", Value: 1}, {Key: "_id", Value: -1}}}})
	return r, err
}
func (r *MongoAIUsageRepository) GetAITask(ctx context.Context, id string) (*models.AITask, error) {
	var doc struct {
		Payload []byte `bson:"payload"`
	}
	err := r.tasks.FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var t models.AITask
	err = json.Unmarshal(doc.Payload, &t)
	return &t, err
}
func (r *MongoAIUsageRepository) SaveAITask(ctx context.Context, t *models.AITask, expected int64) error {
	raw, err := json.Marshal(t)
	if err != nil {
		return err
	}
	summary, err := json.Marshal(aiSummary(t))
	if err != nil {
		return err
	}
	doc := bson.M{"_id": t.ID, "revision": t.Revision, "feature": t.Feature, "provider": t.Provider, "model": t.Model, "object_id": t.ObjectID, "object_name": t.ObjectName, "actor": t.Actor, "mode": t.Mode, "status": t.Status, "adoption": t.Adoption, "created_at": t.CreatedAt, "input_version": t.InputVersion, "payload": raw, "summary": summary}
	if expected == 0 {
		_, err = r.tasks.InsertOne(ctx, doc)
		if mongo.IsDuplicateKeyError(err) {
			return ErrAlreadyExists
		}
		return err
	}
	res, err := r.tasks.ReplaceOne(ctx, bson.M{"_id": t.ID, "revision": expected}, doc)
	if err == nil && res.MatchedCount == 0 {
		return ErrAlreadyExists
	}
	return err
}
func aiMongoFilter(f models.AIFilter) bson.M {
	q := bson.M{}
	for _, p := range []struct{ k, v string }{{"object_id", f.ObjectID}, {"input_version", f.InputVersion}, {"feature", f.Feature}, {"provider", f.Provider}, {"model", f.Model}, {"status", f.Status}, {"mode", f.Mode}, {"adoption", f.Adoption}} {
		if p.v != "" {
			q[p.k] = p.v
		}
	}
	dates := bson.M{}
	if !f.From.IsZero() {
		dates["$gte"] = f.From
	}
	if !f.To.IsZero() {
		dates["$lt"] = f.To
	}
	if len(dates) > 0 {
		q["created_at"] = dates
	}
	if f.Active {
		q["$and"] = bson.A{bson.M{"status": bson.M{"$in": bson.A{"running", "queued", "cancelling"}}}}
	}
	if f.Query != "" {
		or := bson.A{}
		for _, k := range []string{"_id", "object_id", "object_name", "actor"} {
			or = append(or, bson.M{k: bson.M{"$regex": regexp.QuoteMeta(f.Query), "$options": "i"}})
		}
		q["$or"] = or
	}
	return q
}
func (r *MongoAIUsageRepository) ListAITasks(ctx context.Context, f models.AIFilter) (*models.AIPage, error) {
	q := aiMongoFilter(f)
	out := &models.AIPage{Items: []*models.AITask{}}
	var err error
	out.Total, err = r.tasks.CountDocuments(ctx, q)
	if err != nil {
		return nil, err
	}
	if f.Cursor != "" {
		q["_id"] = bson.M{"$lt": f.Cursor}
	}
	limit := aiLimit(f)
	cur, err := r.tasks.Find(ctx, q, options.Find().SetSort(bson.D{{Key: "_id", Value: -1}}).SetLimit(int64(limit+1)).SetProjection(bson.M{"summary": 1}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var doc struct {
			Summary []byte `bson:"summary"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		var t models.AITask
		if err := json.Unmarshal(doc.Summary, &t); err != nil {
			return nil, err
		}
		out.Items = append(out.Items, &t)
	}
	if err = cur.Err(); err != nil {
		return nil, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = out.Items[limit-1].ID
	}
	return out, nil
}
func (r *MongoAIUsageRepository) AIStatistics(ctx context.Context, f models.AIFilter) (*models.AIStatistics, error) {
	a := newAIAggregator()
	var first struct {
		Summary []byte `bson:"summary"`
	}
	err := r.tasks.FindOne(ctx, bson.M{}, options.FindOne().SetSort(bson.D{{Key: "created_at", Value: 1}}).SetProjection(bson.M{"summary": 1})).Decode(&first)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	if err == nil {
		var t models.AITask
		if err = json.Unmarshal(first.Summary, &t); err != nil {
			return nil, err
		}
		a.out.FirstRecordedAt = &t.CreatedAt
	}
	cur, err := r.tasks.Find(ctx, aiMongoFilter(f), options.Find().SetProjection(bson.M{"summary": 1}).SetBatchSize(200))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var doc struct {
			Summary []byte `bson:"summary"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		var t models.AITask
		if err := json.Unmarshal(doc.Summary, &t); err != nil {
			return nil, err
		}
		a.add(&t)
	}
	return a.result(), cur.Err()
}
func (r *MongoAIUsageRepository) LoadAIState(ctx context.Context, key string) (int64, []byte, error) {
	var v struct {
		Revision int64  `bson:"revision"`
		Payload  []byte `bson:"payload"`
	}
	err := r.states.FindOne(ctx, bson.M{"_id": key}).Decode(&v)
	if errors.Is(err, mongo.ErrNoDocuments) {
		err = ErrNotFound
	}
	return v.Revision, v.Payload, err
}
func (r *MongoAIUsageRepository) SaveAIState(ctx context.Context, key string, expected int64, b []byte) error {
	doc := bson.M{"_id": key, "revision": expected + 1, "payload": b}
	if expected == 0 {
		_, err := r.states.InsertOne(ctx, doc)
		if mongo.IsDuplicateKeyError(err) {
			return ErrAlreadyExists
		}
		return err
	}
	res, err := r.states.ReplaceOne(ctx, bson.M{"_id": key, "revision": expected}, doc)
	if err == nil && res.MatchedCount == 0 {
		return ErrAlreadyExists
	}
	return err
}

var _ AIUsageRepository = (*MongoAIUsageRepository)(nil)
