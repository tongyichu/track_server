package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tongyichu/track_server/internal/handler"
	"github.com/tongyichu/track_server/internal/models"
)

// TestGetUserDetail_NotFound verifies 404 is returned when user does not exist.
func TestGetUserDetail_NotFound(t *testing.T) {
	e := newTestEnv()
	token := e.generateTestToken(9999)
	w := e.perform(http.MethodGet, "/api/v1/user/9999/detail", nil, authHeader(token))
	resp := w.Result()
	if resp.StatusCode() != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", resp.StatusCode())
	}
}

func TestGetUserDetail_ForbiddenWhenUserIDMismatch(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 1002, Nickname: "Bob"})

	w := e.perform(http.MethodGet, "/api/v1/user/1002/detail", nil, authHeader(token))
	resp := w.Result()
	if resp.StatusCode() != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", resp.StatusCode())
	}
}

// TestGetUserDetail_Success verifies user detail can be retrieved when exists.
func TestGetUserDetail_Success(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 1001, Nickname: "Alice", Phone: "13800000000"})

	w := e.perform(http.MethodGet, "/api/v1/user/1001/detail", nil, authHeader(token))
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode())
	}
	var out handler.StandardResponse[struct {
		models.User
		TotalDistance  float64 `json:"total_distance"`
		TrackCount     int64   `json:"track_count"`
		TrackUsedCount int64   `json:"track_used_count"`
	}]
	decodeJSON(t, resp.Body(), &out)
	if out.Code != 0 {
		t.Fatalf("expected code 0, got %d", out.Code)
	}
	data := out.Data
	if data.Nickname != "Alice" {
		t.Fatalf("expected nickname Alice, got %s", data.Nickname)
	}
	if data.Phone != "13800000000" {
		t.Fatalf("expected phone 13800000000, got %s", data.Phone)
	}
	if data.ID != 1001 {
		t.Fatalf("expected user id 1001, got %d", data.ID)
	}
	if data.AvatarURL != "/api/v1/static/default_avatars/girl_01.png" {
		t.Fatalf("expected default avatar_url, got %q", data.AvatarURL)
	}
	if data.TotalDistance != 0 {
		t.Fatalf("expected total_distance 0, got %v", data.TotalDistance)
	}
	if data.TrackCount != 0 {
		t.Fatalf("expected track_count 0, got %d", data.TrackCount)
	}
	if data.TrackUsedCount != 0 {
		t.Fatalf("expected track_used_count 0, got %d", data.TrackUsedCount)
	}
}

func TestGetUserDetail_Stats(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1001)
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 1001, Nickname: "Alice"})

	start := time.Date(2026, 4, 24, 8, 0, 0, 0, time.UTC)
	// 计入统计：2 条已完成轨迹（normal/private）
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk-a", UserID: 1001, Title: "A", StartTime: start, IsRunning: false, Status: models.TrackStatusNormal, Distance: 1200})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk-b", UserID: 1001, Title: "B", StartTime: start.Add(-time.Hour), IsRunning: false, Status: models.TrackStatusPrivate, Distance: 800})
	// 不计入：进行中/删除
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk-running", UserID: 1001, Title: "R", StartTime: start.Add(-2 * time.Hour), IsRunning: true, Status: models.TrackStatusNormal, Distance: 999})
	_ = e.trackRepo.Create(ctx, &models.Track{ID: "trk-deleted", UserID: 1001, Title: "D", StartTime: start.Add(-3 * time.Hour), IsRunning: false, Status: models.TrackStatusDeleted, Distance: 999})

	// 导航使用次数：只会统计到 trk-a/trk-b（因为统计口径来自“我的轨迹”集合）
	_ = e.navigationRepo.AddNavigation(ctx, 2001, "trk-a")
	_ = e.navigationRepo.AddNavigation(ctx, 2002, "trk-a")
	_ = e.navigationRepo.AddNavigation(ctx, 2003, "trk-b")
	_ = e.navigationRepo.AddNavigation(ctx, 2004, "trk-deleted")

	w := e.perform(http.MethodGet, "/api/v1/user/1001/detail", nil, authHeader(token))
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode())
	}
	var out handler.StandardResponse[struct {
		models.User
		TotalDistance  float64 `json:"total_distance"`
		TrackCount     int64   `json:"track_count"`
		TrackUsedCount int64   `json:"track_used_count"`
	}]
	decodeJSON(t, resp.Body(), &out)
	if out.Code != 0 {
		t.Fatalf("expected code 0, got %d", out.Code)
	}
	data := out.Data
	if data.TrackCount != 2 {
		t.Fatalf("expected track_count 2, got %d", data.TrackCount)
	}
	if data.TotalDistance != 2000 {
		t.Fatalf("expected total_distance 2000, got %v", data.TotalDistance)
	}
	if data.TrackUsedCount != 3 {
		t.Fatalf("expected track_used_count 3, got %d", data.TrackUsedCount)
	}
}

// TestGetUserDetail_AvatarURLUsesStaticAsset verifies avatar_url is rewritten to local static URL.
func TestGetUserDetail_AvatarURLUsesStaticAsset(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1005)
	avatarOSSURL := "https://example-bucket.oss-cn-hangzhou.aliyuncs.com/prod/avatar/1005.png?x-oss-signature=abc"
	if err := os.WriteFile(filepath.Join(e.avatarCacheDir, "1005.png"), []byte("avatar"), 0o644); err != nil {
		t.Fatalf("seed avatar cache failed: %v", err)
	}
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 1005, Nickname: "Carol", AvatarURL: avatarOSSURL})

	w := e.perform(http.MethodGet, "/api/v1/user/1005/detail", nil, authHeader(token))
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode())
	}
	var out handler.StandardResponse[struct {
		models.User
		TotalDistance  float64 `json:"total_distance"`
		TrackCount     int64   `json:"track_count"`
		TrackUsedCount int64   `json:"track_used_count"`
	}]
	decodeJSON(t, resp.Body(), &out)
	if out.Code != 0 {
		t.Fatalf("expected code 0, got %d", out.Code)
	}
	if out.Data.AvatarURL != "/api/v1/static/avatars/1005.png" {
		t.Fatalf("expected static avatar_url, got %q", out.Data.AvatarURL)
	}
}

// TestUpdateNameAndAvatar verifies profile update endpoints.
func TestUpdateNameAndAvatar(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1002)
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 1002, AvatarURL: "https://example.com/old-avatar.png"})
	oldAvatarPath := filepath.Join(e.avatarCacheDir, "1002.png")
	if err := os.WriteFile(oldAvatarPath, []byte("old-avatar"), 0o644); err != nil {
		t.Fatalf("seed old avatar cache failed: %v", err)
	}

	// update name
	namePayload, _ := json.Marshal(map[string]string{"name": "Bob"})
	w1 := e.perform(http.MethodPut, "/api/v1/user/profile/update", namePayload, authHeader(token))
	if w1.Result().StatusCode() != http.StatusOK {
		t.Fatalf("update name should succeed")
	}

	// update avatar with invalid payload
	w2 := e.perform(http.MethodPut, "/api/v1/user/profile/update", []byte(`{}`), authHeader(token))
	if w2.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("invalid payload should return 400")
	}

	// update avatar with valid payload
	avatarPayload, _ := json.Marshal(map[string]string{"avatar_url": "https://example.com/a.png"})
	w3 := e.perform(http.MethodPut, "/api/v1/user/profile/update", avatarPayload, authHeader(token))
	if w3.Result().StatusCode() != http.StatusOK {
		t.Fatalf("update avatar should succeed")
	}
	var out handler.StandardResponse[*models.User]
	decodeJSON(t, w3.Result().Body(), &out)
	if out.Code != 0 || out.Data == nil {
		t.Fatalf("expected standard response with data")
	}
	if out.Data.AvatarURL != "/api/v1/static/avatars/1002.png" {
		t.Fatalf("expected rewritten avatar_url, got %q", out.Data.AvatarURL)
	}
	if _, err := os.Stat(oldAvatarPath); !os.IsNotExist(err) {
		t.Fatalf("expected old avatar cache removed, stat err=%v", err)
	}
}

// TestUpdateSignatureAndLanguage verifies signature and client language endpoints.
func TestUpdateSignatureAndLanguage(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1003)
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 1003})

	sigPayload, _ := json.Marshal(map[string]string{"signature": "hello world"})
	w1 := e.perform(http.MethodPut, "/api/v1/user/profile/update", sigPayload, authHeader(token))
	if w1.Result().StatusCode() != http.StatusOK {
		t.Fatalf("update signature should succeed")
	}

	langPayload, _ := json.Marshal(map[string]string{"client_language": "en-US"})
	w2 := e.perform(http.MethodPut, "/api/v1/user/profile/client_language?user_id=1003", langPayload, authHeader(token))
	if w2.Result().StatusCode() != http.StatusOK {
		t.Fatalf("update language should succeed")
	}
}

// TestUpdatePhone verifies phone endpoint.
func TestUpdatePhone(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	token := e.generateTestToken(1004)
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 1004})

	w1 := e.perform(http.MethodPut, "/api/v1/user/profile/phone?user_id=1004", []byte(`{}`), authHeader(token))
	if w1.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("invalid phone payload should return 400")
	}

	phonePayload, _ := json.Marshal(map[string]string{"phone": "13900000000"})
	w2 := e.perform(http.MethodPut, "/api/v1/user/profile/phone?user_id=1004", phonePayload, authHeader(token))
	if w2.Result().StatusCode() != http.StatusOK {
		t.Fatalf("update phone should succeed")
	}

	user, err := e.userRepo.FindByID(ctx, 1004)
	if err != nil {
		t.Fatalf("find user after phone update failed: %v", err)
	}
	if user.Phone != "13900000000" {
		t.Fatalf("expected phone 13900000000, got %s", user.Phone)
	}
}

// TestUserHandlers_InvalidUserID verifies invalid numeric user IDs are rejected.
func TestUserHandlers_InvalidUserID(t *testing.T) {
	e := newTestEnv()
	token := e.generateTestToken(1001)

	w1 := e.perform(http.MethodGet, "/api/v1/user/not-a-number/detail", nil, authHeader(token))
	if w1.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("invalid path user_id should return 400")
	}

	// profile update: empty name should be rejected
	namePayload, _ := json.Marshal(map[string]string{"name": ""})
	w2 := e.perform(http.MethodPut, "/api/v1/user/profile/update", namePayload, authHeader(token))
	if w2.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("invalid profile payload should return 400")
	}
}
