package service

import (
	"context"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

func (s *TrackService) companionTrackIDs(ctx context.Context, refs []repository.CompanionTrackReference) (map[string]bool, error) {
	reader, ok := s.companions.(repository.CompanionTrackOriginRepository)
	if !ok {
		return map[string]bool{}, nil
	}
	return reader.FindCompanionTrackIDs(ctx, refs)
}

func (s *TrackService) fillTrackCompanionOrigin(ctx context.Context, track *models.Track) error {
	origins, err := s.companionTrackIDs(ctx, []repository.CompanionTrackReference{{TrackID: track.ID, SessionID: track.SessionID, UserID: track.UserID}})
	if err != nil {
		return err
	}
	track.IsCompanion = origins[track.ID]
	return nil
}

func (s *TrackService) fillSummaryCompanionOrigins(ctx context.Context, summaries []*models.TrackSummary) error {
	refs := make([]repository.CompanionTrackReference, 0, len(summaries))
	for _, item := range summaries {
		refs = append(refs, repository.CompanionTrackReference{TrackID: item.ID, SessionID: item.SessionID, UserID: item.UserID})
	}
	origins, err := s.companionTrackIDs(ctx, refs)
	if err != nil {
		return err
	}
	for _, item := range summaries {
		item.IsCompanion = origins[item.ID]
		item.Labels = buildTrackDisplayLabels(item.ShapeLabel, item.TrackType, item.PrimaryScenicSpot, item.IsFeatured, item.IsCompanion)
	}
	return nil
}

func (s *TrackService) fillMySummaryCompanionOrigins(ctx context.Context, summaries []*models.MyTrackSummary) error {
	refs := make([]repository.CompanionTrackReference, 0, len(summaries))
	for _, item := range summaries {
		refs = append(refs, repository.CompanionTrackReference{TrackID: item.ID, SessionID: item.SessionID, UserID: item.UserID})
	}
	origins, err := s.companionTrackIDs(ctx, refs)
	if err != nil {
		return err
	}
	for _, item := range summaries {
		item.IsCompanion = origins[item.ID]
	}
	return nil
}
