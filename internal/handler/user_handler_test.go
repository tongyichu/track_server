package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"trackapp-server/internal/models"
)

// TestGetUserDetail_NotFound verifies 404 is returned when user does not exist.
func TestGetUserDetail_NotFound(t *testing.T) {
	e := newTestEnv()
	w := e.perform(http.MethodGet, "/api/user/u-notfound/detail", nil)
	resp := w.Result()
	if resp.StatusCode() != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", resp.StatusCode())
	}
}

// TestGetUserDetail_Success verifies user detail can be retrieved when exists.
func TestGetUserDetail_Success(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: "u1", Nickname: "Alice"})

	w := e.perform(http.MethodGet, "/api/user/u1/detail", nil)
	resp := w.Result()
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode())
	}
	var u models.User
	decodeJSON(t, resp.Body(), &u)
	if u.Nickname != "Alice" {
		t.Fatalf("expected nickname Alice, got %s", u.Nickname)
	}
}

// TestUpdateNameAndAvatar verifies profile update endpoints.
func TestUpdateNameAndAvatar(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: "u2"})

	// update name
	namePayload, _ := json.Marshal(map[string]string{"name": "Bob"})
	w1 := e.perform(http.MethodPut, "/api/user/profile/name?user_id=u2", namePayload)
	if w1.Result().StatusCode() != http.StatusOK {
		t.Fatalf("update name should succeed")
	}

	// update avatar with invalid payload
	w2 := e.perform(http.MethodPut, "/api/user/profile/photo?user_id=u2", []byte(`{}`))
	if w2.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("invalid avatar payload should return 400")
	}

	// update avatar with valid payload
	avatarPayload, _ := json.Marshal(map[string]string{"avatar_url": "https://example.com/a.png"})
	w3 := e.perform(http.MethodPut, "/api/user/profile/photo?user_id=u2", avatarPayload)
	if w3.Result().StatusCode() != http.StatusOK {
		t.Fatalf("update avatar should succeed")
	}
}

// TestUpdateSignatureAndLanguage verifies signature and client language endpoints.
func TestUpdateSignatureAndLanguage(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: "u3"})

	sigPayload, _ := json.Marshal(map[string]string{"signature": "hello world"})
	w1 := e.perform(http.MethodPut, "/api/user/profile/signature?user_id=u3", sigPayload)
	if w1.Result().StatusCode() != http.StatusOK {
		t.Fatalf("update signature should succeed")
	}

	langPayload, _ := json.Marshal(map[string]string{"client_language": "en-US"})
	w2 := e.perform(http.MethodPut, "/api/user/profile/client_language?user_id=u3", langPayload)
	if w2.Result().StatusCode() != http.StatusOK {
		t.Fatalf("update language should succeed")
	}
}
