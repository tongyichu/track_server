package repository

import (
	"context"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func followerCountIDs(userIDs []int64) ([]int64, map[int64]int64) {
	ids := make([]int64, 0, len(userIDs))
	counts := make(map[int64]int64, len(userIDs))
	for _, id := range userIDs {
		if _, found := counts[id]; id > 0 && !found {
			ids = append(ids, id)
			counts[id] = 0
		}
	}
	return ids, counts
}

func (r *InMemoryFollowRepository) CountFollowersByUserIDs(ctx context.Context, userIDs []int64) (map[int64]int64, error) {
	ids, counts := followerCountIDs(userIDs)
	if len(ids) == 0 {
		return counts, nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, following := range r.follows {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for id := range following {
			if _, found := counts[id]; found {
				counts[id]++
			}
		}
	}
	return counts, nil
}

func (r *MySQLFollowRepository) CountFollowersByUserIDs(ctx context.Context, userIDs []int64) (map[int64]int64, error) {
	ids, counts := followerCountIDs(userIDs)
	if len(ids) == 0 {
		return counts, nil
	}
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx, `SELECT followee_user_id,COUNT(*) FROM user_follows
		WHERE followee_user_id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")+`) GROUP BY followee_user_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, count int64
		if err := rows.Scan(&id, &count); err != nil {
			return nil, err
		}
		counts[id] = count
	}
	return counts, rows.Err()
}

func (r *MongoFollowRepository) CountFollowersByUserIDs(ctx context.Context, userIDs []int64) (map[int64]int64, error) {
	ids, counts := followerCountIDs(userIDs)
	if len(ids) == 0 {
		return counts, nil
	}
	rows, err := r.collection.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"followee_user_id": bson.M{"$in": ids}}}},
		{{Key: "$group", Value: bson.M{"_id": "$followee_user_id", "count": bson.M{"$sum": 1}}}},
	})
	if err != nil {
		return nil, err
	}
	defer rows.Close(ctx)
	for rows.Next(ctx) {
		var row struct {
			ID    int64 `bson:"_id"`
			Count int64 `bson:"count"`
		}
		if err := rows.Decode(&row); err != nil {
			return nil, err
		}
		counts[row.ID] = row.Count
	}
	return counts, rows.Err()
}
