package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

func (s *CompanionService) recordAutomaticEvent(ctx context.Context, session *models.CompanionSession, eventType string, targetUserID int64, eventTime time.Time) {
	if s == nil || s.repo == nil || session == nil || eventTime.IsZero() {
		return
	}
	if _, ok := companionEventTypes[eventType]; !ok {
		return
	}
	count, err := s.repo.CountEventsBySessionID(ctx, session.SessionID)
	if err != nil {
		log.Printf("count companion events failed: session=%s err=%v", session.SessionID, err)
		return
	}
	if count >= companionEventMaxPerSession {
		log.Printf("companion automatic event skipped at limit: session=%s type=%s", session.SessionID, eventType)
		return
	}
	event := &models.CompanionEvent{
		SessionID:     session.SessionID,
		OwnerUserID:   session.OwnerUserID,
		EventType:     eventType,
		TargetUserID:  targetUserID,
		EventTime:     eventTime,
		ClientEventID: fmt.Sprintf("server:%s:%d:%d", eventType, targetUserID, eventTime.UnixNano()),
		CreatedAt:     time.Now(),
	}
	if err := s.repo.InsertEvent(ctx, event); err != nil && !errors.Is(err, repository.ErrAlreadyExists) {
		log.Printf("insert companion automatic event failed: session=%s type=%s err=%v", session.SessionID, eventType, err)
	}
}
