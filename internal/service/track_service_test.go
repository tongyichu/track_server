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
}
