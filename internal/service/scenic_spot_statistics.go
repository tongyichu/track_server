package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type ScenicSpotMissingStat struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int64  `json:"count"`
}

type ScenicSpotTopTracks struct {
	SpotID        string   `json:"scenic_spot_id"`
	Name          string   `json:"name"`
	City          string   `json:"city"`
	TrackCount    int64    `json:"track_count"`
	MissingFields []string `json:"missing_fields"`
}

type ScenicSpotAdminStats struct {
	AsOf            time.Time               `json:"as_of"`
	TotalSpots      int64                   `json:"total_spots"`
	CoveredCities   int64                   `json:"covered_cities"`
	LinkedTracks    int64                   `json:"linked_tracks"`
	IncompleteSpots int64                   `json:"incomplete_spots"`
	Missing         []ScenicSpotMissingStat `json:"missing"`
	TopSpots        []ScenicSpotTopTracks   `json:"top_spots"`
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

func (s *ScenicSpotCatalogService) Statistics(ctx context.Context) (*ScenicSpotAdminStats, error) {
	out := &ScenicSpotAdminStats{
		AsOf: time.Now().In(placeQuotaZone),
		Missing: []ScenicSpotMissingStat{
			{Key: "description", Label: "缺简介"}, {Key: "photos", Label: "无图片"},
			{Key: "coordinates", Label: "缺有效坐标"}, {Key: "address", Label: "缺地址"},
		},
		TopSpots: []ScenicSpotTopTracks{},
	}
	spots := []*models.ScenicSpot{}
	cursor := ""
	for {
		page, _, err := s.spots.ListScenicSpots(ctx, repository.ScenicSpotListFilter{Cursor: cursor, Limit: 100, VisibleOnly: true})
		if err != nil {
			return nil, err
		}
		spots = append(spots, page...)
		if len(page) < 100 {
			break
		}
		cursor = page[len(page)-1].ID
	}
	cities := make(map[string]struct{})
	missingCounts := make(map[string]int64)
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
			for _, key := range missing {
				missingCounts[key]++
			}
			out.LinkedTracks += count
			if count > 0 {
				out.TopSpots = append(out.TopSpots, ScenicSpotTopTracks{
					SpotID: spot.ID, Name: spot.Name, City: city, TrackCount: count, MissingFields: missing,
				})
			}
		}
	}
	out.CoveredCities = int64(len(cities))
	for i := range out.Missing {
		out.Missing[i].Count = missingCounts[out.Missing[i].Key]
	}
	sort.Slice(out.TopSpots, func(i, j int) bool {
		if out.TopSpots[i].TrackCount == out.TopSpots[j].TrackCount {
			return out.TopSpots[i].SpotID < out.TopSpots[j].SpotID
		}
		return out.TopSpots[i].TrackCount > out.TopSpots[j].TrackCount
	})
	if len(out.TopSpots) > 10 {
		out.TopSpots = out.TopSpots[:10]
	}
	return out, nil
}
