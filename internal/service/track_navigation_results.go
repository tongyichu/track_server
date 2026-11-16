package service

import (
	"context"
	"errors"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

func (s *TrackService) ListNavigationResultTracks(ctx context.Context, viewer int64, trackID, rawCursor string, limit int) (*models.TrackSummaryPage, error) {
	if viewer <= 0 || strings.TrimSpace(trackID) == "" {
		return nil, invalidArg("user and track are required")
	}
	track, err := s.tracks.FindByID(ctx, trackID)
	if err != nil {
		return nil, err
	}
	if track.UserID != viewer {
		return nil, repository.ErrForbidden
	}
	if track.Status == models.TrackStatusDeleted || !track.DeletedAt.IsZero() {
		return nil, repository.ErrNotFound
	}
	cursor, err := decodeTrackListCursor(rawCursor)
	if err != nil {
		return nil, err
	}
	reader, ok := s.navigations.(repository.NavigationResultTrackRepository)
	if !ok {
		return nil, errors.New("navigation result storage unavailable")
	}
	limit = normalizeTrackPageLimit(limit)
	tracks, err := reader.ListResultTracks(ctx, trackID, viewer, cursor, limit+1)
	if err != nil {
		return nil, err
	}
	hasMore := len(tracks) > limit
	if hasMore {
		tracks = tracks[:limit]
	}
	summaries, err := s.BuildTrackSummaries(ctx, viewer, tracks)
	if err != nil {
		return nil, err
	}
	return buildTrackSummaryPage(summaries, hasMore)
}
