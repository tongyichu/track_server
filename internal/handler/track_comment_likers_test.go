package handler_test

import (
	"context"
	"github.com/tongyichu/track_server/internal/handler"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/service"
	"net/url"
	"testing"
	"time"
)

func TestCommentLikersPaginationPermissionsAndReactions(t *testing.T) {
	e := newTestEnv()
	defer e.close()
	seedLikes(t, e)
	ctx := context.Background()
	root := postComment(t, e, 1001, "NO.00000001", "likers-root", "hello", "")
	other := postComment(t, e, 1002, "NO.00000001", "likers-other", "other", root.ID)
	base := "/api/v1/track/NO.00000001/comments/" + root.ID + "/likes"
	get := func(path string) service.CommentLikerPage {
		t.Helper()
		resp := e.perform("GET", path, nil, authHeader(e.generateTestToken(1001))).Result()
		if resp.StatusCode() != 200 {
			t.Fatalf("likers %d: %s", resp.StatusCode(), resp.Body())
		}
		var result handler.StandardResponse[service.CommentLikerPage]
		decodeJSON(t, resp.Body(), &result)
		return result.Data
	}
	empty := get(base)
	if empty.Items == nil || len(empty.Items) != 0 || empty.TotalCount != 0 || empty.HasMore {
		t.Fatalf("empty: %+v", empty)
	}
	for _, user := range []int64{1001, 1002, 1003} {
		if _, err := e.commentSvc.React(ctx, user, "NO.00000001", root.ID, 1); err != nil {
			t.Fatal(err)
		}
	}
	first := get(base + "?limit=2")
	if len(first.Items) != 2 || first.TotalCount != 3 || !first.HasMore || first.Items[0].ID != 1003 || first.Items[1].ID != 1002 || first.Items[0].Nickname == "" || first.Items[0].AvatarURL == "" {
		t.Fatalf("first: %+v", first)
	}
	second := get(base + "?limit=2&cursor=" + url.QueryEscape(first.NextCursor))
	if second.HasMore || len(second.Items) != 1 || second.Items[0].ID != 1001 {
		t.Fatalf("self like/second: %+v", second)
	}
	for _, path := range []string{base + "?cursor=broken", base + "?limit=0", base + "?limit=-1", base + "?limit=x", "/api/v1/track/NO.00000001/comments/" + other.ID + "/likes?cursor=" + url.QueryEscape(first.NextCursor)} {
		resp := e.perform("GET", path, nil, authHeader(e.generateTestToken(1001))).Result()
		if resp.StatusCode() != 400 {
			t.Fatalf("%s: %d", path, resp.StatusCode())
		}
	}
	if resp := e.perform("GET", base, nil).Result(); resp.StatusCode() != 401 {
		t.Fatal("anonymous likers allowed")
	}
	if resp := e.perform("GET", "/api/v1/track/NO.00000002/comments/"+root.ID+"/likes", nil, authHeader(e.generateTestToken(1001))).Result(); resp.StatusCode() != 404 {
		t.Fatal("cross track list allowed")
	}
	if _, err := e.commentSvc.React(ctx, 1002, "NO.00000001", root.ID, -1); err != nil {
		t.Fatal(err)
	}
	if _, err := e.commentSvc.React(ctx, 1003, "NO.00000001", root.ID, 0); err != nil {
		t.Fatal(err)
	}
	remaining := get(base)
	if remaining.TotalCount != 1 || len(remaining.Items) != 1 || remaining.Items[0].ID != 1001 {
		t.Fatalf("cancel/dislike: %+v", remaining)
	}
	if err := e.restrictionRepo.CreateAccountRestriction(ctx, &models.AccountRestriction{UserID: 1001, Status: models.AccountRestrictionStatusActive, Operator: "test", CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	get(base)
	track, _ := e.trackRepo.FindByID(ctx, "NO.00000001")
	track.Status = models.TrackStatusPrivate
	if err := e.trackRepo.Update(ctx, track); err != nil {
		t.Fatal(err)
	}
	if resp := e.perform("GET", base, nil, authHeader(e.generateTestToken(1001))).Result(); resp.StatusCode() != 404 {
		t.Fatal("private track likers leaked")
	}
	track.Status = models.TrackStatusNormal
	if err := e.trackRepo.Update(ctx, track); err != nil {
		t.Fatal(err)
	}
	deleteComment(t, e, 1001, root.ID)
	if resp := e.perform("GET", base, nil, authHeader(e.generateTestToken(1001))).Result(); resp.StatusCode() != 404 {
		t.Fatal("deleted root likers leaked")
	}
}
