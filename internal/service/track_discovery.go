package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type DiscoverSpot struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CoverURL  string `json:"cover_url"`
	SourceURL string `json:"source_url,omitempty"`
}

type DiscoverSpotsPage struct {
	Items      []DiscoverSpot `json:"items"`
	NextCursor string         `json:"next_cursor,omitempty"`
	HasMore    bool           `json:"has_more"`
}

type discoverSpotCursor struct {
	TrackCount int64  `json:"track_count"`
	ID         string `json:"id"`
}

func decodeDiscoverSpotCursor(raw string) (*discoverSpotCursor, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, invalidArg("invalid cursor")
	}
	var cursor discoverSpotCursor
	if json.Unmarshal(data, &cursor) != nil || cursor.ID == "" || cursor.TrackCount < 0 {
		return nil, invalidArg("invalid cursor")
	}
	return &cursor, nil
}

func encodeDiscoverSpotCursor(count int64, id string) string {
	data, _ := json.Marshal(discoverSpotCursor{TrackCount: count, ID: id})
	return base64.RawURLEncoding.EncodeToString(data)
}

func (s *TrackService) DiscoverSpotTracks(ctx context.Context, userID int64, spotID, rawCursor string, limit int) (*models.TrackSummaryPage, error) {
	if s.scenicCatalog == nil || !s.scenicCatalog.Enabled() {
		return nil, repository.ErrNotFound
	}
	spot, err := s.scenicCatalog.spots.GetScenicSpot(ctx, strings.TrimSpace(spotID))
	if err != nil {
		return nil, err
	}
	if !scenicSpotClientVisible(spot) {
		return nil, repository.ErrNotFound
	}
	limit = normalizeTrackPageLimit(limit)
	cursor := strings.TrimSpace(rawCursor)
	selected := make([]*models.Track, 0, limit+1)
	anchors := make([]string, 0, limit+1)
	hasMore := false
	for batch := 0; batch < 10; batch++ {
		links, err := s.scenicCatalog.spots.ListTrackScenicSpotLinks(ctx, spot.ID, cursor, 100)
		if err != nil {
			return nil, err
		}
		if len(links) == 0 {
			break
		}
		ids := make([]string, 0, len(links))
		for _, link := range links {
			ids = append(ids, link.TrackID)
		}
		byID, err := s.tracks.FindByIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, link := range links {
			cursor = link.TrackID
			track := byID[link.TrackID]
			if track == nil || track.Status != models.TrackStatusNormal || track.IsRunning || track.RawTrackURL == "" {
				continue
			}
			selected = append(selected, track)
			anchors = append(anchors, link.TrackID)
			if len(selected) > limit {
				hasMore = true
				break
			}
		}
		if hasMore {
			break
		}
		if len(links) < 100 {
			break
		}
		if batch == 9 {
			hasMore = true
		}
	}
	if len(selected) > limit {
		selected = selected[:limit]
	}
	summaries, err := s.BuildTrackSummaries(ctx, userID, selected)
	if err != nil {
		return nil, err
	}
	page := &models.TrackSummaryPage{Items: summaries, HasMore: hasMore}
	if hasMore {
		if len(anchors) >= limit {
			page.NextCursor = anchors[limit-1]
		} else {
			page.NextCursor = cursor
		}
	}
	return page, nil
}

func (s *TrackService) DiscoverSpots(ctx context.Context, cityCode, cursor string, limit int) (*DiscoverSpotsPage, error) {
	page := &DiscoverSpotsPage{Items: []DiscoverSpot{}}
	if s.scenicCatalog == nil || !s.scenicCatalog.Enabled() {
		return page, nil
	}
	anchor, err := decodeDiscoverSpotCursor(cursor)
	if err != nil {
		return nil, err
	}
	limit = normalizeTrackPageLimit(limit)
	filter := repository.ScenicSpotListFilter{VisibleOnly: true, Limit: 100}
	if code := strings.TrimSpace(cityCode); code != "" {
		filter.CityCodes = []string{code}
		filter.RelatedCitySpotIDs, err = repository.ScenicSpotIDsByTrackCities(ctx, s.scenicCatalog.spots, s.tracks, filter.CityCodes)
		if err != nil {
			return nil, err
		}
	}
	type rankedSpot struct {
		spot  *models.ScenicSpot
		count int64
	}
	ranked := make([]rankedSpot, 0)
	for {
		spots, _, err := s.scenicCatalog.spots.ListScenicSpots(ctx, filter)
		if err != nil {
			return nil, err
		}
		if len(spots) == 0 {
			break
		}
		ids := make([]string, 0, len(spots))
		for _, spot := range spots {
			ids = append(ids, spot.ID)
		}
		counts, err := repository.CountPublicScenicSpotTracks(ctx, s.scenicCatalog.spots, s.tracks, ids)
		if err != nil {
			return nil, err
		}
		for _, spot := range spots {
			count := int64(0)
			if stats := counts[spot.ID]; stats != nil {
				count = stats.TrackCount
			}
			ranked = append(ranked, rankedSpot{spot: spot, count: count})
		}
		if len(spots) < filter.Limit {
			break
		}
		filter.Cursor = spots[len(spots)-1].ID
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].count != ranked[j].count {
			return ranked[i].count > ranked[j].count
		}
		return ranked[i].spot.ID < ranked[j].spot.ID
	})
	start := 0
	if anchor != nil {
		start = sort.Search(len(ranked), func(i int) bool {
			return ranked[i].count < anchor.TrackCount ||
				(ranked[i].count == anchor.TrackCount && ranked[i].spot.ID > anchor.ID)
		})
	}
	end := min(start+limit, len(ranked))
	for _, item := range ranked[start:end] {
		spot := item.spot
		cover := spot.CoverURL
		source := ""
		if len(spot.Photos) > 0 && strings.TrimSpace(spot.Photos[0].URL) != "" {
			cover = spot.Photos[0].URL
			source = spot.Photos[0].SourceURL
		}
		page.Items = append(page.Items, DiscoverSpot{ID: spot.ID, Name: spot.Name, CoverURL: cover, SourceURL: source})
	}
	page.HasMore = end < len(ranked)
	if page.HasMore {
		last := ranked[end-1]
		page.NextCursor = encodeDiscoverSpotCursor(last.count, last.spot.ID)
	}
	return page, nil
}

func (s *TrackService) DiscoverShapes(ctx context.Context, userID int64, cityCode, rawCursor string, limit int) (*models.TrackSummaryPage, error) {
	limit = normalizeTrackPageLimit(limit)
	cursor, err := decodeTrackListCursor(rawCursor)
	if err != nil {
		return nil, err
	}
	tracks, err := s.tracks.ListShaped(ctx, strings.TrimSpace(cityCode), cursor, limit+1)
	if err != nil {
		return nil, err
	}
	hasMore := len(tracks) > limit
	if hasMore {
		tracks = tracks[:limit]
	}
	summaries, err := s.BuildTrackSummaries(ctx, userID, tracks)
	if err != nil {
		return nil, err
	}
	return buildTrackSummaryPage(summaries, hasMore)
}

type ActivityDay struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}
type ActivityCalendar struct {
	StartDate  string        `json:"start_date"`
	EndDate    string        `json:"end_date"`
	TotalCount int           `json:"total_count"`
	Days       []ActivityDay `json:"days"`
}

func (s *TrackService) ActivityCalendar(ctx context.Context, userID int64, timezone string) (*ActivityCalendar, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, invalidArg("invalid timezone")
	}
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	// 为客户端连续 12 个月的首月完整周保留缓冲。
	from := today.AddDate(0, 0, -374)
	until := today.AddDate(0, 0, 1)
	starts, err := s.tracks.ListActivityStartTimes(ctx, userID, from, until)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int)
	for _, start := range starts {
		counts[start.In(loc).Format("2006-01-02")]++
	}
	result := &ActivityCalendar{StartDate: from.Format("2006-01-02"), EndDate: today.Format("2006-01-02"), TotalCount: len(starts), Days: make([]ActivityDay, 0, len(counts))}
	for day := from; !day.After(today); day = day.AddDate(0, 0, 1) {
		date := day.Format("2006-01-02")
		if count := counts[date]; count > 0 {
			result.Days = append(result.Days, ActivityDay{Date: date, Count: count})
		}
	}
	return result, nil
}
