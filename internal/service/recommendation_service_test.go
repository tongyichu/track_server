package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type unavailableRecommendationRepository struct {
	repository.RecommendationRepository
}

func (r unavailableRecommendationRepository) GetFeedSession(context.Context, string) (*models.RecommendationFeedSession, error) {
	return nil, errors.New("temporary database failure")
}

func TestRecommendationServiceCreatesStableCityFeed(t *testing.T) {
	ctx := context.Background()
	trackRepo := repository.NewInMemoryTrackRepository()
	collectRepo := repository.NewInMemoryCollectRepository()
	navigationRepo := repository.NewInMemoryNavigationRepository()
	userRepo := repository.NewInMemoryUserRepository()
	followRepo := repository.NewInMemoryFollowRepository()
	recommendRepo := repository.NewInMemoryRecommendationRepository()
	trackSvc := NewTrackService(trackRepo, collectRepo)
	trackSvc.SetUserRepository(userRepo)
	trackSvc.SetNavigationRepository(navigationRepo)
	now := time.Now()
	tracks := []*models.Track{
		{ID: "NO.00000001", UserID: 1, CityCode: "330100", TrackType: "hiking", Title: "own", RawTrackURL: "oss://own", StartTime: now, Status: models.TrackStatusNormal},
		{ID: "NO.00000002", UserID: 2, CityCode: "330100", TrackType: "hiking", Title: "a", RawTrackURL: "oss://a", TrackScreenshotURL: "oss://a.png", StartTime: now.Add(-time.Minute), Status: models.TrackStatusNormal},
		{ID: "NO.00000003", UserID: 3, CityCode: "330100", TrackType: "running", Title: "b", RawTrackURL: "oss://b", StartTime: now.Add(-2 * time.Minute), Status: models.TrackStatusNormal},
		{ID: "NO.00000004", UserID: 4, CityCode: "330100", TrackType: "riding", Title: "c", RawTrackURL: "oss://c", StartTime: now.Add(-3 * time.Minute), Status: models.TrackStatusNormal},
		{ID: "NO.00000005", UserID: 5, CityCode: "110100", TrackType: "hiking", Title: "other city", RawTrackURL: "oss://d", StartTime: now.Add(-4 * time.Minute), Status: models.TrackStatusNormal},
	}
	for _, track := range tracks {
		if err := trackRepo.Create(ctx, track); err != nil {
			t.Fatal(err)
		}
	}
	stats := make([]*models.RecommendationItemStats, 0, len(tracks))
	for index, track := range tracks {
		stats = append(stats, &models.RecommendationItemStats{TrackID: track.ID, StatDate: now, CollectCount: int64(index), UpdatedAt: now})
	}
	if err := recommendRepo.UpsertItemStats(ctx, stats); err != nil {
		t.Fatal(err)
	}
	svc := NewRecommendationService(RecommendationConfig{Enabled: true, FeedTTL: time.Hour, CandidateLimit: 20, FeedSize: 20}, recommendRepo, trackRepo, userRepo, collectRepo, navigationRepo, followRepo, trackSvc, nil)

	first, err := svc.ListRecommend(ctx, 1, ListRecommendInput{CityCode: "330100", Limit: 2, ClientLanguage: "zh-CN"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Recommendation == nil || first.Recommendation.RequestID == "" {
		t.Fatalf("missing recommendation metadata: %+v", first)
	}
	if len(first.Items) != 2 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("unexpected first page: %+v", first)
	}
	for _, item := range first.Items {
		if item.UserID == 1 || item.CityCode != "330100" {
			t.Fatalf("eligibility filter failed: %+v", item)
		}
		if item.RecommendRank <= 0 || item.CandidateSource == "" || item.RecommendReason == "" {
			t.Fatalf("missing attribution: %+v", item)
		}
	}
	second, err := svc.ListRecommend(ctx, 1, ListRecommendInput{CityCode: "330100", Limit: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.HasMore {
		t.Fatalf("unexpected second page: %+v", second)
	}
	if second.Recommendation.RequestID != first.Recommendation.RequestID {
		t.Fatal("request id changed within feed session")
	}
}

func TestRecommendationSessionSkipsInvalidContentAndKeepsRank(t *testing.T) {
	ctx := context.Background()
	trackRepo := repository.NewInMemoryTrackRepository()
	collectRepo := repository.NewInMemoryCollectRepository()
	navigationRepo := repository.NewInMemoryNavigationRepository()
	userRepo := repository.NewInMemoryUserRepository()
	recommendRepo := repository.NewInMemoryRecommendationRepository()
	trackSvc := NewTrackService(trackRepo, collectRepo)
	trackSvc.SetUserRepository(userRepo)
	trackSvc.SetNavigationRepository(navigationRepo)
	now := time.Now()
	for index := 1; index <= 4; index++ {
		track := &models.Track{ID: "NO.0000000" + string(rune('0'+index)), UserID: int64(index + 10), CityCode: "330100", RawTrackURL: "oss://track", StartTime: now.Add(-time.Duration(index) * time.Minute), Status: models.TrackStatusNormal}
		if err := trackRepo.Create(ctx, track); err != nil {
			t.Fatal(err)
		}
	}
	stats := []*models.RecommendationItemStats{}
	for index := 1; index <= 4; index++ {
		stats = append(stats, &models.RecommendationItemStats{TrackID: "NO.0000000" + string(rune('0'+index)), StatDate: now, UpdatedAt: now})
	}
	_ = recommendRepo.UpsertItemStats(ctx, stats)
	svc := NewRecommendationService(RecommendationConfig{Enabled: true, FeedTTL: time.Hour, CandidateLimit: 10, FeedSize: 10}, recommendRepo, trackRepo, userRepo, collectRepo, navigationRepo, repository.NewInMemoryFollowRepository(), trackSvc, nil)
	first, err := svc.ListRecommend(ctx, 1, ListRecommendInput{CityCode: "330100", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	cursor, ok, err := decodeRecommendationCursor(first.NextCursor)
	if err != nil || !ok {
		t.Fatalf("decode cursor: %v", err)
	}
	session, err := recommendRepo.GetFeedSession(ctx, cursor.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	invalidID := session.Items[cursor.Offset].TrackID
	invalid, err := trackRepo.FindByID(ctx, invalidID)
	if err != nil {
		t.Fatal(err)
	}
	invalid.Status = models.TrackStatusPrivate
	if err := trackRepo.Update(ctx, invalid); err != nil {
		t.Fatal(err)
	}
	next, err := svc.ListRecommend(ctx, 1, ListRecommendInput{CityCode: "330100", Limit: 1, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Items) != 1 {
		t.Fatalf("expected scan-forward replacement: %+v", next)
	}
	if next.Items[0].RecommendRank != cursor.Offset+2 {
		t.Fatalf("rank should keep gap, got %d", next.Items[0].RecommendRank)
	}
}

func TestRecommendationSessionExpired(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryRecommendationRepository()
	trackRepo := repository.NewInMemoryTrackRepository()
	collectRepo := repository.NewInMemoryCollectRepository()
	navRepo := repository.NewInMemoryNavigationRepository()
	userRepo := repository.NewInMemoryUserRepository()
	trackSvc := NewTrackService(trackRepo, collectRepo)
	svc := NewRecommendationService(RecommendationConfig{Enabled: true}, repo, trackRepo, userRepo, collectRepo, navRepo, repository.NewInMemoryFollowRepository(), trackSvc, nil)
	session := &models.RecommendationFeedSession{RequestID: "rec_expired", UserID: 7, Strategy: models.RecommendationStrategyLegacy, CreatedAt: time.Now().Add(-2 * time.Hour), ExpiresAt: time.Now().Add(-time.Hour)}
	if err := repo.SaveFeedSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	cursor, _ := encodeRecommendationCursor(&recommendationCursor{Protocol: "feed-v1", RequestID: session.RequestID, Offset: 0})
	_, err := svc.ListRecommend(ctx, 7, ListRecommendInput{Cursor: cursor})
	if !errors.Is(err, ErrRecommendCursorExpired) {
		t.Fatalf("got %v", err)
	}
}

func TestRecommendationExplorationQuotaAndRouteGroupDiversity(t *testing.T) {
	exploration := make([]*recommendationCandidate, 0, 10)
	for index := 0; index < 10; index++ {
		exploration = append(exploration, &recommendationCandidate{track: &models.Track{ID: string(rune('a' + index)), UserID: int64(index + 1)}, source: models.RecommendationSourceExploration})
	}
	if got := selectPersonalizedCandidates(exploration, 10, 2); len(got) != 2 {
		t.Fatalf("exploration candidates=%d, want 2", len(got))
	}
	candidates := []*recommendationCandidate{
		{track: &models.Track{ID: "a", UserID: 1, TrackType: "hiking"}, groupID: "same"},
		{track: &models.Track{ID: "b", UserID: 2, TrackType: "running"}, groupID: "same"},
		{track: &models.Track{ID: "c", UserID: 3, TrackType: "riding"}, groupID: "other"},
	}
	got := diversifyCandidates(candidates, 2)
	if len(got) != 2 || got[0].groupID == got[1].groupID {
		t.Fatalf("route group was not diversified: %+v", got)
	}
}

func TestRecommendationRouteGroupDiversityBackfillsWithoutPanic(t *testing.T) {
	candidates := []*recommendationCandidate{
		{track: &models.Track{ID: "a", UserID: 1, TrackType: "hiking"}, groupID: "group-a"},
		{track: &models.Track{ID: "b", UserID: 2, TrackType: "hiking"}, groupID: "group-b"},
		// c 会先被连续运动类型约束跳过，随后 d 选中同一个 RouteGroup。
		// 旧实现下一轮会删除 c 并在空 remaining 上访问下标 0。
		{track: &models.Track{ID: "c", UserID: 3, TrackType: "hiking"}, groupID: "group-c"},
		{track: &models.Track{ID: "d", UserID: 4, TrackType: "running"}, groupID: "group-c"},
	}

	got := diversifyCandidates(candidates, len(candidates))
	if len(got) != len(candidates) {
		t.Fatalf("candidates=%d, want %d: %+v", len(got), len(candidates), got)
	}
	seen := make(map[string]struct{}, len(got))
	for _, candidate := range got {
		seen[candidate.track.ID] = struct{}{}
	}
	for _, candidate := range candidates {
		if _, ok := seen[candidate.track.ID]; !ok {
			t.Fatalf("candidate %s was dropped: %+v", candidate.track.ID, got)
		}
	}
}

func TestRecommendationRouteGroupDiversityUsesSameGroupAsFallback(t *testing.T) {
	candidates := []*recommendationCandidate{
		{track: &models.Track{ID: "a", UserID: 1, TrackType: "hiking"}, groupID: "same"},
		{track: &models.Track{ID: "b", UserID: 2, TrackType: "running"}, groupID: "same"},
		{track: &models.Track{ID: "c", UserID: 3, TrackType: "riding"}, groupID: "same"},
	}

	got := diversifyCandidates(candidates, len(candidates))
	if len(got) != len(candidates) {
		t.Fatalf("same-group candidates=%d, want %d: %+v", len(got), len(candidates), got)
	}
}

func TestRecommendationStronglyPenalizesAlreadyCollectedTrack(t *testing.T) {
	ctx := context.Background()
	trackRepo := repository.NewInMemoryTrackRepository()
	collectRepo := repository.NewInMemoryCollectRepository()
	navRepo := repository.NewInMemoryNavigationRepository()
	userRepo := repository.NewInMemoryUserRepository()
	recommendRepo := repository.NewInMemoryRecommendationRepository()
	trackSvc := NewTrackService(trackRepo, collectRepo)
	trackSvc.SetUserRepository(userRepo)
	trackSvc.SetNavigationRepository(navRepo)
	now := time.Now()
	collected := &models.Track{ID: "NO.00000011", UserID: 11, CityCode: "330100", TrackType: "hiking", Title: "collected", RawTrackURL: "oss://one", StartTime: now, Status: models.TrackStatusNormal}
	fresh := &models.Track{ID: "NO.00000012", UserID: 12, CityCode: "330100", TrackType: "hiking", Title: "fresh", RawTrackURL: "oss://two", StartTime: now.Add(-time.Second), Status: models.TrackStatusNormal}
	_ = trackRepo.Create(ctx, collected)
	_ = trackRepo.Create(ctx, fresh)
	_ = collectRepo.AddCollect(ctx, 1, collected.ID)
	_ = recommendRepo.UpsertItemStats(ctx, []*models.RecommendationItemStats{{TrackID: collected.ID, StatDate: now, UpdatedAt: now}, {TrackID: fresh.ID, StatDate: now, UpdatedAt: now}})
	svc := NewRecommendationService(RecommendationConfig{Enabled: true, FeedTTL: time.Hour, CandidateLimit: 10, FeedSize: 10}, recommendRepo, trackRepo, userRepo, collectRepo, navRepo, repository.NewInMemoryFollowRepository(), trackSvc, nil)
	page, err := svc.ListRecommend(ctx, 1, ListRecommendInput{CityCode: "330100", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("items=%d", len(page.Items))
	}
	if page.Items[0].ID != fresh.ID {
		t.Fatalf("collected track was not penalized: %s before %s", page.Items[0].ID, fresh.ID)
	}
}

func TestRecommendationExistingSessionDoesNotFallbackWhenRepositoryUnavailable(t *testing.T) {
	base := repository.NewInMemoryRecommendationRepository()
	repo := unavailableRecommendationRepository{RecommendationRepository: base}
	trackRepo := repository.NewInMemoryTrackRepository()
	collectRepo := repository.NewInMemoryCollectRepository()
	trackSvc := NewTrackService(trackRepo, collectRepo)
	svc := NewRecommendationService(RecommendationConfig{Enabled: true}, repo, trackRepo, repository.NewInMemoryUserRepository(), collectRepo, repository.NewInMemoryNavigationRepository(), repository.NewInMemoryFollowRepository(), trackSvc, nil)
	cursor, _ := encodeRecommendationCursor(&recommendationCursor{Protocol: "feed-v1", RequestID: "rec_unavailable", Offset: 0})
	_, err := svc.ListRecommend(context.Background(), 1, ListRecommendInput{Cursor: cursor})
	if !errors.Is(err, ErrRecommendSessionUnavailable) {
		t.Fatalf("got %v", err)
	}
}
