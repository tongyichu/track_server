package handler_test

import (
	"context"
	"testing"

	"github.com/tongyichu/track_server/internal/handler"
	"github.com/tongyichu/track_server/internal/models"
)

func TestTrackInformationIncludesCommentSummary(t *testing.T) {
	e := newTestEnv()
	defer e.close()
	seedLikes(t, e)
	root := postComment(t, e, 1001, "NO.00000001", "summary-root", "路线不错", "")
	if root.CommentCount == nil || *root.CommentCount != 1 {
		t.Fatalf("post omitted total: %+v", root)
	}
	reply := postComment(t, e, 1002, "NO.00000001", "summary-reply", "谢谢", root.ID)
	if reply.CommentCount == nil || *reply.CommentCount != 2 {
		t.Fatalf("reply omitted total: %+v", reply)
	}
	if err := e.collectRepo.AddCollect(context.Background(), 1002, "NO.00000001"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/api/v1/track/NO.00000001/detail",
		"/api/v1/track/recommend/list",
		"/api/v1/track/search/list?keyword=like",
		"/api/v1/track/my/list",
		"/api/v1/track/collected/list",
		"/api/v1/user/1002/tracks",
	} {
		resp := e.perform("GET", path, nil, authHeader(e.generateTestToken(1002))).Result()
		if resp.StatusCode() != 200 {
			t.Fatalf("%s: %d %s", path, resp.StatusCode(), resp.Body())
		}
		var result handler.StandardResponse[map[string]interface{}]
		decodeJSON(t, resp.Body(), &result)
		row := result.Data
		if entries, ok := row["items"].([]interface{}); ok {
			row = nil
			for _, entry := range entries {
				item := entry.(map[string]interface{})
				if item["id"] == "NO.00000001" {
					row = item
				}
			}
		}
		if row["comment_count"] != float64(2) || row["can_comment"] != true {
			t.Fatalf("%s omitted comment summary: %+v", path, row)
		}
	}
	resp := e.perform("DELETE", "/api/v1/track/NO.00000001/comments/"+reply.ID, nil, authHeader(e.generateTestToken(1002))).Result()
	var deletion handler.StandardResponse[map[string]interface{}]
	decodeJSON(t, resp.Body(), &deletion)
	if resp.StatusCode() != 200 || deletion.Data["comment_count"] != float64(1) {
		t.Fatalf("delete omitted updated total: %s", resp.Body())
	}
	track, _ := e.trackRepo.FindByID(context.Background(), "NO.00000001")
	track.Status = models.TrackStatusPrivate
	if err := e.trackRepo.Update(context.Background(), track); err != nil {
		t.Fatal(err)
	}
	resp = e.perform("GET", "/api/v1/track/NO.00000001/detail", nil, authHeader(e.generateTestToken(1002))).Result()
	var detail handler.StandardResponse[models.Track]
	decodeJSON(t, resp.Body(), &detail)
	if detail.Data.CanComment || detail.Data.CommentCount != 0 {
		t.Fatalf("private track exposed comments: %+v", detail.Data)
	}
	resp = e.perform("GET", "/api/v1/track/NO.00000001/comments/status", nil, authHeader(e.generateTestToken(1002))).Result()
	if resp.StatusCode() != 404 {
		t.Fatal("standalone comment status endpoint remains registered")
	}
}
