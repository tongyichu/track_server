package service

import (
	"context"
	"strconv"
	"time"

	"github.com/tongyichu/track_server/internal/repository"
)

type RealtimeNetworkNode struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	UserID      string `json:"user_id,omitempty"`
	Label       string `json:"label"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	Connections int    `json:"connections"`
	ClientID    string `json:"client_id,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
}

type RealtimeNetworkEdge struct {
	ID     string     `json:"id"`
	Source string     `json:"source"`
	Target string     `json:"target"`
	Kind   string     `json:"kind"`
	LastAt *time.Time `json:"last_at,omitempty"`
}

func (s *MessageService) RealtimeNetworkEdges(ctx context.Context, users []int64) ([]RealtimeNetworkEdge, bool, error) {
	edges := []RealtimeNetworkEdge{}
	if len(users) < 2 {
		return edges, true, nil
	}
	if !s.Enabled() || !s.durable {
		return nil, false, repository.ErrForbidden
	}
	rows, err := s.repo.ListMessageRecords(ctx, repository.MessageFilter{Kinds: []string{"thread"}, Participants: users, ActiveOnly: true, HasMessages: true, Limit: 401})
	if err != nil {
		return nil, false, err
	}
	complete := len(rows) <= 400
	if !complete {
		rows = rows[:400]
	}
	for _, row := range rows {
		at := row.CreatedAt
		edges = append(edges, RealtimeNetworkEdge{ID: row.ID, Source: strconv.FormatInt(row.UserID, 10), Target: strconv.FormatInt(row.PeerID, 10), Kind: "direct", LastAt: &at})
	}
	return edges, complete, nil
}
