package service

import (
	"context"

	"github.com/tongyichu/track_server/internal/models"
)

type TrackCollectStatus struct {
	Status       string `json:"status"`
	Collected    bool   `json:"collected"`
	CollectCount int64  `json:"collect_count"`
}

func (s *TrackService) collectCount(ctx context.Context, trackID string) (int64, error) {
	if s.collects == nil {
		return 0, nil
	}
	counts, err := s.collects.CountByTrackIDs(ctx, []string{trackID})
	return counts[trackID], err
}

func (s *TrackService) FillTrackCollectedState(ctx context.Context, viewer int64, track *models.Track) error {
	track.Collected = false
	if viewer <= 0 || s.collects == nil {
		return nil
	}
	collected, err := s.collects.IsCollected(ctx, viewer, track.ID)
	track.Collected = collected
	return err
}

func (s *TrackService) fillTrackCollectSummary(ctx context.Context, viewer int64, track *models.Track) error {
	count, err := s.collectCount(ctx, track.ID)
	if err != nil {
		return err
	}
	track.CollectCount = count
	return s.FillTrackCollectedState(ctx, viewer, track)
}

func (s *TrackService) CollectionStatus(ctx context.Context, viewer int64, trackID string) (*TrackCollectStatus, error) {
	track := &models.Track{ID: trackID}
	if err := s.fillTrackCollectSummary(ctx, viewer, track); err != nil {
		return nil, err
	}
	return &TrackCollectStatus{Status: "ok", Collected: track.Collected, CollectCount: track.CollectCount}, nil
}
