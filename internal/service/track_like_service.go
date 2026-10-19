package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type TrackLikeStatus struct {
	models.TrackLikeStats
	CanLike bool `json:"can_like"`
}
type ReceivedLikeUser struct {
	ID        int64  `json:"id"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
}
type ReceivedLike struct {
	User    ReceivedLikeUser     `json:"user"`
	Track   *models.TrackSummary `json:"track"`
	LikedAt time.Time            `json:"liked_at"`
}
type ReceivedLikePage struct {
	Items      []*ReceivedLike `json:"items"`
	TotalCount int64           `json:"total_count"`
	NextCursor string          `json:"next_cursor,omitempty"`
	HasMore    bool            `json:"has_more"`
}
type TrackLikeService struct {
	likes      repository.TrackLikeRepository
	tracks     repository.TrackRepository
	users      *UserService
	trackViews *TrackService
}

func NewTrackLikeService(likes repository.TrackLikeRepository, tracks repository.TrackRepository, users *UserService, trackViews *TrackService) *TrackLikeService {
	return &TrackLikeService{likes: likes, tracks: tracks, users: users, trackViews: trackViews}
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
func (s *TrackLikeService) Received(ctx context.Context, viewer, target int64, cursor string, limit int) (*ReceivedLikePage, error) {
	if viewer <= 0 || target <= 0 {
		return nil, invalidArg("user_id must be positive")
	}
	if _, err := s.users.users.FindByID(ctx, target); err != nil {
		return nil, err
	}
	var c *models.TrackLikeCursor
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return nil, invalidArg("invalid like cursor")
		}
		c = &models.TrackLikeCursor{}
		if json.Unmarshal(raw, c) != nil || c.CreatedAt.IsZero() || c.TrackID == "" || c.UserID <= 0 {
			return nil, invalidArg("invalid like cursor")
		}
	}
	limit = normalizeTrackPageLimit(limit)
	records, err := s.likes.ListReceived(ctx, target, c, limit+1)
	if err != nil {
		return nil, err
	}
	count, err := s.likes.CountReceived(ctx, target)
	if err != nil {
		return nil, err
	}
	page := &ReceivedLikePage{Items: make([]*ReceivedLike, 0), TotalCount: count, HasMore: len(records) > limit}
	if page.HasMore {
		records = records[:limit]
		last := records[len(records)-1]
		raw, err := json.Marshal(models.TrackLikeCursor{CreatedAt: last.CreatedAt, TrackID: last.TrackID, UserID: last.UserID})
		if err != nil {
			return nil, err
		}
		page.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.TrackID)
	}
	byID, err := s.tracks.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	tracks := make([]*models.Track, 0, len(records))
	visible := make([]*models.TrackLike, 0, len(records))
	for _, record := range records {
		if t := byID[record.TrackID]; likeEligible(t) && t.UserID == target && record.UserID != target {
			tracks = append(tracks, t)
			visible = append(visible, record)
		}
	}
	summaries, err := s.trackViews.BuildTrackSummaries(ctx, viewer, tracks)
	if err != nil {
		return nil, err
	}
	users := make(map[int64]ReceivedLikeUser)
	for i, record := range visible {
		user, ok := users[record.UserID]
		if !ok {
			u, err := s.users.users.FindByID(ctx, record.UserID)
			if err != nil && !errors.Is(err, repository.ErrNotFound) {
				return nil, err
			}
			if u != nil {
				copy := *u
				s.users.decorateUserAvatar(ctx, &copy)
				user = ReceivedLikeUser{ID: copy.ID, Nickname: copy.Nickname, AvatarURL: copy.AvatarURL}
			}
			users[record.UserID] = user
		}
		page.Items = append(page.Items, &ReceivedLike{User: user, Track: summaries[i], LikedAt: record.CreatedAt})
	}
	return page, nil
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
