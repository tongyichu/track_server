package repository

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

// InMemoryTrackRepository is an in-memory implementation of TrackRepository for tests and development.
type InMemoryTrackRepository struct {
	mu     sync.RWMutex
	tracks map[string]*models.Track
}

// InMemoryTrackWaypointRepository is an in-memory implementation of TrackWaypointRepository.
type InMemoryTrackWaypointRepository struct {
	mu          sync.RWMutex
	nextID      uint64
	waypoints   map[uint64]*models.TrackWaypoint
	waypointIDs map[string][]uint64
}

// NewInMemoryTrackRepository creates a new in-memory track repository.
func NewInMemoryTrackRepository() *InMemoryTrackRepository {
	return &InMemoryTrackRepository{tracks: make(map[string]*models.Track)}
}

// NewInMemoryTrackWaypointRepository creates a new in-memory track waypoint repository.
func NewInMemoryTrackWaypointRepository() *InMemoryTrackWaypointRepository {
	return &InMemoryTrackWaypointRepository{
		nextID:      1,
		waypoints:   make(map[uint64]*models.TrackWaypoint),
		waypointIDs: make(map[string][]uint64),
	}
}

// Create stores a new track.
func (r *InMemoryTrackRepository) Create(_ context.Context, t *models.Track) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t.ID == "" {
		return errors.New("track id is required")
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	if t.StartTime.IsZero() {
		t.StartTime = t.CreatedAt
	}
	if t.EndTime.IsZero() {
		t.EndTime = t.StartTime
	}
	t.UpdatedAt = t.CreatedAt
	r.tracks[t.ID] = t
	return nil
}

// Update updates an existing track.
func (r *InMemoryTrackRepository) Update(_ context.Context, t *models.Track) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tracks[t.ID]; !ok {
		return ErrNotFound
	}
	t.UpdatedAt = time.Now()
	r.tracks[t.ID] = t
	return nil
}

// FindByID finds a track by id.
func (r *InMemoryTrackRepository) FindByID(_ context.Context, id string) (*models.Track, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tracks[id]
	if !ok {
		return nil, ErrNotFound
	}
	return t, nil
}

// FindRunningByUserID finds the running track of a user.
func (r *InMemoryTrackRepository) FindRunningByUserID(_ context.Context, userID int64) (*models.Track, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, t := range r.tracks {
		if t.UserID == userID && t.Status == models.TrackStatusNormal {
			return t, nil
		}
	}
	return nil, ErrNotFound
}

// ListRecommend returns a simple slice of tracks as recommendation.
func (r *InMemoryTrackRepository) ListRecommend(_ context.Context, _ int64, limit int) ([]*models.Track, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res := make([]*models.Track, 0, limit)
	for _, t := range r.tracks {
		res = append(res, t)
		if limit > 0 && len(res) >= limit {
			break
		}
	}
	return res, nil
}

// Search performs a naive keyword search based on track name.
func (r *InMemoryTrackRepository) Search(_ context.Context, keyword string, limit int) ([]*models.Track, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res := make([]*models.Track, 0, limit)
	for _, t := range r.tracks {
		if keyword == "" || containsIgnoreCase(t.Title, keyword) {
			res = append(res, t)
			if limit > 0 && len(res) >= limit {
				break
			}
		}
	}
	return res, nil
}

// Create stores a new waypoint.
func (r *InMemoryTrackWaypointRepository) Create(_ context.Context, waypoint *models.TrackWaypoint) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if waypoint.TrackID == "" {
		return errors.New("track waypoint track_id is required")
	}
	if waypoint.ID == 0 {
		waypoint.ID = r.nextID
		r.nextID++
	}
	if waypoint.CreatedAt.IsZero() {
		waypoint.CreatedAt = time.Now()
	}
	clone := *waypoint
	r.waypoints[clone.ID] = &clone
	r.waypointIDs[clone.TrackID] = append(r.waypointIDs[clone.TrackID], clone.ID)
	return nil
}

// ListByTrackID lists all multimedia waypoints of a track ordered by node time.
func (r *InMemoryTrackWaypointRepository) ListByTrackID(_ context.Context, trackID string) ([]*models.TrackWaypoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := r.waypointIDs[trackID]
	res := make([]*models.TrackWaypoint, 0, len(ids))
	for _, id := range ids {
		waypoint, ok := r.waypoints[id]
		if !ok {
			continue
		}
		clone := *waypoint
		res = append(res, &clone)
	}
	sort.SliceStable(res, func(i, j int) bool {
		if res[i].NodeTime.Equal(res[j].NodeTime) {
			return res[i].ID < res[j].ID
		}
		return res[i].NodeTime.Before(res[j].NodeTime)
	})
	return res, nil
}

// InMemoryUserRepository is an in-memory implementation of UserRepository.
type InMemoryUserRepository struct {
	mu    sync.RWMutex
	users map[int64]*models.User
}

// NewInMemoryUserRepository creates an in-memory user repository.
func NewInMemoryUserRepository() *InMemoryUserRepository {
	return &InMemoryUserRepository{users: make(map[int64]*models.User)}
}

// CreateIfNotExists creates the user if it does not exist.
func (r *InMemoryUserRepository) CreateIfNotExists(_ context.Context, u *models.User) (*models.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.users[u.ID]; ok {
		return existing, nil
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now()
	}
	u.UpdatedAt = u.CreatedAt
	r.users[u.ID] = u
	return u, nil
}

// FindByID finds a user by id.
func (r *InMemoryUserRepository) FindByID(_ context.Context, id int64) (*models.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.users[id]
	if !ok {
		return nil, ErrNotFound
	}
	return u, nil
}

// Update updates a user profile.
func (r *InMemoryUserRepository) Update(_ context.Context, u *models.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.users[u.ID]; !ok {
		return ErrNotFound
	}
	u.UpdatedAt = time.Now()
	r.users[u.ID] = u
	return nil
}

// InMemoryCollectRepository is an in-memory implementation of CollectRepository.
type InMemoryCollectRepository struct {
	mu       sync.RWMutex
	collects map[int64]map[string]*models.TrackCollect // userID -> trackID -> collect
}

// NewInMemoryCollectRepository creates an in-memory collect repository.
func NewInMemoryCollectRepository() *InMemoryCollectRepository {
	return &InMemoryCollectRepository{collects: make(map[int64]map[string]*models.TrackCollect)}
}

// IsCollected returns whether the track is collected by user.
func (r *InMemoryCollectRepository) IsCollected(_ context.Context, userID int64, trackID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tracks, ok := r.collects[userID]
	if !ok {
		return false, nil
	}
	_, ok = tracks[trackID]
	return ok, nil
}

// AddCollect adds a collect record.
func (r *InMemoryCollectRepository) AddCollect(_ context.Context, userID int64, trackID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.collects[userID]; !ok {
		r.collects[userID] = make(map[string]*models.TrackCollect)
	}
	r.collects[userID][trackID] = &models.TrackCollect{
		UserID:    userID,
		TrackID:   trackID,
		CreatedAt: time.Now(),
	}
	return nil
}

// RemoveCollect removes a collect record.
func (r *InMemoryCollectRepository) RemoveCollect(_ context.Context, userID int64, trackID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	tracks, ok := r.collects[userID]
	if !ok {
		return nil
	}
	delete(tracks, trackID)
	return nil
}

// InMemoryLoginLogRepository is an in-memory implementation of LoginLogRepository.
type InMemoryLoginLogRepository struct {
	mu      sync.RWMutex
	nextID  int64
	byUser  map[int64][]*models.LoginLog
	ordered []*models.LoginLog
}

// NewInMemoryLoginLogRepository creates an in-memory login log repository.
func NewInMemoryLoginLogRepository() *InMemoryLoginLogRepository {
	return &InMemoryLoginLogRepository{
		nextID: 1,
		byUser: make(map[int64][]*models.LoginLog),
	}
}

// Create stores a new login log record.
func (r *InMemoryLoginLogRepository) Create(_ context.Context, log *models.LoginLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := *log
	if clone.ID == 0 {
		clone.ID = r.nextID
		r.nextID++
	}
	if clone.CreatedAt.IsZero() {
		clone.CreatedAt = time.Now()
	}
	r.byUser[clone.UserID] = append(r.byUser[clone.UserID], &clone)
	r.ordered = append(r.ordered, &clone)
	log.ID = clone.ID
	log.CreatedAt = clone.CreatedAt
	return nil
}

// ListByUserID lists login logs of a user in reverse chronological order.
func (r *InMemoryLoginLogRepository) ListByUserID(_ context.Context, userID int64, limit int) ([]*models.LoginLog, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	logs := r.byUser[userID]
	if limit <= 0 || limit > len(logs) {
		limit = len(logs)
	}
	res := make([]*models.LoginLog, 0, limit)
	for i := len(logs) - 1; i >= 0 && len(res) < limit; i-- {
		clone := *logs[i]
		res = append(res, &clone)
	}
	return res, nil
}

// NewInMemoryRepositories is a helper to create a full set of in-memory repositories.
func NewInMemoryRepositories() (TrackRepository, UserRepository, CollectRepository, LoginLogRepository) {
	return NewInMemoryTrackRepository(), NewInMemoryUserRepository(), NewInMemoryCollectRepository(), NewInMemoryLoginLogRepository()
}

func containsIgnoreCase(s, sub string) bool {
	if sub == "" {
		return true
	}
	// simple ASCII case-insensitive contains
	ls := len(s)
	lsb := len(sub)
	if lsb == 0 || lsb > ls {
		return false
	}
	// convert both to lower without allocation-heavy operations
	for i := 0; i <= ls-lsb; i++ {
		match := true
		for j := 0; j < lsb; j++ {
			cs := s[i+j]
			ct := sub[j]
			if cs >= 'A' && cs <= 'Z' {
				cs += 'a' - 'A'
			}
			if ct >= 'A' && ct <= 'Z' {
				ct += 'a' - 'A'
			}
			if cs != ct {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
