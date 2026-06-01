package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/tongyichu/track_server/internal/service"
)

func TestCompanionAutoClose_DefaultsApplied(t *testing.T) {
	job := NewCompanionAutoClose(nil)
	if got := job.Name(); got != "companion_session_autoclose" {
		t.Fatalf("unexpected job name: %s", got)
	}
	if got := job.Spec(); got != defaultCompanionAutoCloseSpec {
		t.Fatalf("expected default spec %q, got %q", defaultCompanionAutoCloseSpec, got)
	}
}

func TestCompanionAutoClose_RunRequiresService(t *testing.T) {
	job := NewCompanionAutoClose(nil)
	if err := job.Run(context.Background()); err == nil {
		t.Fatalf("expected nil service error")
	}
}

func TestCompanionAutoClose_Run(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewCompanionService(repo, nil)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	job := NewCompanionAutoClose(svc, WithCompanionAutoCloseNowFunc(func() time.Time { return now }))
	if err := job.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}
