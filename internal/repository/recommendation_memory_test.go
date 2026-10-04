package repository

import (
	"context"
	"testing"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

func TestInMemoryRecommendationFeedSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRecommendationRepository()
	now := time.Now()
	session := &models.RecommendationFeedSession{RequestID: "rec_test", UserID: 1, Strategy: models.RecommendationStrategyHybrid, Items: []models.RecommendationFeedItem{{TrackID: "NO.00000001", CandidateSource: models.RecommendationSourceCityHot, RecommendReason: "近期热门"}}, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := repo.SaveFeedSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetFeedSession(ctx, session.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	got.Items[0].TrackID = "changed"
	again, err := repo.GetFeedSession(ctx, session.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Items[0].TrackID != "NO.00000001" {
		t.Fatal("stored session was mutated")
	}
	again.CreatedAt = now.Add(-2 * time.Hour)
	again.ExpiresAt = now.Add(-time.Second)
	if err := repo.SaveFeedSession(ctx, again); err != nil {
		t.Fatal(err)
	}
	deleted, err := repo.DeleteExpiredFeedSessions(ctx, now, 10)
	if err != nil || deleted != 1 {
		t.Fatalf("deleted=%d err=%v", deleted, err)
	}
	if _, err := repo.GetFeedSession(ctx, session.RequestID); err != ErrNotFound {
		t.Fatalf("got %v", err)
	}
}

func TestRecommendationFeedSessionRejectsMoreThanTwoHundredItems(t *testing.T) {
	now := time.Now()
	session := &models.RecommendationFeedSession{RequestID: "rec_large", UserID: 1, Strategy: models.RecommendationStrategyLegacy, Items: make([]models.RecommendationFeedItem, 201), CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := NewInMemoryRecommendationRepository().SaveFeedSession(context.Background(), session); err == nil {
		t.Fatal("expected feed size validation error")
	}
}
