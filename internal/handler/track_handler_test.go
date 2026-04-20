package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"

	"github.com/tongyichu/track_server/internal/handler"
	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

// testEnv bundles server and in-memory dependencies for HTTP tests.
type testEnv struct {
	h            *server.Hertz
	trackRepo    repository.TrackRepository
	userRepo     repository.UserRepository
	collectRepo  repository.CollectRepository
	loginLogRepo repository.LoginLogRepository
	loginSvc     *service.LoginService
}

// newTestEnv creates a fresh Hertz server wired with in-memory repositories.
func newTestEnv() *testEnv {
	trackRepo, userRepo, collectRepo, loginLogRepo := repository.NewInMemoryRepositories()
	trackSvc := service.NewTrackService(trackRepo, collectRepo)
	userSvc := service.NewUserService(userRepo)
	loginSvc := service.NewLoginService(userRepo, loginLogRepo, "", "")

	h := server.Default()
	handler.RegisterRoutes(h, handler.Deps{TrackService: trackSvc, UserService: userSvc, LoginService: loginSvc})

	return &testEnv{h: h, trackRepo: trackRepo, userRepo: userRepo, collectRepo: collectRepo, loginLogRepo: loginLogRepo, loginSvc: loginSvc}
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
	w := e.perform(http.MethodPost, "/api/v1/track/create", nil, ut.Header{Key: middleware.HeaderUserID, Value: "1001"})
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode())
	}
	var track models.Track
	decodeJSON(t, resp.Body(), &track)
	if track.UserID != 1001 {
		t.Fatalf("expected user_id 1001, got %d", track.UserID)
	}
	if track.Status != models.TrackStatusNormal {
		t.Fatalf("expected status normal, got %d", track.Status)
	}
}

// TestCreateTrack_MissingHeader verifies 400 is returned when user header is missing.
func TestCreateTrack_MissingHeader(t *testing.T) {
	e := newTestEnv()
	w := e.perform(http.MethodPost, "/api/v1/track/create", nil)
	resp := w.Result()
	if resp.StatusCode() != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode())
	}
}

// TestGetRunningTrack_Empty verifies when no running track exists, running=false is returned.
func TestGetRunningTrack_Empty(t *testing.T) {
	e := newTestEnv()
	w := e.perform(http.MethodGet, "/api/v1/track/running?user_id=1001", nil)
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode())
	}
	var payload struct {
		Running bool `json:"running"`
	}
	decodeJSON(t, resp.Body(), &payload)
	if payload.Running {
		t.Fatalf("expected running=false, got true")
	}
}

// TestGetTrackMap_NotFound verifies map endpoint returns 404 when track not exists.
func TestGetTrackMap_NotFound(t *testing.T) {
	e := newTestEnv()
	w := e.perform(http.MethodGet, "/api/v1/track/nonexist/map", nil)
	resp := w.Result()
	if resp.StatusCode() != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", resp.StatusCode())
	}
}

// TestRecommendAndSearch verifies recommend and search endpoints work with simple data.
func TestRecommendAndSearch(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	// seed one track
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk1", UserID: 1001, Title: "西湖徒步"})

	w1 := e.perform(http.MethodGet, "/api/v1/track/recommend/list", nil, ut.Header{Key: middleware.HeaderUserID, Value: "1001"})
	resp1 := w1.Result()
	if resp1.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp1.StatusCode())
	}
	var recs []models.TrackSummary
	decodeJSON(t, resp1.Body(), &recs)
	if len(recs) == 0 {
		t.Fatalf("expected non-empty recommend list")
	}

	w2 := e.perform(http.MethodGet, "/api/v1/track/search/list?keyword=西湖", nil)
	resp2 := w2.Result()
	if resp2.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp2.StatusCode())
	}
	var search []models.TrackSummary
	decodeJSON(t, resp2.Body(), &search)
	if len(search) != 1 || search[0].ID != "trk1" {
		t.Fatalf("unexpected search result: %+v", search)
	}
}

// TestCollectAndUncollect verifies collect related endpoints.
func TestCollectAndUncollect(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk2", UserID: 1002, Title: "黄山登顶"})

	// initial collect status
	w0 := e.perform(http.MethodGet, "/api/v1/user/1001/collect?track_id=trk2", nil)
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
	w1 := e.perform(http.MethodPost, "/api/v1/track_collect?user_id=1001&track_id=trk2", nil)
	if w1.Result().StatusCode() != http.StatusOK {
		t.Fatalf("collect should succeed")
	}

	// status should be true
	w2 := e.perform(http.MethodGet, "/api/v1/user/1001/collect?track_id=trk2", nil)
	var status2 struct {
		Collected bool `json:"collected"`
	}
	decodeJSON(t, w2.Result().Body(), &status2)
	if !status2.Collected {
		t.Fatalf("expected collected=true")
	}

	// uncollect
	w3 := e.perform(http.MethodDelete, "/api/v1/track_collect?user_id=1001&track_id=trk2", nil)
	if w3.Result().StatusCode() != http.StatusOK {
		t.Fatalf("uncollect should succeed")
	}
}

// TestCreateTrack_InvalidHeader verifies invalid numeric header is rejected.
func TestCreateTrack_InvalidHeader(t *testing.T) {
	e := newTestEnv()
	w := e.perform(http.MethodPost, "/api/v1/track/create", nil, ut.Header{Key: middleware.HeaderUserID, Value: "u1"})
	if w.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("invalid X-User-ID header should return 400")
	}
}

// TestTrackHandlers_InvalidUserID verifies invalid user_id query/path values are rejected.
func TestTrackHandlers_InvalidUserID(t *testing.T) {
	e := newTestEnv()

	w1 := e.perform(http.MethodGet, "/api/v1/track/running?user_id=bad", nil)
	if w1.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("invalid running query user_id should return 400")
	}

	w2 := e.perform(http.MethodGet, "/api/v1/user/bad/collect?track_id=trk2", nil)
	if w2.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("invalid collect path user_id should return 400")
	}
}
