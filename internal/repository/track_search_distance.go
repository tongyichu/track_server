package repository

import (
	"math"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
)

func searchDistance(ranges []models.TrackDistanceRange) models.TrackDistanceRange {
	if len(ranges) == 0 {
		return models.TrackDistanceRange{}
	}
	return ranges[0]
}

func matchesSearchDistance(value float64, ranges []models.TrackDistanceRange) bool {
	r := searchDistance(ranges)
	if r.MinMeters == nil && r.MaxMeters == nil {
		return true
	}
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0) &&
		(r.MinMeters == nil || value >= *r.MinMeters) && (r.MaxMeters == nil || value < *r.MaxMeters)
}

func searchDistanceSQL(query string, args []interface{}, ranges []models.TrackDistanceRange) (string, []interface{}) {
	r := searchDistance(ranges)
	if r.MinMeters == nil && r.MaxMeters == nil {
		return query, args
	}
	query += " AND distance > 0 AND distance <= ?"
	args = append(args, math.MaxFloat64)
	if r.MinMeters != nil {
		query += " AND distance >= ?"
		args = append(args, *r.MinMeters)
	}
	if r.MaxMeters != nil {
		query += " AND distance < ?"
		args = append(args, *r.MaxMeters)
	}
	return query, args
}

func applySearchDistanceMongo(filter bson.M, ranges []models.TrackDistanceRange) {
	r := searchDistance(ranges)
	if r.MinMeters == nil && r.MaxMeters == nil {
		return
	}
	condition := bson.M{"$gt": 0, "$lte": math.MaxFloat64}
	if r.MinMeters != nil {
		condition["$gte"] = *r.MinMeters
	}
	if r.MaxMeters != nil {
		condition["$lt"] = *r.MaxMeters
	}
	filter["distance"] = condition
}
