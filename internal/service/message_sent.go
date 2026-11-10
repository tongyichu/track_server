package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

func (s *MessageService) FindSentDirect(ctx context.Context, user, peer int64, clientID string) (*models.DirectMessage, error) {
	if len(clientID) < 8 || len(clientID) > 80 {
		return nil, invalidArg("invalid client ID")
	}
	row, thread, err := s.getThread(ctx, user, peer)
	if err != nil {
		return nil, err
	}
	if thread.Pending != nil {
		if err = s.flushThread(ctx, row, thread); err != nil {
			return nil, err
		}
	}
	id := "D." + messageHash(fmt.Sprintf("%d:%d:%s", user, peer, clientID))
	row, err = s.repo.GetMessageRecord(ctx, id)
	if err != nil {
		return nil, err
	}
	if row.Kind != "dm" || row.ThreadID != messageThreadID(user, peer) || row.PeerID != user {
		return nil, repository.ErrForbidden
	}
	var stored models.StoredDirectMessage
	if err = json.Unmarshal(row.Payload, &stored); err != nil {
		return nil, err
	}
	if err = s.decorateDirect(ctx, user, []*models.DirectMessage{&stored.Message}); err != nil {
		return nil, err
	}
	return &stored.Message, nil
}
