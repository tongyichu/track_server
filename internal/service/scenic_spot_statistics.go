package service

import (
	"context"
	"sort"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type ScenicSpotDailyStats struct {
	Day       string `json:"day"`
	Requests  int64  `json:"requests"`
	Successes int64  `json:"successes"`
	Failures  int64  `json:"failures"`
}
type ScenicSpotProviderStats struct {
	Provider  string `json:"provider"`
	Requests  int64  `json:"requests"`
	Successes int64  `json:"successes"`
	Failures  int64  `json:"failures"`
}
type ScenicSpotTopUsage struct {
	SpotID     string `json:"scenic_spot_id"`
	Name       string `json:"name,omitempty"`
	Status     string `json:"status,omitempty"`
	Requests   int64  `json:"requests"`
	TrackCount int64  `json:"track_count"`
}
type ScenicSpotAdminStats struct {
	FromDay               string                    `json:"from_day"`
	ToDay                 string                    `json:"to_day"`
	TotalSpots            int64                     `json:"total_spots"`
	PublishedSpots        int64                     `json:"published_spots"`
	DraftSpots            int64                     `json:"draft_spots"`
	ArchivedSpots         int64                     `json:"archived_spots"`
	LinkedTracks          int64                     `json:"linked_tracks"`
	PublishedLinkedTracks int64                     `json:"published_linked_tracks"`
	CallsToday            int64                     `json:"calls_today"`
	Calls7Days            int64                     `json:"calls_7_days"`
	Calls30Days           int64                     `json:"calls_30_days"`
	Successes30Days       int64                     `json:"successes_30_days"`
	Failures30Days        int64                     `json:"failures_30_days"`
	Providers             []ScenicSpotProviderStats `json:"providers"`
	Daily                 []ScenicSpotDailyStats    `json:"daily"`
	TopSpots              []ScenicSpotTopUsage      `json:"top_spots"`
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
	now := time.Now().In(placeQuotaZone)
	start := now.AddDate(0, 0, -29)
	fromDay, toDay := start.Format("2006-01-02"), now.Format("2006-01-02")
	usage, err := s.spots.ListScenicSpotProviderUsage(ctx, fromDay, toDay)
	if err != nil {
		return nil, err
	}
	out := &ScenicSpotAdminStats{FromDay: fromDay, ToDay: toDay, Daily: make([]ScenicSpotDailyStats, 0, 30), Providers: []ScenicSpotProviderStats{}, TopSpots: []ScenicSpotTopUsage{}}
	daily := map[string]*ScenicSpotDailyStats{}
	byProvider := map[string]*ScenicSpotProviderStats{}
	top := map[string]*ScenicSpotTopUsage{}
	for i := 0; i < 30; i++ {
		day := start.AddDate(0, 0, i).Format("2006-01-02")
		daily[day] = &ScenicSpotDailyStats{Day: day}
	}
	for _, item := range usage {
		d := daily[item.UsageDay]
		if d == nil {
			continue
		}
		d.Requests += item.Requests
		d.Successes += item.Successes
		d.Failures += item.Failures
		p := byProvider[item.Provider]
		if p == nil {
			p = &ScenicSpotProviderStats{Provider: item.Provider}
			byProvider[item.Provider] = p
		}
		p.Requests += item.Requests
		p.Successes += item.Successes
		p.Failures += item.Failures
		t := top[item.SpotID]
		if t == nil {
			t = &ScenicSpotTopUsage{SpotID: item.SpotID}
			top[item.SpotID] = t
		}
		t.Requests += item.Requests
		if item.UsageDay == toDay {
			out.CallsToday += item.Requests
		}
		if item.UsageDay >= now.AddDate(0, 0, -6).Format("2006-01-02") {
			out.Calls7Days += item.Requests
		}
		out.Calls30Days += item.Requests
		out.Successes30Days += item.Successes
		out.Failures30Days += item.Failures
	}
	for i := 0; i < 30; i++ {
		day := start.AddDate(0, 0, i).Format("2006-01-02")
		out.Daily = append(out.Daily, *daily[day])
	}
	for _, p := range byProvider {
		out.Providers = append(out.Providers, *p)
	}
	sort.Slice(out.Providers, func(i, j int) bool { return out.Providers[i].Provider < out.Providers[j].Provider })

	cursor := ""
	spots := []*models.ScenicSpot{}
	for {
		page, _, e := s.spots.ListScenicSpots(ctx, repository.ScenicSpotListFilter{Cursor: cursor, Limit: 100})
		if e != nil {
			return nil, e
		}
		spots = append(spots, page...)
		if len(page) < 100 {
			break
		}
		cursor = page[len(page)-1].ID
	}
	ids := make([]string, 0, len(spots))
	for _, spot := range spots {
		ids = append(ids, spot.ID)
		out.TotalSpots++
		switch spot.Status {
		case models.ScenicSpotPublished:
			out.PublishedSpots++
		case models.ScenicSpotDraft:
			out.DraftSpots++
		case models.ScenicSpotArchived:
			out.ArchivedSpots++
		}
	}
	linkCounts, err := s.spots.CountScenicSpotLinksBySpotIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	spotByID := make(map[string]*models.ScenicSpot, len(spots))
	for _, spot := range spots {
		spotByID[spot.ID] = spot
		count := linkCounts[spot.ID]
		out.LinkedTracks += count
		if spot.Status == models.ScenicSpotPublished {
			out.PublishedLinkedTracks += count
		}
	}
	for _, item := range top {
		if spot := spotByID[item.SpotID]; spot != nil {
			item.Name, item.Status = spot.Name, spot.Status
		}
		item.TrackCount = linkCounts[item.SpotID]
		out.TopSpots = append(out.TopSpots, *item)
	}
	sort.Slice(out.TopSpots, func(i, j int) bool {
		if out.TopSpots[i].Requests == out.TopSpots[j].Requests {
			return out.TopSpots[i].SpotID < out.TopSpots[j].SpotID
		}
		return out.TopSpots[i].Requests > out.TopSpots[j].Requests
	})
	if len(out.TopSpots) > 10 {
		out.TopSpots = out.TopSpots[:10]
	}
	return out, nil
}
