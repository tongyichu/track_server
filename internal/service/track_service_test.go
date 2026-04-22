package service

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

// trackIDPattern 校验新的轨迹 ID 规则：固定前缀 `NO.` + 8 位大写 base36 编码。
var trackIDPattern = regexp.MustCompile(`^NO\.[0-9A-Z]{8}$`)

// TestCreateTrackAssignsRecordFields verifies create uses the new track_records fields.
func TestCreateTrackAssignsRecordFields(t *testing.T) {
	trackRepo, _, collectRepo, _ := repository.NewInMemoryRepositories()
	svc := NewTrackService(trackRepo, collectRepo)

	track, err := svc.CreateTrack(context.Background(), 1001, CreateTrackInput{})
	if err != nil {
		t.Fatalf("CreateTrack returned error: %v", err)
	}
	if track.ID == "" {
		t.Fatalf("expected generated id, got empty string")
	}
	if !trackIDPattern.MatchString(track.ID) {
		t.Fatalf("expected generated id to match %s, got %q", trackIDPattern.String(), track.ID)
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

func TestGenerateTrackID_FormatAndUniqueness(t *testing.T) {
	const total = 10
	// 使用内存仓储的本地序列模拟连续发号，验证编码格式和短序列范围内的不重复性。
	seen := make(map[string]struct{}, total)
	trackRepo, _, _, _ := repository.NewInMemoryRepositories()

	for i := 0; i < total; i++ {
		id, err := trackRepo.NextTrackID(context.Background())
		if err != nil {
			t.Fatalf("NextTrackID returned error: %v", err)
		}
		if !trackIDPattern.MatchString(id) {
			t.Fatalf("expected generated id to match %s, got %q", trackIDPattern.String(), id)
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("generated duplicate track id %q", id)
		}
		seen[id] = struct{}{}
	}
}

func TestCreateTrack_UsesProvidedFields(t *testing.T) {
	trackRepo, _, collectRepo, _ := repository.NewInMemoryRepositories()
	svc := NewTrackService(trackRepo, collectRepo)

	start := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	title := "傍晚夜跑"
	distance := 5200.5
	duration := uint32(1800)
	elevationGain := 120
	rawURL := "https://example.com/raw/track.json"
	screenshotURL := "https://example.com/track.png"
	isRunning := false
	avgSpeed := 10.4

	track, err := svc.CreateTrack(context.Background(), 1001, CreateTrackInput{
		Title:              &title,
		StartTime:          &start,
		EndTime:            &end,
		Distance:           &distance,
		Duration:           &duration,
		ElevationGain:      &elevationGain,
		RawTrackURL:        &rawURL,
		TrackScreenshotURL: &screenshotURL,
		IsRunning:          &isRunning,
		AvgSpeedKmh:        &avgSpeed,
	})
	if err != nil {
		t.Fatalf("CreateTrack returned error: %v", err)
	}
	if !track.StartTime.Equal(start) {
		t.Fatalf("expected start_time %v, got %v", start, track.StartTime)
	}
	if track.Title != title {
		t.Fatalf("expected title %q, got %q", title, track.Title)
	}
	if !track.EndTime.Equal(end) {
		t.Fatalf("expected end_time %v, got %v", end, track.EndTime)
	}
	if track.Distance != distance {
		t.Fatalf("expected distance %v, got %v", distance, track.Distance)
	}
	if track.Duration != duration {
		t.Fatalf("expected duration %v, got %v", duration, track.Duration)
	}
	if track.ElevationGain != elevationGain {
		t.Fatalf("expected elevation_gain %v, got %v", elevationGain, track.ElevationGain)
	}
	if track.RawTrackURL != rawURL {
		t.Fatalf("expected raw_track_url %q, got %q", rawURL, track.RawTrackURL)
	}
	if track.TrackScreenshotURL != screenshotURL {
		t.Fatalf("expected track_screenshot_url %q, got %q", screenshotURL, track.TrackScreenshotURL)
	}
	if track.IsRunning != isRunning {
		t.Fatalf("expected is_running %v, got %v", isRunning, track.IsRunning)
	}
	if track.AvgSpeedKmh != avgSpeed {
		t.Fatalf("expected avg_speed_kmh %v, got %v", avgSpeed, track.AvgSpeedKmh)
	}
}

func TestCreateTrack_InvalidTimeRange(t *testing.T) {
	trackRepo, _, collectRepo, _ := repository.NewInMemoryRepositories()
	svc := NewTrackService(trackRepo, collectRepo)

	start := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	end := start.Add(-time.Minute)

	_, err := svc.CreateTrack(context.Background(), 1001, CreateTrackInput{StartTime: &start, EndTime: &end})
	if err == nil {
		t.Fatalf("expected invalid time range error")
	}
	if _, ok := err.(*InvalidArgumentError); !ok {
		t.Fatalf("expected InvalidArgumentError, got %T: %v", err, err)
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
