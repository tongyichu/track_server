package service

import (
	"context"
	"errors"
	"strings"
)

func (s *CompanionService) TransferSessionOwner(ctx context.Context, owner int64, id string, target int64) (*CompanionSessionState, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("companion service not configured")
	}
	id = strings.TrimSpace(id)
	if owner <= 0 || target <= 0 || target == owner || id == "" {
		return nil, invalidArg("invalid ownership transfer")
	}
	session, err := s.repo.TransferOwner(ctx, id, owner, target)
	if err != nil {
		return nil, err
	}
	s.publishControlEvent(ctx, id, CompanionControlEvent{
		Event: CompanionControlEventOwnerTransferred, SessionID: id, MemberUserID: target, OperatorUserID: owner, At: session.UpdatedAt,
	})
	return s.buildSessionState(ctx, session, owner, false)
}
