package service

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type ScenicSpotMapPoint struct {
	SpotID    string  `json:"scenic_spot_id"`
	Name      string  `json:"name"`
	City      string  `json:"city"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type ScenicSpotSpatialStats struct {
	CoordinateSystem string               `json:"coordinate_system"`
	Unlocated        int64                `json:"unlocated"`
	Points           []ScenicSpotMapPoint `json:"points"`
}

type ScenicSpotTrendPoint struct {
	At    time.Time `json:"at"`
	Label string    `json:"label"`
	Count int64     `json:"count"`
}

type ScenicSpotCreationTrend struct {
	Days   int                    `json:"days"`
	Bucket string                 `json:"bucket"`
	Start  time.Time              `json:"start"`
	End    time.Time              `json:"end"`
	Total  int64                  `json:"total"`
	Items  []ScenicSpotTrendPoint `json:"items"`
}

type ScenicSpotAdminStats struct {
	AsOf            time.Time               `json:"as_of"`
	TotalSpots      int64                   `json:"total_spots"`
	CoveredCities   int64                   `json:"covered_cities"`
	LinkedTracks    int64                   `json:"linked_tracks"`
	IncompleteSpots int64                   `json:"incomplete_spots"`
	Spatial         ScenicSpotSpatialStats  `json:"spatial"`
	Trend           ScenicSpotCreationTrend `json:"trend"`
}

func newScenicSpotCreationTrend(now time.Time, days int) ScenicSpotCreationTrend {
	trend := ScenicSpotCreationTrend{Days: days, Bucket: "day", End: now, Items: []ScenicSpotTrendPoint{}}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	trend.Start = today.AddDate(0, 0, 1-days)
	if days == 1 {
		trend.Bucket = "hour"
		trend.Start = now.Add(-24 * time.Hour)
		for i := 0; i < 24; i++ {
			at := trend.Start.Add(time.Duration(i) * time.Hour)
			trend.Items = append(trend.Items, ScenicSpotTrendPoint{At: at, Label: at.Format("15:04")})
		}
	} else {
		for i := 0; i < days; i++ {
			at := trend.Start.AddDate(0, 0, i)
			trend.Items = append(trend.Items, ScenicSpotTrendPoint{At: at, Label: at.Format("01-02")})
		}
	}
	return trend
}

func (trend *ScenicSpotCreationTrend) add(createdAt time.Time) {
	if createdAt.IsZero() || createdAt.Before(trend.Start) || !createdAt.Before(trend.End) {
		return
	}
	width := 24 * time.Hour
	if trend.Bucket == "hour" {
		width = time.Hour
	}
	index := int(createdAt.Sub(trend.Start) / width)
	if index >= 0 && index < len(trend.Items) {
		trend.Items[index].Count++
		trend.Total++
	}
}

func (s *ScenicSpotCatalogService) RelatedTrackStatistics(ctx context.Context, spotIDs []string) (map[string]*repository.ScenicSpotTrackStatistics, error) {
	return repository.CountScenicSpotTracks(ctx, s.spots, s.tracks, spotIDs)
}

func (s *ScenicSpotCatalogService) RecordProviderUsage(ctx context.Context, spotID, provider, operation string, succeeded bool) {
	if s == nil || s.spots == nil || spotID == "" {
		return
	}
	day := time.Now().In(placeQuotaZone).Format("2006-01-02")
	_ = s.spots.IncrementScenicSpotProviderUsage(ctx, spotID, provider, day, operation, succeeded)
}

func (s *ScenicSpotCatalogService) RecordProviderUsageForTrack(ctx context.Context, trackID, provider, operation string, succeeded bool) {
	if s == nil || s.spots == nil || trackID == "" {
		return
	}
	link, err := s.spots.GetTrackScenicSpotLink(ctx, trackID)
	if err != nil {
		return
	}
	s.RecordProviderUsage(ctx, link.SpotID, provider, operation, succeeded)
}

func (s *ScenicSpotCatalogService) Statistics(ctx context.Context, days int) (*ScenicSpotAdminStats, error) {
	if days != 1 && days != 7 && days != 30 {
		return nil, invalidScenicSpot("days must be 1, 7 or 30")
	}
	now := time.Now().In(placeQuotaZone)
	out := &ScenicSpotAdminStats{
		AsOf:    now,
		Spatial: ScenicSpotSpatialStats{CoordinateSystem: "GCJ02", Points: []ScenicSpotMapPoint{}},
		Trend:   newScenicSpotCreationTrend(now, days),
	}
	spots := []*models.ScenicSpot{}
	cursor := ""
	for {
		page, _, err := s.spots.ListScenicSpots(ctx, repository.ScenicSpotListFilter{Cursor: cursor, Limit: 100})
		if err != nil {
			return nil, err
		}
		for _, spot := range page {
			out.Trend.add(spot.CreatedAt)
			if scenicSpotClientVisible(spot) {
				spots = append(spots, spot)
			}
		}
		if len(page) < 100 {
			break
		}
		cursor = page[len(page)-1].ID
	}
	cities := make(map[string]struct{})
	// Batch IDs to bound database parameter counts as the catalog grows.
	for start := 0; start < len(spots); start += 500 {
		batch := spots[start:min(start+500, len(spots))]
		ids := make([]string, 0, len(batch))
		for _, spot := range batch {
			ids = append(ids, spot.ID)
		}
		counts, err := repository.CountCompletedScenicSpotTracks(ctx, s.spots, s.tracks, ids)
		if err != nil {
			return nil, err
		}
		for _, spot := range batch {
			out.TotalSpots++
			cityNames := make(map[string]struct{})
			code := strings.TrimSpace(spot.CityCode)
			city := config.CityNameByCode(code)
			if city != "" {
				cities[code] = struct{}{}
				cityNames[city] = struct{}{}
			}
			var count int64
			if stats := counts[spot.ID]; stats != nil {
				count = stats.TrackCount
				for code := range stats.CityCodes {
					if name := config.CityNameByCode(code); name != "" {
						cities[code] = struct{}{}
						cityNames[name] = struct{}{}
					}
				}
			}
			if len(cityNames) == 0 && strings.TrimSpace(spot.CityName) != "" {
				cityNames[strings.TrimSpace(spot.CityName)] = struct{}{}
			}
			labels := make([]string, 0, len(cityNames))
			for name := range cityNames {
				labels = append(labels, name)
			}
			sort.Strings(labels)
			city = strings.Join(labels, "、")
			missing := repository.ScenicSpotMissingFields(spot)
			if len(missing) > 0 {
				out.IncompleteSpots++
			}
			out.LinkedTracks += count
			if validMapLatLng(spot.Latitude, spot.Longitude) && spot.Latitude != 0 && spot.Longitude != 0 {
				point, err := mapPointToGCJ02(models.TrackPoint{Latitude: spot.Latitude, Longitude: spot.Longitude}, spot.CoordinateSystem)
				if err == nil {
					out.Spatial.Points = append(out.Spatial.Points, ScenicSpotMapPoint{
						SpotID: spot.ID, Name: spot.Name, City: city,
						Latitude: math.Round(point.Latitude*1e6) / 1e6, Longitude: math.Round(point.Longitude*1e6) / 1e6,
					})
					continue
				}
			}
			out.Spatial.Unlocated++
		}
	}
	out.CoveredCities = int64(len(cities))
	return out, nil
}
