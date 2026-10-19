package repository

import (
	"context"
	"sync"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoTrackLikeRepository struct {
	collection *mongo.Collection
	indexMu    sync.Mutex
	indexed    bool
}

func NewMongoTrackLikeRepository(collection *mongo.Collection) *MongoTrackLikeRepository {
	return &MongoTrackLikeRepository{collection: collection}
}

func (r *MongoTrackLikeRepository) ensureIndexes(ctx context.Context) error {
	r.indexMu.Lock()
	defer r.indexMu.Unlock()
	if r.indexed {
		return nil
	}
	_, err := r.collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "track_id", Value: 1}}, Options: options.Index().SetUnique(true).SetName("uk_track_like_user_track")},
		{Keys: bson.D{{Key: "track_id", Value: 1}, {Key: "created_at", Value: -1}, {Key: "user_id", Value: -1}}, Options: options.Index().SetName("idx_track_like_track_time")},
		{Keys: bson.D{{Key: "track_id", Value: 1}}, Options: options.Index().SetName("idx_track_like_track")},
		{Keys: bson.D{{Key: "created_at", Value: -1}, {Key: "track_id", Value: -1}, {Key: "user_id", Value: -1}}, Options: options.Index().SetName("idx_track_like_time")},
	})
	r.indexed = err == nil
	return err
}
func (r *MongoTrackLikeRepository) SetLike(ctx context.Context, userID int64, trackID string, liked bool) error {
	filter := bson.M{"user_id": userID, "track_id": trackID}
	if !liked {
		_, err := r.collection.DeleteOne(ctx, filter)
		return err
	}
	// Never accept writes before the unique relationship index exists, including after a failed first attempt.
	if err := r.ensureIndexes(ctx); err != nil {
		return err
	}
	_, err := r.collection.UpdateOne(ctx, filter, bson.M{"$setOnInsert": models.TrackLike{UserID: userID, TrackID: trackID, CreatedAt: time.Now().UTC()}}, options.Update().SetUpsert(true))
	if mongo.IsDuplicateKeyError(err) {
		return nil
	}
	return err
}
func (r *MongoTrackLikeRepository) Stats(ctx context.Context, userID int64, ids []string) (map[string]models.TrackLikeStats, error) {
	result := make(map[string]models.TrackLikeStats)
	if len(ids) == 0 {
		return result, nil
	}
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{"track_id": bson.M{"$in": ids}}}},
		bson.D{{Key: "$group", Value: bson.M{"_id": "$track_id", "count": bson.M{"$sum": 1}, "liked": bson.M{"$max": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$user_id", userID}}, 1, 0}}}}}},
	}
	cur, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var row struct {
			ID    string `bson:"_id"`
			Count int64  `bson:"count"`
			Liked int    `bson:"liked"`
		}
		if err := cur.Decode(&row); err != nil {
			return nil, err
		}
		result[row.ID] = models.TrackLikeStats{Liked: row.Liked != 0, LikeCount: row.Count}
	}
	return result, cur.Err()
}
func receivedLikesPipeline(ownerID int64) mongo.Pipeline {
	return mongo.Pipeline{
		bson.D{{Key: "$lookup", Value: bson.M{"from": "tracks", "localField": "track_id", "foreignField": "_id", "as": "track"}}},
		bson.D{{Key: "$unwind", Value: "$track"}},
		bson.D{{Key: "$match", Value: bson.M{"user_id": bson.M{"$ne": ownerID}, "track.user_id": ownerID, "track.status": models.TrackStatusNormal, "track.is_running": false, "track.raw_track_url": bson.M{"$type": "string", "$regex": `\S`}}}},
	}
}
func (r *MongoTrackLikeRepository) CountReceived(ctx context.Context, ownerID int64) (int64, error) {
	pipeline := append(receivedLikesPipeline(ownerID), bson.D{{Key: "$count", Value: "count"}})
	cur, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cur.Close(ctx)
	if cur.Next(ctx) {
		var row struct {
			Count int64 `bson:"count"`
		}
		if err := cur.Decode(&row); err != nil {
			return 0, err
		}
		return row.Count, nil
	}
	return 0, cur.Err()
}
func (r *MongoTrackLikeRepository) ListReceived(ctx context.Context, ownerID int64, c *models.TrackLikeCursor, limit int) ([]*models.TrackLike, error) {
	pipeline := receivedLikesPipeline(ownerID)
	if c != nil {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.M{"$or": bson.A{
			bson.M{"created_at": bson.M{"$lt": c.CreatedAt}},
			bson.M{"created_at": c.CreatedAt, "track_id": bson.M{"$lt": c.TrackID}},
			bson.M{"created_at": c.CreatedAt, "track_id": c.TrackID, "user_id": bson.M{"$lt": c.UserID}},
		}}}})
	}
	pipeline = append(pipeline, bson.D{{Key: "$sort", Value: bson.D{{Key: "created_at", Value: -1}, {Key: "track_id", Value: -1}, {Key: "user_id", Value: -1}}}}, bson.D{{Key: "$limit", Value: int64(limit)}}, bson.D{{Key: "$project", Value: bson.M{"track": 0}}})
	cur, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	result := make([]*models.TrackLike, 0)
	if err := cur.All(ctx, &result); err != nil {
		return nil, err
	}
	return result, nil
}
