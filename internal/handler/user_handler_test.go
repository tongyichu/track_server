package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/tongyichu/track_server/internal/models"
)

// TestGetUserDetail_NotFound verifies 404 is returned when user does not exist.
func TestGetUserDetail_NotFound(t *testing.T) {
	e := newTestEnv()
	w := e.perform(http.MethodGet, "/api/v1/user/9999/detail", nil)
	resp := w.Result()
	if resp.StatusCode() != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", resp.StatusCode())
	}
}

// TestGetUserDetail_Success verifies user detail can be retrieved when exists.
func TestGetUserDetail_Success(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 1001, Nickname: "Alice", Phone: "13800000000"})

	w := e.perform(http.MethodGet, "/api/v1/user/1001/detail", nil)
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode())
	}
	var u models.User
	decodeJSON(t, resp.Body(), &u)
	if u.Nickname != "Alice" {
		t.Fatalf("expected nickname Alice, got %s", u.Nickname)
	}
	if u.Phone != "13800000000" {
		t.Fatalf("expected phone 13800000000, got %s", u.Phone)
	}
	if u.ID != 1001 {
		t.Fatalf("expected user id 1001, got %d", u.ID)
	}
}

// TestUpdateNameAndAvatar verifies profile update endpoints.
func TestUpdateNameAndAvatar(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 1002})

	// update name
	namePayload, _ := json.Marshal(map[string]string{"name": "Bob"})
	w1 := e.perform(http.MethodPut, "/api/v1/user/profile/name?user_id=1002", namePayload)
	if w1.Result().StatusCode() != http.StatusOK {
		t.Fatalf("update name should succeed")
	}

	// update avatar with invalid payload
	w2 := e.perform(http.MethodPut, "/api/v1/user/profile/photo?user_id=1002", []byte(`{}`))
	if w2.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("invalid avatar payload should return 400")
	}

	// update avatar with valid payload
	avatarPayload, _ := json.Marshal(map[string]string{"avatar_url": "https://example.com/a.png"})
	w3 := e.perform(http.MethodPut, "/api/v1/user/profile/photo?user_id=1002", avatarPayload)
	if w3.Result().StatusCode() != http.StatusOK {
		t.Fatalf("update avatar should succeed")
	}
}

// TestUpdateSignatureAndLanguage verifies signature and client language endpoints.
func TestUpdateSignatureAndLanguage(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 1003})

	sigPayload, _ := json.Marshal(map[string]string{"signature": "hello world"})
	w1 := e.perform(http.MethodPut, "/api/v1/user/profile/signature?user_id=1003", sigPayload)
	if w1.Result().StatusCode() != http.StatusOK {
		t.Fatalf("update signature should succeed")
	}

	langPayload, _ := json.Marshal(map[string]string{"client_language": "en-US"})
	w2 := e.perform(http.MethodPut, "/api/v1/user/profile/client_language?user_id=1003", langPayload)
	if w2.Result().StatusCode() != http.StatusOK {
		t.Fatalf("update language should succeed")
	}
}

// TestUpdatePhone verifies phone endpoint.
func TestUpdatePhone(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 1004})

	w1 := e.perform(http.MethodPut, "/api/v1/user/profile/phone?user_id=1004", []byte(`{}`))
	if w1.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("invalid phone payload should return 400")
	}

	phonePayload, _ := json.Marshal(map[string]string{"phone": "13900000000"})
	w2 := e.perform(http.MethodPut, "/api/v1/user/profile/phone?user_id=1004", phonePayload)
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

	w1 := e.perform(http.MethodGet, "/api/v1/user/not-a-number/detail", nil)
	if w1.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("invalid path user_id should return 400")
	}

	namePayload, _ := json.Marshal(map[string]string{"name": "Bob"})
	w2 := e.perform(http.MethodPut, "/api/v1/user/profile/name?user_id=bad", namePayload)
	if w2.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("invalid query user_id should return 400")
	}
}
