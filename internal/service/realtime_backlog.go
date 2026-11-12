package service

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type RealtimeMessage struct {
	ID         string    `json:"id"`
	SenderID   string    `json:"sender_id"`
	ReceiverID string    `json:"receiver_id,omitempty"`
	Content    string    `json:"content"`
	TrackID    string    `json:"track_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// RealtimeDirectMessages reads stored content without changing read or delivery state.
func (s *MessageService) RealtimeDirectMessages(ctx context.Context, user int64) ([]RealtimeMessage, error) {
	if !s.Enabled() || !s.durable || user <= 0 {
		return nil, repository.ErrForbidden
	}
	rows, err := s.repo.ListMessageRecords(ctx, repository.MessageFilter{Kinds: []string{"dm"}, UserID: user, EitherUser: true, ActiveOnly: true, Limit: 30})
	if err != nil {
		return nil, err
	}
	items := make([]RealtimeMessage, 0, len(rows))
	for _, row := range rows {
		var stored models.StoredDirectMessage
		if err := json.Unmarshal(row.Payload, &stored); err != nil {
			return nil, err
		}
		v := stored.Message
		items = append(items, RealtimeMessage{ID: row.ID, SenderID: strconv.FormatInt(v.SenderID, 10), ReceiverID: strconv.FormatInt(v.ReceiverID, 10), Content: v.Text, TrackID: v.TrackID, CreatedAt: row.CreatedAt})
	}
	return items, nil
}

func (s *CompanionService) SetRealtimeMonitor(m *RealtimeMonitor) { s.realtime = m }

type RealtimeBacklog struct {
	Status         string     `json:"status"`
	Pending        *int64     `json:"pending"`
	Journals       *int64     `json:"journals"`
	OldestRecordAt *time.Time `json:"oldest_record_at,omitempty"`
}

func (s *MessageService) RealtimeBacklog(ctx context.Context) (RealtimeBacklog, error) {
	v := RealtimeBacklog{Status: "unavailable"}
	if s == nil || !s.Enabled() || !s.durable {
		return v, nil
	}
	f := repository.MessageFilter{Kinds: append([]string{"dm", "notice", "user_state", "notice_read"}, interactionKinds...), PendingOnly: true, MaxTime: messageNow()}
	pending, err := s.repo.CountMessageRecords(ctx, f)
	if err != nil {
		return v, err
	}
	journals, err := s.repo.CountMessageRecords(ctx, repository.MessageFilter{Kinds: []string{"thread"}, PendingOnly: true})
	if err != nil {
		return v, err
	}
	if pending > 0 {
		f.Limit = 1
		f.Asc = true
		rows, err := s.repo.ListMessageRecords(ctx, f)
		if err != nil {
			return v, err
		}
		if len(rows) > 0 {
			at := rows[0].CreatedAt
			v.OldestRecordAt = &at
		}
	}
	v.Status = "ok"
	v.Pending, v.Journals = &pending, &journals
	return v, nil
}
