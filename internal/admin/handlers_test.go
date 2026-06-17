package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	module := NewModule(map[string]string{"admin": string(placeholderPasswordHash)}, nil, nil, staticRoot, nil, nil, nil, nil, nil, nil, nil, feedbackSvc, nil)
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
	module := NewModule(map[string]string{"admin": string(placeholderPasswordHash)}, nil, nil, t.TempDir(), userRepo, nil, nil, nil, nil, nil, nil, feedbackSvc, nil)
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

func TestAdminRouteGroupOperations(t *testing.T) {
	ctx := context.Background()
	trackRepo := repository.NewInMemoryTrackRepository()
	mapRepo := repository.NewInMemoryTrackMapRepository(trackRepo)
	now := time.Now()
	index1 := adminTestGeoIndex("NO.00001001", now)
	index2 := adminTestGeoIndex("NO.00001002", now)
	index2.StartLat += 0.001
	index2.EndLat += 0.001
	if err := mapRepo.UpsertTrackGeoIndex(ctx, index1); err != nil {
		t.Fatal(err)
	}
	if err := mapRepo.UpsertTrackGeoIndex(ctx, index2); err != nil {
		t.Fatal(err)
	}
	group := &models.TrackRouteGroup{
		GroupID:               "RG.00001001",
		Name:                  "旧名称",
		TrackType:             "hiking",
		Status:                models.TrackRouteGroupStatusActive,
		CityCodes:             []string{"810000"},
		CoordinateSystem:      "GCJ02",
		CenterLat:             index1.CenterLat,
		CenterLng:             index1.CenterLng,
		RadiusM:               1000,
		MinLat:                index1.MinLat,
		MinLng:                index1.MinLng,
		MaxLat:                index1.MaxLat,
		MaxLng:                index1.MaxLng,
		Distance:              index1.Distance,
		RepresentativeTrackID: index1.TrackID,
		MemberCount:           2,
		Source:                models.TrackRouteGroupSourceAuto,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if err := mapRepo.UpsertRouteGroup(ctx, group); err != nil {
		t.Fatal(err)
	}
	for _, member := range []*models.TrackRouteGroupMember{
		{GroupID: group.GroupID, TrackID: index1.TrackID, SimilarityScore: 1, MatchDirection: models.TrackRouteGroupMemberDirectionForward, Role: models.TrackRouteGroupMemberRoleRepresentative, Source: models.TrackRouteGroupSourceAuto, CreatedAt: now, UpdatedAt: now},
		{GroupID: group.GroupID, TrackID: index2.TrackID, SimilarityScore: 0.9, MatchDirection: models.TrackRouteGroupMemberDirectionForward, Role: models.TrackRouteGroupMemberRoleMember, Source: models.TrackRouteGroupSourceAuto, CreatedAt: now, UpdatedAt: now},
	} {
		if err := mapRepo.UpsertRouteGroupMember(ctx, member); err != nil {
			t.Fatal(err)
		}
	}

	h := server.Default()
	routeGroupSvc := service.NewTrackRouteGroupService(mapRepo)
	module := NewModule(map[string]string{"admin": string(placeholderPasswordHash)}, nil, nil, t.TempDir(), nil, trackRepo, nil, mapRepo, nil, nil, nil, nil, routeGroupSvc)
	module.RegisterRoutes(h)
	defer module.Close()
	session, err := module.Auth.store.Create("admin")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	cookie := ut.Header{Key: "Cookie", Value: sessionCookieName + "=" + session.Token}
	jsonHeader := ut.Header{Key: "Content-Type", Value: "application/json"}

	resp := ut.PerformRequest(h.Engine, http.MethodGet, "/admin/api/route-groups?limit=10", nil, cookie)
	if resp.Code != http.StatusOK {
		t.Fatalf("list route groups status=%d body=%s", resp.Code, resp.Body.String())
	}
	var listOut struct {
		Data struct {
			Items []models.TrackRouteGroup `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &listOut); err != nil {
		t.Fatalf("decode route group list: %v", err)
	}
	if len(listOut.Data.Items) != 1 {
		t.Fatalf("expected 1 route group, got %d", len(listOut.Data.Items))
	}
	if listOut.Data.Items[0].RadiusM <= 0 {
		t.Fatalf("route group list should include radius_m: %+v", listOut.Data.Items[0])
	}

	renameBody := []byte(`{"name":"麦理浩径徒步路线"}`)
	resp = ut.PerformRequest(h.Engine, http.MethodPut, "/admin/api/route-groups/RG.00001001/name", &ut.Body{Body: bytes.NewBuffer(renameBody), Len: len(renameBody)}, cookie, jsonHeader)
	if resp.Code != http.StatusOK {
		t.Fatalf("rename status=%d body=%s", resp.Code, resp.Body.String())
	}
	representativeBody := []byte(`{"track_id":"NO.00001002"}`)
	resp = ut.PerformRequest(h.Engine, http.MethodPut, "/admin/api/route-groups/RG.00001001/representative", &ut.Body{Body: bytes.NewBuffer(representativeBody), Len: len(representativeBody)}, cookie, jsonHeader)
	if resp.Code != http.StatusOK {
		t.Fatalf("set representative status=%d body=%s", resp.Code, resp.Body.String())
	}
	resp = ut.PerformRequest(h.Engine, http.MethodDelete, "/admin/api/route-groups/RG.00001001/members/NO.00001001", nil, cookie)
	if resp.Code != http.StatusOK {
		t.Fatalf("remove member status=%d body=%s", resp.Code, resp.Body.String())
	}
	got, err := mapRepo.FindRouteGroup(ctx, group.GroupID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "麦理浩径徒步路线" || got.RepresentativeTrackID != "NO.00001002" || got.MemberCount != 1 {
		t.Fatalf("unexpected route group after ops: %+v", got)
	}
}

func TestAdminListTracksRewritesScreenshotURL(t *testing.T) {
	ctx := context.Background()
	trackRepo := repository.NewInMemoryTrackRepository()
	now := time.Now()
	if err := trackRepo.Create(ctx, &models.Track{
		ID:                        "NO.00003001",
		UserID:                    1001,
		Title:                     "带截图轨迹",
		TrackType:                 "hiking",
		StartTime:                 now,
		EndTime:                   now.Add(time.Hour),
		Status:                    models.TrackStatusNormal,
		TrackScreenshotURL:        "https://bucket.oss-cn-beijing.aliyuncs.com/screenshots/NO.00003001.png?signature=abc",
		TrackNoMapBgScreenshotURL: "https://bucket.oss-cn-beijing.aliyuncs.com/screenshots/NO.00003001_no_map_bg.png?signature=abc",
		RawTrackURL:               "/api/v1/static/raw_tracks/NO.00003001.json",
		CreatedAt:                 now,
		UpdatedAt:                 now,
	}); err != nil {
		t.Fatal(err)
	}
	staticRoot := t.TempDir()
	screenshotCache, err := service.NewAssetCacheService(
		filepath.Join(staticRoot, "screenshots"),
		"/api/v1/static/screenshots",
		[]string{".png", ".jpg", ".jpeg", ".webp", ".svg"},
		".png",
	)
	if err != nil {
		t.Fatalf("create screenshot cache: %v", err)
	}
	screenshotCache.SetDownloader(adminFakeDownloader{})

	h := server.Default()
	module := NewModule(map[string]string{"admin": string(placeholderPasswordHash)}, nil, nil, staticRoot, nil, trackRepo, nil, nil, nil, nil, nil, nil, nil)
	module.Handler.SetScreenshotCache(screenshotCache)
	module.RegisterRoutes(h)
	defer module.Close()
	session, err := module.Auth.store.Create("admin")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	cookie := ut.Header{Key: "Cookie", Value: sessionCookieName + "=" + session.Token}

	resp := ut.PerformRequest(h.Engine, http.MethodGet, "/admin/api/tracks?limit=20", nil, cookie)
	if resp.Code != http.StatusOK {
		t.Fatalf("list tracks status=%d body=%s", resp.Code, resp.Body.String())
	}
	var out struct {
		Data struct {
			Items []models.Track `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode tracks: %v", err)
	}
	if len(out.Data.Items) != 1 {
		t.Fatalf("expected 1 track, got %d", len(out.Data.Items))
	}
	got := out.Data.Items[0]
	if got.TrackScreenshotURL != "/admin/api/static/screenshots/NO.00003001.png" {
		t.Fatalf("unexpected screenshot url: %q", got.TrackScreenshotURL)
	}
	if got.TrackNoMapBgScreenshotURL != "/admin/api/static/screenshots/NO.00003001_no_map_bg.png" {
		t.Fatalf("unexpected no-map screenshot url: %q", got.TrackNoMapBgScreenshotURL)
	}
	if got.RawTrackURL != "/admin/api/static/raw_tracks/NO.00003001.json" {
		t.Fatalf("unexpected raw track url: %q", got.RawTrackURL)
	}
	if _, err := os.Stat(filepath.Join(staticRoot, "screenshots", "NO.00003001.png")); err != nil {
		t.Fatalf("expected cached screenshot: %v", err)
	}
}

type adminFakeDownloader struct{}

func (adminFakeDownloader) DownloadObject(_ int64, _ string, localPath string) error {
	return os.WriteFile(localPath, testPNGBytes(), 0o644)
}

func TestAdminDeleteTrackCleansRelatedData(t *testing.T) {
	ctx := context.Background()
	trackRepo := repository.NewInMemoryTrackRepository()
	collectRepo := repository.NewInMemoryCollectRepository()
	mapRepo := repository.NewInMemoryTrackMapRepository(trackRepo)
	now := time.Now()
	trackID := "NO.00002001"
	if err := trackRepo.Create(ctx, &models.Track{
		ID:        trackID,
		UserID:    1001,
		Title:     "待删除轨迹",
		TrackType: "hiking",
		StartTime: now,
		EndTime:   now.Add(time.Hour),
		Status:    models.TrackStatusNormal,
		IsRunning: false,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := collectRepo.AddCollect(ctx, 2002, trackID); err != nil {
		t.Fatal(err)
	}
	index := adminTestGeoIndex(trackID, now)
	if err := mapRepo.UpsertTrackGeoIndex(ctx, index); err != nil {
		t.Fatal(err)
	}
	group := &models.TrackRouteGroup{
		GroupID:               "RG.00002001",
		TrackType:             "hiking",
		Status:                models.TrackRouteGroupStatusActive,
		CityCodes:             []string{"810000"},
		CoordinateSystem:      "GCJ02",
		CenterLat:             index.CenterLat,
		CenterLng:             index.CenterLng,
		RadiusM:               1000,
		MinLat:                index.MinLat,
		MinLng:                index.MinLng,
		MaxLat:                index.MaxLat,
		MaxLng:                index.MaxLng,
		Distance:              index.Distance,
		RepresentativeTrackID: trackID,
		MemberCount:           1,
		Source:                models.TrackRouteGroupSourceAuto,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if err := mapRepo.UpsertRouteGroup(ctx, group); err != nil {
		t.Fatal(err)
	}
	if err := mapRepo.UpsertRouteGroupMember(ctx, &models.TrackRouteGroupMember{
		GroupID: group.GroupID, TrackID: trackID, SimilarityScore: 1,
		MatchDirection: models.TrackRouteGroupMemberDirectionForward,
		Role:           models.TrackRouteGroupMemberRoleRepresentative,
		Source:         models.TrackRouteGroupSourceAuto,
		CreatedAt:      now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	h := server.Default()
	module := NewModule(map[string]string{"admin": string(placeholderPasswordHash)}, nil, nil, t.TempDir(), nil, trackRepo, collectRepo, mapRepo, nil, nil, nil, nil, nil)
	module.RegisterRoutes(h)
	defer module.Close()
	session, err := module.Auth.store.Create("admin")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	cookie := ut.Header{Key: "Cookie", Value: sessionCookieName + "=" + session.Token}

	resp := ut.PerformRequest(h.Engine, http.MethodDelete, "/admin/api/tracks/"+trackID, nil, cookie)
	if resp.Code != http.StatusOK {
		t.Fatalf("delete track status=%d body=%s", resp.Code, resp.Body.String())
	}
	got, err := trackRepo.FindByID(ctx, trackID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != models.TrackStatusDeleted || got.IsRunning || got.DeletedAt.IsZero() {
		t.Fatalf("track was not soft deleted correctly: %+v", got)
	}
	if collected, err := collectRepo.IsCollected(ctx, 2002, trackID); err != nil || collected {
		t.Fatalf("collect cleanup collected=%v err=%v", collected, err)
	}
	if _, err := mapRepo.FindTrackGeoIndex(ctx, trackID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("geo index cleanup err=%v", err)
	}
	if _, err := mapRepo.FindRouteGroup(ctx, group.GroupID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("representative route group should be archived, err=%v", err)
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

func adminTestGeoIndex(trackID string, now time.Time) *models.TrackGeoIndex {
	points := []models.TrackPoint{
		{Index: 0, Latitude: 22.3, Longitude: 114.1},
		{Index: 1, Latitude: 22.31, Longitude: 114.11},
		{Index: 2, Latitude: 22.32, Longitude: 114.12},
	}
	return &models.TrackGeoIndex{
		TrackID:            trackID,
		UserID:             1001,
		CityCode:           "810000",
		TrackType:          "hiking",
		CoordinateSystem:   "GCJ02",
		StartLat:           points[0].Latitude,
		StartLng:           points[0].Longitude,
		EndLat:             points[2].Latitude,
		EndLng:             points[2].Longitude,
		CenterLat:          22.31,
		CenterLng:          114.11,
		MinLat:             22.3,
		MinLng:             114.1,
		MaxLat:             22.32,
		MaxLng:             114.12,
		Distance:           3000,
		PointCount:         len(points),
		SimplifiedPolyline: points,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
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
