package service

import (
	"context"
	"time"

	"github.com/tongyichu/track_server/internal/repository"
)

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
