package repository

import (
	"context"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type ScenicSpotTrackStatistics struct {
	TrackCount int64
	CityCodes  map[string]struct{}
}

// CountCompletedScenicSpotTracks joins scenic links to valid completed tracks in the active backend.
func CountCompletedScenicSpotTracks(ctx context.Context, spots ScenicSpotRepository, tracks TrackRepository, spotIDs []string) (map[string]*ScenicSpotTrackStatistics, error) {
	return scenicSpotTrackStatistics(ctx, spots, tracks, spotIDs, true)
}

// CountScenicSpotTracks counts existing related tracks, matching the admin detail list.
func CountScenicSpotTracks(ctx context.Context, spots ScenicSpotRepository, tracks TrackRepository, spotIDs []string) (map[string]*ScenicSpotTrackStatistics, error) {
	return scenicSpotTrackStatistics(ctx, spots, tracks, spotIDs, false)
}

func scenicSpotTrackStatistics(ctx context.Context, spots ScenicSpotRepository, tracks TrackRepository, spotIDs []string, completedOnly bool) (map[string]*ScenicSpotTrackStatistics, error) {
	ids := uniqueNonEmptyStrings(spotIDs)
	out := make(map[string]*ScenicSpotTrackStatistics, len(ids))
	add := func(id, city string, count int64) {
		if out[id] == nil {
			out[id] = &ScenicSpotTrackStatistics{CityCodes: make(map[string]struct{})}
		}
		out[id].TrackCount += count
		if city = strings.TrimSpace(city); city != "" {
			out[id].CityCodes[city] = struct{}{}
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	switch repo := spots.(type) {
	case *MySQLScenicSpotRepository:
		args := make([]any, len(ids))
		for i, id := range ids {
			args[i] = id
		}
		placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
		completedFilter := ""
		if completedOnly {
			completedFilter = " AND " + completedTrackAliasSQL
		}
		rows, err := repo.db.QueryContext(ctx, `SELECT l.spot_id,COALESCE(t.city_code,''),COUNT(*) FROM track_scenic_spot_links l
			INNER JOIN track_records t ON t.id=l.track_id
			WHERE l.spot_id IN (`+placeholders+`)`+completedFilter+` GROUP BY l.spot_id,t.city_code`, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var id, city string
			var count int64
			if err := rows.Scan(&id, &city, &count); err != nil {
				return nil, err
			}
			add(id, city, count)
		}
		return out, rows.Err()
	case *MongoScenicSpotRepository:
		pipeline := mongo.Pipeline{
			{{Key: "$match", Value: bson.M{"scenic_spot_id": bson.M{"$in": ids}}}},
			{{Key: "$lookup", Value: bson.M{"from": "tracks", "localField": "_id", "foreignField": "_id", "as": "track"}}},
			{{Key: "$unwind", Value: "$track"}},
		}
		if completedOnly {
			pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.M{
				"track.status": bson.M{"$ne": models.TrackStatusDeleted}, "track.is_running": false,
				"track.raw_track_url": bson.M{"$nin": bson.A{nil, ""}},
				"track.deleted_at":    bson.M{"$in": bson.A{nil, time.Time{}}},
			}}})
		}
		pipeline = append(pipeline, bson.D{{Key: "$group", Value: bson.M{"_id": bson.M{"spot": "$scenic_spot_id", "city": bson.M{"$ifNull": bson.A{"$track.city_code", ""}}}, "count": bson.M{"$sum": 1}}}})
		cur, err := repo.links.Aggregate(ctx, pipeline)
		if err != nil {
			return nil, err
		}
		defer cur.Close(ctx)
		for cur.Next(ctx) {
			var row struct {
				ID struct {
					Spot string `bson:"spot"`
					City string `bson:"city"`
				} `bson:"_id"`
				Count int64 `bson:"count"`
			}
			if err := cur.Decode(&row); err != nil {
				return nil, err
			}
			add(row.ID.Spot, row.ID.City, row.Count)
		}
		return out, cur.Err()
	case *MemoryScenicSpotRepository:
		trackRepo, ok := tracks.(*InMemoryTrackRepository)
		if !ok || trackRepo == nil {
			return nil, errAdminStatisticsRepositoriesUnavailable
		}
		wanted := make(map[string]bool, len(ids))
		for _, id := range ids {
			wanted[id] = true
		}
		links := make(map[string]string)
		repo.mu.RLock()
		for _, link := range repo.links {
			if wanted[link.SpotID] {
				links[link.TrackID] = link.SpotID
			}
		}
		repo.mu.RUnlock()
		trackRepo.mu.RLock()
		defer trackRepo.mu.RUnlock()
		for id, spotID := range links {
			track := trackRepo.tracks[id]
			if track == nil {
				continue
			}
			if completedOnly && (track.Status == models.TrackStatusDeleted || !track.DeletedAt.IsZero() || track.IsRunning || track.RawTrackURL == "") {
				continue
			}
			add(spotID, track.CityCode, 1)
		}
		return out, nil
	default:
		return nil, errAdminStatisticsRepositoriesUnavailable
	}
}
