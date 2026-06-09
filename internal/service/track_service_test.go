package service

import (
	"context"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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
	trackRepo, _, collectRepo, _, _, _, _ := repository.NewInMemoryRepositories()
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

func TestCreateRunningTrackRejectsActiveCompanionSession(t *testing.T) {
	trackRepo, userRepo, collectRepo, _, _, _, companionRepo := repository.NewInMemoryRepositories()
	svc := NewTrackService(trackRepo, collectRepo)
	svc.SetCompanionRepository(companionRepo)
	ctx := context.Background()
	if _, err := userRepo.CreateIfNotExists(ctx, &models.User{ID: 1001, Nickname: "owner"}); err != nil {
		t.Fatalf("CreateIfNotExists returned error: %v", err)
	}
	companionSvc := NewCompanionService(companionRepo, userRepo)
	created, err := companionSvc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "同行中"})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	if _, err := svc.CreateTrack(ctx, 1001, CreateTrackInput{}); err == nil || err.Error() != "you already joined an active companion session: 同行中" {
		t.Fatalf("expected active companion rejection, got %v", err)
	}
	running := true
	if _, err := svc.CreateTrack(ctx, 1001, CreateTrackInput{IsRunning: &running}); err == nil || err.Error() != "you already joined an active companion session: 同行中" {
		t.Fatalf("expected explicit running rejection, got %v", err)
	}
	notRunning := false
	track, err := svc.CreateTrack(ctx, 1001, CreateTrackInput{IsRunning: &notRunning, SessionID: &created.Session.SessionID})
	if err != nil {
		t.Fatalf("expected completed track to be allowed, got %v", err)
	}
	if track.IsRunning || track.SessionID != created.Session.SessionID {
		t.Fatalf("unexpected completed track: %+v", track)
	}
}

func TestGenerateTrackID_FormatAndUniqueness(t *testing.T) {
	const total = 10
	// 使用内存仓储的本地序列模拟连续发号，验证编码格式和短序列范围内的不重复性。
	seen := make(map[string]struct{}, total)
	trackRepo, _, _, _, _, _, _ := repository.NewInMemoryRepositories()

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

func TestTrackServiceListTrackTypes(t *testing.T) {
	trackRepo, _, collectRepo, _, _, _, _ := repository.NewInMemoryRepositories()
	svc := NewTrackService(trackRepo, collectRepo)

	if got := svc.ListTrackTypes(); !slices.Equal(got, []string{"徒步", "跑步", "爬山", "骑行", "自驾"}) {
		t.Fatalf("expected default track types, got %#v", got)
	}

	svc.SetTrackTypes(config.ParseTrackTypes("徒步,跑步,滑雪,徒步"))
	if got := svc.ListTrackTypes(); !slices.Equal(got, []string{"徒步", "跑步", "滑雪"}) {
		t.Fatalf("expected configured track types, got %#v", got)
	}
	options := svc.ListTrackTypeOptions()
	if len(options) != 3 {
		t.Fatalf("expected 3 track type options, got %#v", options)
	}
	if options[0].Type != "hiking" || options[0].Name != "徒步" || options[0].ThemeColor != "#345631" || options[0].IconURL != "/api/v1/static/track_type_icon/hiking.svg" || options[0].IconAnimURL != "" {
		t.Fatalf("unexpected first track type option: %#v", options[0])
	}
	if options[2].Type != "滑雪" || options[2].Name != "滑雪" || options[2].ThemeColor != "" || options[2].IconURL != "/api/v1/static/track_type_icon/%E6%BB%91%E9%9B%AA.svg" || options[2].IconAnimURL != "" {
		t.Fatalf("unexpected third track type option: %#v", options[2])
	}
}

func TestListTrackTypeOptionsWithStats(t *testing.T) {
	trackRepo, _, collectRepo, _, _, _, _ := repository.NewInMemoryRepositories()
	svc := NewTrackService(trackRepo, collectRepo)
	ctx := context.Background()
	now := time.Now()

	_ = trackRepo.Create(ctx, &models.Track{ID: "hike-month", UserID: 1001, TrackType: "徒步", StartTime: now.AddDate(0, 0, -10), Distance: 100, Duration: 50, CaloriesBurned: 10, IsRunning: false, Status: models.TrackStatusNormal})
	_ = trackRepo.Create(ctx, &models.Track{ID: "hike-year", UserID: 1001, TrackType: "徒步", StartTime: now.AddDate(0, -2, 0), Distance: 200, Duration: 80, CaloriesBurned: 20, IsRunning: false, Status: models.TrackStatusPrivate})
	_ = trackRepo.Create(ctx, &models.Track{ID: "run-month", UserID: 1001, TrackType: "跑步", StartTime: now.AddDate(0, 0, -5), Distance: 300, Duration: 120, CaloriesBurned: 30, IsRunning: false, Status: models.TrackStatusNormal})
	_ = trackRepo.Create(ctx, &models.Track{ID: "run-year", UserID: 1001, TrackType: "跑步", StartTime: now.AddDate(0, -6, 0), Distance: 400, Duration: 180, CaloriesBurned: 40, IsRunning: false, Status: models.TrackStatusNormal})
	_ = trackRepo.Create(ctx, &models.Track{ID: "old", UserID: 1001, TrackType: "徒步", StartTime: now.AddDate(-2, 0, 0), Distance: 999, Duration: 999, CaloriesBurned: 999, IsRunning: false, Status: models.TrackStatusNormal})
	_ = trackRepo.Create(ctx, &models.Track{ID: "running", UserID: 1001, TrackType: "徒步", StartTime: now.AddDate(0, 0, -1), Distance: 888, Duration: 888, CaloriesBurned: 888, IsRunning: true, Status: models.TrackStatusNormal})
	_ = trackRepo.Create(ctx, &models.Track{ID: "other-user", UserID: 1002, TrackType: "徒步", StartTime: now.AddDate(0, 0, -1), Distance: 777, Duration: 777, CaloriesBurned: 777, IsRunning: false, Status: models.TrackStatusNormal})

	items, err := svc.ListTrackTypeOptionsWithStats(ctx, 1001)
	if err != nil {
		t.Fatalf("ListTrackTypeOptionsWithStats returned error: %v", err)
	}
	if len(items) != 5 {
		t.Fatalf("expected 5 track types, got %#v", items)
	}
	if items[0].Type != "hiking" || items[0].Name != "徒步" {
		t.Fatalf("unexpected first track type: %#v", items[0])
	}
	if items[0].Milestone.Month.Distance != 100 || items[0].Milestone.Month.TrackCount != 1 || items[0].Milestone.Month.Duration != 50 || items[0].Milestone.Month.Calories != 10 {
		t.Fatalf("unexpected hiking month stats: %#v", items[0].Milestone.Month)
	}
	if items[0].Milestone.Year.Distance != 300 || items[0].Milestone.Year.TrackCount != 2 || items[0].Milestone.Year.Duration != 130 || items[0].Milestone.Year.Calories != 30 {
		t.Fatalf("unexpected hiking year stats: %#v", items[0].Milestone.Year)
	}
	if items[1].Type != "running" || items[1].Name != "跑步" {
		t.Fatalf("unexpected second track type: %#v", items[1])
	}
	if items[1].Milestone.Month.Distance != 300 || items[1].Milestone.Month.TrackCount != 1 || items[1].Milestone.Month.Duration != 120 || items[1].Milestone.Month.Calories != 30 {
		t.Fatalf("unexpected running month stats: %#v", items[1].Milestone.Month)
	}
	if items[1].Milestone.Year.Distance != 700 || items[1].Milestone.Year.TrackCount != 2 || items[1].Milestone.Year.Duration != 300 || items[1].Milestone.Year.Calories != 70 {
		t.Fatalf("unexpected running year stats: %#v", items[1].Milestone.Year)
	}
	if items[2].Milestone.Month.TrackCount != 0 || items[2].Milestone.Year.TrackCount != 0 {
		t.Fatalf("expected climbing stats empty, got %#v", items[2].Milestone)
	}
}

func TestCreateTrack_UsesProvidedFields(t *testing.T) {
	trackRepo, _, collectRepo, _, _, _, _ := repository.NewInMemoryRepositories()
	svc := NewTrackService(trackRepo, collectRepo)

	start := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	title := "傍晚夜跑"
	distance := 5200.5
	duration := uint32(1800)
	caloriesBurned := 96.5
	elevationGain := 120
	rawURL := "https://example.com/raw/track.json"
	screenshotURL := "https://example.com/track.png"
	isRunning := false
	avgSpeed := 10.4
	trackType := "running"
	coordinateSystem := "GCJ02"
	sessionID := "sess_companion_001"

	track, err := svc.CreateTrack(context.Background(), 1001, CreateTrackInput{
		Title:              &title,
		SessionID:          &sessionID,
		TrackType:          &trackType,
		CoordinateSystem:   &coordinateSystem,
		StartTime:          &start,
		EndTime:            &end,
		Distance:           &distance,
		Duration:           &duration,
		CaloriesBurned:     &caloriesBurned,
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
	if track.SessionID != sessionID {
		t.Fatalf("expected session_id %q, got %q", sessionID, track.SessionID)
	}
	if track.TrackType != trackType {
		t.Fatalf("expected track_type %q, got %q", trackType, track.TrackType)
	}
	if track.CoordinateSystem != coordinateSystem {
		t.Fatalf("expected coordinate_system %q, got %q", coordinateSystem, track.CoordinateSystem)
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
	if track.CaloriesBurned != caloriesBurned {
		t.Fatalf("expected calories_burned %v, got %v", caloriesBurned, track.CaloriesBurned)
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
	trackRepo, _, collectRepo, _, _, _, _ := repository.NewInMemoryRepositories()
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
	trackRepo, _, collectRepo, _, _, _, _ := repository.NewInMemoryRepositories()
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

func TestCreateTrack_LocateAddrAllows255Runes(t *testing.T) {
	trackRepo, _, collectRepo, _, _, _, _ := repository.NewInMemoryRepositories()
	svc := NewTrackService(trackRepo, collectRepo)
	ctx := context.Background()

	locateAddr := strings.Repeat("京", 255)
	track, err := svc.CreateTrack(ctx, 1001, CreateTrackInput{LocateAddr: &locateAddr})
	if err != nil {
		t.Fatalf("CreateTrack returned error: %v", err)
	}
	if track.LocateAddr != locateAddr {
		t.Fatalf("expected locate_addr length %d, got %d", len([]rune(locateAddr)), len([]rune(track.LocateAddr)))
	}

	tooLong := locateAddr + "京"
	_, err = svc.CreateTrack(ctx, 1001, CreateTrackInput{LocateAddr: &tooLong})
	if err == nil {
		t.Fatalf("expected locate_addr too long error")
	}
	if err.Error() != "locate_addr is too long" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateTrackInfo_PartialUpdate(t *testing.T) {
	trackRepo, _, collectRepo, _, _, _, _ := repository.NewInMemoryRepositories()
	svc := NewTrackService(trackRepo, collectRepo)
	ctx := context.Background()

	_ = trackRepo.Create(ctx, &models.Track{ID: "trk1", UserID: 1001, Title: "t", Distance: 0, Duration: 2, ElevationGain: 0, TrackScreenshotURL: "already.jpg", IsRunning: true})

	sessionID := "sess_update_001"
	cityCode := "330100"
	locateAddr := "杭州市西湖区"
	coordinateSystem := "WGS84"
	distance := 123.4
	avg := 9.8
	noMapBg := "https://<bucket>.oss-<region>.aliyuncs.com/prod/track/.../xxx_no_map_bg.jpg"
	screenshot := "https://<bucket>.oss-<region>.aliyuncs.com/prod/track/.../xxx.jpg"
	duration := uint32(99)
	patch := TrackInfoPatch{SessionID: &sessionID, CityCode: &cityCode, LocateAddr: &locateAddr, CoordinateSystem: &coordinateSystem, Distance: &distance, Duration: &duration, TrackScreenshotURL: &screenshot, TrackNoMapBgScreenshotURL: &noMapBg, AvgSpeedKmh: &avg}

	track, err := svc.UpdateTrackInfo(ctx, 1001, "trk1", patch)
	if err != nil {
		t.Fatalf("UpdateTrackInfo returned error: %v", err)
	}
	// 只有当字段为空时才允许补全。
	if track.Distance != 123.4 {
		t.Fatalf("expected distance 123.4, got %v", track.Distance)
	}
	if track.SessionID != sessionID {
		t.Fatalf("expected session_id %q, got %q", sessionID, track.SessionID)
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
	if track.CoordinateSystem != coordinateSystem {
		t.Fatalf("expected coordinate_system %q, got %q", coordinateSystem, track.CoordinateSystem)
	}
	if track.AvgSpeedKmh != 9.8 {
		t.Fatalf("expected avg_speed_kmh 9.8, got %v", track.AvgSpeedKmh)
	}
}

func TestUpdateTrackInfo_EmptyPatch(t *testing.T) {
	trackRepo, _, collectRepo, _, _, _, _ := repository.NewInMemoryRepositories()
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
	trackRepo, _, collectRepo, _, _, _, _ := repository.NewInMemoryRepositories()
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
	trackRepo, _, collectRepo, _, _, _, _ := repository.NewInMemoryRepositories()
	svc := NewTrackService(trackRepo, collectRepo)
	ctx := context.Background()

	_ = trackRepo.Create(ctx, &models.Track{ID: "trk-del-other", UserID: 1002, Title: "t"})
	if err := svc.DeleteTrack(ctx, 1001, "trk-del-other"); err == nil {
		t.Fatalf("expected forbidden error")
	}
}

func TestGenerateUniqueDefaultNickname_UsesCuratedBase(t *testing.T) {
	_, userRepo, _, loginLogRepo, _, _, _ := repository.NewInMemoryRepositories()
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
	_, userRepo, _, loginLogRepo, _, _, _ := repository.NewInMemoryRepositories()
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
