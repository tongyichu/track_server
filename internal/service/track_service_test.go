package service

import (
	"context"
	"strings"
	"testing"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

// TestCreateTrackAssignsRecordFields verifies create uses the new track_records fields.
func TestCreateTrackAssignsRecordFields(t *testing.T) {
	trackRepo, _, collectRepo, _ := repository.NewInMemoryRepositories()
	svc := NewTrackService(trackRepo, collectRepo)

	track, err := svc.CreateTrack(context.Background(), 1001)
	if err != nil {
		t.Fatalf("CreateTrack returned error: %v", err)
	}
	if track.ID == "" {
		t.Fatalf("expected generated id, got empty string")
	}
	if !strings.HasPrefix(track.ID, "No.") {
		t.Fatalf("expected generated id prefix No., got %q", track.ID)
	}
	if track.Title != "新的轨迹" {
		t.Fatalf("expected default title, got %q", track.Title)
	}
	if track.Status != models.TrackStatusNormal {
		t.Fatalf("expected status normal, got %d", track.Status)
	}
	if !track.IsRunning {
		t.Fatalf("expected created track to be running")
	}
}

func TestUpdateTrackInfo_PartialUpdate(t *testing.T) {
	trackRepo, _, collectRepo, _ := repository.NewInMemoryRepositories()
	svc := NewTrackService(trackRepo, collectRepo)
	ctx := context.Background()

	_ = trackRepo.Create(ctx, &models.Track{ID: "trk1", UserID: 1001, Title: "t", Distance: 1, Duration: 2, ElevationGain: 3, IsRunning: true})

	distance := 123.4
	isRunning := false
	avg := 9.8
	patch := TrackInfoPatch{Distance: &distance, IsRunning: &isRunning, AvgSpeedKmh: &avg}

	track, err := svc.UpdateTrackInfo(ctx, 1001, "trk1", patch)
	if err != nil {
		t.Fatalf("UpdateTrackInfo returned error: %v", err)
	}
	if track.Distance != 123.4 {
		t.Fatalf("expected distance 123.4, got %v", track.Distance)
	}
	if track.IsRunning {
		t.Fatalf("expected is_running false")
	}
	if track.AvgSpeedKmh != 9.8 {
		t.Fatalf("expected avg_speed_kmh 9.8, got %v", track.AvgSpeedKmh)
	}
	if track.Duration != 2 {
		t.Fatalf("expected duration unchanged 2, got %v", track.Duration)
	}
}

func TestUpdateTrackInfo_EmptyPatch(t *testing.T) {
	trackRepo, _, collectRepo, _ := repository.NewInMemoryRepositories()
	svc := NewTrackService(trackRepo, collectRepo)
	ctx := context.Background()
	_ = trackRepo.Create(ctx, &models.Track{ID: "trk1", UserID: 1001, Title: "t"})
	_, err := svc.UpdateTrackInfo(ctx, 1001, "trk1", TrackInfoPatch{})
	if err == nil {
		t.Fatalf("expected error for empty patch")
	}
	if _, ok := err.(*InvalidArgumentError); !ok {
		t.Fatalf("expected InvalidArgumentError, got %T: %v", err, err)
	}
}
