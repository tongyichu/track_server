package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type ReceivedLikeTrack struct {
	Track             *models.TrackSummary `json:"track"`
	ReceivedLikeCount int64                `json:"received_like_count"`
	RecentUsers       []ReceivedLikeUser   `json:"recent_users"`
	LastLikedAt       time.Time            `json:"last_liked_at"`
}
type ReceivedLikeTrackPage struct {
	Items      []*ReceivedLikeTrack `json:"items"`
	TotalCount int64                `json:"total_count"`
	NextCursor string               `json:"next_cursor,omitempty"`
	HasMore    bool                 `json:"has_more"`
}
type TrackLiker struct {
	User    ReceivedLikeUser `json:"user"`
	LikedAt time.Time        `json:"liked_at"`
}
type TrackLikerPage struct {
	Items      []*TrackLiker `json:"items"`
	TotalCount int64         `json:"total_count"`
	NextCursor string        `json:"next_cursor,omitempty"`
	HasMore    bool          `json:"has_more"`
}

func likeCursorEncode(value interface{}) (string, error) {
	raw, err := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(raw), err
}
func likeCursorDecode(value string, target interface{}) error {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || json.Unmarshal(raw, target) != nil {
		return invalidArg("invalid like cursor")
	}
	return nil
}
func (s *TrackLikeService) requireLikeProfile(ctx context.Context, viewer, target int64) error {
	if viewer <= 0 || target <= 0 {
		return invalidArg("user_id must be positive")
	}
	_, err := s.users.users.FindByID(ctx, target)
	return err
}
func (s *TrackLikeService) likeUsers(ctx context.Context, ids []int64) (map[int64]ReceivedLikeUser, error) {
	users, err := s.users.users.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make(map[int64]ReceivedLikeUser, len(users))
	for id, user := range users {
		copy := *user
		s.users.decorateUserAvatar(ctx, &copy)
		result[id] = ReceivedLikeUser{ID: copy.ID, Nickname: copy.Nickname, AvatarURL: copy.AvatarURL}
	}
	return result, nil
}
func (s *TrackLikeService) ReceivedTracks(ctx context.Context, viewer, target int64, cursor string, limit int) (*ReceivedLikeTrackPage, error) {
	if err := s.requireLikeProfile(ctx, viewer, target); err != nil {
		return nil, err
	}
	var c *models.ReceivedLikeTrackCursor
	if cursor != "" {
		c = &models.ReceivedLikeTrackCursor{}
		if err := likeCursorDecode(cursor, c); err != nil {
			return nil, err
		}
		if c.LastLikedAt.IsZero() || c.TrackID == "" {
			return nil, invalidArg("invalid like cursor")
		}
	}
	limit = normalizeTrackPageLimit(limit)
	groups, err := s.likes.ListReceivedGroups(ctx, target, c, limit+1)
	if err != nil {
		return nil, err
	}
	count, err := s.likes.CountReceivedGroups(ctx, target)
	if err != nil {
		return nil, err
	}
	page := &ReceivedLikeTrackPage{Items: make([]*ReceivedLikeTrack, 0), TotalCount: count, HasMore: len(groups) > limit}
	if page.HasMore {
		groups = groups[:limit]
		last := groups[len(groups)-1]
		page.NextCursor, err = likeCursorEncode(models.ReceivedLikeTrackCursor{TrackID: last.TrackID, LastLikedAt: last.LastLikedAt})
		if err != nil {
			return nil, err
		}
	}
	ids := make([]string, 0, len(groups))
	userIDs := make([]int64, 0, len(groups)*3)
	for _, group := range groups {
		ids = append(ids, group.TrackID)
		for _, like := range group.RecentLikes {
			userIDs = append(userIDs, like.UserID)
		}
	}
	byID, err := s.tracks.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	tracks := make([]*models.Track, 0, len(groups))
	visible := make([]*models.ReceivedLikeTrack, 0, len(groups))
	for _, group := range groups {
		track := byID[group.TrackID]
		if likeEligible(track) && track.UserID == target {
			tracks = append(tracks, track)
			visible = append(visible, group)
		}
	}
	summaries, err := s.trackViews.BuildTrackSummaries(ctx, viewer, tracks)
	if err != nil {
		return nil, err
	}
	users, err := s.likeUsers(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	for i, group := range visible {
		item := &ReceivedLikeTrack{Track: summaries[i], ReceivedLikeCount: group.LikeCount, LastLikedAt: group.LastLikedAt, RecentUsers: make([]ReceivedLikeUser, 0, 3)}
		sort.Slice(group.RecentLikes, func(i, j int) bool {
			a, b := group.RecentLikes[i], group.RecentLikes[j]
			if a.CreatedAt.Equal(b.CreatedAt) {
				return a.UserID > b.UserID
			}
			return a.CreatedAt.After(b.CreatedAt)
		})
		for _, like := range group.RecentLikes {
			if like.UserID != target {
				if user, ok := users[like.UserID]; ok {
					item.RecentUsers = append(item.RecentUsers, user)
				}
			}
			if len(item.RecentUsers) == 3 {
				break
			}
		}
		page.Items = append(page.Items, item)
	}
	return page, nil
}
func (s *TrackLikeService) ReceivedTrackLikers(ctx context.Context, viewer, target int64, trackID, cursor string, limit int) (*TrackLikerPage, error) {
	if err := s.requireLikeProfile(ctx, viewer, target); err != nil {
		return nil, err
	}
	track, err := s.tracks.FindByID(ctx, trackID)
	if err != nil {
		return nil, err
	}
	if !likeEligible(track) || track.UserID != target {
		return nil, repository.ErrNotFound
	}
	var c *models.TrackLikeCursor
	if cursor != "" {
		c = &models.TrackLikeCursor{}
		if err := likeCursorDecode(cursor, c); err != nil {
			return nil, err
		}
		if c.CreatedAt.IsZero() || c.TrackID != trackID || c.UserID <= 0 {
			return nil, invalidArg("invalid like cursor")
		}
	}
	limit = normalizeTrackPageLimit(limit)
	records, err := s.likes.ListReceivedTrackLikers(ctx, target, trackID, c, limit+1)
	if err != nil {
		return nil, err
	}
	stats, err := s.likes.Stats(ctx, target, []string{trackID})
	if err != nil {
		return nil, err
	}
	count := stats[trackID].LikeCount
	if stats[trackID].Liked {
		count--
	}
	page := &TrackLikerPage{Items: make([]*TrackLiker, 0), TotalCount: count, HasMore: len(records) > limit}
	if page.HasMore {
		records = records[:limit]
		last := records[len(records)-1]
		page.NextCursor, err = likeCursorEncode(models.TrackLikeCursor{TrackID: trackID, CreatedAt: last.CreatedAt, UserID: last.UserID})
		if err != nil {
			return nil, err
		}
	}
	ids := make([]int64, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.UserID)
	}
	users, err := s.likeUsers(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		page.Items = append(page.Items, &TrackLiker{User: users[record.UserID], LikedAt: record.CreatedAt})
	}
	return page, nil
}
