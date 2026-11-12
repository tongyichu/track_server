package repository

import (
	"context"
	"strings"

	"github.com/tongyichu/track_server/internal/config"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var adminTrackMissingKeys = []string{"place_candidate", "scenic_spot", "description", "city", "previews"}

func emptyAdminTrackMissingCounts() map[string]int64 {
	counts := map[string]int64{"all": 0}
	for _, key := range adminTrackMissingKeys {
		counts[key] = 0
	}
	return counts
}

func (r *MySQLTrackRepository) CountAdminTrackMissing(ctx context.Context, filter AdminTrackListFilter) (map[string]int64, error) {
	filter.Missing = ""
	base, baseArgs := adminTrackSQLBase(filter)
	fields, args := []string{"COUNT(*)"}, []any{}
	for _, key := range adminTrackMissingKeys {
		item := filter
		item.Missing = key
		condition, values := adminTrackMissingSQL(item)
		fields = append(fields, "COALESCE(SUM(CASE WHEN ("+condition+") THEN 1 ELSE 0 END),0)")
		args = append(args, values...)
	}
	values := make([]int64, len(fields))
	targets := make([]any, len(values))
	for i := range values {
		targets[i] = &values[i]
	}
	if err := r.db.QueryRowContext(ctx, "SELECT "+strings.Join(fields, ",")+" "+base, append(args, baseArgs...)...).Scan(targets...); err != nil {
		return nil, err
	}
	counts := emptyAdminTrackMissingCounts()
	counts["all"] = values[0]
	for i, key := range adminTrackMissingKeys {
		counts[key] = values[i+1]
	}
	return counts, nil
}

func (r *MongoTrackRepository) CountAdminTrackMissing(ctx context.Context, filter AdminTrackListFilter) (map[string]int64, error) {
	counts := emptyAdminTrackMissingCounts()
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: adminTrackMongoBase(filter)}},
		{{Key: "$lookup", Value: bson.M{"from": "track_scenic_spot_links", "localField": "_id", "foreignField": "_id", "as": "_admin_links"}}},
		{{Key: "$lookup", Value: bson.M{"from": "scenic_spots", "localField": "_admin_links.scenic_spot_id", "foreignField": "_id", "as": "_admin_spots"}}},
	}
	conditions := adminTrackMissingMongoConditions(filter)
	facets := bson.M{"all": mongo.Pipeline{{{Key: "$count", Value: "total"}}}}
	for _, key := range adminTrackMissingKeys {
		facets[key] = mongo.Pipeline{{{Key: "$match", Value: conditions[key]}}, {{Key: "$count", Value: "total"}}}
	}
	pipeline = append(pipeline, bson.D{{Key: "$facet", Value: facets}})
	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	if !cursor.Next(ctx) {
		return counts, cursor.Err()
	}
	var result map[string][]struct {
		Total int64 `bson:"total"`
	}
	if err := cursor.Decode(&result); err != nil {
		return nil, err
	}
	for key, rows := range result {
		if len(rows) > 0 {
			counts[key] = rows[0].Total
		}
	}
	return counts, nil
}

func (r *InMemoryTrackRepository) CountAdminTrackMissing(ctx context.Context, filter AdminTrackListFilter) (map[string]int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	linked := r.adminLinkedScenicTracks("scenic_spot")
	candidates := make(map[string]bool, len(filter.PlaceCandidateTrackIDs))
	for _, id := range filter.PlaceCandidateTrackIDs {
		candidates[id] = true
	}
	users := make(map[int64]struct{}, len(filter.UserIDs))
	cities := make(map[string]struct{}, len(filter.CityCodes))
	for _, id := range filter.UserIDs {
		users[id] = struct{}{}
	}
	for _, code := range filter.CityCodes {
		cities[config.CanonicalCityCode(code)] = struct{}{}
	}
	counts := emptyAdminTrackMissingCounts()
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, track := range r.tracks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !adminTrackMatchesBase(track, filter, users, cities) {
			continue
		}
		counts["all"]++
		for _, key := range adminTrackMissingKeys {
			if adminTrackMatchesMissing(track, key, linked[track.ID], candidates[track.ID]) {
				counts[key]++
			}
		}
	}
	return counts, nil
}
