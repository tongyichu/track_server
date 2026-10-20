package handler_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/tongyichu/track_server/internal/handler"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/service"
)

func seedLikes(t *testing.T, e *testEnv) {
	t.Helper()
	ctx := context.Background()
	for _, id := range []int64{1001, 1002, 1003} {
		if _, err := e.userRepo.CreateIfNotExists(ctx, &models.User{ID: id, Nickname: fmt.Sprintf("user%d", id)}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= 3; i++ {
		if err := e.trackRepo.Create(ctx, &models.Track{ID: fmt.Sprintf("NO.%08d", i), UserID: 1002, Title: "like route", Status: models.TrackStatusNormal, StartTime: time.Now().UTC(), RawTrackURL: "https://example.test/route.json"}); err != nil {
			t.Fatal(err)
		}
	}
}
func likeRequest(t *testing.T, e *testEnv, user int64, method, path string) service.TrackLikeStatus {
	t.Helper()
	resp := e.perform(method, path, nil, authHeader(e.generateTestToken(user))).Result()
	if resp.StatusCode() != 200 {
		t.Fatalf("%s %s: status=%d body=%s", method, path, resp.StatusCode(), resp.Body())
	}
	var result handler.StandardResponse[service.TrackLikeStatus]
	decodeJSON(t, resp.Body(), &result)
	return result.Data
}
func TestTrackLikesIdempotencyViewerStateAndCounts(t *testing.T) {
	e := newTestEnv()
	seedLikes(t, e)
	path := "/api/v1/track/NO.00000001/like"
	for i := 0; i < 2; i++ {
		s := likeRequest(t, e, 1001, http.MethodPost, path)
		if !s.Liked || s.LikeCount != 1 || !s.CanLike {
			t.Fatalf("duplicate like changed count: %+v", s)
		}
	}
	s := likeRequest(t, e, 1003, http.MethodGet, path+"/status")
	if s.Liked || s.LikeCount != 1 {
		t.Fatalf("viewer state leaked: %+v", s)
	}
	s = likeRequest(t, e, 1003, http.MethodPost, path)
	if s.LikeCount != 2 {
		t.Fatal("second user must add another like")
	}
	s = likeRequest(t, e, 1002, http.MethodGet, path+"/status")
	if !s.CanLike || s.Liked || s.LikeCount != 2 {
		t.Fatalf("owner should be allowed to like their track: %+v", s)
	}
	for _, route := range []string{"/api/v1/track/NO.00000001/detail", "/api/v1/track/recommend/list", "/api/v1/user/1002/tracks"} {
		resp := e.perform(http.MethodGet, route, nil, authHeader(e.generateTestToken(1001))).Result()
		if resp.StatusCode() != 200 {
			t.Fatalf("%s status=%d", route, resp.StatusCode())
		}
		var result handler.StandardResponse[map[string]interface{}]
		decodeJSON(t, resp.Body(), &result)
		data := result.Data
		if route != "/api/v1/track/NO.00000001/detail" {
			found := false
			for _, entry := range data["items"].([]interface{}) {
				row := entry.(map[string]interface{})
				if row["id"] == "NO.00000001" {
					data = row
					found = true
					break
				}
			}
			if !found {
				t.Fatal("liked track missing from list")
			}
		}
		if data["liked"] != true || data["like_count"] != float64(2) {
			t.Fatalf("%s: missing like state: %v", route, data)
		}
	}
	for i := 0; i < 2; i++ {
		s := likeRequest(t, e, 1001, http.MethodDelete, path)
		if s.Liked || s.LikeCount != 1 {
			t.Fatalf("duplicate unlike changed count: %+v", s)
		}
	}
	resp := e.perform(http.MethodGet, "/api/v1/user/1002/detail", nil, authHeader(e.generateTestToken(1001))).Result()
	var profile handler.StandardResponse[service.UserProfileDetail]
	decodeJSON(t, resp.Body(), &profile)
	if profile.Data.ReceivedLikeCount != 1 {
		t.Fatalf("wrong received count: %d", profile.Data.ReceivedLikeCount)
	}
}

func TestLikesRejectInvalidTargetsAndUnauthorizedWrites(t *testing.T) {
	e := newTestEnv()
	seedLikes(t, e)
	ctx := context.Background()
	for _, test := range []struct {
		user   int64
		path   string
		status int
	}{{0, "/api/v1/track/NO.00000001/like", 401}, {1001, "/api/v1/track/unknown/like", 404}} {
		headers := authHeader("")
		if test.user > 0 {
			headers = authHeader(e.generateTestToken(test.user))
		}
		resp := e.perform(http.MethodPost, test.path, nil, headers).Result()
		if resp.StatusCode() != test.status {
			t.Fatalf("status=%d expected=%d", resp.StatusCode(), test.status)
		}
	}
	for index, change := range []func(*models.Track){func(t *models.Track) { t.IsRunning = true }, func(t *models.Track) { t.Status = models.TrackStatusPrivate }, func(t *models.Track) { t.Status = models.TrackStatusDeleted }, func(t *models.Track) { t.RawTrackURL = "" }} {
		track := &models.Track{ID: fmt.Sprintf("NO.%08d", index+4), UserID: 1002, Status: models.TrackStatusNormal, RawTrackURL: "https://example.test/route.json"}
		change(track)
		if err := e.trackRepo.Create(ctx, track); err != nil {
			t.Fatal(err)
		}
		resp := e.perform(http.MethodPost, "/api/v1/track/"+track.ID+"/like", nil, authHeader(e.generateTestToken(1001))).Result()
		if resp.StatusCode() != 404 {
			t.Fatalf("non-public/unuploaded route was liked: %d", resp.StatusCode())
		}
		if err := e.trackRepo.Update(ctx, track); err != nil {
			t.Fatal(err)
		}
	}
	s := likeRequest(t, e, 1001, http.MethodDelete, "/api/v1/track/unknown/like")
	if s.Liked || s.LikeCount != 0 {
		t.Fatal("unlike missing route must remain idempotent")
	}
}

func TestRestrictedAccountCannotLikeButCanRemoveLike(t *testing.T) {
	e := newTestEnv()
	defer e.close()
	seedLikes(t, e)
	path := "/api/v1/track/NO.00000001/like"
	likeRequest(t, e, 1001, http.MethodPost, path)
	if err := e.restrictionRepo.CreateAccountRestriction(context.Background(), &models.AccountRestriction{UserID: 1001, Status: models.AccountRestrictionStatusActive, Reason: "test", Operator: "test", CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	response := e.perform(http.MethodPost, path, nil, authHeader(e.generateTestToken(1001))).Result()
	if response.StatusCode() != http.StatusForbidden {
		t.Fatalf("restricted like returned %d", response.StatusCode())
	}
	status := likeRequest(t, e, 1001, http.MethodDelete, path)
	if status.Liked || status.LikeCount != 0 {
		t.Fatalf("restricted user cannot remove like: %+v", status)
	}
}

func TestSelfLikeCountsForTrackButNotReceivedLikes(t *testing.T) {
	e := newTestEnv()
	defer e.close()
	seedLikes(t, e)
	path := "/api/v1/track/NO.00000001/like"
	for i := 0; i < 2; i++ {
		status := likeRequest(t, e, 1002, http.MethodPost, path)
		if !status.Liked || !status.CanLike || status.LikeCount != 1 {
			t.Fatalf("self-like must be allowed and idempotent: %+v", status)
		}
	}
	checkReceived := func(want int64) {
		t.Helper()
		for _, viewer := range []int64{1001, 1002} {
			resp := e.perform(http.MethodGet, "/api/v1/user/1002/detail", nil, authHeader(e.generateTestToken(viewer))).Result()
			if resp.StatusCode() != 200 {
				t.Fatalf("profile status=%d", resp.StatusCode())
			}
			var profile handler.StandardResponse[service.UserProfileDetail]
			decodeJSON(t, resp.Body(), &profile)
			if profile.Data.ReceivedLikeCount != want {
				t.Fatalf("self-like counted in profile: %d want %d", profile.Data.ReceivedLikeCount, want)
			}
		}
	}
	checkReceived(0)
	resp := e.perform(http.MethodGet, "/api/v1/track/my/list", nil, authHeader(e.generateTestToken(1002))).Result()
	var ownList handler.StandardResponse[models.MyTrackSummaryPage]
	decodeJSON(t, resp.Body(), &ownList)
	found := false
	for _, track := range ownList.Data.Items {
		if track.ID == "NO.00000001" {
			found = true
			if !track.Liked || track.LikeCount != 1 {
				t.Fatalf("own list lost self-like: %+v", track)
			}
		}
	}
	if !found {
		t.Fatal("track missing from owner list")
	}
	status := likeRequest(t, e, 1001, http.MethodPost, path)
	if !status.Liked || status.LikeCount != 2 {
		t.Fatalf("self and other likes should both count for track: %+v", status)
	}
	// Self-likes on other tracks must also stay out of the profile total.
	likeRequest(t, e, 1002, http.MethodPost, "/api/v1/track/NO.00000002/like")
	checkReceived(1)
	for i := 0; i < 2; i++ {
		status = likeRequest(t, e, 1002, http.MethodDelete, path)
		if status.Liked || status.LikeCount != 1 {
			t.Fatalf("unlike own track must be idempotent: %+v", status)
		}
	}
	checkReceived(1)
	likeRequest(t, e, 1001, http.MethodDelete, path)
	checkReceived(0)
}

func TestReceivedLikeListRoutesRemoved(t *testing.T) {
	e := newTestEnv()
	defer e.close()
	seedLikes(t, e)
	for _, path := range []string{
		"/api/v1/user/1002/likes/received/list",
		"/api/v1/user/1002/likes/received/tracks",
		"/api/v1/user/1002/likes/received/tracks/NO.00000001/users",
	} {
		resp := e.perform(http.MethodGet, path, nil, authHeader(e.generateTestToken(1001))).Result()
		if resp.StatusCode() != http.StatusNotFound {
			t.Fatalf("removed route %s returned %d", path, resp.StatusCode())
		}
	}
}
