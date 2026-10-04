package repository

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

// InMemoryRecommendationRepository keeps recommendation available after database fallback.
type InMemoryRecommendationRepository struct {
	mu       sync.RWMutex
	sessions map[string]*models.RecommendationFeedSession
	profiles map[int64]*models.RecommendationUserProfile
	stats    map[string]*models.RecommendationItemStats
}

func NewInMemoryRecommendationRepository() *InMemoryRecommendationRepository {
	return &InMemoryRecommendationRepository{
		sessions: make(map[string]*models.RecommendationFeedSession),
		profiles: make(map[int64]*models.RecommendationUserProfile),
		stats:    make(map[string]*models.RecommendationItemStats),
	}
}

func (r *InMemoryRecommendationRepository) SaveFeedSession(_ context.Context, session *models.RecommendationFeedSession) error {
	if err := validateRecommendationFeedSession(session); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := *session
	clone.Items = append([]models.RecommendationFeedItem(nil), session.Items...)
	r.sessions[session.RequestID] = &clone
	return nil
}

func validateRecommendationFeedSession(session *models.RecommendationFeedSession) error {
	if session == nil || session.RequestID == "" {
		return errors.New("recommendation request id is required")
	}
	if session.UserID <= 0 {
		return errors.New("recommendation user id is required")
	}
	switch session.Strategy {
	case models.RecommendationStrategyPersonalized, models.RecommendationStrategyHybrid, models.RecommendationStrategyLegacy:
	default:
		return errors.New("recommendation strategy is invalid")
	}
	if len(session.Items) > 200 {
		return errors.New("recommendation feed session exceeds 200 items")
	}
	if session.ExpiresAt.IsZero() || !session.ExpiresAt.After(session.CreatedAt) {
		return errors.New("recommendation feed session expiration is invalid")
	}
	for _, item := range session.Items {
		if item.TrackID == "" || item.CandidateSource == "" || item.RecommendReason == "" {
			return errors.New("recommendation feed item is invalid")
		}
	}
	return nil
}

func (r *InMemoryRecommendationRepository) GetFeedSession(_ context.Context, requestID string) (*models.RecommendationFeedSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	session, ok := r.sessions[requestID]
	if !ok {
		return nil, ErrNotFound
	}
	clone := *session
	clone.Items = append([]models.RecommendationFeedItem(nil), session.Items...)
	return &clone, nil
}

func (r *InMemoryRecommendationRepository) DeleteExpiredFeedSessions(_ context.Context, now time.Time, limit int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var deleted int64
	for id, session := range r.sessions {
		if limit > 0 && deleted >= int64(limit) {
			break
		}
		if session != nil && !session.ExpiresAt.After(now) {
			delete(r.sessions, id)
			deleted++
		}
	}
	return deleted, nil
}

func (r *InMemoryRecommendationRepository) GetUserProfile(_ context.Context, userID int64) (*models.RecommendationUserProfile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	profile, ok := r.profiles[userID]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneRecommendationUserProfile(profile), nil
}

func (r *InMemoryRecommendationRepository) UpsertUserProfiles(_ context.Context, profiles []*models.RecommendationUserProfile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, profile := range profiles {
		if profile != nil && profile.UserID > 0 {
			r.profiles[profile.UserID] = cloneRecommendationUserProfile(profile)
		}
	}
	return nil
}

func (r *InMemoryRecommendationRepository) ListItemStats(_ context.Context, trackIDs []string) (map[string]*models.RecommendationItemStats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]*models.RecommendationItemStats, len(trackIDs))
	for _, id := range trackIDs {
		if stat, ok := r.stats[id]; ok && stat != nil {
			clone := *stat
			result[id] = &clone
		}
	}
	return result, nil
}

func (r *InMemoryRecommendationRepository) UpsertItemStats(_ context.Context, stats []*models.RecommendationItemStats) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, stat := range stats {
		if stat != nil && stat.TrackID != "" {
			clone := *stat
			r.stats[stat.TrackID] = &clone
		}
	}
	return nil
}

func cloneRecommendationUserProfile(profile *models.RecommendationUserProfile) *models.RecommendationUserProfile {
	clone := *profile
	clone.TrackTypeWeights = cloneRecommendationFloatMap(profile.TrackTypeWeights)
	clone.CityWeights = cloneRecommendationFloatMap(profile.CityWeights)
	clone.AuthorWeights = cloneRecommendationFloatMap(profile.AuthorWeights)
	clone.CollectedTrackIDs = append([]string(nil), profile.CollectedTrackIDs...)
	clone.NavigatedTrackIDs = append([]string(nil), profile.NavigatedTrackIDs...)
	return &clone
}

func cloneRecommendationFloatMap(source map[string]float64) map[string]float64 {
	if source == nil {
		return nil
	}
	result := make(map[string]float64, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
