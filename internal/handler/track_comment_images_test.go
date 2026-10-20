package handler_test

import (
	"bytes"
	"context"
	"fmt"
	"github.com/tongyichu/track_server/internal/handler"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/service"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"testing"
)

type commentImageObjects struct{ data []byte }

func (o *commentImageObjects) CommentUpload(_ context.Context, user int64, id string) (*service.CommentUpload, error) {
	key := fmt.Sprintf("reserved/comments/%d/%s.jpg", user, id)
	return &service.CommentUpload{ObjectKey: key, OSSURL: "https://example.test/" + key, Credential: &service.OSSTemporaryCredential{Dir: "reserved/"}}, nil
}
func (o *commentImageObjects) VerifyCommentImage(_ context.Context, i *models.CommentImage) error {
	if i.ByteSize != int64(len(o.data)) {
		return fmt.Errorf("image not uploaded")
	}
	return nil
}
func (o *commentImageObjects) DownloadCommentImage(_ int64, _ string, destination string) error {
	return os.WriteFile(destination, o.data, 0600)
}
func TestCommentImagesBindingAndVisibility(t *testing.T) {
	e := newTestEnv()
	defer e.close()
	seedLikes(t, e)
	bitmap := image.NewRGBA(image.Rect(0, 0, 2, 2))
	bitmap.Set(0, 0, color.RGBA{R: 200, A: 255})
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, bitmap, nil); err != nil {
		t.Fatal(err)
	}
	objects := &commentImageObjects{buffer.Bytes()}
	cache, err := service.NewAssetCacheService(t.TempDir(), "/api/v1/comment-images", []string{".jpg"}, ".jpg")
	if err != nil {
		t.Fatal(err)
	}
	e.commentSvc.SetImages(objects, cache)
	base := "/api/v1/track/NO.00000001/comments"
	resp := e.perform("POST", base+"/images/prepare", commentBody(map[string]interface{}{"width": 2, "height": 2, "byte_size": buffer.Len()}), authHeader(e.generateTestToken(1001))).Result()
	if resp.StatusCode() != 200 {
		t.Fatal(string(resp.Body()))
	}
	var prepared handler.StandardResponse[service.CommentUpload]
	decodeJSON(t, resp.Body(), &prepared)
	id := prepared.Data.ImageID
	resp = e.perform("POST", base, commentBody(map[string]interface{}{"client_comment_id": "two-image-comment", "image_ids": []string{id, "CI.extra"}}), authHeader(e.generateTestToken(1001))).Result()
	if resp.StatusCode() != 400 {
		t.Fatal("more than one comment image accepted", resp.StatusCode())
	}
	input := map[string]interface{}{"client_comment_id": "image-comment", "content": "", "image_ids": []string{id}}
	resp = e.perform("POST", base, commentBody(input), authHeader(e.generateTestToken(1003))).Result()
	if resp.StatusCode() != 403 {
		t.Fatal("foreign image accepted", resp.StatusCode())
	}
	resp = e.perform("POST", base, commentBody(input), authHeader(e.generateTestToken(1001))).Result()
	if resp.StatusCode() != 200 {
		t.Fatal(string(resp.Body()))
	}
	var posted handler.StandardResponse[service.CommentView]
	decodeJSON(t, resp.Body(), &posted)
	if len(posted.Data.Images) != 1 || posted.Data.Images[0].ID != id || posted.Data.Content != "" {
		t.Fatal("image-only comment was not returned")
	}
	imageURL := posted.Data.Images[0].URL
	resp = e.perform("GET", imageURL, nil, authHeader(e.generateTestToken(1003))).Result()
	if resp.StatusCode() != 200 || !bytes.Equal(resp.Body(), buffer.Bytes()) {
		t.Fatal("valid image unavailable", resp.StatusCode())
	}
	input["client_comment_id"] = "reuse-image-id"
	resp = e.perform("POST", base, commentBody(input), authHeader(e.generateTestToken(1001))).Result()
	if resp.StatusCode() != 403 {
		t.Fatal("bound image reused")
	}
	track, _ := e.trackRepo.FindByID(context.Background(), "NO.00000001")
	track.Status = models.TrackStatusPrivate
	if err := e.trackRepo.Update(context.Background(), track); err != nil {
		t.Fatal(err)
	}
	resp = e.perform("GET", imageURL, nil, authHeader(e.generateTestToken(1003))).Result()
	if resp.StatusCode() != 404 {
		t.Fatal("private image leaked")
	}
	track.Status = models.TrackStatusNormal
	if err := e.trackRepo.Update(context.Background(), track); err != nil {
		t.Fatal(err)
	}
	deleteComment(t, e, 1001, posted.Data.ID)
	resp = e.perform("GET", imageURL, nil, authHeader(e.generateTestToken(1003))).Result()
	if resp.StatusCode() != 404 {
		t.Fatal("deleted image leaked")
	}
}
