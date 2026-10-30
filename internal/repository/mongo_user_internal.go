package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func (r *MongoUserRepository) UpdateInternal(ctx context.Context, userID int64, isInternal bool) error {
	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": userID}, bson.M{"$set": bson.M{
		"is_internal": isInternal,
		"updated_at":  time.Now(),
	}})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}
