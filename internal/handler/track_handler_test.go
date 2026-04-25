package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
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

// testEnv bundles server and in-memory dependencies for HTTP tests.
type testEnv struct {
	h              *server.Hertz
	trackRepo      repository.TrackRepository
	userRepo       repository.UserRepository
	collectRepo    repository.CollectRepository
	loginLogRepo   repository.LoginLogRepository
	loginSvc       *service.LoginService
	tokenBlacklist *middleware.TokenBlacklist
	staticRoot     string
	avatarCacheDir string
}

// newTestEnv creates a fresh Hertz server wired with in-memory repositories.
func newTestEnv() *testEnv {
	trackRepo, userRepo, collectRepo, loginLogRepo := repository.NewInMemoryRepositories()
	trackSvc := service.NewTrackService(trackRepo, collectRepo)
	trackSvc.SetUserRepository(userRepo)
	userSvc := service.NewUserService(userRepo)
	loginSvc := service.NewLoginService(userRepo, loginLogRepo, "", "", testJWTSecret)
	tokenBlacklist := middleware.NewTokenBlacklist()
	staticRoot, _ := os.MkdirTemp("", "track_server_test_static_")
	avatarCacheDir := filepath.Join(staticRoot, "avatars")
	avatarCache, err := service.NewAssetCacheService(
		avatarCacheDir,
		"/api/v1/static/avatars",
		[]string{".png", ".jpg", ".jpeg", ".webp"},
		".png",
	)
	if err == nil {
		userSvc.SetAvatarCache(avatarCache)
	}

	h := server.Default()
	handler.RegisterRoutes(h, handler.Deps{
		TrackService:   trackSvc,
		UserService:    userSvc,
		LoginService:   loginSvc,
		JWTSecret:      testJWTSecret,
		TokenBlacklist: tokenBlacklist,
		StaticRoot:     staticRoot,
	})

	return &testEnv{h: h, trackRepo: trackRepo, userRepo: userRepo, collectRepo: collectRepo, loginLogRepo: loginLogRepo, loginSvc: loginSvc, tokenBlacklist: tokenBlacklist, staticRoot: staticRoot, avatarCacheDir: avatarCacheDir}
}

func (e *testEnv) generateTestToken(userID int64) string {
	claims := jwtlib.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iat":     time.Now().Unix(),
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

func TestCreateTrack_WithBody(t *testing.T) {
	e := newTestEnv()
	token := e.generateTestToken(1001)
	body, _ := json.Marshal(map[string]interface{}{
		"title":                "傍晚夜跑",
		"city_code":            "330100",
		"track_type":           "跑步",
		"start_time":           "2026-04-20T12:00:00Z",
		"end_time":             "2026-04-20T12:30:00Z",
		"distance":             1500.5,
		"duration":             1800,
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
	if result.Data.TrackType != "跑步" {
		t.Fatalf("unexpected track_type: %q", result.Data.TrackType)
	}
	if result.Data.Duration != 1800 {
		t.Fatalf("expected duration 1800, got %v", result.Data.Duration)
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

	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk-detail", UserID: 1001, Title: "详情轨迹", IsRunning: false, Status: models.TrackStatusNormal})

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
}

// TestRecommendAndSearch verifies recommend and search endpoints work with simple data.
func TestRecommendAndSearch(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)
	startTime := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 1001, Nickname: "Alice", AvatarURL: "https://example.com/avatar.png"})
	// seed one normal track and one running track (running should be excluded from recommend list)
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk1", UserID: 1001, CityCode: "330100", TrackType: "徒步", Title: "西湖徒步", StartTime: startTime, IsRunning: false})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk-running", UserID: 1001, Title: "进行中跑步", IsRunning: true})
	_ = e.collectRepo.AddCollect(ctx, 1001, "trk1")

	w1 := e.perform(http.MethodGet, "/api/v1/track/recommend/list", nil, authHeader(token), ut.Header{Key: middleware.HeaderUserID, Value: "1001"})
	resp1 := w1.Result()
	if resp1.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp1.StatusCode())
	}
	var result handler.StandardResponse[[]models.TrackSummary]
	decodeJSON(t, resp1.Body(), &result)
	if result.Code != 0 {
		t.Fatalf("expected code 0, got %d", result.Code)
	}
	if len(result.Data) != 1 {
		t.Fatalf("expected recommend list size 1 (exclude running), got %d", len(result.Data))
	}
	if result.Data[0].ID != "trk1" {
		t.Fatalf("expected recommend track id trk1, got %q", result.Data[0].ID)
	}
	if !result.Data[0].Collected {
		t.Fatalf("expected recommend track trk1 collected=true")
	}
	if result.Data[0].CollectCount != 1 {
		t.Fatalf("expected recommend track trk1 collect_count=1, got %d", result.Data[0].CollectCount)
	}
	if result.Data[0].UserAvatarURL != "https://example.com/avatar.png" {
		t.Fatalf("expected recommend track trk1 user_avatar_url set, got %q", result.Data[0].UserAvatarURL)
	}
	if result.Data[0].Nickname != "Alice" {
		t.Fatalf("expected recommend track trk1 nickname=Alice, got %q", result.Data[0].Nickname)
	}
	if result.Data[0].CityCode != "330100" {
		t.Fatalf("expected recommend track trk1 city_code=330100, got %q", result.Data[0].CityCode)
	}
	if result.Data[0].CityName != "杭州市" {
		t.Fatalf("expected recommend track trk1 city_name=杭州市, got %q", result.Data[0].CityName)
	}
	if result.Data[0].TrackType != "徒步" {
		t.Fatalf("expected recommend track trk1 track_type=徒步, got %q", result.Data[0].TrackType)
	}
	if got := result.Data[0].StartTime.Format(time.RFC3339); got != "2026-04-20T12:00:00Z" {
		t.Fatalf("expected recommend track trk1 start_time=2026-04-20T12:00:00Z, got %s", got)
	}

	w2 := e.perform(http.MethodGet, "/api/v1/track/search/list?keyword=西湖", nil, authHeader(token))
	resp2 := w2.Result()
	if resp2.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp2.StatusCode())
	}
	var search []models.TrackSummary
	decodeJSON(t, resp2.Body(), &search)
	if len(search) != 1 || search[0].ID != "trk1" {
		t.Fatalf("unexpected search result: %+v", search)
	}
	if search[0].TrackType != "徒步" {
		t.Fatalf("expected search track trk1 track_type=徒步, got %q", search[0].TrackType)
	}
	if got := search[0].StartTime.Format(time.RFC3339); got != "2026-04-20T12:00:00Z" {
		t.Fatalf("expected search track trk1 start_time=2026-04-20T12:00:00Z, got %s", got)
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
		ID:                 "trk-upd",
		UserID:             1001,
		Title:              "轨迹",
		Distance:           100,
		Duration:           10,
		ElevationGain:      1,
		RawTrackURL:        "old-url",
		TrackScreenshotURL: "old-ss",
		IsRunning:          true,
		AvgSpeedKmh:        1.2,
		Status:             models.TrackStatusNormal,
	})

	body, _ := json.Marshal(map[string]interface{}{
		"distance":       200.5,
		"is_running":     false,
		"avg_speed_kmh":  12.3,
		"raw_track_url":  "new-url",
		"screenshot_url": "new-ss",
		"elevation_gain": 23,
		"duration":       99,
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
	if result.Data.Duration != 99 {
		t.Fatalf("expected duration 99, got %v", result.Data.Duration)
	}
	if result.Data.ElevationGain != 23 {
		t.Fatalf("expected elevation_gain 23, got %v", result.Data.ElevationGain)
	}
	if result.Data.RawTrackURL != "new-url" {
		t.Fatalf("expected raw_track_url new-url, got %q", result.Data.RawTrackURL)
	}
	if result.Data.TrackScreenshotURL != "new-ss" {
		t.Fatalf("expected screenshot_url new-ss, got %q", result.Data.TrackScreenshotURL)
	}
	if result.Data.IsRunning {
		t.Fatalf("expected is_running false")
	}
	if result.Data.AvgSpeedKmh != 12.3 {
		t.Fatalf("expected avg_speed_kmh 12.3, got %v", result.Data.AvgSpeedKmh)
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
