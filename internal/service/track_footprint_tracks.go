package service

import (
	"context"
	"sort"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
)

func (s *TrackService) FootprintTracks(ctx context.Context, userID int64, regionID, trackType, cursorRaw string, limit int) (*models.TrackSummaryPage, error) {
	regionID = strings.TrimSpace(regionID)
	if regionID == "" {
		return nil, invalidArg("region_id is required")
	}
	cursor, err := decodeTrackListCursor(cursorRaw)
	if err != nil {
		return nil, err
	}
	geo, known, err := s.footprintRegionsForMap(ctx)
	if err != nil {
		return nil, err
	}
	allowed := map[string]bool{}
	for _, c := range geo.Cities {
		if c.ID == regionID || c.ProvinceID == regionID {
			allowed[c.ID] = true
		}
	}
	if len(allowed) == 0 {
		return nil, invalidArg("unknown footprint region")
	}
	tracks, err := s.footprintTracks(ctx, userID, trackType)
	if err != nil {
		return nil, err
	}
	selected := make([]models.FootprintTrack, 0)
	for _, t := range tracks {
		if !allowed[known[strings.TrimSpace(t.CityCode)]] {
			continue
		}
		if cursor != nil && (t.StartTime.After(cursor.StartTime) || (t.StartTime.Equal(cursor.StartTime) && t.ID >= cursor.ID)) {
			continue
		}
		selected = append(selected, t)
	}
	sort.Slice(selected, func(i, j int) bool {
		if selected[i].StartTime.Equal(selected[j].StartTime) {
			return selected[i].ID > selected[j].ID
		}
		return selected[i].StartTime.After(selected[j].StartTime)
	})
	limit = normalizeTrackPageLimit(limit)
	hasMore := len(selected) > limit
	selected = selected[:min(limit, len(selected))]
	ids := make([]string, 0, len(selected))
	for _, t := range selected {
		ids = append(ids, t.ID)
	}
	byID, err := s.tracks.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	values := make([]*models.Track, 0, len(ids))
	for _, id := range ids {
		t := byID[id]
		if t != nil && t.UserID == userID && !t.IsRunning && strings.TrimSpace(t.RawTrackURL) != "" && (t.Status == models.TrackStatusNormal || t.Status == models.TrackStatusPrivate) && allowed[known[strings.TrimSpace(t.CityCode)]] {
			values = append(values, t)
		}
	}
	page, err := buildTrackSummaryPage(toSummaries(values), false)
	if err != nil {
		return nil, err
	}
	page.HasMore = hasMore
	if hasMore && len(selected) > 0 {
		anchor := selected[len(selected)-1]
		page.NextCursor, err = encodeTrackListCursor(anchor.StartTime, anchor.ID)
	}
	return page, err
}
