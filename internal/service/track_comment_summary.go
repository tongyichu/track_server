package service

import (
	"context"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

func (s *TrackService) SetTrackCommentRepository(comments repository.CommentRepository) {
	s.comments = comments
}

func (s *TrackService) trackCommentCounts(ctx context.Context, ids []string) (map[string]int64, error) {
	if s.comments == nil {
		return map[string]int64{}, nil
	}
	return s.comments.Totals(ctx, ids)
}

func (s *TrackService) fillTrackCommentState(ctx context.Context, track *models.Track) error {
	track.CanComment = likeEligible(track)
	track.CommentCount = 0
	if !track.CanComment || s.comments == nil {
		return nil
	}
	count, err := s.comments.Total(ctx, track.ID)
	track.CommentCount = count
	return err
}
