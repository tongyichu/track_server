package handler_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/tongyichu/track_server/internal/handler"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/service"
)

func TestProfileTracksVisibilityPaginationAndViewerState(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	for _, id := range []int64{1001, 1002} {
		if _, err := e.userRepo.CreateIfNotExists(ctx, &models.User{ID: id, Nickname: fmt.Sprintf("user%d", id)}); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now().UTC().Truncate(time.Second)
	for i := 1; i <= 8; i++ {
		track := &models.Track{ID: fmt.Sprintf("NO.%08d", i), UserID: 1002, Title: "public route", RawTrackURL: "https://example.com/route.json", StartTime: start, Status: models.TrackStatusNormal}
		switch i {
		case 4:
			track.Status = models.TrackStatusPrivate
		case 5:
			track.Status = models.TrackStatusDeleted
		case 6:
			track.IsRunning = true
		case 7:
			track.RawTrackURL = " "
		case 8:
			track.UserID = 1001
		}
		if err := e.trackRepo.Create(ctx, track); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.collectRepo.AddCollect(ctx, 1001, "NO.00000003"); err != nil {
		t.Fatal(err)
	}
	get := func(path string) service.ProfileTrackPage {
		t.Helper()
		response := e.perform(http.MethodGet, path, nil, authHeader(e.generateTestToken(1001))).Result()
		if response.StatusCode() != 200 {
			t.Fatalf("status=%d body=%s", response.StatusCode(), response.Body())
		}
		var result handler.StandardResponse[service.ProfileTrackPage]
		decodeJSON(t, response.Body(), &result)
		return result.Data
	}
	first := get("/api/v1/user/1002/tracks?limit=2")
	if first.TotalCount != 3 || len(first.Items) != 2 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("unexpected first page: %+v", first)
	}
	if first.Items[0].ID != "NO.00000003" || first.Items[1].ID != "NO.00000002" {
		t.Fatal("unstable ordering")
	}
	if !first.Items[0].Collected || first.Items[1].Collected {
		t.Fatal("collected must use the viewer identity")
	}
	second := get("/api/v1/user/1002/tracks?limit=2&cursor=" + url.QueryEscape(first.NextCursor))
	if len(second.Items) != 1 || second.Items[0].ID != "NO.00000001" || second.HasMore || second.TotalCount != 3 {
		t.Fatalf("unexpected second page: %+v", second)
	}
	// Public routes keep their visibility rules even when queried by their owner.
	response := e.perform(http.MethodGet, "/api/v1/user/1002/tracks", nil, authHeader(e.generateTestToken(1002))).Result()
	var own handler.StandardResponse[service.ProfileTrackPage]
	decodeJSON(t, response.Body(), &own)
	if own.Data.TotalCount != 3 {
		t.Fatal("public endpoint exposed private tracks to owner")
	}
}

func TestProfileCollectionsScanPastInvisibleRecordsAndKeepViewerIdentity(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	for _, id := range []int64{1001, 1002, 1003} {
		if _, err := e.userRepo.CreateIfNotExists(ctx, &models.User{ID: id, Nickname: "user"}); err != nil {
			t.Fatal(err)
		}
	}
	// Collect visible items first, then enough stale/private records to fill a whole repository batch.
	for i := 1; i <= 205; i++ {
		id := fmt.Sprintf("NO.%08d", i)
		if i != 205 {
			status := models.TrackStatusPrivate
			if i <= 3 {
				status = models.TrackStatusNormal
			}
			if i == 202 {
				status = models.TrackStatusDeleted
			}
			if err := e.trackRepo.Create(ctx, &models.Track{ID: id, UserID: 1003, Title: "route", Status: status, IsRunning: i == 203, StartTime: time.Now().UTC()}); err != nil {
				t.Fatal(err)
			}
		}
		if err := e.collectRepo.AddCollect(ctx, 1002, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.collectRepo.AddCollect(ctx, 1001, "NO.00000003"); err != nil {
		t.Fatal(err)
	}
	var all []*models.TrackSummary
	cursor := ""
	for page := 0; page < 4; page++ {
		response := e.perform(http.MethodGet, "/api/v1/user/1002/collections?limit=1&cursor="+url.QueryEscape(cursor), nil, authHeader(e.generateTestToken(1001))).Result()
		if response.StatusCode() != 200 {
			t.Fatalf("status=%d body=%s", response.StatusCode(), response.Body())
		}
		var result handler.StandardResponse[service.ProfileTrackPage]
		decodeJSON(t, response.Body(), &result)
		if result.Data.TotalCount != 3 || len(result.Data.Items) != 1 {
			t.Fatalf("invisible records broke pagination: %+v", result.Data)
		}
		all = append(all, result.Data.Items...)
		if !result.Data.HasMore {
			break
		}
		cursor = result.Data.NextCursor
		if cursor == "" {
			t.Fatal("missing next cursor")
		}
	}
	if len(all) != 3 {
		t.Fatalf("got %d visible tracks", len(all))
	}
	seen := map[string]bool{}
	for _, item := range all {
		if seen[item.ID] {
			t.Fatal("duplicate track across pages")
		}
		seen[item.ID] = true
		if item.Collected != (item.ID == "NO.00000003") {
			t.Fatal("returned profile owner's collection state instead of viewer's")
		}
	}
}

func TestProfileTrackEndpointsValidateIdentityAndInputs(t *testing.T) {
	e := newTestEnv()
	if _, err := e.userRepo.CreateIfNotExists(context.Background(), &models.User{ID: 1002, Nickname: "empty profile"}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"tracks", "collections"} {
		base := "/api/v1/user/1002/" + kind
		for _, test := range []struct {
			path   string
			token  bool
			status int
		}{
			{base, false, 401},
			{"/api/v1/user/0/" + kind, true, 400},
			{"/api/v1/user/invalid/" + kind, true, 400},
			{"/api/v1/user/999999/" + kind, true, 404},
			{base + "?limit=-1", true, 400},
			{base + "?cursor=invalid", true, 400},
			{base, true, 200},
		} {
			headers := authHeader("")
			if test.token {
				headers = authHeader(e.generateTestToken(1001))
			}
			response := e.perform(http.MethodGet, test.path, nil, headers).Result()
			if response.StatusCode() != test.status {
				t.Fatalf("%s: status=%d expected=%d body=%s", test.path, response.StatusCode(), test.status, response.Body())
			}
			if test.status == 200 {
				var page handler.StandardResponse[service.ProfileTrackPage]
				decodeJSON(t, response.Body(), &page)
				if page.Data.Items == nil || len(page.Data.Items) != 0 || page.Data.TotalCount != 0 || page.Data.HasMore {
					t.Fatal("empty profile must return an empty array")
				}
			}
		}
	}
}
