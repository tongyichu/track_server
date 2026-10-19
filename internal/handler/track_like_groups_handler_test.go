package handler_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/tongyichu/track_server/internal/handler"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/service"
)

func receivedGroups(t *testing.T, e *testEnv, path string) service.ReceivedLikeTrackPage {
	t.Helper()
	response := e.perform(http.MethodGet, path, nil, authHeader(e.generateTestToken(1002))).Result()
	if response.StatusCode() != 200 {
		t.Fatalf("%s: %d %s", path, response.StatusCode(), response.Body())
	}
	var page handler.StandardResponse[service.ReceivedLikeTrackPage]
	decodeJSON(t, response.Body(), &page)
	return page.Data
}
func TestReceivedLikeGroupsCollapsePopularTrackBeforePagination(t *testing.T) {
	e := newTestEnv()
	defer e.close()
	seedLikes(t, e)
	ctx := context.Background()
	for i := 0; i < 120; i++ {
		id := int64(10000 + i)
		if _, err := e.userRepo.CreateIfNotExists(ctx, &models.User{ID: id, Nickname: fmt.Sprintf("liker%d", i)}); err != nil {
			t.Fatal(err)
		}
		if err := e.likeRepo.SetLike(ctx, id, "NO.00000001", true); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.likeRepo.SetLike(ctx, 1001, "NO.00000002", true); err != nil {
		t.Fatal(err)
	}
	if err := e.likeRepo.SetLike(ctx, 1003, "NO.00000003", true); err != nil {
		t.Fatal(err)
	}
	hidden, _ := e.trackRepo.FindByID(ctx, "NO.00000003")
	hidden.Status = models.TrackStatusPrivate
	if err := e.trackRepo.Update(ctx, hidden); err != nil {
		t.Fatal(err)
	}
	// A recent self-like must neither add a group nor move that track to the top.
	likeRequest(t, e, 1002, http.MethodPost, "/api/v1/track/NO.00000001/like")
	first := receivedGroups(t, e, "/api/v1/user/1002/likes/received/tracks?limit=1")
	if first.TotalCount != 2 || len(first.Items) != 1 || !first.HasMore || first.Items[0].Track.ID != "NO.00000002" {
		t.Fatalf("groups were not formed before limit: %+v", first)
	}
	second := receivedGroups(t, e, "/api/v1/user/1002/likes/received/tracks?limit=1&cursor="+url.QueryEscape(first.NextCursor))
	if second.TotalCount != 2 || len(second.Items) != 1 || second.HasMore {
		t.Fatalf("bad second group page: %+v", second)
	}
	popular := second.Items[0]
	if popular.Track.ID != "NO.00000001" || popular.ReceivedLikeCount != 120 || popular.Track.LikeCount != 121 || !popular.Track.Liked || len(popular.RecentUsers) != 3 {
		t.Fatalf("bad popular group: %+v", popular)
	}
	for _, user := range popular.RecentUsers {
		if user.ID < 10000 || user.Nickname == "" {
			t.Fatalf("invalid recent liker: %+v", user)
		}
	}
	cursor := ""
	seen := make(map[int64]bool)
	for i := 0; i < 3; i++ {
		path := "/api/v1/user/1002/likes/received/tracks/NO.00000001/users?limit=50&cursor=" + url.QueryEscape(cursor)
		response := e.perform(http.MethodGet, path, nil, authHeader(e.generateTestToken(1001))).Result()
		if response.StatusCode() != 200 {
			t.Fatalf("likers status %d", response.StatusCode())
		}
		var page handler.StandardResponse[service.TrackLikerPage]
		decodeJSON(t, response.Body(), &page)
		if page.Data.TotalCount != 120 {
			t.Fatalf("self-like leaked into liker count: %d", page.Data.TotalCount)
		}
		for _, item := range page.Data.Items {
			if seen[item.User.ID] || item.User.ID == 1002 || item.LikedAt.IsZero() {
				t.Fatalf("invalid liker: %+v", item)
			}
			seen[item.User.ID] = true
		}
		if page.Data.HasMore != (i < 2) {
			t.Fatalf("bad has_more: %+v", page.Data)
		}
		cursor = page.Data.NextCursor
	}
	if len(seen) != 120 {
		t.Fatalf("lost likers: %d", len(seen))
	}
	// Cancel the only like on one track, then make the remaining track private.
	likeRequest(t, e, 1001, http.MethodDelete, "/api/v1/track/NO.00000002/like")
	after := receivedGroups(t, e, "/api/v1/user/1002/likes/received/tracks")
	if after.TotalCount != 1 || len(after.Items) != 1 || after.Items[0].ReceivedLikeCount != 120 {
		t.Fatalf("cancel did not update grouping: %+v", after)
	}
	hidden, _ = e.trackRepo.FindByID(ctx, "NO.00000001")
	hidden.Status = models.TrackStatusPrivate
	if err := e.trackRepo.Update(ctx, hidden); err != nil {
		t.Fatal(err)
	}
	empty := receivedGroups(t, e, "/api/v1/user/1002/likes/received/tracks")
	if empty.TotalCount != 0 || len(empty.Items) != 0 || empty.HasMore {
		t.Fatalf("private track leaked: %+v", empty)
	}
}
func TestReceivedLikeGroupValidationAndTrackVisibility(t *testing.T) {
	e := newTestEnv()
	defer e.close()
	seedLikes(t, e)
	for _, test := range []struct {
		path   string
		status int
	}{
		{"/api/v1/user/0/likes/received/tracks", 400},
		{"/api/v1/user/1002/likes/received/tracks?cursor=invalid", 400},
		{"/api/v1/user/1002/likes/received/tracks?limit=-1", 400},
		{"/api/v1/user/99999/likes/received/tracks", 404},
		{"/api/v1/user/1001/likes/received/tracks/NO.00000001/users", 404},
		{"/api/v1/user/1002/likes/received/tracks/missing/users", 404},
		{"/api/v1/user/1002/likes/received/tracks/NO.00000001/users?cursor=invalid", 400},
		{"/api/v1/user/1002/likes/received/tracks/NO.00000001/users?limit=0", 400},
	} {
		response := e.perform(http.MethodGet, test.path, nil, authHeader(e.generateTestToken(1001))).Result()
		if response.StatusCode() != test.status {
			t.Fatalf("%s status %d want %d", test.path, response.StatusCode(), test.status)
		}
	}
	response := e.perform(http.MethodGet, "/api/v1/user/1002/likes/received/tracks", nil, authHeader("")).Result()
	if response.StatusCode() != 401 {
		t.Fatal("groups must require auth")
	}
}
