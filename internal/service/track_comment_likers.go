package service

import (
	"context"
	"encoding/base64"
	"encoding/json"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type CommentLikerPage struct {
	Items      []CommentUser `json:"items"`
	TotalCount int64         `json:"total_count"`
	HasMore    bool          `json:"has_more"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

func (s *CommentService) Likers(ctx context.Context, viewer int64, trackID, id, cursor string, limit int) (*CommentLikerPage, error) {
	if _, err := s.eligible(ctx, viewer, trackID); err != nil {
		return nil, err
	}
	comment, err := s.repo.Find(ctx, id)
	if err != nil {
		return nil, err
	}
	if comment.TrackID != trackID || comment.Deleted {
		return nil, repository.ErrNotFound
	}
	var before int64
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		var value models.CommentLikerCursor
		if err != nil || json.Unmarshal(raw, &value) != nil || value.TrackID != trackID || value.CommentID != id || value.UserID <= 0 {
			return nil, invalidArg("invalid comment liker cursor")
		}
		before = value.UserID
	}
	if limit < 0 {
		return nil, invalidArg("invalid limit")
	}
	limit = normalizeTrackPageLimit(limit)
	ids, err := s.repo.Likers(ctx, id, before, limit+1)
	if err != nil {
		return nil, err
	}
	page := &CommentLikerPage{Items: []CommentUser{}, TotalCount: comment.LikeCount, HasMore: len(ids) > limit}
	if page.HasMore {
		ids = ids[:limit]
		raw, _ := json.Marshal(models.CommentLikerCursor{TrackID: trackID, CommentID: id, UserID: ids[len(ids)-1]})
		page.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	users, err := s.commentUsers(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		user := users[id]
		if user.ID == 0 {
			user.ID = id
		}
		page.Items = append(page.Items, user)
	}
	return page, nil
}

func (s *CommentService) commentUsers(ctx context.Context, ids []int64) (map[int64]CommentUser, error) {
	users, err := s.users.users.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make(map[int64]CommentUser, len(users))
	for id, u := range users {
		avatar := fallbackAvatarURL(id, u.AvatarURL)
		if cache := s.users.avatarCache; cache != nil && shouldRewriteAvatarURL(avatar) {
			key := formatUserAvatarCacheKey(id)
			if ext, ok := cache.Exists(key); ok {
				avatar = cache.LocalURL(key, ext)
			} else {
				cache.PrefetchAsync(id, key, avatar)
				avatar = defaultAvatarURL(id)
			}
		}
		result[id] = CommentUser{id, u.Nickname, avatar}
	}
	return result, nil
}
