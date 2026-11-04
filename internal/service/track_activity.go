package service

import (
	"context"
	"errors"
	"math"
	"sort"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type ActivityTypeDistribution struct {
	Items []models.ActivityTypeTotal `json:"items"`
}

func (s *TrackService) ActivityTypeDistribution(ctx context.Context, userID int64) (*ActivityTypeDistribution, error) {
	if userID <= 0 {
		return nil, invalidArg("userID is required")
	}
	repo, ok := s.tracks.(repository.ActivityTypeStatisticsRepository)
	if !ok {
		return nil, errors.New("activity type statistics unavailable")
	}
	items, err := repo.AggregateActivityTypes(ctx, userID)
	if err != nil {
		return nil, err
	}
	groups := make(map[string]*models.ActivityTypeTotal)
	for _, item := range items {
		code := normalizeTrackTypeCode(item.TrackType)
		if code == "" {
			code = "other"
		}
		total := groups[code]
		if total == nil {
			total = &models.ActivityTypeTotal{TrackType: code}
			groups[code] = total
		}
		total.TrackCount += max(item.TrackCount, 0)
		total.DurationSeconds += max(item.DurationSeconds, 0)
		if item.DistanceMeters > 0 && !math.IsNaN(item.DistanceMeters) && !math.IsInf(item.DistanceMeters, 0) {
			total.DistanceMeters += item.DistanceMeters
		}
	}
	out := &ActivityTypeDistribution{Items: make([]models.ActivityTypeTotal, 0, len(groups))}
	for _, item := range groups {
		out.Items = append(out.Items, *item)
	}
	sort.Slice(out.Items, func(i, j int) bool {
		if out.Items[i].DurationSeconds != out.Items[j].DurationSeconds {
			return out.Items[i].DurationSeconds > out.Items[j].DurationSeconds
		}
		return out.Items[i].TrackType < out.Items[j].TrackType
	})
	return out, nil
}
