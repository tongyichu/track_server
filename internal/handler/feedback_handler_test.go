package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"

	"github.com/tongyichu/track_server/internal/handler"
	"github.com/tongyichu/track_server/internal/models"
)

func TestFeedbackSubmitListImageAndOpsStatus(t *testing.T) {
	e := newTestEnv()
	defer e.close()
	ctx := context.Background()
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 3001, Nickname: "feedback-user"})
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 3002, Nickname: "other-user"})

	body, contentType := feedbackMultipartBody(t, "轨迹保存后封面偶现为空", [][]byte{testPNGBytes(), testPNGBytes()})
	w := e.perform(http.MethodPost, "/api/v1/feedback", body, authHeader(e.generateTestToken(3001)), ut.Header{Key: "Content-Type", Value: contentType})
	if w.Code != http.StatusOK {
		t.Fatalf("submit status=%d body=%s", w.Code, w.Body.String())
	}
	var submit handler.StandardResponse[*models.Feedback]
	decodeJSON(t, w.Body.Bytes(), &submit)
	if submit.Data == nil || submit.Data.FeedbackID == "" {
		t.Fatalf("missing feedback id: %+v", submit.Data)
	}
	if got := len(submit.Data.Images); got != 2 {
		t.Fatalf("images=%d, want 2", got)
	}
	if submit.Data.Images[0].URL == "" {
		t.Fatalf("missing image url")
	}

	w = e.perform(http.MethodGet, "/api/v1/feedback/list", nil, authHeader(e.generateTestToken(3001)))
	if w.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", w.Code, w.Body.String())
	}
	var page handler.StandardResponse[*models.FeedbackPage]
	decodeJSON(t, w.Body.Bytes(), &page)
	if page.Data == nil || len(page.Data.Items) != 1 || page.Data.Items[0].FeedbackID != submit.Data.FeedbackID {
		t.Fatalf("unexpected list page: %+v", page.Data)
	}

	imageURL := "/api/v1/feedback/" + submit.Data.FeedbackID + "/images/1"
	w = e.perform(http.MethodGet, imageURL, nil, authHeader(e.generateTestToken(3001)))
	if w.Code != http.StatusOK {
		t.Fatalf("image status=%d body=%s", w.Code, w.Body.String())
	}
	if ct := string(w.Result().Header.Get("Content-Type")); ct != "image/png" {
		t.Fatalf("content-type=%q, want image/png", ct)
	}

	w = e.perform(http.MethodGet, imageURL, nil, authHeader(e.generateTestToken(3002)))
	if w.Code != http.StatusForbidden {
		t.Fatalf("other user image status=%d body=%s", w.Code, w.Body.String())
	}

	opsBody := []byte(`{"status":"processing","reply":"已记录"}`)
	w = e.perform(http.MethodPut, "/api/v1/ops/feedback/"+submit.Data.FeedbackID+"/status", opsBody, ut.Header{Key: "X-Internal-Token", Value: e.internalToken})
	if w.Code != http.StatusOK {
		t.Fatalf("ops status update=%d body=%s", w.Code, w.Body.String())
	}
	w = e.perform(http.MethodGet, "/api/v1/ops/feedback/"+submit.Data.FeedbackID, nil, ut.Header{Key: "X-Internal-Token", Value: e.internalToken})
	if w.Code != http.StatusOK {
		t.Fatalf("ops get=%d body=%s", w.Code, w.Body.String())
	}
	var detail handler.StandardResponse[*models.Feedback]
	decodeJSON(t, w.Body.Bytes(), &detail)
	if detail.Data.Status != models.FeedbackStatusProcessing || detail.Data.Reply != "已记录" {
		b, _ := json.Marshal(detail.Data)
		t.Fatalf("unexpected ops detail: %s", b)
	}

	opsBody = []byte(`{"status":"resolved"}`)
	w = e.perform(http.MethodPut, "/api/v1/ops/feedback/"+submit.Data.FeedbackID+"/status", opsBody, ut.Header{Key: "X-Internal-Token", Value: e.internalToken})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("resolved without reply status=%d body=%s", w.Code, w.Body.String())
	}

	opsBody = []byte(`{"status":"resolved","reply":"问题已修复，请更新后重试"}`)
	w = e.perform(http.MethodPut, "/api/v1/ops/feedback/"+submit.Data.FeedbackID+"/status", opsBody, ut.Header{Key: "X-Internal-Token", Value: e.internalToken})
	if w.Code != http.StatusOK {
		t.Fatalf("ops resolve status=%d body=%s", w.Code, w.Body.String())
	}
	w = e.perform(http.MethodGet, "/api/v1/feedback/"+submit.Data.FeedbackID, nil, authHeader(e.generateTestToken(3001)))
	if w.Code != http.StatusOK {
		t.Fatalf("user detail after resolve status=%d body=%s", w.Code, w.Body.String())
	}
	var userDetail handler.StandardResponse[*models.Feedback]
	decodeJSON(t, w.Body.Bytes(), &userDetail)
	if userDetail.Data.Status != models.FeedbackStatusResolved || userDetail.Data.Reply != "问题已修复，请更新后重试" {
		b, _ := json.Marshal(userDetail.Data)
		t.Fatalf("user cannot see resolved reply: %s", b)
	}
}

func TestFeedbackRejectsTooManyImages(t *testing.T) {
	e := newTestEnv()
	defer e.close()
	ctx := context.Background()
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 3101, Nickname: "feedback-user"})

	body, contentType := feedbackMultipartBody(t, "图片太多", [][]byte{testPNGBytes(), testPNGBytes(), testPNGBytes(), testPNGBytes()})
	w := e.perform(http.MethodPost, "/api/v1/feedback", body, authHeader(e.generateTestToken(3101)), ut.Header{Key: "Content-Type", Value: contentType})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestFeedbackRejectsWhenOpenFeedbackLimitExceeded(t *testing.T) {
	e := newTestEnv()
	defer e.close()
	ctx := context.Background()
	_, _ = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 3201, Nickname: "feedback-user"})
	token := e.generateTestToken(3201)

	var firstID string
	for i := 0; i < 5; i++ {
		body, contentType := feedbackMultipartBody(t, "反馈内容", nil)
		w := e.perform(http.MethodPost, "/api/v1/feedback", body, authHeader(token), ut.Header{Key: "Content-Type", Value: contentType})
		if w.Code != http.StatusOK {
			t.Fatalf("submit %d status=%d body=%s", i+1, w.Code, w.Body.String())
		}
		if i == 0 {
			var resp handler.StandardResponse[*models.Feedback]
			decodeJSON(t, w.Body.Bytes(), &resp)
			firstID = resp.Data.FeedbackID
		}
	}

	body, contentType := feedbackMultipartBody(t, "第六条反馈", nil)
	w := e.perform(http.MethodPost, "/api/v1/feedback", body, authHeader(token), ut.Header{Key: "Content-Type", Value: contentType})
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	opsBody := []byte(`{"status":"resolved","reply":"已处理"}`)
	w = e.perform(http.MethodPut, "/api/v1/ops/feedback/"+firstID+"/status", opsBody, ut.Header{Key: "X-Internal-Token", Value: e.internalToken})
	if w.Code != http.StatusOK {
		t.Fatalf("ops resolve status=%d body=%s", w.Code, w.Body.String())
	}

	body, contentType = feedbackMultipartBody(t, "处理后一条新反馈", nil)
	w = e.perform(http.MethodPost, "/api/v1/feedback", body, authHeader(token), ut.Header{Key: "Content-Type", Value: contentType})
	if w.Code != http.StatusOK {
		t.Fatalf("submit after resolve status=%d body=%s", w.Code, w.Body.String())
	}
}

func feedbackMultipartBody(t *testing.T, content string, images [][]byte) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if err := writer.WriteField("content", content); err != nil {
		t.Fatal(err)
	}
	for i, image := range images {
		part, err := writer.CreateFormFile("images", "image.png")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(image); err != nil {
			t.Fatal(err)
		}
		_ = i
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), writer.FormDataContentType()
}

func testPNGBytes() []byte {
	return []byte{
		0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n',
		0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00,
	}
}
