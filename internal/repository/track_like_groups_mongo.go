package repository

import (
	"context"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func (r *MongoTrackLikeRepository) receivedGroupsPipeline(owner int64) mongo.Pipeline {
	return mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{"user_id": owner, "status": models.TrackStatusNormal, "is_running": false, "raw_track_url": bson.M{"$type": "string", "$regex": `\S`}}}},
		bson.D{{Key: "$lookup", Value: bson.M{"from": r.collection.Name(), "let": bson.M{"trackID": "$_id"}, "pipeline": mongo.Pipeline{
			bson.D{{Key: "$match", Value: bson.M{"user_id": bson.M{"$ne": owner}, "$expr": bson.M{"$eq": bson.A{"$track_id", "$$trackID"}}}}},
			bson.D{{Key: "$group", Value: bson.M{"_id": nil, "count": bson.M{"$sum": 1}, "last": bson.M{"$max": "$created_at"}}}},
		}, "as": "likes"}}},
		bson.D{{Key: "$unwind", Value: "$likes"}},
		bson.D{{Key: "$project", Value: bson.M{"_id": 1, "received_like_count": "$likes.count", "last_liked_at": "$likes.last"}}},
	}
}
func (r *MongoTrackLikeRepository) ListReceivedGroups(ctx context.Context, owner int64, cursor *models.ReceivedLikeTrackCursor, limit int) ([]*models.ReceivedLikeTrack, error) {
	pipeline := r.receivedGroupsPipeline(owner)
	if cursor != nil {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.M{"$or": bson.A{bson.M{"last_liked_at": bson.M{"$lt": cursor.LastLikedAt}}, bson.M{"last_liked_at": cursor.LastLikedAt, "_id": bson.M{"$lt": cursor.TrackID}}}}}})
	}
	pipeline = append(pipeline, bson.D{{Key: "$sort", Value: bson.D{{Key: "last_liked_at", Value: -1}, {Key: "_id", Value: -1}}}}, bson.D{{Key: "$limit", Value: int64(limit)}},
		bson.D{{Key: "$lookup", Value: bson.M{"from": r.collection.Name(), "let": bson.M{"trackID": "$_id"}, "pipeline": mongo.Pipeline{
			bson.D{{Key: "$match", Value: bson.M{"user_id": bson.M{"$ne": owner}, "$expr": bson.M{"$eq": bson.A{"$track_id", "$$trackID"}}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "created_at", Value: -1}, {Key: "user_id", Value: -1}}}}, bson.D{{Key: "$limit", Value: 3}},
		}, "as": "recent_likes"}}})
	cur, err := r.collection.Database().Collection("tracks").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	result := make([]*models.ReceivedLikeTrack, 0)
	err = cur.All(ctx, &result)
	return result, err
}
func (r *MongoTrackLikeRepository) CountReceivedGroups(ctx context.Context, owner int64) (int64, error) {
	pipeline := append(r.receivedGroupsPipeline(owner), bson.D{{Key: "$count", Value: "count"}})
	cur, err := r.collection.Database().Collection("tracks").Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cur.Close(ctx)
	var result struct {
		Count int64 `bson:"count"`
	}
	if cur.Next(ctx) {
		if err := cur.Decode(&result); err != nil {
			return 0, err
		}
	}
	return result.Count, cur.Err()
}
func (r *MongoTrackLikeRepository) ListReceivedTrackLikers(ctx context.Context, owner int64, track string, cursor *models.TrackLikeCursor, limit int) ([]*models.TrackLike, error) {
	pipeline := mongo.Pipeline{bson.D{{Key: "$match", Value: bson.M{"track_id": track, "user_id": bson.M{"$ne": owner}}}}}
	pipeline = append(pipeline, receivedLikesPipeline(owner)...)
	if cursor != nil {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.M{"$or": bson.A{bson.M{"created_at": bson.M{"$lt": cursor.CreatedAt}}, bson.M{"created_at": cursor.CreatedAt, "user_id": bson.M{"$lt": cursor.UserID}}}}}})
	}
	pipeline = append(pipeline, bson.D{{Key: "$sort", Value: bson.D{{Key: "created_at", Value: -1}, {Key: "user_id", Value: -1}}}}, bson.D{{Key: "$limit", Value: int64(limit)}}, bson.D{{Key: "$project", Value: bson.M{"track": 0}}})
	cur, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	result := make([]*models.TrackLike, 0)
	err = cur.All(ctx, &result)
	return result, err
}
