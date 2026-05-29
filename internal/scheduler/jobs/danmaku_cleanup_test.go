package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

// fixedNow 返回一个固定的"现在"时间，便于配合 retention 反推 deadline。
var fixedNow = time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)

func newRepo(t *testing.T) repository.CompanionRepository {
	t.Helper()
	_, _, _, _, _, _, repo := repository.NewInMemoryRepositories()
	return repo
}

func mustCreateSession(t *testing.T, repo repository.CompanionRepository, sessionID string, status models.CompanionSessionStatus, endedAt time.Time) {
	t.Helper()
	now := fixedNow.Add(-30 * 24 * time.Hour)
	s := &models.CompanionSession{
		SessionID:   sessionID,
		OwnerUserID: 1,
		Status:      models.CompanionSessionStatusActive,
		Visibility:  models.CompanionSessionVisibilityPrivate,
		JoinToken:   "tk_" + sessionID,
		MaxMembers:  8,
		StartedAt:   now,
		CreatedAt:   now,
	}
	if err := repo.CreateSession(context.Background(), s); err != nil {
		t.Fatalf("CreateSession(%s): %v", sessionID, err)
	}
	if status != models.CompanionSessionStatusActive {
		s.Status = status
		s.EndedAt = endedAt
		if err := repo.UpdateSession(context.Background(), s); err != nil {
			t.Fatalf("UpdateSession(%s): %v", sessionID, err)
		}
	}
}

func mustInsertDanmaku(t *testing.T, repo repository.CompanionRepository, sessionID string, userID int64, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		d := &models.CompanionDanmaku{
			SessionID: sessionID,
			UserID:    userID,
			Content:   "hello",
			CreatedAt: fixedNow.Add(-time.Hour),
		}
		if err := repo.InsertDanmaku(context.Background(), d); err != nil {
			t.Fatalf("InsertDanmaku(%s): %v", sessionID, err)
		}
	}
}

func countDanmaku(t *testing.T, repo repository.CompanionRepository, sessionID string) int64 {
	t.Helper()
	got, err := repo.CountDanmakuBySessionSince(context.Background(), sessionID, time.Time{})
	if err != nil {
		t.Fatalf("CountDanmakuBySessionSince(%s): %v", sessionID, err)
	}
	return got
}

func TestDanmakuCleanup_Run_DeletesEndedSessionsOlderThanRetention(t *testing.T) {
	repo := newRepo(t)

	// retention=7d: deadline = fixedNow - 7d
	// 已结束 10 天 → 应被清理
	mustCreateSession(t, repo, "s_old_ended", models.CompanionSessionStatusEnded, fixedNow.Add(-10*24*time.Hour))
	mustInsertDanmaku(t, repo, "s_old_ended", 100, 3)
	// 已结束 1 天 → 不应被清理（在保留窗口内）
	mustCreateSession(t, repo, "s_recent_ended", models.CompanionSessionStatusEnded, fixedNow.Add(-1*24*time.Hour))
	mustInsertDanmaku(t, repo, "s_recent_ended", 100, 2)
	// active session → 不受影响
	mustCreateSession(t, repo, "s_active", models.CompanionSessionStatusActive, time.Time{})
	mustInsertDanmaku(t, repo, "s_active", 100, 4)

	job := NewDanmakuCleanup(repo, 7, "@every 1h",
		WithDanmakuCleanupNowFunc(func() time.Time { return fixedNow }),
	)
	if err := job.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := countDanmaku(t, repo, "s_old_ended"); got != 0 {
		t.Fatalf("expected old ended session danmaku to be cleared, got %d", got)
	}
	if got := countDanmaku(t, repo, "s_recent_ended"); got != 2 {
		t.Fatalf("expected recent ended session danmaku to remain (2), got %d", got)
	}
	if got := countDanmaku(t, repo, "s_active"); got != 4 {
		t.Fatalf("expected active session danmaku to remain (4), got %d", got)
	}
}

func TestDanmakuCleanup_Run_NoMatchReturnsNil(t *testing.T) {
	repo := newRepo(t)
	mustCreateSession(t, repo, "s_active", models.CompanionSessionStatusActive, time.Time{})
	mustInsertDanmaku(t, repo, "s_active", 100, 2)

	job := NewDanmakuCleanup(repo, 7, "",
		WithDanmakuCleanupNowFunc(func() time.Time { return fixedNow }),
	)
	if err := job.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := countDanmaku(t, repo, "s_active"); got != 2 {
		t.Fatalf("expected no danmaku to be deleted, got %d", got)
	}
}

func TestDanmakuCleanup_DefaultsApplied(t *testing.T) {
	repo := newRepo(t)
	job := NewDanmakuCleanup(repo, 0, "")
	if got := job.Spec(); got != defaultDanmakuCleanupSpec {
		t.Fatalf("expected default spec %q, got %q", defaultDanmakuCleanupSpec, got)
	}
	if want := 7 * 24 * time.Hour; job.retention != want {
		t.Fatalf("expected default retention %s, got %s", want, job.retention)
	}
	if job.Name() != "danmaku_cleanup" {
		t.Fatalf("unexpected job name: %s", job.Name())
	}
}

func TestDanmakuCleanup_RetentionZeroFallback(t *testing.T) {
	repo := newRepo(t)
	// retentionDays=-1 应走默认 7 天
	job := NewDanmakuCleanup(repo, -1, "@every 1h",
		WithDanmakuCleanupNowFunc(func() time.Time { return fixedNow }),
	)
	if want := 7 * 24 * time.Hour; job.retention != want {
		t.Fatalf("expected fallback retention %s, got %s", want, job.retention)
	}
}
