package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

var (
	ErrRecommendCursorExpired      = errors.New("recommend cursor expired")
	ErrRecommendSessionUnavailable = errors.New("recommend session unavailable")
)

type RecommendationConfig struct {
	Enabled        bool
	Timeout        time.Duration
	FeedTTL        time.Duration
	DataMaxAge     time.Duration
	CandidateLimit int
	FeedSize       int
}

type RecommendationService struct {
	config      RecommendationConfig
	repo        repository.RecommendationRepository
	tracks      repository.TrackRepository
	users       repository.UserRepository
	collects    repository.CollectRepository
	navigations repository.NavigationRepository
	follows     repository.FollowRepository
	trackSvc    *TrackService
	submissions *TrackSubmissionService
	trackMap    repository.TrackMapRepository
}

type recommendationCursor struct {
	Protocol  string `json:"protocol"`
	RequestID string `json:"request_id"`
	Offset    int    `json:"offset"`
}

type recommendationCandidate struct {
	track   *models.Track
	score   float64
	source  models.RecommendationCandidateSource
	reason  string
	groupID string
}

func (s *RecommendationService) SetTrackMapRepository(repo repository.TrackMapRepository) {
	s.trackMap = repo
}

func NewRecommendationService(cfg RecommendationConfig, repo repository.RecommendationRepository, tracks repository.TrackRepository, users repository.UserRepository, collects repository.CollectRepository, navigations repository.NavigationRepository, follows repository.FollowRepository, trackSvc *TrackService, submissions *TrackSubmissionService) *RecommendationService {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 250 * time.Millisecond
	}
	if cfg.FeedTTL <= 0 {
		cfg.FeedTTL = time.Hour
	}
	if cfg.DataMaxAge <= 0 {
		cfg.DataMaxAge = 48 * time.Hour
	}
	if cfg.FeedSize <= 0 || cfg.FeedSize > 200 {
		cfg.FeedSize = 200
	}
	if cfg.CandidateLimit < cfg.FeedSize {
		cfg.CandidateLimit = cfg.FeedSize
	}
	return &RecommendationService{config: cfg, repo: repo, tracks: tracks, users: users, collects: collects, navigations: navigations, follows: follows, trackSvc: trackSvc, submissions: submissions}
}

func (s *RecommendationService) ListRecommend(ctx context.Context, userID int64, input ListRecommendInput) (*models.TrackSummaryPage, error) {
	if !s.config.Enabled || s.repo == nil || userID <= 0 {
		return s.trackSvc.ListRecommend(ctx, userID, input)
	}
	if strings.TrimSpace(input.Cursor) != "" {
		cursor, isV2, err := decodeRecommendationCursor(input.Cursor)
		if err != nil {
			return nil, err
		}
		if !isV2 {
			return s.trackSvc.ListRecommend(ctx, userID, input)
		}
		return s.listSessionPage(ctx, userID, input, cursor)
	}

	started := time.Now()
	feedCtx, cancel := context.WithTimeout(ctx, s.config.Timeout)
	session, err := s.buildPersonalizedSession(feedCtx, userID, strings.TrimSpace(input.CityCode), input.ClientLanguage)
	cancel()
	if err != nil {
		log.Printf("[Recommendation] personalized_fallback user_id=%d city_code=%q duration_ms=%d error=%v", userID, input.CityCode, time.Since(started).Milliseconds(), err)
		session, err = s.buildLegacySession(ctx, userID, strings.TrimSpace(input.CityCode), input.ClientLanguage)
	}
	if err != nil {
		log.Printf("[Recommendation] hard_legacy user_id=%d city_code=%q error=%v", userID, input.CityCode, err)
		return s.trackSvc.ListRecommend(ctx, userID, input)
	}
	if err := s.repo.SaveFeedSession(ctx, session); err != nil {
		log.Printf("[Recommendation] session_save_failed request_id=%s error=%v", session.RequestID, err)
		return s.trackSvc.ListRecommend(ctx, userID, input)
	}
	sourceCounts := make(map[models.RecommendationCandidateSource]int)
	for _, item := range session.Items {
		sourceCounts[item.CandidateSource]++
	}
	log.Printf("[Recommendation] feed_created request_id=%s user_id=%d city_code=%q strategy=%s items=%d sources=%v duration_ms=%d", session.RequestID, userID, session.CityCode, session.Strategy, len(session.Items), sourceCounts, time.Since(started).Milliseconds())
	return s.buildSessionPage(ctx, session, 0, normalizeTrackPageLimit(input.Limit))
}

func (s *RecommendationService) listSessionPage(ctx context.Context, userID int64, input ListRecommendInput, cursor *recommendationCursor) (*models.TrackSummaryPage, error) {
	session, err := s.repo.GetFeedSession(ctx, cursor.RequestID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrRecommendCursorExpired
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRecommendSessionUnavailable, err)
	}
	if session.UserID != userID || !session.ExpiresAt.After(time.Now()) {
		return nil, ErrRecommendCursorExpired
	}
	if strings.TrimSpace(input.CityCode) != session.CityCode {
		return nil, invalidArg("recommend cursor does not match city_code")
	}
	if cursor.Offset < 0 || cursor.Offset > len(session.Items) {
		return nil, invalidArg("invalid cursor")
	}
	return s.buildSessionPage(ctx, session, cursor.Offset, normalizeTrackPageLimit(input.Limit))
}

func (s *RecommendationService) buildSessionPage(ctx context.Context, session *models.RecommendationFeedSession, offset, limit int) (*models.TrackSummaryPage, error) {
	page := &models.TrackSummaryPage{Items: []*models.TrackSummary{}, Recommendation: &models.RecommendationMetadata{RequestID: session.RequestID, Strategy: session.Strategy}}
	if offset >= len(session.Items) {
		return page, nil
	}
	ids := make([]string, 0, len(session.Items)-offset)
	for _, item := range session.Items[offset:] {
		ids = append(ids, item.TrackID)
	}
	tracksByID, err := s.tracks.FindByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRecommendSessionUnavailable, err)
	}
	selectedTracks := make([]*models.Track, 0, limit)
	selectedItems := make([]models.RecommendationFeedItem, 0, limit)
	selectedRanks := make([]int, 0, limit)
	nextOffset := len(session.Items)
	for index := offset; index < len(session.Items); index++ {
		item := session.Items[index]
		track := tracksByID[item.TrackID]
		if !isRecommendationTrackVisible(track, session.UserID, session.CityCode) {
			continue
		}
		if len(selectedTracks) < limit {
			selectedTracks = append(selectedTracks, track)
			selectedItems = append(selectedItems, item)
			selectedRanks = append(selectedRanks, index+1)
			continue
		}
		nextOffset = index
		page.HasMore = true
		break
	}
	summaries, err := s.trackSvc.BuildTrackSummaries(ctx, session.UserID, selectedTracks)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRecommendSessionUnavailable, err)
	}
	for index, summary := range summaries {
		summary.RecommendRank = selectedRanks[index]
		summary.CandidateSource = selectedItems[index].CandidateSource
		summary.RecommendReason = selectedItems[index].RecommendReason
	}
	page.Items = summaries
	if page.HasMore {
		page.NextCursor, err = encodeRecommendationCursor(&recommendationCursor{Protocol: "feed-v1", RequestID: session.RequestID, Offset: nextOffset})
		if err != nil {
			return nil, err
		}
	}
	return page, nil
}

func (s *RecommendationService) buildPersonalizedSession(ctx context.Context, userID int64, cityCode, language string) (*models.RecommendationFeedSession, error) {
	tracks, following, approved, err := s.recallCandidateTracks(ctx, userID, cityCode)
	if err != nil {
		return nil, fmt.Errorf("candidate recall: %w", err)
	}
	if len(tracks) == 0 {
		return nil, errors.New("candidate recall returned no tracks")
	}
	profile, err := s.repo.GetUserProfile(ctx, userID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	if profile != nil && time.Since(profile.GeneratedAt) > s.config.DataMaxAge {
		return nil, errors.New("user profile is stale")
	}
	trackIDs := make([]string, 0, len(tracks))
	for _, track := range tracks {
		trackIDs = append(trackIDs, track.ID)
	}
	stats, err := s.repo.ListItemStats(ctx, trackIDs)
	if err != nil {
		return nil, err
	}
	if len(stats) == 0 {
		return nil, errors.New("item stats are not ready")
	}
	for _, stat := range stats {
		if stat != nil && time.Since(stat.UpdatedAt) > s.config.DataMaxAge {
			return nil, errors.New("item stats are stale")
		}
	}
	groups := make(map[string]string)
	if s.trackMap != nil {
		members, groupErr := s.trackMap.ListAllRouteGroupMembers(ctx)
		if groupErr == nil {
			trackSet := make(map[string]struct{}, len(trackIDs))
			for _, id := range trackIDs {
				trackSet[id] = struct{}{}
			}
			for _, member := range members {
				if member != nil {
					if _, ok := trackSet[member.TrackID]; ok {
						groups[member.TrackID] = member.GroupID
					}
				}
			}
		}
	}
	interacted := make(map[string]struct{})
	if profile != nil {
		for _, id := range profile.CollectedTrackIDs {
			interacted[id] = struct{}{}
		}
		for _, id := range profile.NavigatedTrackIDs {
			interacted[id] = struct{}{}
		}
	}
	recentCollects, err := s.collects.ListByUserID(ctx, userID, nil, 2000)
	if err != nil {
		return nil, err
	}
	for _, item := range recentCollects {
		if item != nil {
			interacted[item.TrackID] = struct{}{}
		}
	}
	recentNavigations, err := s.navigations.ListByUserID(ctx, userID, 2000)
	if err != nil {
		return nil, err
	}
	for _, item := range recentNavigations {
		if item != nil {
			interacted[item.TrackID] = struct{}{}
		}
	}
	candidates := make([]*recommendationCandidate, 0, len(tracks))
	for _, track := range tracks {
		stat := stats[track.ID]
		var affinity, popularity, author, quality float64
		if profile != nil {
			affinity = recommendationClamp01(profile.TrackTypeWeights[track.TrackType])*0.7 + recommendationClamp01(profile.CityWeights[track.CityCode])*0.3
			author = recommendationClamp01(profile.AuthorWeights[strconv.FormatInt(track.UserID, 10)])
		}
		if stat != nil {
			engagement := float64(stat.CollectCount)*3 + float64(stat.NavigateCount)*5 + float64(stat.ClickCount)*0.5 + float64(stat.DetailViewCount)
			popularity = recommendationClamp01(math.Log1p(engagement) / 8)
		}
		_, isFollowedAuthor := following[track.UserID]
		if isFollowedAuthor {
			author = math.Max(author, 1)
		}
		quality = contentQuality(track, approved[track.ID])
		freshness := math.Exp(-math.Max(0, time.Since(track.StartTime).Hours()) / (24 * 30))
		exploration := stableUnitFloat(fmt.Sprintf("%d:%s", userID, track.ID))
		score := 0.35*affinity + 0.20*popularity + 0.15*author + 0.15*quality + 0.10*freshness + 0.05*exploration
		if _, ok := interacted[track.ID]; ok {
			score -= 0.75
		}
		source := models.RecommendationSourceExploration
		best := 0.05 * exploration
		if value := 0.35 * affinity; value > best {
			source, best = models.RecommendationSourceContentAffinity, value
		}
		if value := 0.20 * popularity; value > best {
			source, best = models.RecommendationSourceCityHot, value
		}
		if value := 0.15 * author; isFollowedAuthor && value > best {
			source, best = models.RecommendationSourceFollowedAuthor, value
		} else if !isFollowedAuthor && value > best {
			source, best = models.RecommendationSourceContentAffinity, value
		}
		if value := 0.15 * quality; value > best {
			source = models.RecommendationSourceQuality
		}
		candidates = append(candidates, &recommendationCandidate{track: track, score: score, source: source, reason: recommendationReason(source, track, language), groupID: groups[track.ID]})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].track.ID > candidates[j].track.ID
		}
		return candidates[i].score > candidates[j].score
	})
	strategy := models.RecommendationStrategyPersonalized
	personalizedLimit := s.config.FeedSize
	if profile == nil || profile.PositiveEventCount < 3 {
		strategy = models.RecommendationStrategyHybrid
		personalizedLimit = int(math.Ceil(float64(s.config.FeedSize) * 0.85))
	}
	candidates = selectPersonalizedCandidates(candidates, personalizedLimit, int(math.Ceil(float64(s.config.FeedSize)*0.15)))
	if len(candidates) < s.config.FeedSize {
		selected := make(map[string]struct{}, len(candidates))
		for _, candidate := range candidates {
			selected[candidate.track.ID] = struct{}{}
		}
		for _, track := range tracks {
			if len(candidates) >= s.config.FeedSize {
				break
			}
			if _, ok := selected[track.ID]; ok {
				continue
			}
			selected[track.ID] = struct{}{}
			candidates = append(candidates, &recommendationCandidate{track: track, source: models.RecommendationSourceLegacyFill, reason: recommendationReason(models.RecommendationSourceLegacyFill, track, language), groupID: groups[track.ID]})
			strategy = models.RecommendationStrategyHybrid
		}
	}
	candidates = diversifyCandidates(candidates, s.config.FeedSize)
	return newRecommendationSession(userID, cityCode, strategy, candidates, s.config.FeedTTL)
}

func (s *RecommendationService) recallCandidateTracks(ctx context.Context, userID int64, cityCode string) ([]*models.Track, map[int64]struct{}, map[string]bool, error) {
	following := make(map[int64]struct{})
	approved := make(map[string]bool)
	followRows := []*models.UserFollow{}
	if s.follows != nil {
		var err error
		followRows, err = s.follows.ListFollowing(ctx, userID, nil, 2000)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, row := range followRows {
			if row != nil {
				following[row.FolloweeUserID] = struct{}{}
			}
		}
	}
	result := make([]*models.Track, 0, s.config.CandidateLimit)
	seen := make(map[string]struct{}, s.config.CandidateLimit)
	appendTrack := func(track *models.Track) {
		if len(result) >= s.config.CandidateLimit || !isRecommendationTrackVisible(track, userID, cityCode) {
			return
		}
		if _, ok := seen[track.ID]; ok {
			return
		}
		seen[track.ID] = struct{}{}
		result = append(result, track)
	}
	// Reserve a bounded followed-author recall lane so older followed content is not crowded out by recent tracks.
	for index, row := range followRows {
		if index >= 8 || len(result) >= 80 {
			break
		}
		if row == nil {
			continue
		}
		tracks, err := s.tracks.ListByUserID(ctx, row.FolloweeUserID, nil, 10)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, track := range tracks {
			appendTrack(track)
		}
	}
	// Pull approved submissions as an explicit quality lane before filling with the recent public pool.
	if s.submissions != nil {
		ids, err := s.submissions.ListApprovedTrackIDs(ctx, 100)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, id := range ids {
			approved[id] = true
		}
		byID, err := s.tracks.FindByIDs(ctx, ids)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, id := range ids {
			appendTrack(byID[id])
		}
	}
	recent, err := s.listCandidateTracks(ctx, userID, cityCode, s.config.CandidateLimit)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, track := range recent {
		appendTrack(track)
	}
	return result, following, approved, nil
}

func (s *RecommendationService) buildLegacySession(ctx context.Context, userID int64, cityCode, language string) (*models.RecommendationFeedSession, error) {
	tracks, err := s.listCandidateTracks(ctx, userID, cityCode, s.config.FeedSize)
	if err != nil {
		return nil, err
	}
	candidates := make([]*recommendationCandidate, 0, len(tracks))
	for _, track := range tracks {
		candidates = append(candidates, &recommendationCandidate{track: track, source: models.RecommendationSourceLegacy, reason: recommendationReason(models.RecommendationSourceLegacy, track, language)})
	}
	return newRecommendationSession(userID, cityCode, models.RecommendationStrategyLegacy, candidates, s.config.FeedTTL)
}

func (s *RecommendationService) listCandidateTracks(ctx context.Context, userID int64, cityCode string, limit int) ([]*models.Track, error) {
	result := make([]*models.Track, 0, limit)
	seen := make(map[string]struct{}, limit)
	var cursor *models.TrackListCursor
	for len(result) < limit {
		batchSize := limit - len(result)
		if batchSize < 50 {
			batchSize = 50
		}
		if batchSize > 200 {
			batchSize = 200
		}
		batch, err := s.tracks.ListRecommend(ctx, userID, cityCode, cursor, batchSize)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		for _, track := range batch {
			if !isRecommendationTrackVisible(track, userID, cityCode) {
				continue
			}
			if _, ok := seen[track.ID]; ok {
				continue
			}
			seen[track.ID] = struct{}{}
			result = append(result, track)
			if len(result) >= limit {
				break
			}
		}
		last := batch[len(batch)-1]
		cursor = &models.TrackListCursor{StartTime: last.StartTime, ID: last.ID}
		if len(batch) < batchSize {
			break
		}
	}
	return result, nil
}

// RebuildUserProfiles rebuilds derived profiles from authoritative collect and navigation rows.
func (s *RecommendationService) RebuildUserProfiles(ctx context.Context) (int, error) {
	if s.repo == nil {
		return 0, nil
	}
	var cursor *models.UserListCursor
	total := 0
	for {
		users, err := s.users.ListAll(ctx, cursor, 200)
		if err != nil {
			return total, err
		}
		if len(users) == 0 {
			break
		}
		profiles := make([]*models.RecommendationUserProfile, 0, len(users))
		for _, user := range users {
			if user == nil {
				continue
			}
			profile, err := s.buildUserProfile(ctx, user.ID)
			if err != nil {
				return total, err
			}
			profiles = append(profiles, profile)
		}
		if err := s.repo.UpsertUserProfiles(ctx, profiles); err != nil {
			return total, err
		}
		total += len(profiles)
		last := users[len(users)-1]
		cursor = &models.UserListCursor{CreatedAt: last.CreatedAt, ID: last.ID}
		if len(users) < 200 {
			break
		}
	}
	return total, nil
}

func (s *RecommendationService) buildUserProfile(ctx context.Context, userID int64) (*models.RecommendationUserProfile, error) {
	collects, err := s.collects.ListByUserID(ctx, userID, nil, 2000)
	if err != nil {
		return nil, err
	}
	navigations, err := s.navigations.ListByUserID(ctx, userID, 2000)
	if err != nil {
		return nil, err
	}
	completedTracks, err := s.tracks.ListByUserID(ctx, userID, nil, 2000)
	if err != nil {
		return nil, err
	}
	trackIDs := make([]string, 0, len(collects)+len(navigations))
	for _, item := range collects {
		if item != nil {
			trackIDs = append(trackIDs, item.TrackID)
		}
	}
	for _, item := range navigations {
		if item != nil {
			trackIDs = append(trackIDs, item.TrackID)
		}
	}
	tracks, err := s.tracks.FindByIDs(ctx, trackIDs)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	profile := &models.RecommendationUserProfile{UserID: userID, TrackTypeWeights: map[string]float64{}, CityWeights: map[string]float64{}, AuthorWeights: map[string]float64{}, DataThrough: now, GeneratedAt: now}
	apply := func(trackID string, occurredAt time.Time, base float64) {
		track := tracks[trackID]
		if track == nil {
			return
		}
		ageDays := math.Max(0, now.Sub(occurredAt).Hours()/24)
		weight := base * math.Pow(0.5, ageDays/30)
		profile.TrackTypeWeights[track.TrackType] += weight
		profile.CityWeights[track.CityCode] += weight
		profile.AuthorWeights[strconv.FormatInt(track.UserID, 10)] += weight
		profile.PositiveEventCount++
	}
	for _, item := range collects {
		if item != nil {
			profile.CollectedTrackIDs = append(profile.CollectedTrackIDs, item.TrackID)
			apply(item.TrackID, item.CreatedAt, 4)
		}
	}
	for _, item := range navigations {
		if item != nil {
			profile.NavigatedTrackIDs = append(profile.NavigatedTrackIDs, item.TrackID)
			apply(item.TrackID, item.CreatedAt, 5)
		}
	}
	for _, track := range completedTracks {
		if track == nil || track.IsRunning || track.Status == models.TrackStatusDeleted {
			continue
		}
		occurredAt := track.EndTime
		if occurredAt.IsZero() {
			occurredAt = track.StartTime
		}
		ageDays := math.Max(0, now.Sub(occurredAt).Hours()/24)
		weight := 2 * math.Pow(0.5, ageDays/30)
		profile.TrackTypeWeights[track.TrackType] += weight
		profile.CityWeights[track.CityCode] += weight
		profile.PositiveEventCount++
	}
	normalizeRecommendationWeights(profile.TrackTypeWeights)
	normalizeRecommendationWeights(profile.CityWeights)
	normalizeRecommendationWeights(profile.AuthorWeights)
	return profile, nil
}

// RebuildItemStats rebuilds the latest daily strong-behaviour material aggregates.
func (s *RecommendationService) RebuildItemStats(ctx context.Context) (int, error) {
	if s.repo == nil {
		return 0, nil
	}
	tracks, err := s.listAllRecommendationTracks(ctx)
	if err != nil {
		return 0, err
	}
	ids := make([]string, 0, len(tracks))
	for _, track := range tracks {
		ids = append(ids, track.ID)
	}
	collectCounts, err := s.collects.CountByTrackIDs(ctx, ids)
	if err != nil {
		return 0, err
	}
	navigationCounts, err := s.navigations.CountByTrackIDs(ctx, ids)
	if err != nil {
		return 0, err
	}
	existingStats, err := s.repo.ListItemStats(ctx, ids)
	if err != nil {
		return 0, err
	}
	now := time.Now()
	statDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	stats := make([]*models.RecommendationItemStats, 0, len(ids))
	for _, id := range ids {
		collects, navs := collectCounts[id], navigationCounts[id]
		stat := &models.RecommendationItemStats{TrackID: id, StatDate: statDate, CollectCount: collects, NavigateCount: navs, DataThrough: now, GeneratedAt: now, UpdatedAt: now}
		if previous := existingStats[id]; previous != nil {
			stat.ImpressionCount, stat.ClickCount, stat.DetailViewCount = previous.ImpressionCount, previous.ClickCount, previous.DetailViewCount
		}
		stat.HotScore = math.Log1p(float64(collects)*3 + float64(navs)*5 + float64(stat.ClickCount)*0.5 + float64(stat.DetailViewCount))
		stats = append(stats, stat)
	}
	if err := s.repo.UpsertItemStats(ctx, stats); err != nil {
		return 0, err
	}
	return len(stats), nil
}

func (s *RecommendationService) listAllRecommendationTracks(ctx context.Context) ([]*models.Track, error) {
	result := make([]*models.Track, 0)
	var cursor *models.TrackListCursor
	for {
		batch, err := s.tracks.ListRecommend(ctx, 0, "", cursor, 500)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		for _, track := range batch {
			if isRecommendationTrackVisible(track, 0, "") {
				result = append(result, track)
			}
		}
		last := batch[len(batch)-1]
		cursor = &models.TrackListCursor{StartTime: last.StartTime, ID: last.ID}
		if len(batch) < 500 {
			break
		}
	}
	return result, nil
}

func (s *RecommendationService) CleanupFeedSessions(ctx context.Context) (int64, error) {
	if s.repo == nil {
		return 0, nil
	}
	return s.repo.DeleteExpiredFeedSessions(ctx, time.Now(), 1000)
}

func normalizeRecommendationWeights(weights map[string]float64) {
	maxWeight := 0.0
	for _, weight := range weights {
		if weight > maxWeight {
			maxWeight = weight
		}
	}
	if maxWeight <= 0 {
		return
	}
	for key, weight := range weights {
		weights[key] = weight / maxWeight
	}
}

func newRecommendationSession(userID int64, cityCode string, strategy models.RecommendationStrategy, candidates []*recommendationCandidate, ttl time.Duration) (*models.RecommendationFeedSession, error) {
	requestID, err := newRecommendationRequestID()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	session := &models.RecommendationFeedSession{RequestID: requestID, UserID: userID, CityCode: cityCode, Strategy: strategy, CreatedAt: now, ExpiresAt: now.Add(ttl), Items: make([]models.RecommendationFeedItem, 0, len(candidates))}
	for _, candidate := range candidates {
		session.Items = append(session.Items, models.RecommendationFeedItem{TrackID: candidate.track.ID, CandidateSource: candidate.source, RecommendReason: candidate.reason})
	}
	return session, nil
}

func isRecommendationTrackVisible(track *models.Track, userID int64, cityCode string) bool {
	return track != nil && track.UserID != userID && !track.IsRunning && track.Status == models.TrackStatusNormal && strings.TrimSpace(track.RawTrackURL) != "" && (cityCode == "" || track.CityCode == cityCode)
}

func diversifyCandidates(sorted []*recommendationCandidate, limit int) []*recommendationCandidate {
	result := make([]*recommendationCandidate, 0, min(limit, len(sorted)))
	remaining := make([]*recommendationCandidate, 0, len(sorted))
	for _, candidate := range sorted {
		if candidate != nil && candidate.track != nil {
			remaining = append(remaining, candidate)
		}
	}
	selectedGroups := make(map[string]struct{})
	for len(result) < limit && len(remaining) > 0 {
		blockAuthors := make(map[int64]int)
		blockStart := (len(result) / 20) * 20
		for _, existing := range result[blockStart:] {
			blockAuthors[existing.track.UserID]++
		}

		pickCandidate := func(allowRepeatedGroup, enforcePageDiversity bool) int {
			for index, candidate := range remaining {
				if !allowRepeatedGroup && candidate.groupID != "" {
					if _, repeated := selectedGroups[candidate.groupID]; repeated {
						continue
					}
				}
				if !enforcePageDiversity {
					return index
				}

				consecutiveType := 0
				for cursor := len(result) - 1; cursor >= 0 && result[cursor].track.TrackType == candidate.track.TrackType; cursor-- {
					consecutiveType++
				}
				if blockAuthors[candidate.track.UserID] >= 2 || consecutiveType >= 2 {
					continue
				}
				return index
			}
			return -1
		}

		// RouteGroup、作者和运动类型都是多样性软约束：先选同时满足全部
		// 约束的候选；不足时依次允许同组补位、放宽页内约束，确保不会
		// 因同组候选被永久丢弃而缩短 Feed。
		index := pickCandidate(false, true)
		if index < 0 {
			index = pickCandidate(true, true)
		}
		if index < 0 {
			index = pickCandidate(false, false)
		}
		if index < 0 {
			index = pickCandidate(true, false)
		}
		if index < 0 {
			break
		}

		candidate := remaining[index]
		result = append(result, candidate)
		if candidate.groupID != "" {
			selectedGroups[candidate.groupID] = struct{}{}
		}
		remaining = append(remaining[:index], remaining[index+1:]...)
	}
	return result
}

func selectPersonalizedCandidates(sorted []*recommendationCandidate, limit, explorationLimit int) []*recommendationCandidate {
	selected := make([]*recommendationCandidate, 0, min(limit, len(sorted)))
	explorationCount := 0
	for _, candidate := range sorted {
		if len(selected) >= limit {
			break
		}
		if candidate.source == models.RecommendationSourceExploration {
			if explorationCount >= explorationLimit {
				continue
			}
			explorationCount++
		}
		selected = append(selected, candidate)
	}
	return diversifyCandidates(selected, limit)
}

func contentQuality(track *models.Track, approved bool) float64 {
	value := 0.0
	if strings.TrimSpace(track.Title) != "" {
		value += 0.2
	}
	if strings.TrimSpace(track.LocateAddr) != "" {
		value += 0.15
	}
	if track.TrackScreenshotURL != "" || track.TrackNoMapBgScreenshotURL != "" {
		value += 0.25
	}
	if track.Distance > 0 && track.Duration > 0 {
		value += 0.15
	}
	if approved {
		value += 0.25
	}
	return recommendationClamp01(value)
}

func recommendationReason(source models.RecommendationCandidateSource, track *models.Track, language string) string {
	english := strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "en")
	city := config.CityNameByCode(track.CityCode)
	if city == "" {
		city = track.CityCode
	}
	if english {
		switch source {
		case models.RecommendationSourceContentAffinity:
			return "Matches your route interests"
		case models.RecommendationSourceCityHot:
			if city != "" {
				return "Popular near " + city
			}
			return "Popular recently"
		case models.RecommendationSourceFollowedAuthor:
			return "From an author you follow"
		case models.RecommendationSourceQuality:
			return "A featured quality route"
		case models.RecommendationSourceExploration:
			return "A fresh route to explore"
		default:
			return "Recommended route"
		}
	}
	switch source {
	case models.RecommendationSourceContentAffinity:
		if track.TrackType != "" {
			return "你常看的" + recommendationTrackTypeName(track.TrackType) + "路线"
		}
		return "符合你的路线偏好"
	case models.RecommendationSourceCityHot:
		if city != "" {
			return city + "近期热门"
		}
		return "近期热门路线"
	case models.RecommendationSourceFollowedAuthor:
		return "你关注的作者发布"
	case models.RecommendationSourceQuality:
		return "优质精选路线"
	case models.RecommendationSourceExploration:
		return "发现一条新路线"
	default:
		return "为你推荐"
	}
}

func recommendationTrackTypeName(trackType string) string {
	for _, item := range config.DefaultTrackTypeConfigs {
		if item.Type == trackType || item.Name == trackType {
			return item.Name
		}
	}
	return trackType
}

func newRecommendationRequestID() (string, error) {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "rec_" + hex.EncodeToString(raw), nil
}
func encodeRecommendationCursor(cursor *recommendationCursor) (string, error) {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
func decodeRecommendationCursor(raw string) (*recommendationCursor, bool, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return nil, false, invalidArg("invalid cursor")
	}
	cursor := &recommendationCursor{}
	if err := json.Unmarshal(decoded, cursor); err != nil {
		return nil, false, nil
	}
	if cursor.Protocol != "feed-v1" {
		return nil, false, nil
	}
	if cursor.RequestID == "" || cursor.Offset < 0 {
		return nil, true, invalidArg("invalid cursor")
	}
	return cursor, true, nil
}
func stableUnitFloat(value string) float64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(value))
	return float64(hash.Sum64()%10000) / 10000
}
func recommendationClamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
