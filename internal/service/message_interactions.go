package service

import (
	"context"
	"fmt"
	"github.com/tongyichu/track_server/internal/models"
	"log"
)

func (s *TrackService) SetMessageService(m *MessageService)     { s.messages = m }
func (s *UserService) SetMessageService(m *MessageService)      { s.messages = m }
func (s *TrackLikeService) SetMessageService(m *MessageService) { s.messages = m }
func (s *CommentService) SetMessageService(m *MessageService)   { s.messages = m }
func logMessageEvent(err error) {
	if err != nil {
		log.Printf("inbox interaction write failed: %v", err)
	}
}
func (s *TrackService) notifyTrackRelation(ctx context.Context, kind string, user int64, id string, active bool) {
	if !s.messages.Enabled() {
		return
	}
	track, err := s.tracks.FindByID(ctx, id)
	if err != nil {
		return
	}
	logMessageEvent(s.messages.EmitInteraction(ctx, kind, user, track.UserID, id, "", "", fmt.Sprintf("%d:%s", user, id), active))
}
func (s *TrackLikeService) notifyLike(ctx context.Context, user int64, id string, active bool) {
	if !s.messages.Enabled() {
		return
	}
	track, err := s.tracks.FindByID(ctx, id)
	if err != nil {
		return
	}
	logMessageEvent(s.messages.EmitInteraction(ctx, "like", user, track.UserID, id, "", "", fmt.Sprintf("%d:%s", user, id), active))
}
func (s *UserService) notifyFollow(ctx context.Context, user, peer int64, active bool) {
	if !s.messages.Enabled() {
		return
	}
	logMessageEvent(s.messages.EmitInteraction(ctx, "follow", user, peer, "", "", "", fmt.Sprintf("%d:%d", user, peer), active))
}
func (s *CommentService) notifyComment(ctx context.Context, user int64, track *models.Track, c *models.TrackComment, active bool) {
	if !s.messages.Enabled() || c == nil {
		return
	}
	recipient := track.UserID
	kind := "comment"
	if c.ReplyToID != "" {
		quoted, err := s.repo.Find(ctx, c.ReplyToID)
		if err != nil {
			return
		}
		recipient = quoted.UserID
		kind = "reply"
	}
	logMessageEvent(s.messages.EmitInteraction(ctx, kind, c.UserID, recipient, track.ID, c.ID, c.Content, c.ID, active))
}
func (s *CommentService) notifyCommentReaction(ctx context.Context, user int64, c *models.TrackComment, active bool) {
	if !s.messages.Enabled() || c == nil {
		return
	}
	logMessageEvent(s.messages.EmitInteraction(ctx, "comment_like", user, c.UserID, c.TrackID, c.ID, c.Content, fmt.Sprintf("%d:%s", user, c.ID), active))
}
