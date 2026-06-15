package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	jwtlib "github.com/golang-jwt/jwt/v5"

	"github.com/tongyichu/track_server/internal/handler"
	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

const testJWTSecret = "test_jwt_secret"
const testInternalToken = "test_internal_token"

// testEnv bundles server and in-memory dependencies for HTTP tests.
type testEnv struct {
	h                  *server.Hertz
	trackRepo          repository.TrackRepository
	userRepo           repository.UserRepository
	collectRepo        repository.CollectRepository
	followRepo         repository.FollowRepository
	loginLogRepo       repository.LoginLogRepository
	navigationRepo     repository.NavigationRepository
	companionRepo      repository.CompanionRepository
	feedbackRepo       repository.FeedbackRepository
	achievementRepo    repository.AchievementRepository
	loginSvc           *service.LoginService
	tokenBlacklist     *middleware.TokenBlacklist
	internalToken      string
	staticRoot         string
	avatarCacheDir     string
	screenshotCacheDir string
}

// newTestEnv creates a fresh Hertz server wired with in-memory repositories.
func newTestEnv() *testEnv {
	trackRepo, userRepo, collectRepo, loginLogRepo, navigationRepo, _, companionRepo := repository.NewInMemoryRepositories()
	followRepo := repository.NewInMemoryFollowRepository()
	achievementRepo := repository.NewInMemoryAchievementRepository()
	feedbackRepo := repository.NewInMemoryFeedbackRepository()
	trackSvc := service.NewTrackService(trackRepo, collectRepo)
	trackSvc.SetUserRepository(userRepo)
	trackSvc.SetNavigationRepository(navigationRepo)
	trackSvc.SetCompanionRepository(companionRepo)
	achievementSvc := service.NewAchievementService(achievementRepo, trackRepo)
	trackSvc.SetAchievementService(achievementSvc)
	userSvc := service.NewUserService(userRepo)
	userSvc.SetTrackRepository(trackRepo)
	userSvc.SetNavigationRepository(navigationRepo)
	userSvc.SetFollowRepository(followRepo)
	userSvc.SetAchievementService(achievementSvc)
	loginSvc := service.NewLoginService(userRepo, loginLogRepo, "", "", testJWTSecret)
	companionSvc := service.NewCompanionService(companionRepo, userRepo)
	companionSvc.SetTrackRepository(trackRepo)
	companionSvc.SetMQTTOptions(service.CompanionMQTTOptions{
		BrokerURL:        "mqtt://127.0.0.1:1883",
		WebsocketURL:     "ws://127.0.0.1:8083/mqtt",
		TopicPrefix:      "companion",
		CredentialTTL:    time.Hour,
		CredentialSecret: "test_companion_secret",
	})
	tokenBlacklist := middleware.NewTokenBlacklist()
	staticRoot, _ := os.MkdirTemp("", "track_server_test_static_")
	feedbackSvc := service.NewFeedbackService(feedbackRepo, filepath.Join(staticRoot, "private_feedback", "images"))
	avatarCacheDir := filepath.Join(staticRoot, "avatars")
	avatarCache, err := service.NewAssetCacheService(
		avatarCacheDir,
		"/api/v1/static/avatars",
		[]string{".png", ".jpg", ".jpeg", ".webp"},
		".png",
	)
	if err == nil {
		trackSvc.SetAvatarCache(avatarCache)
		userSvc.SetAvatarCache(avatarCache)
		companionSvc.SetAvatarCache(avatarCache)
	}
	screenshotCacheDir := filepath.Join(staticRoot, "screenshots")
	screenshotCache, err := service.NewAssetCacheService(
		screenshotCacheDir,
		"/api/v1/static/screenshots",
		[]string{".png", ".jpg", ".jpeg", ".webp", ".svg"},
		".png",
	)
	if err == nil {
		companionSvc.SetScreenshotCache(screenshotCache)
	}

	h := server.Default()
	handler.RegisterRoutes(h, handler.Deps{
		TrackService:               trackSvc,
		UserService:                userSvc,
		LoginService:               loginSvc,
		CompanionService:           companionSvc,
		AchievementService:         achievementSvc,
		FeedbackService:            feedbackSvc,
		JWTSecret:                  testJWTSecret,
		TokenBlacklist:             tokenBlacklist,
		CompanionMQTTInternalToken: testInternalToken,
		OpsInternalToken:           testInternalToken,
		StaticRoot:                 staticRoot,
	})

	return &testEnv{h: h, trackRepo: trackRepo, userRepo: userRepo, collectRepo: collectRepo, followRepo: followRepo, loginLogRepo: loginLogRepo, navigationRepo: navigationRepo, companionRepo: companionRepo, feedbackRepo: feedbackRepo, achievementRepo: achievementRepo, loginSvc: loginSvc, tokenBlacklist: tokenBlacklist, internalToken: testInternalToken, staticRoot: staticRoot, avatarCacheDir: avatarCacheDir, screenshotCacheDir: screenshotCacheDir}
}

func (e *testEnv) close() {
	if e == nil {
		return
	}
	if e.tokenBlacklist != nil {
		e.tokenBlacklist.Close()
	}
	if e.staticRoot != "" {
		_ = os.RemoveAll(e.staticRoot)
	}
}

func (e *testEnv) generateTestToken(userID int64) string {
	return e.generateTestTokenWith(userID, 1, time.Hour)
}

func (e *testEnv) generateTestTokenWith(userID int64, tokenVersion int64, ttl time.Duration) string {
	claims := jwtlib.MapClaims{
		"user_id":       userID,
		"token_version": tokenVersion,
		"exp":           time.Now().Add(ttl).Unix(),
		"iat":           time.Now().Unix(),
	}
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	tokenStr, _ := token.SignedString([]byte(testJWTSecret))
	return tokenStr
}

func authHeader(token string) ut.Header {
	return ut.Header{Key: "Authorization", Value: "Bearer " + token}
}

// perform performs an HTTP request against the Hertz engine with common headers.
func (e *testEnv) perform(method, url string, body []byte, extraHeaders ...ut.Header) *ut.ResponseRecorder {
	var b *ut.Body
	if body != nil {
		b = &ut.Body{Body: bytes.NewBuffer(body), Len: len(body)}
	}
	headers := []ut.Header{
		{Key: middleware.HeaderPlatform, Value: "android"},
		{Key: middleware.HeaderClientVersion, Value: "1.0.0"},
		{Key: middleware.HeaderClientLanguage, Value: "zh-CN"},
		{Key: middleware.HeaderLocation, Value: "30.0,120.0"},
	}
	headers = append(headers, extraHeaders...)
	return ut.PerformRequest(e.h.Engine, method, url, b, headers...)
}

// decodeJSON is a helper to unmarshal response body into v.
func decodeJSON(t *testing.T, respBody []byte, v interface{}) {
	t.Helper()
	if err := json.Unmarshal(respBody, v); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
}

func TestAchievementLevelRulesPage_PublicHTML(t *testing.T) {
	e := newTestEnv()

	w := e.perform(http.MethodGet, "/api/v1/achievement/level-rules.html", nil)
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", resp.StatusCode(), string(w.Body.Bytes()))
	}
	contentType := string(resp.Header.Get("Content-Type"))
	if !strings.Contains(contentType, "text/html") {
		t.Fatalf("expected text/html content type, got %q", contentType)
	}
	body := string(resp.Body())
	for _, want := range []string{"成长等级规则", "单次 XP = 距离 XP + 时长 XP + 爬升 XP + 内容奖励 XP", "Lv.10"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected level rules page to contain %q", want)
		}
	}

	w = e.perform(http.MethodGet, "/api/v1/achievement/level-rules.html?lang=english&is_dark=true", nil)
	resp = w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected query page status 200, got %d body=%s", resp.StatusCode(), string(w.Body.Bytes()))
	}
	body = string(resp.Body())
	for _, want := range []string{`params.get("lang") === "english"`, `params.get("is_dark") === "true"`, `data-lang-panel="en"`, "Level Rules"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected level rules page with query support to contain %q", want)
		}
	}
}

// TestCreateTrack_Success verifies POST /api/track/create succeeds with valid headers.
func TestCreateTrack_Success(t *testing.T) {
	e := newTestEnv()
	token := e.generateTestToken(1001)
	w := e.perform(http.MethodPost, "/api/v1/track/create", nil, authHeader(token), ut.Header{Key: middleware.HeaderUserID, Value: "1001"})
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode())
	}
	var result handler.StandardResponse[*models.Track]
	decodeJSON(t, resp.Body(), &result)
	if result.Code != 0 {
		t.Fatalf("expected code 0, got %d", result.Code)
	}
	if result.Data == nil {
		t.Fatalf("expected non-nil data")
	}
	if result.Data.UserID != 1001 {
		t.Fatalf("expected user_id 1001, got %d", result.Data.UserID)
	}
	if result.Data.Status != models.TrackStatusNormal {
		t.Fatalf("expected status normal, got %d", result.Data.Status)
	}
	if !result.Data.IsRunning {
		t.Fatalf("expected created track to be running")
	}
}

func TestListTrackTypes(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)
	now := time.Now()

	_ = e.trackRepo.Create(ctx, &models.Track{ID: "hike-month", UserID: 1001, Title: "徒步月内", TrackType: "徒步", StartTime: now.AddDate(0, 0, -10), Distance: 120.5, Duration: 600, CaloriesBurned: 80.5, IsRunning: false, Status: models.TrackStatusNormal})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "hike-year", UserID: 1001, Title: "徒步年内", TrackType: "徒步", StartTime: now.AddDate(0, -2, 0), Distance: 80, Duration: 400, CaloriesBurned: 60, IsRunning: false, Status: models.TrackStatusPrivate})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "run-month", UserID: 1001, Title: "跑步月内", TrackType: "跑步", StartTime: now.AddDate(0, 0, -5), Distance: 300, Duration: 700, CaloriesBurned: 90, IsRunning: false, Status: models.TrackStatusNormal})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "run-year", UserID: 1001, Title: "跑步年内", TrackType: "跑步", StartTime: now.AddDate(0, -8, 0), Distance: 500, Duration: 900, CaloriesBurned: 100, IsRunning: false, Status: models.TrackStatusNormal})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "old", UserID: 1001, Title: "历史", TrackType: "徒步", StartTime: now.AddDate(-2, 0, 0), Distance: 999, Duration: 999, CaloriesBurned: 999, IsRunning: false, Status: models.TrackStatusNormal})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "stats-running", UserID: 1001, Title: "进行中", TrackType: "徒步", StartTime: now.AddDate(0, 0, -1), Distance: 999, Duration: 999, CaloriesBurned: 999, IsRunning: true, Status: models.TrackStatusNormal})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "stats-other", UserID: 1002, Title: "他人", TrackType: "徒步", StartTime: now.AddDate(0, 0, -1), Distance: 999, Duration: 999, CaloriesBurned: 999, IsRunning: false, Status: models.TrackStatusNormal})

	// X-User-ID 即使被伪造成其他用户，也应以 token 解析出的 AuthUserID=1001 为准。
	w := e.perform(http.MethodGet, "/api/v1/track/types", nil, authHeader(token), ut.Header{Key: middleware.HeaderUserID, Value: "1002"})
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", resp.StatusCode(), string(resp.Body()))
	}
	var result handler.StandardResponse[[]models.TrackTypeOption]
	decodeJSON(t, resp.Body(), &result)
	if result.Code != 0 {
		t.Fatalf("expected code 0, got %d", result.Code)
	}
	expected := []struct {
		typeCode   string
		name       string
		themeColor string
		iconURL    string
	}{
		{typeCode: "hiking", name: "徒步", themeColor: "#345631", iconURL: "/api/v1/static/track_type_icon/hiking.svg"},
		{typeCode: "running", name: "跑步", themeColor: "#F26A4B", iconURL: "/api/v1/static/track_type_icon/running.svg"},
		{typeCode: "climbing", name: "爬山", themeColor: "#6C4CE1", iconURL: "/api/v1/static/track_type_icon/climbing.svg"},
		{typeCode: "riding", name: "骑行", themeColor: "#2F80ED", iconURL: "/api/v1/static/track_type_icon/riding.svg"},
		{typeCode: "driving", name: "自驾", themeColor: "#F5A623", iconURL: "/api/v1/static/track_type_icon/driving.svg"},
	}
	if len(result.Data) != len(expected) {
		t.Fatalf("expected %d track types, got %#v", len(expected), result.Data)
	}
	for i, item := range expected {
		if result.Data[i].Type != item.typeCode {
			t.Fatalf("expected track type[%d].type=%q, got %q", i, item.typeCode, result.Data[i].Type)
		}
		if result.Data[i].Name != item.name {
			t.Fatalf("expected track type[%d].name=%q, got %q", i, item.name, result.Data[i].Name)
		}
		if result.Data[i].ThemeColor != item.themeColor {
			t.Fatalf("expected track type[%d].theme_color=%q, got %q", i, item.themeColor, result.Data[i].ThemeColor)
		}
		if result.Data[i].IconURL != item.iconURL {
			t.Fatalf("expected track type[%d].icon_url=%q, got %q", i, item.iconURL, result.Data[i].IconURL)
		}
		if result.Data[i].IconAnimURL != "" {
			t.Fatalf("expected track type[%d].icon_anim_url empty, got %q", i, result.Data[i].IconAnimURL)
		}
	}
	if got := result.Data[0].Milestone.Month; got.TrackCount != 1 || got.Distance != 120.5 || got.Duration != 600 || got.Calories != 80.5 {
		t.Fatalf("unexpected hiking month milestone: %#v", got)
	}
	if got := result.Data[0].Milestone.Year; got.TrackCount != 2 || got.Distance != 200.5 || got.Duration != 1000 || got.Calories != 140.5 {
		t.Fatalf("unexpected hiking year milestone: %#v", got)
	}
	if got := result.Data[1].Milestone.Month; got.TrackCount != 1 || got.Distance != 300 || got.Duration != 700 || got.Calories != 90 {
		t.Fatalf("unexpected running month milestone: %#v", got)
	}
	if got := result.Data[1].Milestone.Year; got.TrackCount != 2 || got.Distance != 800 || got.Duration != 1600 || got.Calories != 190 {
		t.Fatalf("unexpected running year milestone: %#v", got)
	}
	if got := result.Data[2].Milestone; got.Month.TrackCount != 0 || got.Year.TrackCount != 0 {
		t.Fatalf("expected climbing milestone empty, got %#v", got)
	}
}

func TestCreateTrack_WithBody(t *testing.T) {
	e := newTestEnv()
	token := e.generateTestToken(1001)
	body, _ := json.Marshal(map[string]interface{}{
		"title":                "傍晚夜跑",
		"session_id":           "sess_create_001",
		"city_code":            "330100",
		"track_type":           "跑步",
		"source_tag":           " manual_seed ",
		"coordinate_system":    "GCJ02",
		"start_time":           "2026-04-20T12:00:00Z",
		"end_time":             "2026-04-20T12:30:00Z",
		"distance":             1500.5,
		"duration":             1800,
		"calories_burned":      96.5,
		"elevation_gain":       66,
		"raw_track_url":        "https://example.com/raw/track.json",
		"track_screenshot_url": "https://example.com/track.png",
		"is_running":           false,
		"avg_speed_kmh":        3.0,
	})

	w := e.perform(http.MethodPost, "/api/v1/track/create", body, authHeader(token), ut.Header{Key: middleware.HeaderUserID, Value: "1001"})
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", resp.StatusCode(), string(resp.Body()))
	}
	var result handler.StandardResponse[*models.Track]
	decodeJSON(t, resp.Body(), &result)
	if result.Code != 0 {
		t.Fatalf("expected code 0, got %d", result.Code)
	}
	if result.Data == nil {
		t.Fatalf("expected non-nil data")
	}
	if result.Data.Distance != 1500.5 {
		t.Fatalf("expected distance 1500.5, got %v", result.Data.Distance)
	}
	if result.Data.Title != "傍晚夜跑" {
		t.Fatalf("expected title 傍晚夜跑, got %q", result.Data.Title)
	}
	if result.Data.SessionID != "sess_create_001" {
		t.Fatalf("expected session_id sess_create_001, got %q", result.Data.SessionID)
	}
	if result.Data.TrackType != "running" {
		t.Fatalf("unexpected track_type: %q", result.Data.TrackType)
	}
	if result.Data.SourceTag != "manual_seed" {
		t.Fatalf("unexpected source_tag: %q", result.Data.SourceTag)
	}
	if result.Data.CoordinateSystem != "GCJ02" {
		t.Fatalf("unexpected coordinate_system: %q", result.Data.CoordinateSystem)
	}
	if result.Data.Duration != 1800 {
		t.Fatalf("expected duration 1800, got %v", result.Data.Duration)
	}
	if result.Data.CaloriesBurned != 96.5 {
		t.Fatalf("expected calories_burned 96.5, got %v", result.Data.CaloriesBurned)
	}
	if result.Data.ElevationGain != 66 {
		t.Fatalf("expected elevation_gain 66, got %v", result.Data.ElevationGain)
	}
	if result.Data.RawTrackURL != "https://example.com/raw/track.json" {
		t.Fatalf("unexpected raw_track_url: %q", result.Data.RawTrackURL)
	}
	if result.Data.TrackScreenshotURL != "https://example.com/track.png" {
		t.Fatalf("unexpected track_screenshot_url: %q", result.Data.TrackScreenshotURL)
	}
	if result.Data.CityCode != "330100" {
		t.Fatalf("unexpected city_code: %q", result.Data.CityCode)
	}
	if result.Data.IsRunning {
		t.Fatalf("expected is_running false")
	}
	if result.Data.AvgSpeedKmh != 3.0 {
		t.Fatalf("expected avg_speed_kmh 3.0, got %v", result.Data.AvgSpeedKmh)
	}
	if got := result.Data.StartTime.Format(time.RFC3339); got != "2026-04-20T12:00:00Z" {
		t.Fatalf("expected start_time 2026-04-20T12:00:00Z, got %s", got)
	}
	if got := result.Data.EndTime.Format(time.RFC3339); got != "2026-04-20T12:30:00Z" {
		t.Fatalf("expected end_time 2026-04-20T12:30:00Z, got %s", got)
	}
}

func TestCreateTrackRejectsActiveCompanionSession(t *testing.T) {
	e := newTestEnv()
	ensureTestUser(t, e, 1001, "owner")
	token := e.generateTestToken(1001)

	w := e.perform(http.MethodPost, "/api/v1/companion/session/create", []byte(`{"title":"同行中"}`), authHeader(token))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected create companion status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}

	w = e.perform(http.MethodPost, "/api/v1/track/create", []byte(`{}`), authHeader(token))
	if w.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("expected create running track status 400, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var resp struct {
		Error string `json:"error"`
	}
	decodeJSON(t, w.Body.Bytes(), &resp)
	if resp.Error != "you already joined an active companion session: 同行中" {
		t.Fatalf("unexpected error response: %+v", resp)
	}

	w = e.perform(http.MethodPost, "/api/v1/track/create", []byte(`{"is_running":false,"session_id":"sess_done"}`), authHeader(token))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected completed track status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
}

func TestAchievementRewardsAfterCompletedTrack(t *testing.T) {
	e := newTestEnv()
	token := e.generateTestToken(1001)

	body := []byte(`{"track_type":"跑步","distance":6000,"duration":1800,"is_running":false}`)
	w := e.perform(http.MethodPost, "/api/v1/track/create", body, authHeader(token))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected create completed track status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var createResp handler.StandardResponse[*models.Track]
	decodeJSON(t, w.Body.Bytes(), &createResp)
	if createResp.Data == nil {
		t.Fatalf("expected created track")
	}
	createdEarned := map[string]bool{}
	for _, reward := range createResp.Data.EarnedRewards {
		if reward.Earned {
			createdEarned[reward.Code] = true
		}
	}
	for _, code := range []string{"first_track", "run_5k"} {
		if !createdEarned[code] {
			t.Fatalf("expected create response earned_rewards to include %s, got %+v", code, createResp.Data.EarnedRewards)
		}
	}

	w = e.perform(http.MethodGet, "/api/v1/achievement/rewards", nil, authHeader(token))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected achievement rewards status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var resp handler.StandardResponse[*models.AchievementRewardList]
	decodeJSON(t, w.Body.Bytes(), &resp)
	if resp.Data == nil {
		t.Fatalf("expected response data")
	}
	if resp.Data.Stats.QualifiedTrackCount != 1 {
		t.Fatalf("expected qualified track count 1, got %d", resp.Data.Stats.QualifiedTrackCount)
	}
	earned := map[string]bool{}
	for _, reward := range resp.Data.Rewards {
		if reward.Earned {
			earned[reward.Code] = true
		}
	}
	for _, code := range []string{"first_track", "run_5k"} {
		if !earned[code] {
			t.Fatalf("expected reward %s to be earned", code)
		}
	}
	for _, reward := range resp.Data.Rewards {
		if reward.Type == models.AchievementRewardTypeMilestone {
			t.Fatalf("expected MVP rewards to exclude milestones, got %+v", reward)
		}
	}
}

func TestAchievementRewardsAfterCompletedTrackWithEnglishType(t *testing.T) {
	e := newTestEnv()
	token := e.generateTestToken(1001)

	body := []byte(`{"track_type":"running","distance":6000,"duration":1800,"is_running":false}`)
	w := e.perform(http.MethodPost, "/api/v1/track/create", body, authHeader(token))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected create completed track status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var createResp handler.StandardResponse[*models.Track]
	decodeJSON(t, w.Body.Bytes(), &createResp)
	if createResp.Data == nil {
		t.Fatalf("expected created track")
	}
	if createResp.Data.TrackType != "running" {
		t.Fatalf("expected normalized track_type running, got %q", createResp.Data.TrackType)
	}
	if len(createResp.Data.EarnedRewards) != 2 {
		t.Fatalf("expected create response earned_rewards length 2, got %d: %+v", len(createResp.Data.EarnedRewards), createResp.Data.EarnedRewards)
	}

	rewards, err := e.achievementRepo.ListUserRewards(context.Background(), 1001)
	if err != nil {
		t.Fatalf("list achievement rewards failed: %v", err)
	}
	earned := map[string]bool{}
	for _, reward := range rewards {
		earned[reward.RewardCode] = true
	}
	for _, code := range []string{"first_track", "run_5k"} {
		if !earned[code] {
			t.Fatalf("expected reward %s to be settled for english track_type", code)
		}
	}
}

func TestAchievementRewardsLazyBackfillHistoricalEnglishType(t *testing.T) {
	e := newTestEnv()
	token := e.generateTestToken(1001)
	ctx := context.Background()

	if err := e.trackRepo.Create(ctx, &models.Track{
		ID:        "trk-ach-backfill",
		UserID:    1001,
		Title:     "历史跑步",
		TrackType: "running",
		Distance:  6000,
		Duration:  1800,
		StartTime: time.Now().Add(-time.Hour),
		EndTime:   time.Now(),
		IsRunning: false,
		Status:    models.TrackStatusNormal,
	}); err != nil {
		t.Fatalf("create historical track failed: %v", err)
	}
	before, err := e.achievementRepo.ListUserRewards(ctx, 1001)
	if err != nil {
		t.Fatalf("list rewards before backfill failed: %v", err)
	}
	if len(before) != 0 {
		t.Fatalf("expected no rewards before lazy backfill, got %d", len(before))
	}

	w := e.perform(http.MethodGet, "/api/v1/achievement/rewards", nil, authHeader(token))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected achievement rewards status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	after, err := e.achievementRepo.ListUserRewards(ctx, 1001)
	if err != nil {
		t.Fatalf("list rewards after backfill failed: %v", err)
	}
	earned := map[string]bool{}
	for _, reward := range after {
		earned[reward.RewardCode] = true
	}
	for _, code := range []string{"first_track", "run_5k"} {
		if !earned[code] {
			t.Fatalf("expected reward %s after lazy backfill", code)
		}
	}
}

func TestOpsRefreshAchievementByPhone(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 2001, Phone: "13900002001", Nickname: "ops-user"})
	if err := e.trackRepo.Create(ctx, &models.Track{
		ID:        "trk-ops-ach-refresh",
		UserID:    2001,
		Title:     "历史跑步",
		TrackType: "跑步",
		Distance:  6000,
		Duration:  1800,
		StartTime: time.Now().Add(-time.Hour),
		EndTime:   time.Now(),
		IsRunning: false,
		Status:    models.TrackStatusNormal,
	}); err != nil {
		t.Fatalf("create historical track failed: %v", err)
	}
	before, err := e.achievementRepo.ListUserRewards(ctx, 2001)
	if err != nil {
		t.Fatalf("list rewards before refresh failed: %v", err)
	}
	if len(before) != 0 {
		t.Fatalf("expected no rewards before ops refresh, got %d", len(before))
	}

	body := []byte(`{"phone":"13900002001"}`)
	w := e.perform(http.MethodPost, "/api/v1/ops/achievement/refresh", body, ut.Header{Key: "X-Internal-Token", Value: e.internalToken})
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected ops refresh status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var resp handler.StandardResponse[handler.OpsAchievementRefreshResult]
	decodeJSON(t, w.Body.Bytes(), &resp)
	if resp.Data.UserID != 2001 {
		t.Fatalf("expected user_id 2001, got %d", resp.Data.UserID)
	}
	if resp.Data.NewRewardCount != 2 {
		t.Fatalf("expected 2 new rewards, got %d data=%+v", resp.Data.NewRewardCount, resp.Data)
	}
	if resp.Data.EarnedRewardCount != 2 {
		t.Fatalf("expected 2 earned rewards, got %d", resp.Data.EarnedRewardCount)
	}
	if resp.Data.QualifiedTrackCount != 1 {
		t.Fatalf("expected qualified track count 1, got %d", resp.Data.QualifiedTrackCount)
	}
	if resp.Data.TotalXP <= 0 {
		t.Fatalf("expected total_xp > 0, got %d", resp.Data.TotalXP)
	}
	after, err := e.achievementRepo.ListUserRewards(ctx, 2001)
	if err != nil {
		t.Fatalf("list rewards after refresh failed: %v", err)
	}
	if len(after) != 2 {
		t.Fatalf("expected 2 stored rewards after refresh, got %d", len(after))
	}

	w = e.perform(http.MethodPost, "/api/v1/ops/achievement/refresh", body, ut.Header{Key: "X-Internal-Token", Value: e.internalToken})
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected second ops refresh status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	decodeJSON(t, w.Body.Bytes(), &resp)
	if resp.Data.NewRewardCount != 0 {
		t.Fatalf("expected idempotent second refresh new_reward_count 0, got %d", resp.Data.NewRewardCount)
	}
}

func TestOpsRefreshAchievementRequiresInternalToken(t *testing.T) {
	e := newTestEnv()
	w := e.perform(http.MethodPost, "/api/v1/ops/achievement/refresh", []byte(`{"phone":"13900002001"}`))
	if w.Result().StatusCode() != http.StatusUnauthorized {
		t.Fatalf("expected missing token status 401, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
}

func TestCreateTrack_InvalidTimeRange(t *testing.T) {
	e := newTestEnv()
	token := e.generateTestToken(1001)
	body, _ := json.Marshal(map[string]interface{}{
		"start_time": "2026-04-20T12:30:00Z",
		"end_time":   "2026-04-20T12:00:00Z",
	})

	w := e.perform(http.MethodPost, "/api/v1/track/create", body, authHeader(token), ut.Header{Key: middleware.HeaderUserID, Value: "1001"})
	if w.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Result().StatusCode())
	}
}

func TestCreateTrack_SourceTagTooLong(t *testing.T) {
	e := newTestEnv()
	token := e.generateTestToken(1001)
	body, _ := json.Marshal(map[string]interface{}{
		"source_tag": strings.Repeat("a", 65),
	})

	w := e.perform(http.MethodPost, "/api/v1/track/create", body, authHeader(token), ut.Header{Key: middleware.HeaderUserID, Value: "1001"})
	if w.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Result().StatusCode())
	}
}

func TestCreateTrack_InvalidSourceTag(t *testing.T) {
	e := newTestEnv()
	token := e.generateTestToken(1001)
	body, _ := json.Marshal(map[string]interface{}{
		"source_tag": "official_seed",
	})

	w := e.perform(http.MethodPost, "/api/v1/track/create", body, authHeader(token), ut.Header{Key: middleware.HeaderUserID, Value: "1001"})
	if w.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Result().StatusCode())
	}
}

// TestCreateTrack_NoAuth verifies 401 is returned when no JWT token is provided.
func TestCreateTrack_NoAuth(t *testing.T) {
	e := newTestEnv()
	w := e.perform(http.MethodPost, "/api/v1/track/create", nil)
	resp := w.Result()
	if resp.StatusCode() != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", resp.StatusCode())
	}
}

// TestGetRunningTrack_Empty verifies when no running track exists, running=false is returned.
func TestGetRunningTrack_Empty(t *testing.T) {
	e := newTestEnv()
	token := e.generateTestToken(1001)
	w := e.perform(http.MethodGet, "/api/v1/track/running?user_id=1001", nil, authHeader(token))
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode())
	}
	var payload handler.StandardResponse[handler.RunningTrackResult]
	decodeJSON(t, resp.Body(), &payload)
	if payload.Code != 0 {
		t.Fatalf("expected code 0, got %d", payload.Code)
	}
	if payload.Data.Running {
		t.Fatalf("expected running=false, got true")
	}
}

// TestGetRunningTrack_Success verifies running track can be queried by explicit running flag.
func TestGetRunningTrack_Success(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk-running", UserID: 1001, Title: "进行中的轨迹", IsRunning: true, Status: models.TrackStatusNormal})

	w := e.perform(http.MethodGet, "/api/v1/track/running?user_id=1001", nil, authHeader(token))
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode())
	}
	var payload handler.StandardResponse[handler.RunningTrackResult]
	decodeJSON(t, resp.Body(), &payload)
	if payload.Code != 0 {
		t.Fatalf("expected code 0, got %d", payload.Code)
	}
	if !payload.Data.Running {
		t.Fatalf("expected running=true, got false")
	}
	if payload.Data.Track == nil || payload.Data.Track.ID != "trk-running" {
		t.Fatalf("expected running track id trk-running, got %+v", payload.Data.Track)
	}
}

func TestGetRunningTrack_QueryUserIDMismatch(t *testing.T) {
	e := newTestEnv()
	token := e.generateTestToken(1001)
	w := e.perform(http.MethodGet, "/api/v1/track/running?user_id=1002", nil, authHeader(token))
	if w.Result().StatusCode() != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", w.Result().StatusCode())
	}
}

func TestGetRunningTrack_HeaderUserIDMismatch(t *testing.T) {
	e := newTestEnv()
	token := e.generateTestToken(1001)
	w := e.perform(http.MethodGet, "/api/v1/track/running", nil, authHeader(token), ut.Header{Key: middleware.HeaderUserID, Value: "1002"})
	if w.Result().StatusCode() != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", w.Result().StatusCode())
	}
}

// TestGetTrackMap_NotFound verifies map endpoint returns 404 when track not exists.
func TestGetTrackMap_NotFound(t *testing.T) {
	e := newTestEnv()
	token := e.generateTestToken(1001)
	w := e.perform(http.MethodGet, "/api/v1/track/nonexist/map", nil, authHeader(token))
	resp := w.Result()
	if resp.StatusCode() != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", resp.StatusCode())
	}
}

func TestGetTrackDetail_Success(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)

	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk-detail", UserID: 1001, Title: "详情轨迹", CoordinateSystem: "WGS84", IsRunning: false, Status: models.TrackStatusNormal})

	w := e.perform(http.MethodGet, "/api/v1/track/trk-detail/detail", nil, authHeader(token))
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode())
	}
	var result handler.StandardResponse[*models.Track]
	decodeJSON(t, resp.Body(), &result)
	if result.Code != 0 {
		t.Fatalf("expected code 0, got %d", result.Code)
	}
	if result.Data == nil || result.Data.ID != "trk-detail" {
		t.Fatalf("unexpected data: %+v", result.Data)
	}
	if result.Data.CoordinateSystem != "WGS84" {
		t.Fatalf("unexpected coordinate_system: %q", result.Data.CoordinateSystem)
	}
}

// TestRecommendAndSearch verifies recommend and search endpoints work with simple data.
func TestRecommendAndSearch(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)
	otherToken := e.generateTestToken(2002)
	startTime := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	endTime := startTime.Add(90 * time.Minute)
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 1001, Nickname: "Alice", AvatarURL: "https://example.com/avatar.png"})
	// seed one normal track and one running track (running should be excluded from recommend list)
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk1", UserID: 1001, CityCode: "330100", TrackType: "徒步", Title: "西湖徒步", StartTime: startTime, EndTime: endTime, AvgSpeedKmh: 12.34, CaloriesBurned: 88.8, RawTrackURL: "https://example.com/trk1.dat", IsRunning: false, Status: models.TrackStatusNormal})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk-running", UserID: 1001, Title: "进行中跑步", IsRunning: true})
	_ = e.collectRepo.AddCollect(ctx, 1001, "trk1")

	// other user reports one navigation usage
	wNav := e.perform(http.MethodPost, "/api/v1/track/trk1/navigation/report", nil, authHeader(otherToken))
	if wNav.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected navigation report status 200, got %d", wNav.Result().StatusCode())
	}
	// owner reports should be rejected
	wNavSelf := e.perform(http.MethodPost, "/api/v1/track/trk1/navigation/report", nil, authHeader(token))
	if wNavSelf.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("expected navigation report self status 400, got %d", wNavSelf.Result().StatusCode())
	}

	w1 := e.perform(http.MethodGet, "/api/v1/track/recommend/list", nil, authHeader(token), ut.Header{Key: middleware.HeaderUserID, Value: "1001"})
	resp1 := w1.Result()
	if resp1.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp1.StatusCode())
	}
	var result handler.StandardResponse[*models.TrackSummaryPage]
	decodeJSON(t, resp1.Body(), &result)
	if result.Code != 0 {
		t.Fatalf("expected code 0, got %d", result.Code)
	}
	if result.Data == nil {
		t.Fatalf("expected recommend page data")
	}
	if len(result.Data.Items) != 1 {
		t.Fatalf("expected recommend list size 1 (exclude running), got %d", len(result.Data.Items))
	}
	if result.Data.HasMore {
		t.Fatalf("expected recommend has_more=false")
	}
	if result.Data.NextCursor != "" {
		t.Fatalf("expected recommend next_cursor empty, got %q", result.Data.NextCursor)
	}
	if result.Data.Items[0].ID != "trk1" {
		t.Fatalf("expected recommend track id trk1, got %q", result.Data.Items[0].ID)
	}
	if !result.Data.Items[0].Collected {
		t.Fatalf("expected recommend track trk1 collected=true")
	}
	if result.Data.Items[0].CollectCount != 1 {
		t.Fatalf("expected recommend track trk1 collect_count=1, got %d", result.Data.Items[0].CollectCount)
	}
	if result.Data.Items[0].UserAvatarURL != "https://example.com/avatar.png" {
		t.Fatalf("expected recommend track trk1 user_avatar_url set, got %q", result.Data.Items[0].UserAvatarURL)
	}
	if result.Data.Items[0].Nickname != "Alice" {
		t.Fatalf("expected recommend track trk1 nickname=Alice, got %q", result.Data.Items[0].Nickname)
	}
	if result.Data.Items[0].CityCode != "330100" {
		t.Fatalf("expected recommend track trk1 city_code=330100, got %q", result.Data.Items[0].CityCode)
	}
	if result.Data.Items[0].CityName != "杭州市" {
		t.Fatalf("expected recommend track trk1 city_name=杭州市, got %q", result.Data.Items[0].CityName)
	}
	if result.Data.Items[0].TrackType != "徒步" {
		t.Fatalf("expected recommend track trk1 track_type=徒步, got %q", result.Data.Items[0].TrackType)
	}
	if got := result.Data.Items[0].StartTime.Format(time.RFC3339); got != "2026-04-20T12:00:00Z" {
		t.Fatalf("expected recommend track trk1 start_time=2026-04-20T12:00:00Z, got %s", got)
	}
	if got := result.Data.Items[0].EndTime.Format(time.RFC3339); got != "2026-04-20T13:30:00Z" {
		t.Fatalf("expected recommend track trk1 end_time=2026-04-20T13:30:00Z, got %s", got)
	}
	if got := result.Data.Items[0].AvgSpeedKmh; got != 12.34 {
		t.Fatalf("expected recommend track trk1 avg_speed_kmh=12.34, got %v", got)
	}
	if got := result.Data.Items[0].CaloriesBurned; got != 88.8 {
		t.Fatalf("expected recommend track trk1 calories_burned=88.8, got %v", got)
	}
	if result.Data.Items[0].NavigateCount != 1 {
		t.Fatalf("expected recommend track trk1 navigate_count=1, got %d", result.Data.Items[0].NavigateCount)
	}

	w2 := e.perform(http.MethodGet, "/api/v1/track/search/list?keyword=西湖", nil, authHeader(token))
	resp2 := w2.Result()
	if resp2.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp2.StatusCode())
	}
	var search handler.StandardResponse[*models.TrackSummaryPage]
	decodeJSON(t, resp2.Body(), &search)
	if search.Code != 0 {
		t.Fatalf("expected search code 0, got %d", search.Code)
	}
	if search.Data == nil {
		t.Fatalf("expected search page data")
	}
	if len(search.Data.Items) != 1 || search.Data.Items[0].ID != "trk1" {
		t.Fatalf("unexpected search result: %+v", search)
	}
	if search.Data.HasMore {
		t.Fatalf("expected search has_more=false")
	}
	if search.Data.NextCursor != "" {
		t.Fatalf("expected search next_cursor empty, got %q", search.Data.NextCursor)
	}
	if search.Data.Items[0].TrackType != "徒步" {
		t.Fatalf("expected search track trk1 track_type=徒步, got %q", search.Data.Items[0].TrackType)
	}
	if got := search.Data.Items[0].StartTime.Format(time.RFC3339); got != "2026-04-20T12:00:00Z" {
		t.Fatalf("expected search track trk1 start_time=2026-04-20T12:00:00Z, got %s", got)
	}
	if got := search.Data.Items[0].EndTime.Format(time.RFC3339); got != "2026-04-20T13:30:00Z" {
		t.Fatalf("expected search track trk1 end_time=2026-04-20T13:30:00Z, got %s", got)
	}
	if got := search.Data.Items[0].AvgSpeedKmh; got != 12.34 {
		t.Fatalf("expected search track trk1 avg_speed_kmh=12.34, got %v", got)
	}
	if got := search.Data.Items[0].CaloriesBurned; got != 88.8 {
		t.Fatalf("expected search track trk1 calories_burned=88.8, got %v", got)
	}
	if search.Data.Items[0].NavigateCount != 1 {
		t.Fatalf("expected search track trk1 navigate_count=1, got %d", search.Data.Items[0].NavigateCount)
	}
}

func TestTrackSummaryList_RewritesOSSAvatarToStaticURL(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)
	startTime := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	avatarURL := "https://track-avatar.oss-cn-beijing.aliyuncs.com/avatar/1001.png"

	if err := os.WriteFile(filepath.Join(e.avatarCacheDir, "1001.png"), []byte("avatar"), 0o644); err != nil {
		t.Fatalf("seed avatar cache failed: %v", err)
	}
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 1001, Nickname: "Alice", AvatarURL: avatarURL})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk-oss-avatar", UserID: 1001, Title: "头像静态化", StartTime: startTime, RawTrackURL: "https://example.com/trk-oss-avatar.dat", IsRunning: false, Status: models.TrackStatusNormal})

	w := e.perform(http.MethodGet, "/api/v1/track/recommend/list", nil, authHeader(token), ut.Header{Key: middleware.HeaderUserID, Value: "1001"})
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Result().StatusCode())
	}
	var result handler.StandardResponse[*models.TrackSummaryPage]
	decodeJSON(t, w.Result().Body(), &result)
	if result.Data == nil || len(result.Data.Items) != 1 {
		t.Fatalf("unexpected recommend result: %+v", result)
	}
	if result.Data.Items[0].UserAvatarURL != "/api/v1/static/avatars/1001.png" {
		t.Fatalf("expected recommend user_avatar_url rewritten to static url, got %q", result.Data.Items[0].UserAvatarURL)
	}

	w = e.perform(http.MethodGet, "/api/v1/track/search/list?keyword=头像静态化", nil, authHeader(token))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected search status 200, got %d", w.Result().StatusCode())
	}
	decodeJSON(t, w.Result().Body(), &result)
	if result.Data == nil || len(result.Data.Items) != 1 {
		t.Fatalf("unexpected search result: %+v", result)
	}
	if result.Data.Items[0].UserAvatarURL != "/api/v1/static/avatars/1001.png" {
		t.Fatalf("expected search user_avatar_url rewritten to static url, got %q", result.Data.Items[0].UserAvatarURL)
	}
}

func TestCollectedTracksList_OmitsCollectedField(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)
	startTime := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	endTime := startTime.Add(30 * time.Minute)

	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 1001, Nickname: "Alice", AvatarURL: "https://example.com/avatar.png"})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk-collected-1", UserID: 1001, CityCode: "330100", TrackType: "徒步", Title: "收藏的轨迹", StartTime: startTime, EndTime: endTime, AvgSpeedKmh: 10.0, IsRunning: false, Status: models.TrackStatusNormal})
	_ = e.collectRepo.AddCollect(ctx, 1001, "trk-collected-1")

	w := e.perform(http.MethodGet, "/api/v1/track/collected/list", nil, authHeader(token))
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode())
	}

	var raw map[string]any
	decodeJSON(t, resp.Body(), &raw)
	if code, ok := raw["code"].(float64); !ok || int(code) != 0 {
		t.Fatalf("expected code 0, got %+v", raw["code"])
	}
	data, ok := raw["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %+v", raw["data"])
	}
	if got, ok := data["total_count"].(float64); !ok || int64(got) != 1 {
		t.Fatalf("expected total_count=1, got %+v", data["total_count"])
	}
	items, ok := data["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected items size 1, got %+v", data["items"])
	}
	item0, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("expected item object, got %+v", items[0])
	}
	if _, exists := item0["collected"]; exists {
		t.Fatalf("expected collected field omitted, got %+v", item0)
	}
	if item0["id"] != "trk-collected-1" {
		t.Fatalf("expected id trk-collected-1, got %v", item0["id"])
	}
}

func TestRecommendCursorPagination(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)

	start1 := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	start2 := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	start3 := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)

	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk-c", UserID: 1001, Title: "第三条", StartTime: start3, EndTime: start3.Add(30 * time.Minute), AvgSpeedKmh: 8.8, RawTrackURL: "https://example.com/trk-c.dat", IsRunning: false, Status: models.TrackStatusNormal})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk-a", UserID: 1001, Title: "第一条", StartTime: start1, EndTime: start1.Add(30 * time.Minute), AvgSpeedKmh: 10.1, RawTrackURL: "https://example.com/trk-a.dat", IsRunning: false, Status: models.TrackStatusNormal})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk-b", UserID: 1002, Title: "第二条", StartTime: start2, EndTime: start2.Add(30 * time.Minute), AvgSpeedKmh: 9.5, RawTrackURL: "https://example.com/trk-b.dat", IsRunning: false, Status: models.TrackStatusNormal})

	w1 := e.perform(http.MethodGet, "/api/v1/track/recommend/list?limit=2", nil, authHeader(token), ut.Header{Key: middleware.HeaderUserID, Value: "1001"})
	if w1.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected first page status 200, got %d", w1.Result().StatusCode())
	}
	var page1 handler.StandardResponse[*models.TrackSummaryPage]
	decodeJSON(t, w1.Result().Body(), &page1)
	if page1.Data == nil {
		t.Fatalf("expected first page data")
	}
	if len(page1.Data.Items) != 2 {
		t.Fatalf("expected first page size 2, got %d", len(page1.Data.Items))
	}
	if page1.Data.Items[0].ID != "trk-a" || page1.Data.Items[1].ID != "trk-b" {
		t.Fatalf("unexpected first page order: %+v", page1.Data.Items)
	}
	if got := page1.Data.Items[0].EndTime.Format(time.RFC3339); got != "2026-04-23T12:30:00Z" {
		t.Fatalf("expected first recommend item end_time=2026-04-23T12:30:00Z, got %s", got)
	}
	if got := page1.Data.Items[1].EndTime.Format(time.RFC3339); got != "2026-04-22T12:30:00Z" {
		t.Fatalf("expected second recommend item end_time=2026-04-22T12:30:00Z, got %s", got)
	}
	if got := page1.Data.Items[0].AvgSpeedKmh; got != 10.1 {
		t.Fatalf("expected first recommend item avg_speed_kmh=10.1, got %v", got)
	}
	if got := page1.Data.Items[1].AvgSpeedKmh; got != 9.5 {
		t.Fatalf("expected second recommend item avg_speed_kmh=9.5, got %v", got)
	}
	if !page1.Data.HasMore {
		t.Fatalf("expected first page has_more=true")
	}
	if page1.Data.NextCursor == "" {
		t.Fatalf("expected first page next_cursor")
	}

	w2 := e.perform(http.MethodGet, "/api/v1/track/recommend/list?limit=2&cursor="+page1.Data.NextCursor, nil, authHeader(token), ut.Header{Key: middleware.HeaderUserID, Value: "1001"})
	if w2.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected second page status 200, got %d", w2.Result().StatusCode())
	}
	var page2 handler.StandardResponse[*models.TrackSummaryPage]
	decodeJSON(t, w2.Result().Body(), &page2)
	if page2.Data == nil {
		t.Fatalf("expected second page data")
	}
	if len(page2.Data.Items) != 1 || page2.Data.Items[0].ID != "trk-c" {
		t.Fatalf("unexpected second page items: %+v", page2.Data.Items)
	}
	if got := page2.Data.Items[0].EndTime.Format(time.RFC3339); got != "2026-04-21T12:30:00Z" {
		t.Fatalf("expected second page recommend item end_time=2026-04-21T12:30:00Z, got %s", got)
	}
	if got := page2.Data.Items[0].AvgSpeedKmh; got != 8.8 {
		t.Fatalf("expected second page recommend item avg_speed_kmh=8.8, got %v", got)
	}
	if page2.Data.HasMore {
		t.Fatalf("expected second page has_more=false")
	}
	if page2.Data.NextCursor != "" {
		t.Fatalf("expected second page next_cursor empty, got %q", page2.Data.NextCursor)
	}

	w3 := e.perform(http.MethodGet, "/api/v1/track/recommend/list?cursor=bad-cursor", nil, authHeader(token), ut.Header{Key: middleware.HeaderUserID, Value: "1001"})
	if w3.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("expected invalid cursor status 400, got %d", w3.Result().StatusCode())
	}
}

func TestRecommendAndSearchUseDefaultAvatarWhenMissing(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)
	startTime := time.Date(2026, 4, 24, 8, 0, 0, 0, time.UTC)

	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 1001, Nickname: "Alice"})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk-default-avatar", UserID: 1001, Title: "默认头像轨迹", StartTime: startTime, RawTrackURL: "https://example.com/trk-default-avatar.dat", IsRunning: false, Status: models.TrackStatusNormal})

	w1 := e.perform(http.MethodGet, "/api/v1/track/recommend/list", nil, authHeader(token), ut.Header{Key: middleware.HeaderUserID, Value: "1001"})
	if w1.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected recommend status 200, got %d", w1.Result().StatusCode())
	}
	var recommend handler.StandardResponse[*models.TrackSummaryPage]
	decodeJSON(t, w1.Result().Body(), &recommend)
	if recommend.Data == nil || len(recommend.Data.Items) != 1 {
		t.Fatalf("unexpected recommend result: %+v", recommend)
	}
	if recommend.Data.Items[0].UserAvatarURL != "/api/v1/static/default_avatars/girl_01.png" {
		t.Fatalf("expected recommend default user_avatar_url, got %q", recommend.Data.Items[0].UserAvatarURL)
	}

	w2 := e.perform(http.MethodGet, "/api/v1/track/search/list?keyword=默认头像", nil, authHeader(token))
	if w2.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected search status 200, got %d", w2.Result().StatusCode())
	}
	var search handler.StandardResponse[*models.TrackSummaryPage]
	decodeJSON(t, w2.Result().Body(), &search)
	if search.Data == nil || len(search.Data.Items) != 1 {
		t.Fatalf("unexpected search result: %+v", search)
	}
	if search.Data.Items[0].UserAvatarURL != "/api/v1/static/default_avatars/girl_01.png" {
		t.Fatalf("expected search default user_avatar_url, got %q", search.Data.Items[0].UserAvatarURL)
	}
}

func TestSearchCursorPagination(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)

	start1 := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	start2 := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	start3 := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)

	_ = e.trackRepo.Create(ctx, &models.Track{ID: "search-c", UserID: 1001, Title: "西湖夜骑", StartTime: start3, AvgSpeedKmh: 7.2, RawTrackURL: "https://example.com/search-c.dat", IsRunning: false, Status: models.TrackStatusNormal})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "search-a", UserID: 1001, Title: "西湖晨跑", StartTime: start1, AvgSpeedKmh: 11.6, RawTrackURL: "https://example.com/search-a.dat", IsRunning: false, Status: models.TrackStatusNormal})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "search-b", UserID: 1002, Title: "西湖徒步", StartTime: start2, AvgSpeedKmh: 6.4, RawTrackURL: "https://example.com/search-b.dat", IsRunning: false, Status: models.TrackStatusNormal})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "other-keyword", UserID: 1002, Title: "灵隐寺", StartTime: start1, IsRunning: false, Status: models.TrackStatusNormal})

	w1 := e.perform(http.MethodGet, "/api/v1/track/search/list?keyword=西湖&limit=2", nil, authHeader(token), ut.Header{Key: middleware.HeaderUserID, Value: "1001"})
	if w1.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected first search page status 200, got %d", w1.Result().StatusCode())
	}
	var page1 handler.StandardResponse[*models.TrackSummaryPage]
	decodeJSON(t, w1.Result().Body(), &page1)
	if page1.Code != 0 || page1.Data == nil {
		t.Fatalf("unexpected first search page: %+v", page1)
	}
	if len(page1.Data.Items) != 2 {
		t.Fatalf("expected first search page size 2, got %d", len(page1.Data.Items))
	}
	if page1.Data.Items[0].ID != "search-a" || page1.Data.Items[1].ID != "search-b" {
		t.Fatalf("unexpected first search order: %+v", page1.Data.Items)
	}
	if got := page1.Data.Items[0].AvgSpeedKmh; got != 11.6 {
		t.Fatalf("expected first search item avg_speed_kmh=11.6, got %v", got)
	}
	if got := page1.Data.Items[1].AvgSpeedKmh; got != 6.4 {
		t.Fatalf("expected second search item avg_speed_kmh=6.4, got %v", got)
	}
	if !page1.Data.HasMore || page1.Data.NextCursor == "" {
		t.Fatalf("expected first search page has next cursor: %+v", page1.Data)
	}

	w2 := e.perform(http.MethodGet, "/api/v1/track/search/list?keyword=西湖&limit=2&cursor="+page1.Data.NextCursor, nil, authHeader(token), ut.Header{Key: middleware.HeaderUserID, Value: "1001"})
	if w2.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected second search page status 200, got %d", w2.Result().StatusCode())
	}
	var page2 handler.StandardResponse[*models.TrackSummaryPage]
	decodeJSON(t, w2.Result().Body(), &page2)
	if page2.Code != 0 || page2.Data == nil {
		t.Fatalf("unexpected second search page: %+v", page2)
	}
	if len(page2.Data.Items) != 1 || page2.Data.Items[0].ID != "search-c" {
		t.Fatalf("unexpected second search items: %+v", page2.Data.Items)
	}
	if got := page2.Data.Items[0].AvgSpeedKmh; got != 7.2 {
		t.Fatalf("expected second search item avg_speed_kmh=7.2, got %v", got)
	}
	if page2.Data.HasMore || page2.Data.NextCursor != "" {
		t.Fatalf("expected second search page end, got %+v", page2.Data)
	}

	w3 := e.perform(http.MethodGet, "/api/v1/track/search/list?keyword=西湖&cursor=bad-cursor", nil, authHeader(token), ut.Header{Key: middleware.HeaderUserID, Value: "1001"})
	if w3.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("expected invalid search cursor status 400, got %d", w3.Result().StatusCode())
	}
}

func TestListMyTracks_OmitsFields(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)

	start1 := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	start2 := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
	startOther := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)

	// seed tracks: 2 for user 1001 (one private), 1 条 raw_track_url 为空，1 for other user; running should be excluded.
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "my1", UserID: 1001, CityCode: "330100", TrackType: "跑步", SourceTag: "manual_seed", Title: "我的1", StartTime: start1, AvgSpeedKmh: 13.2, RawTrackURL: "https://example.com/my1.dat", IsRunning: false, Status: models.TrackStatusNormal})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "my2", UserID: 1001, CityCode: "330100", TrackType: "徒步", Title: "我的2", StartTime: start2, AvgSpeedKmh: 5.6, RawTrackURL: "https://example.com/my2.dat", IsRunning: false, Status: models.TrackStatusPrivate})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "my-empty-raw", UserID: 1001, Title: "空轨迹文件", StartTime: start2.Add(-time.Hour), IsRunning: false, Status: models.TrackStatusNormal})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "other1", UserID: 2002, Title: "别人的", StartTime: startOther, IsRunning: false, Status: models.TrackStatusNormal})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "my-running", UserID: 1001, Title: "进行中", IsRunning: true, Status: models.TrackStatusNormal})

	w := e.perform(http.MethodGet, "/api/v1/track/my/list", nil, authHeader(token))
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", resp.StatusCode(), string(resp.Body()))
	}

	var result handler.StandardResponse[*models.MyTrackSummaryPage]
	decodeJSON(t, resp.Body(), &result)
	if result.Code != 0 {
		t.Fatalf("expected code 0, got %d", result.Code)
	}
	if result.Data == nil {
		t.Fatalf("expected my list page data")
	}
	if result.Data.TotalCount != 2 {
		t.Fatalf("expected total_count=2, got %d", result.Data.TotalCount)
	}
	if len(result.Data.Items) != 2 {
		t.Fatalf("expected my list size 2, got %d", len(result.Data.Items))
	}
	var raw map[string]interface{}
	decodeJSON(t, resp.Body(), &raw)
	data := raw["data"].(map[string]interface{})
	items := data["items"].([]interface{})
	for _, item := range items {
		if _, ok := item.(map[string]interface{})["source_tag"]; ok {
			t.Fatalf("my track list should not return source_tag")
		}
	}
	if result.Data.HasMore {
		t.Fatalf("expected has_more=false")
	}
	// order should be start_time desc: my2 then my1
	if result.Data.Items[0].ID != "my2" {
		t.Fatalf("expected first id my2, got %v", result.Data.Items[0].ID)
	}
	for _, item := range result.Data.Items {
		if item.CollectCount < 0 {
			t.Fatalf("collect_count should exist")
		}
		if item.NavigateCount < 0 {
			t.Fatalf("navigate_count should exist")
		}
		if item.UserID != 1001 {
			t.Fatalf("unexpected user_id: %v", item.UserID)
		}
	}
	if got := result.Data.Items[0].AvgSpeedKmh; got != 5.6 {
		t.Fatalf("expected first my item avg_speed_kmh=5.6, got %v", got)
	}
	if got := result.Data.Items[1].AvgSpeedKmh; got != 13.2 {
		t.Fatalf("expected second my item avg_speed_kmh=13.2, got %v", got)
	}
}

func TestListMyTracksCursorPagination(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)

	start1 := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	start2 := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	start3 := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)

	_ = e.trackRepo.Create(ctx, &models.Track{ID: "my-c", UserID: 1001, Title: "第三条", StartTime: start3, EndTime: start3.Add(40 * time.Minute), AvgSpeedKmh: 8.3, RawTrackURL: "https://example.com/my-c.dat", IsRunning: false, Status: models.TrackStatusNormal})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "my-a", UserID: 1001, Title: "第一条", StartTime: start1, EndTime: start1.Add(40 * time.Minute), AvgSpeedKmh: 12.1, RawTrackURL: "https://example.com/my-a.dat", IsRunning: false, Status: models.TrackStatusNormal})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "my-b", UserID: 1001, Title: "第二条", StartTime: start2, EndTime: start2.Add(40 * time.Minute), AvgSpeedKmh: 9.9, RawTrackURL: "https://example.com/my-b.dat", IsRunning: false, Status: models.TrackStatusPrivate})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "my-empty-raw-page", UserID: 1001, Title: "不计入 total_count", StartTime: start2.Add(-time.Hour), IsRunning: false, Status: models.TrackStatusNormal})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "other-user", UserID: 1002, Title: "别人的", StartTime: start1, IsRunning: false, Status: models.TrackStatusNormal})

	w1 := e.perform(http.MethodGet, "/api/v1/track/my/list?limit=2", nil, authHeader(token))
	if w1.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected first my page status 200, got %d", w1.Result().StatusCode())
	}
	var page1 handler.StandardResponse[*models.MyTrackSummaryPage]
	decodeJSON(t, w1.Result().Body(), &page1)
	if page1.Data == nil {
		t.Fatalf("expected first my page data")
	}
	if page1.Data.TotalCount != 3 {
		t.Fatalf("expected first my page total_count=3, got %d", page1.Data.TotalCount)
	}
	if len(page1.Data.Items) != 2 {
		t.Fatalf("expected first my page size 2, got %d", len(page1.Data.Items))
	}
	if page1.Data.Items[0].ID != "my-a" || page1.Data.Items[1].ID != "my-b" {
		t.Fatalf("unexpected first my page order: %+v", page1.Data.Items)
	}
	if got := page1.Data.Items[0].EndTime.Format(time.RFC3339); got != "2026-04-23T12:40:00Z" {
		t.Fatalf("expected first my item end_time=2026-04-23T12:40:00Z, got %s", got)
	}
	if got := page1.Data.Items[1].EndTime.Format(time.RFC3339); got != "2026-04-22T12:40:00Z" {
		t.Fatalf("expected second my item end_time=2026-04-22T12:40:00Z, got %s", got)
	}
	if got := page1.Data.Items[0].AvgSpeedKmh; got != 12.1 {
		t.Fatalf("expected first my item avg_speed_kmh=12.1, got %v", got)
	}
	if got := page1.Data.Items[1].AvgSpeedKmh; got != 9.9 {
		t.Fatalf("expected second my item avg_speed_kmh=9.9, got %v", got)
	}
	if !page1.Data.HasMore {
		t.Fatalf("expected first my page has_more=true")
	}
	if page1.Data.NextCursor == "" {
		t.Fatalf("expected first my page next_cursor")
	}

	w2 := e.perform(http.MethodGet, "/api/v1/track/my/list?limit=2&cursor="+page1.Data.NextCursor, nil, authHeader(token))
	if w2.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected second my page status 200, got %d", w2.Result().StatusCode())
	}
	var page2 handler.StandardResponse[*models.MyTrackSummaryPage]
	decodeJSON(t, w2.Result().Body(), &page2)
	if page2.Data == nil {
		t.Fatalf("expected second my page data")
	}
	if page2.Data.TotalCount != 3 {
		t.Fatalf("expected second my page total_count=3, got %d", page2.Data.TotalCount)
	}
	if len(page2.Data.Items) != 1 || page2.Data.Items[0].ID != "my-c" {
		t.Fatalf("unexpected second my page items: %+v", page2.Data.Items)
	}
	if got := page2.Data.Items[0].EndTime.Format(time.RFC3339); got != "2026-04-21T12:40:00Z" {
		t.Fatalf("expected second my item end_time=2026-04-21T12:40:00Z, got %s", got)
	}
	if got := page2.Data.Items[0].AvgSpeedKmh; got != 8.3 {
		t.Fatalf("expected second my item avg_speed_kmh=8.3, got %v", got)
	}
	if page2.Data.HasMore {
		t.Fatalf("expected second my page has_more=false")
	}
	if page2.Data.NextCursor != "" {
		t.Fatalf("expected second my page next_cursor empty, got %q", page2.Data.NextCursor)
	}

	w3 := e.perform(http.MethodGet, "/api/v1/track/my/list?cursor=bad-cursor", nil, authHeader(token))
	if w3.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("expected invalid my cursor status 400, got %d", w3.Result().StatusCode())
	}
}

// TestCollectAndUncollect verifies collect related endpoints.
func TestCollectAndUncollect(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk2", UserID: 1002, Title: "黄山登顶"})

	// initial collect status
	w0 := e.perform(http.MethodGet, "/api/v1/user/1001/collect?track_id=trk2", nil, authHeader(token))
	resp0 := w0.Result()
	if resp0.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp0.StatusCode())
	}
	var status0 struct {
		Collected bool `json:"collected"`
	}
	decodeJSON(t, resp0.Body(), &status0)
	if status0.Collected {
		t.Fatalf("expected collected=false")
	}

	// collect
	w1 := e.perform(http.MethodPost, "/api/v1/track_collect?track_id=trk2", nil, authHeader(token))
	if w1.Result().StatusCode() != http.StatusOK {
		t.Fatalf("collect should succeed")
	}
	var collectResult handler.StandardResponse[handler.StatusResult]
	decodeJSON(t, w1.Result().Body(), &collectResult)
	if collectResult.Code != 0 {
		t.Fatalf("expected collect code 0, got %d", collectResult.Code)
	}
	if collectResult.Data.Status != "ok" {
		t.Fatalf("expected collect status ok, got %q", collectResult.Data.Status)
	}

	// status should be true
	w2 := e.perform(http.MethodGet, "/api/v1/user/1001/collect?track_id=trk2", nil, authHeader(token))
	var status2 struct {
		Collected bool `json:"collected"`
	}
	decodeJSON(t, w2.Result().Body(), &status2)
	if !status2.Collected {
		t.Fatalf("expected collected=true")
	}

	// uncollect
	w3 := e.perform(http.MethodDelete, "/api/v1/track_collect?track_id=trk2", nil, authHeader(token))
	if w3.Result().StatusCode() != http.StatusOK {
		t.Fatalf("uncollect should succeed")
	}
	var uncollectResult handler.StandardResponse[handler.StatusResult]
	decodeJSON(t, w3.Result().Body(), &uncollectResult)
	if uncollectResult.Code != 0 {
		t.Fatalf("expected uncollect code 0, got %d", uncollectResult.Code)
	}
	if uncollectResult.Data.Status != "ok" {
		t.Fatalf("expected uncollect status ok, got %q", uncollectResult.Data.Status)
	}
}

func TestUpdateTrackInfo_Success(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)

	_ = e.trackRepo.Create(ctx, &models.Track{
		ID:                        "trk-upd",
		UserID:                    1001,
		Title:                     "轨迹",
		Distance:                  0,
		Duration:                  10,
		ElevationGain:             0,
		RawTrackURL:               "",
		TrackScreenshotURL:        "old-ss",
		TrackNoMapBgScreenshotURL: "",
		CityCode:                  "",
		LocateAddr:                "",
		IsRunning:                 true,
		AvgSpeedKmh:               0,
		Status:                    models.TrackStatusNormal,
	})

	body, _ := json.Marshal(map[string]interface{}{
		"session_id":                     "sess_update_001",
		"city_code":                      "330100",
		"locate_addr":                    "杭州市西湖区",
		"track_type":                     "running",
		"source_tag":                     "manual_seed",
		"coordinate_system":              "BD09",
		"distance":                       200.5,
		"is_running":                     false,
		"avg_speed_kmh":                  12.3,
		"raw_track_url":                  "new-url",
		"track_screenshot_url":           "new-ss",
		"track_no_map_bg_screenshot_url": "new-no-map-bg",
		"elevation_gain":                 23,
		"duration":                       99,
	})

	w := e.perform(http.MethodPut, "/api/v1/track/trk-upd/update", body, authHeader(token), ut.Header{Key: middleware.HeaderUserID, Value: "1001"})
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", resp.StatusCode(), string(resp.Body()))
	}
	var result handler.StandardResponse[*models.Track]
	decodeJSON(t, resp.Body(), &result)
	if result.Code != 0 {
		t.Fatalf("expected code 0, got %d", result.Code)
	}
	if result.Data == nil {
		t.Fatalf("expected non-nil data")
	}
	if result.Data.ID != "trk-upd" {
		t.Fatalf("expected id trk-upd, got %q", result.Data.ID)
	}
	if result.Data.Distance != 200.5 {
		t.Fatalf("expected distance 200.5, got %v", result.Data.Distance)
	}
	if result.Data.SessionID != "sess_update_001" {
		t.Fatalf("expected session_id sess_update_001, got %q", result.Data.SessionID)
	}
	if result.Data.TrackType != "running" {
		t.Fatalf("expected track_type running, got %q", result.Data.TrackType)
	}
	if result.Data.SourceTag != "manual_seed" {
		t.Fatalf("expected source_tag manual_seed, got %q", result.Data.SourceTag)
	}
	// 只有当字段为空时才允许更新：duration 非空应保持原值。
	if result.Data.Duration != 10 {
		t.Fatalf("expected duration 10 (unchanged), got %v", result.Data.Duration)
	}
	if result.Data.ElevationGain != 23 {
		t.Fatalf("expected elevation_gain 23, got %v", result.Data.ElevationGain)
	}
	if result.Data.RawTrackURL != "new-url" {
		t.Fatalf("expected raw_track_url new-url, got %q", result.Data.RawTrackURL)
	}
	// 只有当字段为空时才允许更新：track_screenshot_url 非空应保持原值。
	if result.Data.TrackScreenshotURL != "old-ss" {
		t.Fatalf("expected track_screenshot_url old-ss (unchanged), got %q", result.Data.TrackScreenshotURL)
	}
	if result.Data.TrackNoMapBgScreenshotURL != "new-no-map-bg" {
		t.Fatalf("expected track_no_map_bg_screenshot_url new-no-map-bg, got %q", result.Data.TrackNoMapBgScreenshotURL)
	}
	if result.Data.CityCode != "330100" {
		t.Fatalf("expected city_code 330100, got %q", result.Data.CityCode)
	}
	if result.Data.LocateAddr != "杭州市西湖区" {
		t.Fatalf("expected locate_addr 杭州市西湖区, got %q", result.Data.LocateAddr)
	}
	if result.Data.CoordinateSystem != "BD09" {
		t.Fatalf("expected coordinate_system BD09, got %q", result.Data.CoordinateSystem)
	}
	// is_running 静默忽略，应保持原值。
	if !result.Data.IsRunning {
		t.Fatalf("expected is_running unchanged true")
	}
	if result.Data.AvgSpeedKmh != 12.3 {
		t.Fatalf("expected avg_speed_kmh 12.3, got %v", result.Data.AvgSpeedKmh)
	}
}

func TestUpdateTrackInfo_SourceTagDoesNotOverwrite(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)

	_ = e.trackRepo.Create(ctx, &models.Track{
		ID:        "trk-source-tag",
		UserID:    1001,
		Title:     "轨迹",
		SourceTag: "legacy_seed",
		Status:    models.TrackStatusNormal,
	})

	body, _ := json.Marshal(map[string]interface{}{
		"source_tag": "manual_seed",
	})

	w := e.perform(http.MethodPut, "/api/v1/track/trk-source-tag/update", body, authHeader(token))
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", resp.StatusCode(), string(resp.Body()))
	}
	var result handler.StandardResponse[*models.Track]
	decodeJSON(t, resp.Body(), &result)
	if result.Data == nil {
		t.Fatalf("expected non-nil data")
	}
	if result.Data.SourceTag != "legacy_seed" {
		t.Fatalf("expected source_tag unchanged legacy_seed, got %q", result.Data.SourceTag)
	}
}

func TestUpdateTrackInfo_IgnoreIsRunning(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)

	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk-upd-running", UserID: 1001, Title: "轨迹", IsRunning: true, Status: models.TrackStatusNormal})
	body, _ := json.Marshal(map[string]interface{}{"is_running": false, "distance": 1})

	w := e.perform(http.MethodPut, "/api/v1/track/trk-upd-running/update", body, authHeader(token), ut.Header{Key: middleware.HeaderUserID, Value: "1001"})
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", w.Result().StatusCode(), string(w.Result().Body()))
	}

	updated, err := e.trackRepo.FindByID(ctx, "trk-upd-running")
	if err != nil {
		t.Fatalf("find updated track failed: %v", err)
	}
	if !updated.IsRunning {
		t.Fatalf("expected is_running unchanged true")
	}
}

func TestUpdateTrackInfo_UsesJWTUserID(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	// token user_id=1001, but header carries a different X-User-ID.
	token := e.generateTestToken(1001)

	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk-upd-jwt", UserID: 1001, Title: "轨迹", Distance: 0, IsRunning: true, Status: models.TrackStatusNormal})
	body, _ := json.Marshal(map[string]interface{}{"distance": 1})

	w := e.perform(http.MethodPut, "/api/v1/track/trk-upd-jwt/update", body, authHeader(token), ut.Header{Key: middleware.HeaderUserID, Value: "1002"})
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", w.Result().StatusCode(), string(w.Result().Body()))
	}

	updated, err := e.trackRepo.FindByID(ctx, "trk-upd-jwt")
	if err != nil {
		t.Fatalf("find updated track failed: %v", err)
	}
	if updated.UserID != 1001 {
		t.Fatalf("expected track user_id 1001 (from JWT), got %d", updated.UserID)
	}
}

func TestUpdateTrackInfo_Forbidden(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)

	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk-other", UserID: 1002, Title: "他人的轨迹"})
	body, _ := json.Marshal(map[string]interface{}{"distance": 1})

	w := e.perform(http.MethodPut, "/api/v1/track/trk-other/update", body, authHeader(token), ut.Header{Key: middleware.HeaderUserID, Value: "1001"})
	if w.Result().StatusCode() != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", w.Result().StatusCode())
	}
}

func TestDeleteTrack_Success(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)

	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk-del", UserID: 1001, Title: "待删除", IsRunning: true, Status: models.TrackStatusNormal})
	_ = e.collectRepo.AddCollect(ctx, 2002, "trk-del")
	_ = e.collectRepo.AddCollect(ctx, 1001, "trk-del")

	w := e.perform(http.MethodDelete, "/api/v1/track/trk-del", nil, authHeader(token), ut.Header{Key: middleware.HeaderUserID, Value: "1001"})
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", w.Result().StatusCode(), string(w.Result().Body()))
	}
	updated, err := e.trackRepo.FindByID(ctx, "trk-del")
	if err != nil {
		t.Fatalf("find updated track failed: %v", err)
	}
	if updated.Status != models.TrackStatusDeleted {
		t.Fatalf("expected status deleted, got %d", updated.Status)
	}
	if updated.DeletedAt.IsZero() {
		t.Fatalf("expected deleted_at set")
	}
	if updated.IsRunning {
		t.Fatalf("expected is_running false")
	}
	// 删除轨迹后应同步清理收藏关系。
	if collected, _ := e.collectRepo.IsCollected(ctx, 1001, "trk-del"); collected {
		t.Fatalf("expected collect record removed for owner")
	}
	if collected, _ := e.collectRepo.IsCollected(ctx, 2002, "trk-del"); collected {
		t.Fatalf("expected collect record removed for other user")
	}
}

func TestDeleteTrack_Forbidden(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)

	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk-del-forbidden", UserID: 1002, Title: "他人的轨迹", Status: models.TrackStatusNormal})

	w := e.perform(http.MethodDelete, "/api/v1/track/trk-del-forbidden", nil, authHeader(token), ut.Header{Key: middleware.HeaderUserID, Value: "1001"})
	if w.Result().StatusCode() != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", w.Result().StatusCode())
	}
}

// TestUploadTrackCloud_ClearsRunning verifies upload_cloud marks running track as finished.
func TestUploadTrackCloud_ClearsRunning(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)
	track := &models.Track{ID: "trk-upload", UserID: 1001, Title: "待上传轨迹", IsRunning: true, Status: models.TrackStatusNormal}
	if err := e.trackRepo.Create(ctx, track); err != nil {
		t.Fatalf("seed track failed: %v", err)
	}

	w := e.perform(http.MethodPost, "/api/v1/track/trk-upload/upload_cloud", nil, authHeader(token))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Result().StatusCode())
	}

	updated, err := e.trackRepo.FindByID(ctx, "trk-upload")
	if err != nil {
		t.Fatalf("find updated track failed: %v", err)
	}
	if updated.IsRunning {
		t.Fatalf("expected uploaded track to be non-running")
	}
}

// TestCreateTrack_JWTOverridesHeader verifies JWT token user_id takes precedence over X-User-ID header.
func TestCreateTrack_JWTOverridesHeader(t *testing.T) {
	e := newTestEnv()
	token := e.generateTestToken(1001)
	w := e.perform(http.MethodPost, "/api/v1/track/create", nil, authHeader(token), ut.Header{Key: middleware.HeaderUserID, Value: "u1"})
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200 (JWT overrides invalid header), got %d", resp.StatusCode())
	}
	var result handler.StandardResponse[*models.Track]
	decodeJSON(t, resp.Body(), &result)
	if result.Code != 0 {
		t.Fatalf("expected code 0, got %d", result.Code)
	}
	if result.Data == nil {
		t.Fatalf("expected non-nil data")
	}
	if result.Data.UserID != 1001 {
		t.Fatalf("expected user_id 1001 from JWT, got %d", result.Data.UserID)
	}
}

// TestTrackHandlers_InvalidUserID verifies invalid user_id query/path values are rejected.
func TestTrackHandlers_InvalidUserID(t *testing.T) {
	e := newTestEnv()
	token := e.generateTestToken(1001)

	w1 := e.perform(http.MethodGet, "/api/v1/track/running?user_id=bad", nil, authHeader(token))
	if w1.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("invalid running query user_id should return 400")
	}

	w2 := e.perform(http.MethodGet, "/api/v1/user/bad/collect?track_id=trk2", nil, authHeader(token))
	if w2.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("invalid collect path user_id should return 400")
	}
}

func TestCollectTrack_UsesJWTUserID(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk-jwt-user", UserID: 1002, Title: "可收藏轨迹"})

	w := e.perform(http.MethodPost, "/api/v1/track_collect?track_id=trk-jwt-user", nil, authHeader(token))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Result().StatusCode())
	}

	collected, err := e.collectRepo.IsCollected(ctx, 1001, "trk-jwt-user")
	if err != nil {
		t.Fatalf("check collect status failed: %v", err)
	}
	if !collected {
		t.Fatalf("expected track to be collected by JWT user")
	}
}

func TestUserFollowFlow(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 1001, Nickname: "Alice"})
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 1002, Nickname: "Bob"})

	w := e.perform(http.MethodPost, "/api/v1/user/1002/follow", nil, authHeader(token))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected follow status 200, got %d", w.Result().StatusCode())
	}
	following, err := e.followRepo.IsFollowing(ctx, 1001, 1002)
	if err != nil {
		t.Fatalf("check follow failed: %v", err)
	}
	if !following {
		t.Fatalf("expected 1001 follows 1002")
	}

	w = e.perform(http.MethodGet, "/api/v1/user/1002/follow/status", nil, authHeader(token))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected status query 200, got %d", w.Result().StatusCode())
	}
	var statusResp handler.StandardResponse[struct {
		IsFollowing bool `json:"is_following"`
	}]
	decodeJSON(t, w.Result().Body(), &statusResp)
	if !statusResp.Data.IsFollowing {
		t.Fatalf("expected is_following=true")
	}

	w = e.perform(http.MethodGet, "/api/v1/user/1002/detail", nil, authHeader(token))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected detail status 200, got %d", w.Result().StatusCode())
	}
	var detailResp handler.StandardResponse[map[string]any]
	decodeJSON(t, w.Result().Body(), &detailResp)
	if detailResp.Data["is_self"] != false {
		t.Fatalf("expected is_self=false, got %v", detailResp.Data["is_self"])
	}
	if detailResp.Data["is_following"] != true {
		t.Fatalf("expected detail is_following=true, got %v", detailResp.Data["is_following"])
	}
	if detailResp.Data["follower_count"].(float64) != 1 {
		t.Fatalf("expected follower_count=1, got %v", detailResp.Data["follower_count"])
	}
	if _, ok := detailResp.Data["phone"]; ok {
		t.Fatalf("expected other user's phone omitted")
	}

	w = e.perform(http.MethodGet, "/api/v1/user/1001/following/list", nil, authHeader(token))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected following list 200, got %d", w.Result().StatusCode())
	}
	var listResp handler.StandardResponse[struct {
		Items []*service.UserFollowListItem `json:"items"`
	}]
	decodeJSON(t, w.Result().Body(), &listResp)
	if len(listResp.Data.Items) != 1 || listResp.Data.Items[0].ID != 1002 {
		t.Fatalf("expected following list contains 1002, got %+v", listResp.Data.Items)
	}

	w = e.perform(http.MethodDelete, "/api/v1/user/1002/follow", nil, authHeader(token))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected unfollow status 200, got %d", w.Result().StatusCode())
	}
	following, err = e.followRepo.IsFollowing(ctx, 1001, 1002)
	if err != nil {
		t.Fatalf("check follow after unfollow failed: %v", err)
	}
	if following {
		t.Fatalf("expected follow relation removed")
	}
}

func TestUserFollowSelfRejected(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 1001, Nickname: "Alice"})

	w := e.perform(http.MethodPost, "/api/v1/user/1001/follow", nil, authHeader(token))
	if w.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("expected follow self status 400, got %d", w.Result().StatusCode())
	}
}
