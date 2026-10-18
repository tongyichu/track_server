package handler_test

import (
	"context"
	"github.com/tongyichu/track_server/internal/handler"
	"github.com/tongyichu/track_server/internal/models"
	"net/http"
	"testing"
)

func TestProfileBackgroundAndGenderRoundTrip(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	_, err := e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 1001, Nickname: "Alice", Signature: "keep me"})
	if err != nil {
		t.Fatal(err)
	}
	token := e.generateTestToken(1001)
	response := e.perform(http.MethodPut, "/api/v1/user/profile/update", []byte(`{"background_url":"https://example.com/cover.jpg","gender":"female"}`), authHeader(token)).Result()
	if response.StatusCode() != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.StatusCode(), response.Body())
	}
	var update handler.StandardResponse[models.User]
	decodeJSON(t, response.Body(), &update)
	if update.Data.BackgroundURL != "https://example.com/cover.jpg" || update.Data.Gender != "female" || update.Data.Signature != "keep me" {
		t.Fatalf("unexpected update: %+v", update.Data)
	}
	response = e.perform(http.MethodGet, "/api/v1/user/1001/detail", nil, authHeader(token)).Result()
	var detail handler.StandardResponse[map[string]interface{}]
	decodeJSON(t, response.Body(), &detail)
	if detail.Data["background_url"] != "https://example.com/cover.jpg" || detail.Data["gender"] != "female" {
		t.Fatalf("unexpected detail: %v", detail.Data)
	}
	// Public profiles include the cover image, while gender remains in the self profile.
	response = e.perform(http.MethodGet, "/api/v1/user/1001/detail", nil, authHeader(e.generateTestToken(1002))).Result()
	decodeJSON(t, response.Body(), &detail)
	// Decode into a fresh map so an omitted field cannot survive from the previous response.
	var public handler.StandardResponse[map[string]interface{}]
	decodeJSON(t, response.Body(), &public)
	if _, exists := public.Data["gender"]; exists {
		t.Fatal("gender leaked to another viewer")
	}
	if public.Data["background_url"] != "https://example.com/cover.jpg" {
		t.Fatal("background missing from public profile")
	}
	response = e.perform(http.MethodPut, "/api/v1/user/profile/update", []byte(`{"background_url":"","gender":""}`), authHeader(token)).Result()
	if response.StatusCode() != http.StatusOK {
		t.Fatal("clear failed")
	}
	stored, _ := e.userRepo.FindByID(ctx, 1001)
	if stored.BackgroundURL != "" || stored.Gender != "" || stored.Nickname != "Alice" {
		t.Fatalf("unexpected stored profile: %+v", stored)
	}
}

func TestProfileRejectsInvalidNewFieldsWithoutChangingUser(t *testing.T) {
	for _, body := range []string{`{"gender":"other"}`, `{"gender":1}`, `{"background_url":"file:///tmp/photo.jpg"}`, `{"background_url":"https:///missing-host"}`, `{"background_url":" https://example.com/a.jpg"}`} {
		t.Run(body, func(t *testing.T) {
			e := newTestEnv()
			_, _ = e.userRepo.CreateIfNotExists(context.Background(), &models.User{ID: 1001, Gender: "male", BackgroundURL: "https://example.com/original.jpg"})
			response := e.perform(http.MethodPut, "/api/v1/user/profile/update", []byte(body), authHeader(e.generateTestToken(1001))).Result()
			if response.StatusCode() != http.StatusBadRequest {
				t.Fatalf("status=%d", response.StatusCode())
			}
			user, _ := e.userRepo.FindByID(context.Background(), 1001)
			if user.Gender != "male" || user.BackgroundURL != "https://example.com/original.jpg" {
				t.Fatal("invalid request changed stored data")
			}
		})
	}
}
