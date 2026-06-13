package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

func TestRewriteAdminStaticURL(t *testing.T) {
	got := rewriteAdminStaticURL("/api/v1/static/avatars/1001.png")
	if got != "/admin/api/static/avatars/1001.png" {
		t.Fatalf("unexpected rewritten URL: %q", got)
	}
	external := "https://example.com/avatar.png"
	if got := rewriteAdminStaticURL(external); got != external {
		t.Fatalf("external URL should not be rewritten: %q", got)
	}
}

func TestCleanAdminStaticPath(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "normal", raw: "/avatars/1001.png", want: "avatars/1001.png", ok: true},
		{name: "traversal", raw: "avatars/../avatars/1001.png", ok: false},
		{name: "empty", raw: "", ok: false},
		{name: "root", raw: "/", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := cleanAdminStaticPath(tt.raw)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("cleanAdminStaticPath(%q)=%q,%v want %q,%v", tt.raw, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestAdminDefaultAvatarPath(t *testing.T) {
	if !isAdminDefaultAvatarPath("default_avatars/girl_01.png") {
		t.Fatal("expected known default avatar path")
	}
	if isAdminDefaultAvatarPath("default_avatars/../../server.log") {
		t.Fatal("unexpected default avatar path match")
	}
}

func TestAdminFeedbackDetailRewritesImageURLAndServesImage(t *testing.T) {
	staticRoot := t.TempDir()
	imageDir := filepath.Join(staticRoot, "private_feedback", "images")
	if err := os.MkdirAll(filepath.Join(imageDir, "1001", "20260613"), 0o755); err != nil {
		t.Fatalf("mkdir image dir: %v", err)
	}
	imagePath := filepath.Join(imageDir, "1001", "20260613", "FB20260613120000ABCDEF_1.png")
	if err := os.WriteFile(imagePath, testPNGBytes(), 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	feedbackRepo := repository.NewInMemoryFeedbackRepository()
	feedbackSvc := service.NewFeedbackService(feedbackRepo, imageDir)
	feedbackID := "FB20260613120000ABCDEF"
	if err := feedbackRepo.Create(context.Background(), &models.Feedback{
		FeedbackID: feedbackID,
		UserID:     1001,
		Content:    "图片无法预览",
		Images: []models.FeedbackImage{{
			ImageID:  "1",
			MimeType: "image/png",
			Size:     int64(len(testPNGBytes())),
		}},
		Status:    models.FeedbackStatusPending,
		CreatedAt: time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("create feedback: %v", err)
	}

	h := server.Default()
	module := NewModule(map[string]string{"admin": string(placeholderPasswordHash)}, nil, nil, staticRoot, nil, nil, nil, nil, nil, feedbackSvc)
	module.RegisterRoutes(h)
	defer module.Close()

	session, err := module.Auth.store.Create("admin")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	cookie := ut.Header{Key: "Cookie", Value: sessionCookieName + "=" + session.Token}
	detailResp := ut.PerformRequest(h.Engine, http.MethodGet, "/admin/api/feedbacks/"+feedbackID, nil, cookie)
	if detailResp.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", detailResp.Code, detailResp.Body.String())
	}

	var detail struct {
		Data models.Feedback `json:"data"`
	}
	if err := json.Unmarshal(detailResp.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if len(detail.Data.Images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(detail.Data.Images))
	}
	wantURL := "/admin/api/feedbacks/" + feedbackID + "/images/1"
	if detail.Data.Images[0].URL != wantURL {
		t.Fatalf("image url=%q want %q", detail.Data.Images[0].URL, wantURL)
	}

	imageResp := ut.PerformRequest(h.Engine, http.MethodGet, detail.Data.Images[0].URL, nil, cookie)
	if imageResp.Code != http.StatusOK {
		t.Fatalf("image status=%d body=%s", imageResp.Code, imageResp.Body.String())
	}
	if got := string(imageResp.Header().ContentType()); got != "image/png" {
		t.Fatalf("content-type=%q", got)
	}
}

func TestAdminListFeedbacksFiltersByVersionAndPhone(t *testing.T) {
	userRepo := repository.NewInMemoryUserRepository()
	feedbackRepo := repository.NewInMemoryFeedbackRepository()
	feedbackSvc := service.NewFeedbackService(feedbackRepo, filepath.Join(t.TempDir(), "feedback", "images"))
	ctx := context.Background()
	_, _ = userRepo.CreateIfNotExists(ctx, &models.User{ID: 1001, Phone: "13900001001", Nickname: "user-a"})
	_, _ = userRepo.CreateIfNotExists(ctx, &models.User{ID: 1002, Phone: "13900001002", Nickname: "user-b"})

	baseTime := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	fixtures := []*models.Feedback{
		{FeedbackID: "FB20260613120000AAAAAAA1", UserID: 1001, Content: "a", AppVersion: "1.2.0", Status: models.FeedbackStatusPending, CreatedAt: baseTime.Add(2 * time.Minute), UpdatedAt: baseTime.Add(2 * time.Minute)},
		{FeedbackID: "FB20260613120000AAAAAAA2", UserID: 1002, Content: "b", AppVersion: "1.3.0", Status: models.FeedbackStatusPending, CreatedAt: baseTime.Add(time.Minute), UpdatedAt: baseTime.Add(time.Minute)},
		{FeedbackID: "FB20260613120000AAAAAAA3", UserID: 1002, Content: "c", AppVersion: "1.2.0", Status: models.FeedbackStatusResolved, CreatedAt: baseTime, UpdatedAt: baseTime},
	}
	for _, item := range fixtures {
		if err := feedbackRepo.Create(ctx, item); err != nil {
			t.Fatalf("create feedback %s: %v", item.FeedbackID, err)
		}
	}

	h := server.Default()
	module := NewModule(map[string]string{"admin": string(placeholderPasswordHash)}, nil, nil, t.TempDir(), userRepo, nil, nil, nil, nil, feedbackSvc)
	module.RegisterRoutes(h)
	defer module.Close()
	session, err := module.Auth.store.Create("admin")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	cookie := ut.Header{Key: "Cookie", Value: sessionCookieName + "=" + session.Token}

	got := performAdminFeedbackList(t, h, "/admin/api/feedbacks?app_version=1.2.0", cookie)
	if len(got.Data.Items) != 2 {
		t.Fatalf("version filter returned %d items, want 2", len(got.Data.Items))
	}

	got = performAdminFeedbackList(t, h, "/admin/api/feedbacks?phone=13900001002", cookie)
	if len(got.Data.Items) != 2 {
		t.Fatalf("phone filter returned %d items, want 2", len(got.Data.Items))
	}
	for _, item := range got.Data.Items {
		if item.UserID != 1002 {
			t.Fatalf("phone filter returned user_id=%d", item.UserID)
		}
	}

	got = performAdminFeedbackList(t, h, "/admin/api/feedbacks?phone=13900009999", cookie)
	if len(got.Data.Items) != 0 {
		t.Fatalf("missing phone returned %d items, want 0", len(got.Data.Items))
	}
}

func performAdminFeedbackList(t *testing.T, h *server.Hertz, target string, headers ...ut.Header) struct {
	Code int                 `json:"code"`
	Data models.FeedbackPage `json:"data"`
} {
	t.Helper()
	resp := ut.PerformRequest(h.Engine, http.MethodGet, target, nil, headers...)
	if resp.Code != http.StatusOK {
		t.Fatalf("%s status=%d body=%s", target, resp.Code, resp.Body.String())
	}
	var out struct {
		Code int                 `json:"code"`
		Data models.FeedbackPage `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %s: %v", target, err)
	}
	return out
}

func testPNGBytes() []byte {
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41,
		0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
		0x00, 0x03, 0x01, 0x01, 0x00, 0x18, 0xdd, 0x8d,
		0xb0, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
		0x44, 0xae, 0x42, 0x60, 0x82,
	}
}
