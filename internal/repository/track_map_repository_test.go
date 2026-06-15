package repository

import (
	"context"
	"testing"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

func TestInMemoryTrackMapRepository_EnqueueDoesNotDelayPendingJob(t *testing.T) {
	repo := NewInMemoryTrackMapRepository(nil)
	ctx := context.Background()
	trackID := "NO.0000000C"
	readyAt := time.Now().Add(-time.Minute)
	later := time.Now().Add(time.Minute)

	if err := repo.EnqueueIndexJob(ctx, trackID, readyAt); err != nil {
		t.Fatalf("enqueue ready job: %v", err)
	}
	if err := repo.EnqueueIndexJob(ctx, trackID, later); err != nil {
		t.Fatalf("enqueue duplicate job: %v", err)
	}

	jobs, err := repo.ClaimPendingIndexJobs(ctx, "worker", time.Now(), 10)
	if err != nil {
		t.Fatalf("claim jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("claimed %d jobs, want 1", len(jobs))
	}
	if jobs[0].TrackID != trackID {
		t.Fatalf("claimed track_id=%s, want %s", jobs[0].TrackID, trackID)
	}
	if jobs[0].Status != models.TrackMapIndexJobProcessing {
		t.Fatalf("claimed status=%s, want %s", jobs[0].Status, models.TrackMapIndexJobProcessing)
	}
}
