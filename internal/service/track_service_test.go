package service

import (
	"context"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

// trackIDPattern 校验新的轨迹 ID 规则：固定前缀 `NO.` + 8 位大写 base36 编码。
var trackIDPattern = regexp.MustCompile(`^NO\.[0-9A-Z]{8}$`)

// TestCreateTrackAssignsRecordFields verifies create uses the new track_records fields.
func TestCreateTrackAssignsRecordFields(t *testing.T) {
	trackRepo, _, collectRepo, _, _ := repository.NewInMemoryRepositories()
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
	if track.TrackType != "" {
		t.Fatalf("expected default track_type empty, got %q", track.TrackType)
	}
}

func TestGenerateTrackID_FormatAndUniqueness(t *testing.T) {
	const total = 10
	// 使用内存仓储的本地序列模拟连续发号，验证编码格式和短序列范围内的不重复性。
	seen := make(map[string]struct{}, total)
	trackRepo, _, _, _, _ := repository.NewInMemoryRepositories()

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
	trackRepo, _, collectRepo, _, _ := repository.NewInMemoryRepositories()
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
	trackType := "跑步"

	track, err := svc.CreateTrack(context.Background(), 1001, CreateTrackInput{
		Title:              &title,
		TrackType:          &trackType,
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
	if track.TrackType != trackType {
		t.Fatalf("expected track_type %q, got %q", trackType, track.TrackType)
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

func TestCreateTrack_CleansTempAssetCacheOnly(t *testing.T) {
	trackRepo, _, collectRepo, _, _ := repository.NewInMemoryRepositories()
	svc := NewTrackService(trackRepo, collectRepo)

	staticRoot := t.TempDir()
	screenshotCache, err := NewAssetCacheService(
		filepath.Join(staticRoot, "screenshots"),
		"/api/v1/static/screenshots",
		[]string{".png", ".jpg", ".jpeg", ".webp"},
		".png",
	)
	if err != nil {
		t.Fatalf("create screenshot cache failed: %v", err)
	}
	rawTrackCache, err := NewAssetCacheService(
		filepath.Join(staticRoot, "raw_tracks"),
		"/api/v1/static/raw_tracks",
		[]string{".dat", ".json", ".gpx", ".bin"},
		".dat",
	)
	if err != nil {
		t.Fatalf("create raw track cache failed: %v", err)
	}
	svc.SetScreenshotCache(screenshotCache)
	svc.SetRawTrackCache(rawTrackCache)

	staleTrackID := "NO.00000001"
	staleScreenshotPath := filepath.Join(staticRoot, "screenshots", staleTrackID+".png")
	staleScreenshotTmpPath := staleScreenshotPath + ".tmp"
	staleRawTrackPath := filepath.Join(staticRoot, "raw_tracks", staleTrackID+".json")
	staleRawTrackTmpPath := staleRawTrackPath + ".tmp"
	if err := os.WriteFile(staleScreenshotPath, []byte("old-screenshot"), 0o644); err != nil {
		t.Fatalf("seed stale screenshot cache failed: %v", err)
	}
	if err := os.WriteFile(staleScreenshotTmpPath, []byte("stale-screenshot-tmp"), 0o644); err != nil {
		t.Fatalf("seed stale screenshot tmp failed: %v", err)
	}
	if err := os.WriteFile(staleRawTrackPath, []byte("old-raw-track"), 0o644); err != nil {
		t.Fatalf("seed stale raw track cache failed: %v", err)
	}
	if err := os.WriteFile(staleRawTrackTmpPath, []byte("stale-raw-track-tmp"), 0o644); err != nil {
		t.Fatalf("seed stale raw track tmp failed: %v", err)
	}

	rawURL := "https://example.com/raw/track.json"
	screenshotURL := "https://example.com/track.png"
	track, err := svc.CreateTrack(context.Background(), 1001, CreateTrackInput{
		RawTrackURL:        &rawURL,
		TrackScreenshotURL: &screenshotURL,
	})
	if err != nil {
		t.Fatalf("CreateTrack returned error: %v", err)
	}
	if track.ID != staleTrackID {
		t.Fatalf("expected first track id %q, got %q", staleTrackID, track.ID)
	}
	if track.TrackScreenshotURL != "/api/v1/static/screenshots/NO.00000001.png" {
		t.Fatalf("expected rewritten screenshot url, got %q", track.TrackScreenshotURL)
	}
	if track.RawTrackURL != "/api/v1/static/raw_tracks/NO.00000001.json" {
		t.Fatalf("expected rewritten raw track url, got %q", track.RawTrackURL)
	}
	if _, err := os.Stat(staleScreenshotPath); err != nil {
		t.Fatalf("expected stale screenshot cache kept, stat err=%v", err)
	}
	if _, err := os.Stat(staleRawTrackPath); err != nil {
		t.Fatalf("expected stale raw track cache kept, stat err=%v", err)
	}
	if _, err := os.Stat(staleScreenshotTmpPath); !os.IsNotExist(err) {
		t.Fatalf("expected stale screenshot tmp removed, stat err=%v", err)
	}
	if _, err := os.Stat(staleRawTrackTmpPath); !os.IsNotExist(err) {
		t.Fatalf("expected stale raw track tmp removed, stat err=%v", err)
	}
}

func TestCreateTrack_InvalidTimeRange(t *testing.T) {
	trackRepo, _, collectRepo, _, _ := repository.NewInMemoryRepositories()
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
	trackRepo, _, collectRepo, _, _ := repository.NewInMemoryRepositories()
	svc := NewTrackService(trackRepo, collectRepo)
	ctx := context.Background()

	_ = trackRepo.Create(ctx, &models.Track{ID: "trk1", UserID: 1001, Title: "t", Distance: 0, Duration: 2, ElevationGain: 0, TrackScreenshotURL: "already.jpg", IsRunning: true})

	cityCode := "330100"
	locateAddr := "杭州市西湖区"
	distance := 123.4
	avg := 9.8
	noMapBg := "https://<bucket>.oss-<region>.aliyuncs.com/prod/track/.../xxx_no_map_bg.jpg"
	screenshot := "https://<bucket>.oss-<region>.aliyuncs.com/prod/track/.../xxx.jpg"
	duration := uint32(99)
	patch := TrackInfoPatch{CityCode: &cityCode, LocateAddr: &locateAddr, Distance: &distance, Duration: &duration, TrackScreenshotURL: &screenshot, TrackNoMapBgScreenshotURL: &noMapBg, AvgSpeedKmh: &avg}

	track, err := svc.UpdateTrackInfo(ctx, 1001, "trk1", patch)
	if err != nil {
		t.Fatalf("UpdateTrackInfo returned error: %v", err)
	}
	// 只有当字段为空时才允许补全。
	if track.Distance != 123.4 {
		t.Fatalf("expected distance 123.4, got %v", track.Distance)
	}
	if track.Duration != 2 {
		t.Fatalf("expected duration unchanged 2, got %v", track.Duration)
	}
	if track.TrackScreenshotURL != "already.jpg" {
		t.Fatalf("expected track_screenshot_url unchanged, got %q", track.TrackScreenshotURL)
	}
	if track.TrackNoMapBgScreenshotURL != noMapBg {
		t.Fatalf("expected track_no_map_bg_screenshot_url %q, got %q", noMapBg, track.TrackNoMapBgScreenshotURL)
	}
	if track.CityCode != cityCode {
		t.Fatalf("expected city_code %q, got %q", cityCode, track.CityCode)
	}
	if track.LocateAddr != locateAddr {
		t.Fatalf("expected locate_addr %q, got %q", locateAddr, track.LocateAddr)
	}
	if track.AvgSpeedKmh != 9.8 {
		t.Fatalf("expected avg_speed_kmh 9.8, got %v", track.AvgSpeedKmh)
	}
}

func TestUpdateTrackInfo_EmptyPatch(t *testing.T) {
	trackRepo, _, collectRepo, _, _ := repository.NewInMemoryRepositories()
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

func TestDeleteTrack_SoftDelete(t *testing.T) {
	trackRepo, _, collectRepo, _, _ := repository.NewInMemoryRepositories()
	svc := NewTrackService(trackRepo, collectRepo)
	ctx := context.Background()

	_ = trackRepo.Create(ctx, &models.Track{ID: "trk-del", UserID: 1001, Title: "t", IsRunning: true, Status: models.TrackStatusNormal})

	if err := svc.DeleteTrack(ctx, 1001, "trk-del"); err != nil {
		t.Fatalf("DeleteTrack returned error: %v", err)
	}
	track, err := trackRepo.FindByID(ctx, "trk-del")
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}
	if track.Status != models.TrackStatusDeleted {
		t.Fatalf("expected status deleted, got %d", track.Status)
	}
	if track.DeletedAt.IsZero() {
		t.Fatalf("expected deleted_at set")
	}
	if track.IsRunning {
		t.Fatalf("expected is_running false after delete")
	}
}

func TestDeleteTrack_Forbidden(t *testing.T) {
	trackRepo, _, collectRepo, _, _ := repository.NewInMemoryRepositories()
	svc := NewTrackService(trackRepo, collectRepo)
	ctx := context.Background()

	_ = trackRepo.Create(ctx, &models.Track{ID: "trk-del-other", UserID: 1002, Title: "t"})
	if err := svc.DeleteTrack(ctx, 1001, "trk-del-other"); err == nil {
		t.Fatalf("expected forbidden error")
	}
}

func TestGenerateUniqueDefaultNickname_UsesCuratedBase(t *testing.T) {
	_, userRepo, _, loginLogRepo, _ := repository.NewInMemoryRepositories()
	svc := NewLoginService(userRepo, loginLogRepo, "", "", "test-secret")
	bases := config.DefaultNicknameBases()
	if len(bases) != 180 {
		t.Fatalf("expected 180 default nickname bases after expansion, got %d", len(bases))
	}

	rand.Seed(7)
	nickname, err := svc.generateUniqueDefaultNickname(context.Background())
	if err != nil {
		t.Fatalf("generateUniqueDefaultNickname returned error: %v", err)
	}

	for _, base := range bases {
		if nickname == base {
			return
		}
	}
	t.Fatalf("expected nickname %q to come from curated base list", nickname)
}

func TestGenerateUniqueDefaultNickname_DuplicateBaseAppendsRandomNumber(t *testing.T) {
	_, userRepo, _, loginLogRepo, _ := repository.NewInMemoryRepositories()
	svc := NewLoginService(userRepo, loginLogRepo, "", "", "test-secret")
	bases := config.DefaultNicknameBases()

	rand.Seed(1)
	base := bases[rand.Intn(len(bases))]
	_, err := userRepo.CreateIfNotExists(context.Background(), &models.User{ID: 1001, Nickname: base})
	if err != nil {
		t.Fatalf("seed duplicate nickname failed: %v", err)
	}

	rand.Seed(1)
	nickname, err := svc.generateUniqueDefaultNickname(context.Background())
	if err != nil {
		t.Fatalf("generateUniqueDefaultNickname returned error: %v", err)
	}
	if nickname == base {
		t.Fatalf("expected duplicate base nickname to append random digits")
	}
	if !strings.HasPrefix(nickname, base) {
		t.Fatalf("expected nickname %q to keep base %q as prefix", nickname, base)
	}
}
