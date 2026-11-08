package repository

import (
	"context"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type DiscoveryShapeLabelRepository interface {
	AggregateDiscoveryShapeLabels(context.Context, string, string) ([]models.DiscoveryShapeLabelCount, error)
}

func (r *InMemoryTrackRepository) AggregateDiscoveryShapeLabels(ctx context.Context, city, trackType string) ([]models.DiscoveryShapeLabelCount, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	counts := make(map[string]int64)
	for _, track := range r.tracks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if track == nil || track.Status != models.TrackStatusNormal || track.IsRunning || strings.TrimSpace(track.RawTrackURL) == "" ||
			strings.TrimSpace(track.ShapeLabel) == "" || !sameRegionCode(track.CityCode, city) || trackType != "" && track.TrackType != trackType {
			continue
		}
		counts[track.ShapeLabel]++
	}
	items := make([]models.DiscoveryShapeLabelCount, 0, len(counts))
	for label, count := range counts {
		items = append(items, models.DiscoveryShapeLabelCount{ShapeLabel: label, TrackCount: count})
	}
	return items, nil
}

func (r *MySQLTrackRepository) AggregateDiscoveryShapeLabels(ctx context.Context, city, trackType string) ([]models.DiscoveryShapeLabelCount, error) {
	codes := regionCodeCandidates([]string{city})
	query := `SELECT CAST(shape_label AS BINARY),COUNT(*) FROM track_records WHERE status=? AND is_running=0
		AND raw_track_url IS NOT NULL AND raw_track_url REGEXP '[^[:space:]]'
		AND shape_label IS NOT NULL AND TRIM(shape_label)<>'' AND city_code IN (` + strings.TrimRight(strings.Repeat("?,", len(codes)), ",") + `)`
	args := []any{models.TrackStatusNormal}
	for _, code := range codes {
		args = append(args, code)
	}
	if trackType != "" {
		query += ` AND CAST(track_type AS BINARY)=CAST(? AS BINARY)`
		args = append(args, trackType)
	}
	// Shape names are exact identifiers, independent of the database's case-insensitive collation.
	query += ` GROUP BY CAST(shape_label AS BINARY)`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]models.DiscoveryShapeLabelCount, 0)
	for rows.Next() {
		var item models.DiscoveryShapeLabelCount
		if err := rows.Scan(&item.ShapeLabel, &item.TrackCount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MongoTrackRepository) AggregateDiscoveryShapeLabels(ctx context.Context, city, trackType string) ([]models.DiscoveryShapeLabelCount, error) {
	filter := bson.M{"status": models.TrackStatusNormal, "is_running": false,
		"raw_track_url": bson.M{"$type": "string", "$regex": `\S`}, "shape_label": bson.M{"$type": "string", "$regex": `\S`},
		"city_code": bson.M{"$in": regionCodeCandidates([]string{city})}}
	if trackType != "" {
		filter["track_type"] = trackType
	}
	cur, err := r.collection.Aggregate(ctx, mongo.Pipeline{
		bson.D{{Key: "$match", Value: filter}},
		bson.D{{Key: "$group", Value: bson.M{"_id": "$shape_label", "track_count": bson.M{"$sum": 1}}}},
	})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	items := make([]models.DiscoveryShapeLabelCount, 0)
	if err := cur.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}

var _ DiscoveryShapeLabelRepository = (*InMemoryTrackRepository)(nil)
var _ DiscoveryShapeLabelRepository = (*MySQLTrackRepository)(nil)
var _ DiscoveryShapeLabelRepository = (*MongoTrackRepository)(nil)
