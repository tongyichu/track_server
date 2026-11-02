package repository

import (
	"context"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// ScenicSpotIDsByTrackCities resolves linked cities before catalog pagination.
func ScenicSpotIDsByTrackCities(ctx context.Context, spots ScenicSpotRepository, tracks TrackRepository, codes []string) ([]string, error) {
	ids := []string{}
	if len(codes) == 0 {
		return ids, nil
	}
	switch repo := spots.(type) {
	case *MySQLScenicSpotRepository:
		args := make([]any, len(codes))
		for i, code := range codes {
			args[i] = code
		}
		rows, err := repo.db.QueryContext(ctx, `SELECT DISTINCT l.spot_id FROM track_scenic_spot_links l JOIN track_records t ON t.id=l.track_id WHERE t.city_code IN (`+strings.TrimRight(strings.Repeat("?,", len(codes)), ",")+`)`, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, rows.Err()
	case *MongoScenicSpotRepository:
		cur, err := repo.links.Aggregate(ctx, mongo.Pipeline{
			{{Key: "$lookup", Value: bson.M{"from": "tracks", "localField": "_id", "foreignField": "_id", "as": "track"}}},
			{{Key: "$match", Value: bson.M{"track.city_code": bson.M{"$in": codes}}}},
			{{Key: "$group", Value: bson.M{"_id": "$scenic_spot_id"}}},
		})
		if err != nil {
			return nil, err
		}
		defer cur.Close(ctx)
		for cur.Next(ctx) {
			var row struct {
				ID string `bson:"_id"`
			}
			if err := cur.Decode(&row); err != nil {
				return nil, err
			}
			ids = append(ids, row.ID)
		}
		return ids, cur.Err()
	case *MemoryScenicSpotRepository:
		trackRepo, ok := tracks.(*InMemoryTrackRepository)
		if !ok || trackRepo == nil {
			return nil, errAdminStatisticsRepositoriesUnavailable
		}
		wanted := map[string]bool{}
		for _, code := range codes {
			wanted[code] = true
		}
		matchingTracks := map[string]bool{}
		trackRepo.mu.RLock()
		for id, track := range trackRepo.tracks {
			if track != nil && wanted[track.CityCode] {
				matchingTracks[id] = true
			}
		}
		trackRepo.mu.RUnlock()
		seen := map[string]bool{}
		repo.mu.RLock()
		defer repo.mu.RUnlock()
		for _, link := range repo.links {
			if matchingTracks[link.TrackID] && !seen[link.SpotID] {
				seen[link.SpotID] = true
				ids = append(ids, link.SpotID)
			}
		}
		return ids, nil
	default:
		return nil, errAdminStatisticsRepositoriesUnavailable
	}
}
