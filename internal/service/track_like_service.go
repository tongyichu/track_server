package service

import (
	"context"
	"errors"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type TrackLikeStatus struct {
	models.TrackLikeStats
	CanLike bool `json:"can_like"`
}
type TrackLikeService struct {
	likes  repository.TrackLikeRepository
	tracks repository.TrackRepository
}

func NewTrackLikeService(likes repository.TrackLikeRepository, tracks repository.TrackRepository) *TrackLikeService {
	return &TrackLikeService{likes: likes, tracks: tracks}
}
func likeEligible(track *models.Track) bool {
	return track != nil && track.Status == models.TrackStatusNormal && !track.IsRunning && strings.TrimSpace(track.RawTrackURL) != ""
}
func (s *TrackLikeService) Status(ctx context.Context, viewer int64, trackID string) (*TrackLikeStatus, error) {
	track, err := s.tracks.FindByID(ctx, trackID)
	if err != nil {
		return nil, err
	}
	if !likeEligible(track) {
		if track.UserID == viewer && track.Status != models.TrackStatusDeleted {
			return &TrackLikeStatus{}, nil
		}
		return nil, repository.ErrNotFound
	}
	stats, err := s.likes.Stats(ctx, viewer, []string{trackID})
	if err != nil {
		return nil, err
	}
	return &TrackLikeStatus{TrackLikeStats: stats[trackID], CanLike: true}, nil
}
func (s *TrackLikeService) Set(ctx context.Context, viewer int64, trackID string, liked bool) (*TrackLikeStatus, error) {
	if viewer <= 0 || strings.TrimSpace(trackID) == "" {
		return nil, invalidArg("user_id and track_id are required")
	}
	if liked {
		track, err := s.tracks.FindByID(ctx, trackID)
		if err != nil {
			return nil, err
		}
		if !likeEligible(track) {
			return nil, repository.ErrNotFound
		}
	}
	if err := s.likes.SetLike(ctx, viewer, trackID, liked); err != nil {
		return nil, err
	}
	status, err := s.Status(ctx, viewer, trackID)
	if !liked && errors.Is(err, repository.ErrNotFound) {
		return &TrackLikeStatus{}, nil
	}
	return status, err
}
func (s *TrackService) SetTrackLikeRepository(likes repository.TrackLikeRepository) { s.likes = likes }
func (s *TrackService) trackLikeStats(ctx context.Context, viewer int64, ids []string) (map[string]models.TrackLikeStats, error) {
	if s.likes == nil {
		return map[string]models.TrackLikeStats{}, nil
	}
	return s.likes.Stats(ctx, viewer, ids)
}
func (s *TrackService) FillTrackLikeState(ctx context.Context, viewer int64, track *models.Track) error {
	if track == nil || track.Status != models.TrackStatusNormal || track.IsRunning {
		return nil
	}
	if strings.TrimSpace(track.RawTrackURL) == "" {
		// Asset cache failures clear the response URL without making the stored route unpublished.
		stored, err := s.tracks.FindByID(ctx, track.ID)
		if err != nil {
			return err
		}
		if !likeEligible(stored) {
			return nil
		}
	}
	stats, err := s.trackLikeStats(ctx, viewer, []string{track.ID})
	if err != nil {
		return err
	}
	track.Liked = stats[track.ID].Liked
	track.LikeCount = stats[track.ID].LikeCount
	return nil
}
func (s *UserService) SetTrackLikeRepository(likes repository.TrackLikeRepository) { s.likes = likes }
