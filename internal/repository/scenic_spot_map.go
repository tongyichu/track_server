package repository

import (
	"context"
	"slices"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (r *MySQLScenicSpotRepository) ListScenicMapSpots(ctx context.Context, bounds models.TrackMapBBox) ([]*models.ScenicSpot, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT spot_id, name, JSON_UNQUOTE(JSON_EXTRACT(data_json, '$.latitude')),
		       JSON_UNQUOTE(JSON_EXTRACT(data_json, '$.longitude')),
		       COALESCE(JSON_UNQUOTE(JSON_EXTRACT(data_json, '$.coordinate_system')), ''),
		       COALESCE(JSON_UNQUOTE(JSON_EXTRACT(data_json, '$.city_code')), '')
		FROM scenic_spots
		WHERE status = ?
		  AND CAST(JSON_UNQUOTE(JSON_EXTRACT(data_json, '$.latitude')) AS DECIMAL(12,8)) BETWEEN ? AND ?
		  AND CAST(JSON_UNQUOTE(JSON_EXTRACT(data_json, '$.longitude')) AS DECIMAL(12,8)) BETWEEN ? AND ?`,
		models.ScenicSpotActive, bounds.MinLatitude, bounds.MaxLatitude, bounds.MinLongitude, bounds.MaxLongitude)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var spots []*models.ScenicSpot
	for rows.Next() {
		spot := &models.ScenicSpot{Status: models.ScenicSpotActive}
		if err := rows.Scan(&spot.ID, &spot.Name, &spot.Latitude, &spot.Longitude, &spot.CoordinateSystem, &spot.CityCode); err != nil {
			return nil, err
		}
		spots = append(spots, spot)
	}
	return spots, rows.Err()
}

func (r *MongoScenicSpotRepository) ListScenicMapSpots(ctx context.Context, bounds models.TrackMapBBox) ([]*models.ScenicSpot, error) {
	cursor, err := r.spots.Find(ctx, bson.M{
		"status":    models.ScenicSpotActive,
		"latitude":  bson.M{"$gte": bounds.MinLatitude, "$lte": bounds.MaxLatitude},
		"longitude": bson.M{"$gte": bounds.MinLongitude, "$lte": bounds.MaxLongitude},
	}, options.Find().SetProjection(bson.M{"name": 1, "latitude": 1, "longitude": 1, "coordinate_system": 1, "city_code": 1, "status": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var spots []*models.ScenicSpot
	err = cursor.All(ctx, &spots)
	return spots, err
}

func (r *MemoryScenicSpotRepository) ListScenicMapSpots(ctx context.Context, bounds models.TrackMapBBox) ([]*models.ScenicSpot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var spots []*models.ScenicSpot
	for _, spot := range r.spots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if spot.Status == models.ScenicSpotActive &&
			spot.Latitude >= bounds.MinLatitude && spot.Latitude <= bounds.MaxLatitude &&
			spot.Longitude >= bounds.MinLongitude && spot.Longitude <= bounds.MaxLongitude {
			spots = append(spots, &models.ScenicSpot{
				ID: spot.ID, Name: spot.Name, CityCode: spot.CityCode, Status: spot.Status,
				Latitude: spot.Latitude, Longitude: spot.Longitude, CoordinateSystem: spot.CoordinateSystem,
			})
		}
	}
	return spots, nil
}

func (r *MySQLScenicSpotRepository) VisitScenicMapLinks(ctx context.Context, spotIDs []string, visit func(trackID, spotID string)) error {
	if len(spotIDs) == 0 {
		return nil
	}
	args := make([]any, len(spotIDs))
	for i, id := range spotIDs {
		args[i] = id
	}
	in := strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")
	rows, err := r.db.QueryContext(ctx, `SELECT track_id, spot_id FROM track_scenic_spot_links WHERE spot_id IN (`+in+`)`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var trackID, spotID string
		if err := rows.Scan(&trackID, &spotID); err != nil {
			return err
		}
		visit(trackID, spotID)
	}
	return rows.Err()
}

func (r *MongoScenicSpotRepository) VisitScenicMapLinks(ctx context.Context, spotIDs []string, visit func(trackID, spotID string)) error {
	if len(spotIDs) == 0 {
		return nil
	}
	cursor, err := r.links.Find(ctx, bson.M{"scenic_spot_id": bson.M{"$in": spotIDs}},
		options.Find().SetProjection(bson.M{"scenic_spot_id": 1}))
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var link models.TrackScenicSpotLink
		if err := cursor.Decode(&link); err != nil {
			return err
		}
		visit(link.TrackID, link.SpotID)
	}
	return cursor.Err()
}

func (r *MemoryScenicSpotRepository) VisitScenicMapLinks(ctx context.Context, spotIDs []string, visit func(trackID, spotID string)) error {
	if len(spotIDs) == 0 {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, link := range r.links {
		if err := ctx.Err(); err != nil {
			return err
		}
		if slices.Contains(spotIDs, link.SpotID) {
			visit(link.TrackID, link.SpotID)
		}
	}
	return nil
}
