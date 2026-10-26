package repository

import (
	"context"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AMap shared places are one-time enrichment, including pre-policy records with
// an old expiry. Request caches, pauses and Google content retain their TTL.
func (r *MySQLTrackPlaceRepository) DeleteExpiredPlaceContent(ctx context.Context, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM track_place_records WHERE expires_at<=? AND (lease_until IS NULL OR lease_until<=?) AND record_key NOT LIKE 'place:place-amap-%' AND (record_key LIKE 'place:%' OR record_key LIKE 'request:%' OR record_key LIKE 'ring:%' OR record_key LIKE 'pause:%') LIMIT 1000`, now, now)
	return err
}
func (r *MongoTrackPlaceRepository) DeleteExpiredPlaceContent(ctx context.Context, now time.Time) error {
	filter := bson.M{"_id": bson.M{"$regex": "^(place|request|ring|pause):", "$not": bson.M{"$regex": "^place:place-amap-"}}, "expires_at": bson.M{"$gt": time.Time{}, "$lte": now}, "$or": bson.A{bson.M{"lease_until": bson.M{"$lte": now}}, bson.M{"lease_until": nil}}}
	cur, err := r.records.Find(ctx, filter, options.Find().SetProjection(bson.M{"_id": 1}).SetLimit(1000))
	if err != nil {
		return err
	}
	defer cur.Close(ctx)
	ids := []string{}
	for cur.Next(ctx) {
		var item struct {
			ID string `bson:"_id"`
		}
		if err := cur.Decode(&item); err != nil {
			return err
		}
		ids = append(ids, item.ID)
	}
	if err := cur.Err(); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	filter["_id"] = bson.M{"$in": ids}
	_, err = r.records.DeleteMany(ctx, filter)
	return err
}
func (r *MemoryTrackPlaceRepository) DeleteExpiredPlaceContent(_ context.Context, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, v := range r.records {
		if !strings.HasPrefix(key, "place:place-amap-") && (strings.HasPrefix(key, "place:") || strings.HasPrefix(key, "request:") || strings.HasPrefix(key, "ring:") || strings.HasPrefix(key, "pause:")) && !v.ExpiresAt.IsZero() && !v.ExpiresAt.After(now) && !v.LeaseUntil.After(now) {
			delete(r.records, key)
		}
	}
	return nil
}
