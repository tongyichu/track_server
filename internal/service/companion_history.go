package service

import (
	"context"
	"errors"
	"strings"

	"github.com/tongyichu/track_server/internal/repository"
)

type CompanionHistoryRemoval struct {
	SessionID string `json:"session_id"`
	Scope     string `json:"scope"`
}

func (s *CompanionService) RemoveHistory(ctx context.Context, user int64, id, scope string) (*CompanionHistoryRemoval, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("companion service not configured")
	}
	if user <= 0 || strings.TrimSpace(id) == "" || (scope != "all" && scope != "self") {
		return nil, invalidArg("session_id and confirmed scope (all/self) are required")
	}
	store, ok := s.repo.(repository.CompanionHistoryRepository)
	if !ok {
		return nil, errors.New("companion history storage unavailable")
	}
	if err := store.RemoveHistory(ctx, user, id, scope); err != nil {
		return nil, err
	}
	return &CompanionHistoryRemoval{SessionID: id, Scope: scope}, nil
}
