package repository

import (
	"context"
	"errors"
	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"time"
)

type MongoMessageRepository struct{ rows *mongo.Collection }

func (r *MongoMessageRepository) MarkMessageDelivered(ctx context.Context, id string, revision int64) error {
	result, err := r.rows.UpdateOne(ctx, bson.M{"_id": id, "revision": revision}, bson.M{"$set": bson.M{"pending": false}})
	if err == nil && result.MatchedCount == 0 {
		return ErrConflict
	}
	return err
}
func NewMongoMessageRepository(ctx context.Context, db *mongo.Database) (*MongoMessageRepository, error) {
	rows := db.Collection("message_records")
	_, err := rows.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "kind", Value: 1}, {Key: "at_micros", Value: -1}, {Key: "_id", Value: -1}}},
		{Keys: bson.D{{Key: "kind", Value: 1}, {Key: "user_id", Value: 1}, {Key: "at_micros", Value: -1}, {Key: "_id", Value: -1}}},
		{Keys: bson.D{{Key: "kind", Value: 1}, {Key: "peer_id", Value: 1}, {Key: "at_micros", Value: -1}, {Key: "_id", Value: -1}}},
		{Keys: bson.D{{Key: "kind", Value: 1}, {Key: "thread_id", Value: 1}, {Key: "seq", Value: 1}}},
		{Keys: bson.D{{Key: "pending", Value: 1}, {Key: "at_micros", Value: 1}, {Key: "_id", Value: 1}}},
	})
	if err != nil {
		return nil, err
	}
	return &MongoMessageRepository{rows}, nil
}
func (r *MongoMessageRepository) GetMessageRecord(ctx context.Context, id string) (*models.MessageRecord, error) {
	v := &models.MessageRecord{}
	err := r.rows.FindOne(ctx, bson.M{"_id": id}).Decode(v)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err == nil {
		v.CreatedAt = time.UnixMicro(v.AtMicros).UTC()
	}
	return v, err
}
func (r *MongoMessageRepository) GetMessageRecords(ctx context.Context, ids []string) (map[string]*models.MessageRecord, error) {
	out := map[string]*models.MessageRecord{}
	if len(ids) == 0 {
		return out, nil
	}
	cur, err := r.rows.Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		v := &models.MessageRecord{}
		if err := cur.Decode(v); err != nil {
			return nil, err
		}
		v.CreatedAt = time.UnixMicro(v.AtMicros).UTC()
		out[v.ID] = v
	}
	return out, cur.Err()
}
func (r *MongoMessageRepository) PutMessageRecord(ctx context.Context, v *models.MessageRecord, expected int64) error {
	v.AtMicros = v.CreatedAt.UnixMicro()
	if v.Revision != expected+1 {
		return ErrConflict
	}
	if expected == 0 {
		_, err := r.rows.InsertOne(ctx, v)
		if mongo.IsDuplicateKeyError(err) {
			return ErrConflict
		}
		return err
	}
	result, err := r.rows.ReplaceOne(ctx, bson.M{"_id": v.ID, "revision": expected}, v)
	if err == nil && result.MatchedCount == 0 {
		return ErrConflict
	}
	return err
}
func messageMongoFilter(f MessageFilter) bson.M {
	parts := bson.A{}
	if len(f.Kinds) > 0 {
		parts = append(parts, bson.M{"kind": bson.M{"$in": f.Kinds}})
	}
	if f.UserID > 0 {
		if f.EitherUser {
			parts = append(parts, bson.M{"$or": bson.A{bson.M{"user_id": f.UserID}, bson.M{"peer_id": f.UserID}}})
		} else {
			parts = append(parts, bson.M{"user_id": f.UserID})
		}
	}
	if f.ThreadID != "" {
		parts = append(parts, bson.M{"thread_id": f.ThreadID})
	}
	if len(f.ThreadsAfter) > 0 {
		var choices bson.A
		for id, seq := range f.ThreadsAfter {
			choices = append(choices, bson.M{"thread_id": id, "seq": bson.M{"$gt": seq}})
		}
		parts = append(parts, bson.M{"$or": choices})
	}
	if !f.Before.IsZero() {
		parts = append(parts, bson.M{"$or": bson.A{bson.M{"at_micros": bson.M{"$lt": f.Before.UnixMicro()}}, bson.M{"at_micros": f.Before.UnixMicro(), "_id": bson.M{"$lt": f.BeforeID}}}})
	}
	if !f.After.IsZero() {
		if f.AfterID != "" {
			parts = append(parts, bson.M{"$or": bson.A{bson.M{"at_micros": bson.M{"$gt": f.After.UnixMicro()}}, bson.M{"at_micros": f.After.UnixMicro(), "_id": bson.M{"$gt": f.AfterID}}}})
		} else {
			parts = append(parts, bson.M{"at_micros": bson.M{"$gt": f.After.UnixMicro()}})
		}
	}
	if !f.MaxTime.IsZero() {
		parts = append(parts, bson.M{"at_micros": bson.M{"$lte": f.MaxTime.UnixMicro()}})
	}
	if f.BeforeSeq > 0 {
		parts = append(parts, bson.M{"seq": bson.M{"$lt": f.BeforeSeq}})
	}
	if f.AfterSeq > 0 {
		parts = append(parts, bson.M{"seq": bson.M{"$gt": f.AfterSeq}})
	}
	if f.ActiveOnly {
		parts = append(parts, bson.M{"active": true})
	}
	if f.PendingOnly {
		parts = append(parts, bson.M{"pending": true})
	}
	if f.UnreadOnly {
		parts = append(parts, bson.M{"flag_a": false})
	}
	if f.HasMessages {
		parts = append(parts, bson.M{"seq": bson.M{"$gt": 0}})
	}
	if f.BlockedBy > 0 {
		parts = append(parts, bson.M{"$or": bson.A{bson.M{"user_id": f.BlockedBy, "flag_a": true}, bson.M{"peer_id": f.BlockedBy, "flag_b": true}}})
	}
	if f.VisibleTo > 0 {
		parts = append(parts, bson.M{"$or": bson.A{
			bson.M{"user_id": f.VisibleTo, "flag_a": bson.M{"$ne": true}, "$expr": bson.M{"$gt": bson.A{"$seq", bson.M{"$ifNull": bson.A{"$clear_a", int64(0)}}}}},
			bson.M{"peer_id": f.VisibleTo, "flag_b": bson.M{"$ne": true}, "$expr": bson.M{"$gt": bson.A{"$seq", bson.M{"$ifNull": bson.A{"$clear_b", int64(0)}}}}},
		}})
	}
	if f.Audience != "" {
		parts = append(parts, bson.M{"$or": bson.A{bson.M{"targets": f.Audience}, bson.M{"targets": bson.M{"$size": 0}}, bson.M{"targets": nil}}})
	}
	if len(parts) == 0 {
		return bson.M{}
	}
	return bson.M{"$and": parts}
}
func messageMongoPipeline(f MessageFilter) mongo.Pipeline {
	p := mongo.Pipeline{bson.D{{Key: "$match", Value: messageMongoFilter(f)}}}
	if f.ExcludeNoticeReadUser > 0 {
		p = append(p, bson.D{{Key: "$lookup", Value: bson.M{"from": "message_records", "let": bson.M{"receipt": bson.M{"$concat": bson.A{fmtMessageUser("read.notice.", f.ExcludeNoticeReadUser) + ".", "$_id"}}}, "pipeline": bson.A{bson.M{"$match": bson.M{"$expr": bson.M{"$eq": bson.A{"$_id", "$$receipt"}}}}}, "as": "_read"}}}, bson.D{{Key: "$match", Value: bson.M{"_read": bson.M{"$size": 0}}}})
	}
	return p
}
func (r *MongoMessageRepository) ListMessageRecords(ctx context.Context, f MessageFilter) ([]*models.MessageRecord, error) {
	p := messageMongoPipeline(f)
	direction := -1
	if f.Asc {
		direction = 1
	}
	sortKeys := bson.D{{Key: "at_micros", Value: direction}, {Key: "_id", Value: direction}}
	if f.ThreadID != "" && len(f.Kinds) == 1 && f.Kinds[0] == "dm" {
		sortKeys = bson.D{{Key: "seq", Value: direction}}
	}
	p = append(p, bson.D{{Key: "$sort", Value: sortKeys}}, bson.D{{Key: "$limit", Value: messageLimit(f)}})
	cur, err := r.rows.Aggregate(ctx, p)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []*models.MessageRecord{}
	err = cur.All(ctx, &out)
	for _, v := range out {
		v.CreatedAt = time.UnixMicro(v.AtMicros).UTC()
	}
	return out, err
}
func (r *MongoMessageRepository) CountMessageRecords(ctx context.Context, f MessageFilter) (int64, error) {
	p := append(messageMongoPipeline(f), bson.D{{Key: "$count", Value: "count"}})
	cur, err := r.rows.Aggregate(ctx, p)
	if err != nil {
		return 0, err
	}
	defer cur.Close(ctx)
	var rows []struct {
		Count int64 `bson:"count"`
	}
	if err = cur.All(ctx, &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return rows[0].Count, nil
}
func (r *MongoMessageRepository) GroupMessageCounts(ctx context.Context, f MessageFilter) (map[string]int64, error) {
	key := "$thread_id"
	if f.GroupByKind {
		key = "$kind"
	}
	p := append(messageMongoPipeline(f), bson.D{{Key: "$group", Value: bson.M{"_id": key, "count": bson.M{"$sum": 1}}}})
	cur, err := r.rows.Aggregate(ctx, p)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := map[string]int64{}
	for cur.Next(ctx) {
		var row struct {
			ID    string `bson:"_id"`
			Count int64  `bson:"count"`
		}
		if err := cur.Decode(&row); err != nil {
			return nil, err
		}
		out[row.ID] = row.Count
	}
	return out, cur.Err()
}

func (r *MongoMessageRepository) PruneMessageCredentials(ctx context.Context, cutoff time.Time) error {
	_, err := r.rows.DeleteMany(ctx, bson.M{"kind": "mqtt_session", "at_micros": bson.M{"$lte": cutoff.UnixMicro()}})
	return err
}
