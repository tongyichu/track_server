package handler_test

import (
	"context"
	"encoding/json"
	"github.com/tongyichu/track_server/internal/handler"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/service"
	"net/url"
	"sync"
	"testing"
	"time"
)

func postComment(t *testing.T, e *testEnv, user int64, track, client, content, target string) service.CommentView {
	t.Helper()
	resp := e.perform("POST", "/api/v1/track/"+track+"/comments", commentBody(map[string]interface{}{"client_comment_id": client, "content": content, "reply_to_id": target}), authHeader(e.generateTestToken(user))).Result()
	if resp.StatusCode() != 200 {
		t.Fatalf("create comment %d: %s", resp.StatusCode(), resp.Body())
	}
	var result handler.StandardResponse[service.CommentView]
	decodeJSON(t, resp.Body(), &result)
	return result.Data
}
func commentPage(t *testing.T, e *testEnv, path string) service.CommentPage {
	t.Helper()
	resp := e.perform("GET", path, nil, authHeader(e.generateTestToken(1001))).Result()
	if resp.StatusCode() != 200 {
		t.Fatalf("list %d: %s", resp.StatusCode(), resp.Body())
	}
	var result handler.StandardResponse[service.CommentPage]
	decodeJSON(t, resp.Body(), &result)
	return result.Data
}
func deleteComment(t *testing.T, e *testEnv, user int64, id string) {
	t.Helper()
	resp := e.perform("DELETE", "/api/v1/track/NO.00000001/comments/"+id, nil, authHeader(e.generateTestToken(user))).Result()
	if resp.StatusCode() != 200 {
		t.Fatalf("delete %d: %s", resp.StatusCode(), resp.Body())
	}
}
func TestCommentsThreadPreviewQuotesAndDeletion(t *testing.T) {
	e := newTestEnv()
	defer e.close()
	seedLikes(t, e)
	base := "/api/v1/track/NO.00000001/comments"
	root := postComment(t, e, 1001, "NO.00000001", "root-001", "路线真不错😀", "")
	author := postComment(t, e, 1002, "NO.00000001", "reply-001", "谢谢分享", root.ID)
	reply := postComment(t, e, 1003, "NO.00000001", "reply-002", "同意", author.ID)
	page := commentPage(t, e, base)
	if page.TotalCount != 3 || len(page.Items) != 1 || page.Items[0].ReplyCount != 2 || page.Items[0].AuthorReply == nil || page.Items[0].AuthorReply.ID != author.ID {
		t.Fatalf("bad root page: %+v", page)
	}
	first := commentPage(t, e, base+"/"+root.ID+"/replies?limit=1")
	if !first.HasMore || first.Root.ID != root.ID || first.TotalCount != 2 || first.Items[0].ID != reply.ID || first.Items[0].Quote == nil || first.Items[0].Quote.Content != "谢谢分享" {
		t.Fatalf("bad reply page: %+v", first)
	}
	second := commentPage(t, e, base+"/"+root.ID+"/replies?limit=1&cursor="+url.QueryEscape(first.NextCursor))
	if second.HasMore || len(second.Items) != 1 || second.Items[0].ID != author.ID {
		t.Fatalf("bad second page: %+v", second)
	}
	deleteComment(t, e, 1002, author.ID)
	deleteComment(t, e, 1002, author.ID)
	page = commentPage(t, e, base)
	if page.TotalCount != 2 || page.Items[0].AuthorReply != nil {
		t.Fatal("deleted author preview leaked")
	}
	first = commentPage(t, e, base+"/"+root.ID+"/replies")
	if !first.Items[0].Quote.Deleted || first.Items[0].Quote.Content != "" {
		t.Fatal("deleted quote content leaked")
	}
	deleteComment(t, e, 1001, root.ID)
	page = commentPage(t, e, base)
	if page.TotalCount != 1 || len(page.Items) != 1 || !page.Items[0].Deleted || page.Items[0].Content != "" {
		t.Fatal("root tombstone or total incorrect")
	}
	deleteComment(t, e, 1003, reply.ID)
	page = commentPage(t, e, base)
	if page.TotalCount != 0 || len(page.Items) != 0 {
		t.Fatal("empty deleted thread still visible")
	}
}
func TestCommentIdempotencyReactionsAndPermissions(t *testing.T) {
	e := newTestEnv()
	defer e.close()
	seedLikes(t, e)
	base := "/api/v1/track/NO.00000001/comments"
	root := postComment(t, e, 1001, "NO.00000001", "CLIENT-0001", "hello", "")
	duplicate := postComment(t, e, 1001, "NO.00000001", "client-0001", "hello", "")
	if root.ID != duplicate.ID {
		t.Fatal("retry duplicated comment")
	}
	conflict := e.perform("POST", base, commentBody(map[string]interface{}{"client_comment_id": "client-0001", "content": "different"}), authHeader(e.generateTestToken(1001))).Result()
	if conflict.StatusCode() != 409 {
		t.Fatalf("conflict status=%d", conflict.StatusCode())
	}
	for _, value := range []int{1, 1, -1, -1, 0, 0, 1} {
		resp := e.perform("PUT", base+"/"+root.ID+"/reaction", commentBody(map[string]int{"reaction": value}), authHeader(e.generateTestToken(1003))).Result()
		if resp.StatusCode() != 200 {
			t.Fatal(string(resp.Body()))
		}
		var result handler.StandardResponse[service.CommentView]
		decodeJSON(t, resp.Body(), &result)
		wantLike, wantDislike := int64(0), int64(0)
		if value == 1 {
			wantLike = 1
		}
		if value == -1 {
			wantDislike = 1
		}
		if result.Data.LikeCount != wantLike || result.Data.DislikeCount != wantDislike || result.Data.Reaction != value {
			t.Fatalf("wrong reaction: %+v", result.Data)
		}
	}
	resp := e.perform("DELETE", base+"/"+root.ID, nil, authHeader(e.generateTestToken(1002))).Result()
	if resp.StatusCode() != 403 {
		t.Fatal("track owner removed someone else's comment")
	}
	if err := e.restrictionRepo.CreateAccountRestriction(context.Background(), &models.AccountRestriction{UserID: 1003, Status: models.AccountRestrictionStatusActive, Operator: "test", CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	for _, value := range []int{-1, 1} {
		resp = e.perform("PUT", base+"/"+root.ID+"/reaction", commentBody(map[string]int{"reaction": value}), authHeader(e.generateTestToken(1003))).Result()
		if resp.StatusCode() != 403 {
			t.Fatal("restricted reaction allowed")
		}
	}
	resp = e.perform("PUT", base+"/"+root.ID+"/reaction", commentBody(map[string]int{"reaction": 0}), authHeader(e.generateTestToken(1003))).Result()
	if resp.StatusCode() != 200 {
		t.Fatal("restricted cancellation blocked")
	}
	resp = e.perform("POST", base, commentBody(map[string]interface{}{"client_comment_id": "restricted", "content": "hello"}), authHeader(e.generateTestToken(1003))).Result()
	if resp.StatusCode() != 403 {
		t.Fatal("restricted comment allowed")
	}
}
func TestCommentVisibilityAndCursorScope(t *testing.T) {
	e := newTestEnv()
	defer e.close()
	seedLikes(t, e)
	ctx := context.Background()
	base := "/api/v1/track/NO.00000001/comments"
	root := postComment(t, e, 1001, "NO.00000001", "vis-root1", "one", "")
	postComment(t, e, 1001, "NO.00000001", "vis-root2", "two", "")
	page := commentPage(t, e, base+"?limit=1")
	for _, path := range []string{base + "?cursor=broken", base + "?limit=0", "/api/v1/track/NO.00000002/comments?cursor=" + url.QueryEscape(page.NextCursor), base + "/" + root.ID + "/replies?cursor=" + url.QueryEscape(page.NextCursor)} {
		resp := e.perform("GET", path, nil, authHeader(e.generateTestToken(1001))).Result()
		if resp.StatusCode() != 400 {
			t.Fatalf("%s status=%d", path, resp.StatusCode())
		}
	}
	for i, change := range []func(*models.Track){func(c *models.Track) { c.Status = models.TrackStatusPrivate }, func(c *models.Track) { c.Status = models.TrackStatusDeleted }, func(c *models.Track) { c.IsRunning = true }, func(c *models.Track) { c.RawTrackURL = "" }} {
		track, _ := e.trackRepo.FindByID(ctx, "NO.00000001")
		track.Status = models.TrackStatusNormal
		track.IsRunning = false
		track.RawTrackURL = "raw"
		change(track)
		if err := e.trackRepo.Update(ctx, track); err != nil {
			t.Fatal(err)
		}
		resp := e.perform("GET", base, nil, authHeader(e.generateTestToken(1001))).Result()
		if resp.StatusCode() != 404 {
			t.Fatalf("ineligible case %d exposed comments", i)
		}
	}
	resp := e.perform("POST", "/api/v1/track/NO.00000002/comments", commentBody(map[string]string{"client_comment_id": "cross-track", "content": "bad", "reply_to_id": root.ID}), authHeader(e.generateTestToken(1001))).Result()
	if resp.StatusCode() != 404 {
		t.Fatal("cross-track reply allowed")
	}
	deleteComment(t, e, 1001, root.ID)
}
func TestConcurrentCommentRetriesKeepSingleTotal(t *testing.T) {
	e := newTestEnv()
	defer e.close()
	seedLikes(t, e)
	ctx := context.Background()
	var wg sync.WaitGroup
	ids := make(chan string, 30)
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := e.commentSvc.Create(ctx, 1001, "NO.00000001", service.CommentInput{ClientID: "concurrent-id", Content: "hello😀"})
			if err != nil {
				t.Error(err)
				return
			}
			ids <- c.ID
		}()
	}
	wg.Wait()
	close(ids)
	first := ""
	for id := range ids {
		if first != "" && id != first {
			t.Fatal("duplicate ids")
		}
		first = id
	}
	summary, err := e.commentSvc.Page(ctx, 1001, "NO.00000001", "", "", 20)
	if err != nil || summary.TotalCount != 1 {
		t.Fatalf("total=%+v err=%v", summary, err)
	}
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func(user int64) {
			defer wg.Done()
			_, err := e.commentSvc.React(ctx, user, "NO.00000001", first, 1)
			if err != nil {
				t.Error(err)
			}
		}(int64(i + 2000))
	}
	wg.Wait()
	page, err := e.commentSvc.Page(ctx, 1001, "NO.00000001", "", "", 20)
	if err != nil || page.Items[0].LikeCount != 25 {
		t.Fatalf("lost likes: %+v %v", page, err)
	}
}

func commentBody(value interface{}) []byte { body, _ := json.Marshal(value); return body }
