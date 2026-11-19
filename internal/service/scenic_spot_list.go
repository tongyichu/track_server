package service

import (
	"context"
	"sort"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type ScenicSpotCatalogPage struct {
	Items           []*models.ScenicSpot
	Total           int64
	NextCursor      string
	HasMore         bool
	TrackStatistics map[string]*repository.ScenicSpotTrackStatistics
}

func (s *ScenicSpotCatalogService) ListRanked(ctx context.Context, f repository.ScenicSpotListFilter, editMode string) (*ScenicSpotCatalogPage, error) {
	filter, err := s.prepareListFilter(ctx, f)
	if err != nil {
		return nil, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	filter.Cursor, filter.Limit = "", 100
	type rankedSpot struct {
		spot       *models.ScenicSpot
		grade      int
		statistics *repository.ScenicSpotTrackStatistics
	}
	ranked := make([]rankedSpot, 0)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		spots, _, err := s.spots.ListScenicSpots(ctx, filter)
		if err != nil {
			return nil, err
		}
		if len(spots) == 0 {
			break
		}
		matches := make([]*models.ScenicSpot, 0, len(spots))
		ids := make([]string, 0, len(spots))
		for _, spot := range spots {
			if editMode != "" && ScenicSpotEditMode(spot) != editMode {
				continue
			}
			matches = append(matches, spot)
			ids = append(ids, spot.ID)
		}
		if len(ids) > 0 {
			statistics, err := s.RelatedTrackStatistics(ctx, ids)
			if err != nil {
				return nil, err
			}
			for _, spot := range matches {
				stats := statistics[spot.ID]
				if stats == nil {
					stats = &repository.ScenicSpotTrackStatistics{}
				}
				ranked = append(ranked, rankedSpot{spot: spot, grade: scenicSpotGradeRank(spot.ScenicGrade), statistics: stats})
			}
		}
		if len(spots) < filter.Limit {
			break
		}
		filter.Cursor = spots[len(spots)-1].ID
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].grade != ranked[j].grade {
			return ranked[i].grade > ranked[j].grade
		}
		if ranked[i].statistics.TrackCount != ranked[j].statistics.TrackCount {
			return ranked[i].statistics.TrackCount > ranked[j].statistics.TrackCount
		}
		return ranked[i].spot.ID < ranked[j].spot.ID
	})
	start := 0
	if cursor := strings.TrimSpace(f.Cursor); cursor != "" {
		found := false
		for i, item := range ranked {
			if item.spot.ID == cursor {
				start, found = i+1, true
				break
			}
		}
		if !found {
			return nil, invalidScenicSpot("cursor expired; reload scenic spots")
		}
	}
	end := min(start+limit, len(ranked))
	page := &ScenicSpotCatalogPage{
		Items:           make([]*models.ScenicSpot, 0, end-start),
		Total:           int64(len(ranked)),
		HasMore:         end < len(ranked),
		TrackStatistics: make(map[string]*repository.ScenicSpotTrackStatistics, end-start),
	}
	for _, item := range ranked[start:end] {
		page.Items = append(page.Items, item.spot)
		page.TrackStatistics[item.spot.ID] = item.statistics
	}
	if page.HasMore {
		page.NextCursor = ranked[end-1].spot.ID
	}
	return page, nil
}

// ScenicSpotEditMode classifies legacy entries and entries explicitly edited by an operator.
func ScenicSpotEditMode(spot *models.ScenicSpot) string {
	if spot.ManualEdited {
		return "manual"
	}
	for _, source := range spot.Sources {
		if source.Status == "active" && source.ProviderID != "" {
			return "automatic"
		}
	}
	if spot.ProviderDataStatus != "synced" && (spot.Name != "" || spot.Category != "" || spot.Address != "" || spot.Description != "" || len(spot.Photos) > 0) {
		return "manual"
	}
	return "automatic"
}
