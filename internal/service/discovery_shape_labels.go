package service

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type DiscoveryShapeLabels struct {
	Items         []models.DiscoveryShapeLabelCount `json:"items"`
	TotalCount    int64                             `json:"total_count"`
	FilterVersion int                               `json:"filter_version"`
}

func (s *TrackService) DiscoveryShapeLabels(ctx context.Context, city, trackType string) (*DiscoveryShapeLabels, error) {
	city, trackType = strings.TrimSpace(city), strings.TrimSpace(trackType)
	if city == "" {
		return nil, invalidArg("city_code is required")
	}
	repo, ok := s.tracks.(repository.DiscoveryShapeLabelRepository)
	if !ok {
		return nil, errors.New("shape label statistics unavailable")
	}
	rows, err := repo.AggregateDiscoveryShapeLabels(ctx, city, trackType)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int64)
	for _, row := range rows {
		if label := strings.TrimSpace(row.ShapeLabel); label != "" && row.TrackCount > 0 {
			counts[label] += row.TrackCount
		}
	}
	result := &DiscoveryShapeLabels{Items: make([]models.DiscoveryShapeLabelCount, 0, len(counts)), FilterVersion: 1}
	for label, count := range counts {
		result.Items = append(result.Items, models.DiscoveryShapeLabelCount{ShapeLabel: label, TrackCount: count})
		result.TotalCount += count
	}
	sort.Slice(result.Items, func(i, j int) bool {
		if result.Items[i].TrackCount != result.Items[j].TrackCount {
			return result.Items[i].TrackCount > result.Items[j].TrackCount
		}
		return result.Items[i].ShapeLabel < result.Items[j].ShapeLabel
	})
	return result, nil
}
