package service

import (
	"context"

	"github.com/tongyichu/track_server/internal/models"
)

func (s *TrackService) trackNavigationCounts(ctx context.Context, trackIDs []string) (map[string]int64, error) {
	if s.navigations == nil || len(trackIDs) == 0 {
		return map[string]int64{}, nil
	}
	return s.navigations.CountByTrackIDs(ctx, trackIDs)
}

func (s *TrackService) fillTrackNavigationSummary(ctx context.Context, track *models.Track) error {
	counts, err := s.trackNavigationCounts(ctx, []string{track.ID})
	if err != nil {
		return err
	}
	track.NavigateCount = counts[track.ID]
	return nil
}

func (s *TrackService) fillSummaryNavigationCounts(ctx context.Context, summaries []*models.TrackSummary) error {
	ids := make([]string, 0, len(summaries))
	seen := make(map[string]bool, len(summaries))
	for _, summary := range summaries {
		if !seen[summary.ID] {
			ids = append(ids, summary.ID)
			seen[summary.ID] = true
		}
	}
	counts, err := s.trackNavigationCounts(ctx, ids)
	if err != nil {
		return err
	}
	for _, summary := range summaries {
		summary.NavigateCount = counts[summary.ID]
	}
	return nil
}
