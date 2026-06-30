package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

// InMemoryTrackRepository is an in-memory implementation of TrackRepository for tests and development.
type InMemoryTrackRepository struct {
	mu           sync.RWMutex
	tracks       map[string]*models.Track
	nextTrackSeq uint64
}

// InMemoryTrackWaypointRepository is an in-memory implementation of TrackWaypointRepository.
type InMemoryTrackWaypointRepository struct {
	mu          sync.RWMutex
	nextID      uint64
	waypoints   map[uint64]*models.TrackWaypoint
	waypointIDs map[string][]uint64
}

// InMemoryNavigationRepository is an in-memory implementation of NavigationRepository.
// It stores navigation usage as append-only records.
type InMemoryNavigationRepository struct {
	mu      sync.RWMutex
	byTrack map[string][]int64
}

// InMemoryAchievementRepository is an in-memory implementation of AchievementRepository.
type InMemoryAchievementRepository struct {
	mu      sync.RWMutex
	nextID  int64
	rewards map[int64]map[string]*models.UserAchievementReward
}

// InMemoryTrackMapRepository stores async map index jobs and geo indexes in memory.
type InMemoryTrackMapRepository struct {
	mu           sync.RWMutex
	tracks       *InMemoryTrackRepository
	jobs         map[string]*models.TrackMapIndexJob
	indexes      map[string]*models.TrackGeoIndex
	routeGroups  map[string]*models.TrackRouteGroup
	routeMembers map[string]map[string]*models.TrackRouteGroupMember
}

type InMemoryFeedbackRepository struct {
	mu        sync.RWMutex
	nextID    int64
	feedbacks map[string]*models.Feedback
}

type InMemoryAccountRestrictionRepository struct {
	mu           sync.RWMutex
	nextID       int64
	restrictions map[int64][]*models.AccountRestriction
}

func NewInMemoryNavigationRepository() *InMemoryNavigationRepository {
	return &InMemoryNavigationRepository{byTrack: make(map[string][]int64)}
}

func NewInMemoryAchievementRepository() *InMemoryAchievementRepository {
	return &InMemoryAchievementRepository{
		nextID:  1,
		rewards: make(map[int64]map[string]*models.UserAchievementReward),
	}
}

func NewInMemoryTrackMapRepository(tracks *InMemoryTrackRepository) *InMemoryTrackMapRepository {
	return &InMemoryTrackMapRepository{
		tracks:       tracks,
		jobs:         make(map[string]*models.TrackMapIndexJob),
		indexes:      make(map[string]*models.TrackGeoIndex),
		routeGroups:  make(map[string]*models.TrackRouteGroup),
		routeMembers: make(map[string]map[string]*models.TrackRouteGroupMember),
	}
}

func NewInMemoryFeedbackRepository() *InMemoryFeedbackRepository {
	return &InMemoryFeedbackRepository{nextID: 1, feedbacks: make(map[string]*models.Feedback)}
}

func NewInMemoryAccountRestrictionRepository() *InMemoryAccountRestrictionRepository {
	return &InMemoryAccountRestrictionRepository{nextID: 1, restrictions: make(map[int64][]*models.AccountRestriction)}
}

// NewInMemoryTrackRepository creates a new in-memory track repository.
func NewInMemoryTrackRepository() *InMemoryTrackRepository {
	return &InMemoryTrackRepository{tracks: make(map[string]*models.Track), nextTrackSeq: 1}
}

// NextTrackID allocates the next track id from the in-memory sequence.
func (r *InMemoryTrackRepository) NextTrackID(_ context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id, err := encodeTrackID(r.nextTrackSeq)
	if err != nil {
		return "", err
	}
	r.nextTrackSeq++
	return id, nil
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
	if _, exists := r.tracks[t.ID]; exists {
		return ErrAlreadyExists
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
	clone := *t
	r.tracks[t.ID] = &clone
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
	clone := *t
	r.tracks[t.ID] = &clone
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
	clone := *t
	return &clone, nil
}

// FindRunningByUserID finds the running track of a user.
func (r *InMemoryTrackRepository) FindRunningByUserID(_ context.Context, userID int64) (*models.Track, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, t := range r.tracks {
		if t.UserID == userID && t.IsRunning {
			return t, nil
		}
	}
	return nil, ErrNotFound
}

// StatsByUserID returns (trackCount, totalDistance) for tracks owned by user.
// 口径与 ListByUserID 保持一致：排除删除与进行中轨迹；其中 trackCount 仅统计 raw_track_url 非空的轨迹。
func (r *InMemoryTrackRepository) StatsByUserID(_ context.Context, userID int64) (int64, float64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var (
		cnt  int64
		dist float64
	)
	for _, t := range r.tracks {
		if t == nil {
			continue
		}
		if t.UserID != userID {
			continue
		}
		if t.IsRunning {
			continue
		}
		if t.Status != models.TrackStatusNormal && t.Status != models.TrackStatusPrivate {
			continue
		}
		if t.RawTrackURL != "" {
			cnt++
		}
		dist += t.Distance
	}
	return cnt, dist, nil
}

// StatsSummaryByUserID returns user track statistics from track records.
// 口径：排除删除与进行中轨迹，统计正常/私密轨迹的总里程、次数、总耗时和总热量。
func (r *InMemoryTrackRepository) StatsSummaryByUserID(_ context.Context, userID int64) (*models.TrackUserStats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := &models.TrackUserStats{}
	for _, t := range r.tracks {
		if t == nil {
			continue
		}
		if t.UserID != userID || t.IsRunning {
			continue
		}
		if t.Status != models.TrackStatusNormal && t.Status != models.TrackStatusPrivate {
			continue
		}
		stats.TrackCount++
		stats.TotalDistance += t.Distance
		stats.TotalDuration += int64(t.Duration)
		stats.TotalCalories += t.CaloriesBurned
	}
	return stats, nil
}

// ListByUserID returns tracks of the given user ordered by start time desc.
// It excludes deleted tracks and running tracks by default.
func (r *InMemoryTrackRepository) ListByUserID(_ context.Context, userID int64, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res := make([]*models.Track, 0)
	for _, t := range r.tracks {
		if t == nil {
			continue
		}
		if t.UserID != userID {
			continue
		}
		if t.IsRunning {
			continue
		}
		if t.Status != models.TrackStatusNormal && t.Status != models.TrackStatusPrivate {
			continue
		}
		if cursor != nil {
			if t.StartTime.After(cursor.StartTime) {
				continue
			}
			if t.StartTime.Equal(cursor.StartTime) && t.ID >= cursor.ID {
				continue
			}
		}
		if t.Status == models.TrackStatusDeleted {
			continue
		}
		res = append(res, t)
	}
	// sort by start_time desc
	sort.SliceStable(res, func(i, j int) bool {
		if res[i].StartTime.Equal(res[j].StartTime) {
			return res[i].ID > res[j].ID
		}
		return res[i].StartTime.After(res[j].StartTime)
	})
	if limit > 0 && len(res) > limit {
		res = res[:limit]
	}
	return res, nil
}

// ListRecommend returns normal completed tracks ordered by start_time desc, id desc.
func (r *InMemoryTrackRepository) ListRecommend(_ context.Context, _ int64, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res := make([]*models.Track, 0)
	for _, t := range r.tracks {
		if t == nil || t.IsRunning {
			continue
		}
		if t.Status != models.TrackStatusNormal {
			continue
		}
		if cursor != nil {
			if t.StartTime.After(cursor.StartTime) {
				continue
			}
			if t.StartTime.Equal(cursor.StartTime) && t.ID >= cursor.ID {
				continue
			}
		}
		res = append(res, t)
	}
	sort.SliceStable(res, func(i, j int) bool {
		if res[i].StartTime.Equal(res[j].StartTime) {
			return res[i].ID > res[j].ID
		}
		return res[i].StartTime.After(res[j].StartTime)
	})
	if limit > 0 && len(res) > limit {
		res = res[:limit]
	}
	return res, nil
}

// Search performs a naive keyword search based on track name.
func (r *InMemoryTrackRepository) Search(_ context.Context, keyword string, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res := make([]*models.Track, 0)
	for _, t := range r.tracks {
		if t == nil {
			continue
		}
		if t.Status != models.TrackStatusNormal {
			continue
		}
		if keyword != "" && !containsIgnoreCase(t.Title, keyword) {
			continue
		}
		if cursor != nil {
			if t.StartTime.After(cursor.StartTime) {
				continue
			}
			if t.StartTime.Equal(cursor.StartTime) && t.ID >= cursor.ID {
				continue
			}
		}
		res = append(res, t)
	}
	sort.SliceStable(res, func(i, j int) bool {
		if res[i].StartTime.Equal(res[j].StartTime) {
			return res[i].ID > res[j].ID
		}
		return res[i].StartTime.After(res[j].StartTime)
	})
	if limit > 0 && len(res) > limit {
		res = res[:limit]
	}
	return res, nil
}

// ListAll 返回全量未删除轨迹（按 start_time desc, id desc）。仅供管理后台使用。
func (r *InMemoryTrackRepository) ListAll(_ context.Context, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]*models.Track, 0, len(r.tracks))
	for _, t := range r.tracks {
		if t == nil {
			continue
		}
		if t.Status == models.TrackStatusDeleted {
			continue
		}
		if cursor != nil && !cursor.StartTime.IsZero() {
			if t.StartTime.After(cursor.StartTime) {
				continue
			}
			if t.StartTime.Equal(cursor.StartTime) && t.ID >= cursor.ID {
				continue
			}
		}
		clone := *t
		res = append(res, &clone)
	}
	sort.SliceStable(res, func(i, j int) bool {
		if res[i].StartTime.Equal(res[j].StartTime) {
			return res[i].ID > res[j].ID
		}
		return res[i].StartTime.After(res[j].StartTime)
	})
	if len(res) > limit {
		res = res[:limit]
	}
	return res, nil
}

// CountAll 返回全量未删除轨迹数量。仅供管理后台使用。
func (r *InMemoryTrackRepository) CountAll(_ context.Context) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var count int64
	for _, t := range r.tracks {
		if t == nil {
			continue
		}
		if t.Status == models.TrackStatusDeleted {
			continue
		}
		count++
	}
	return count, nil
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
		if existing.TokenVersion <= 0 {
			existing.TokenVersion = 1
		}
		return existing, nil
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now()
	}
	if u.TokenVersion <= 0 {
		u.TokenVersion = 1
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
	if u.TokenVersion <= 0 {
		u.TokenVersion = 1
	}
	return u, nil
}

// FindByPhone finds a user by phone.
func (r *InMemoryUserRepository) FindByPhone(_ context.Context, phone string) (*models.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, u := range r.users {
		if u.Phone == phone {
			if u.TokenVersion <= 0 {
				u.TokenVersion = 1
			}
			return u, nil
		}
	}
	return nil, ErrNotFound
}

// FindByNickname finds a user by nickname.
func (r *InMemoryUserRepository) FindByNickname(_ context.Context, nickname string) (*models.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, u := range r.users {
		if u.Nickname == nickname {
			if u.TokenVersion <= 0 {
				u.TokenVersion = 1
			}
			return u, nil
		}
	}
	return nil, ErrNotFound
}

// Update updates a user profile.
func (r *InMemoryUserRepository) Update(_ context.Context, u *models.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.users[u.ID]; !ok {
		return ErrNotFound
	}
	if u.TokenVersion <= 0 {
		u.TokenVersion = 1
	}
	u.UpdatedAt = time.Now()
	r.users[u.ID] = u
	return nil
}

// ListAll 返回全量用户列表，按 created_at desc, id desc 排序，支持游标翻页。
func (r *InMemoryUserRepository) ListAll(_ context.Context, cursor *models.UserListCursor, limit int) ([]*models.User, error) {
	if limit <= 0 {
		limit = 20
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]*models.User, 0, len(r.users))
	for _, u := range r.users {
		if u == nil {
			continue
		}
		if cursor != nil && !cursor.CreatedAt.IsZero() {
			if u.CreatedAt.After(cursor.CreatedAt) {
				continue
			}
			if u.CreatedAt.Equal(cursor.CreatedAt) && u.ID >= cursor.ID {
				continue
			}
		}
		clone := *u
		if clone.TokenVersion <= 0 {
			clone.TokenVersion = 1
		}
		items = append(items, &clone)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

// CountAll 返回全量用户数量。
func (r *InMemoryUserRepository) CountAll(_ context.Context) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return int64(len(r.users)), nil
}

func (r *InMemoryAccountRestrictionRepository) CreateAccountRestriction(_ context.Context, restriction *models.AccountRestriction) error {
	if restriction == nil || restriction.UserID <= 0 {
		return errors.New("account restriction user_id is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := *restriction
	if clone.ID <= 0 {
		clone.ID = r.nextID
		r.nextID++
	}
	if clone.CreatedAt.IsZero() {
		clone.CreatedAt = time.Now()
	}
	if clone.UpdatedAt.IsZero() {
		clone.UpdatedAt = clone.CreatedAt
	}
	if clone.Status == "" {
		clone.Status = models.AccountRestrictionStatusActive
	}
	r.restrictions[clone.UserID] = append(r.restrictions[clone.UserID], &clone)
	restriction.ID = clone.ID
	restriction.CreatedAt = clone.CreatedAt
	restriction.UpdatedAt = clone.UpdatedAt
	return nil
}

func (r *InMemoryAccountRestrictionRepository) FindActiveAccountRestriction(_ context.Context, userID int64, now time.Time) (*models.AccountRestriction, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := r.restrictions[userID]
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		if item == nil || item.Status != models.AccountRestrictionStatusActive {
			continue
		}
		if item.ExpiresAt != nil && !item.ExpiresAt.After(now) {
			continue
		}
		clone := *item
		return &clone, nil
	}
	return nil, ErrNotFound
}

func (r *InMemoryAccountRestrictionRepository) ListAccountRestrictionsByUserID(_ context.Context, userID int64, limit int) ([]*models.AccountRestriction, error) {
	if limit <= 0 {
		limit = 50
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := r.restrictions[userID]
	res := make([]*models.AccountRestriction, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		clone := *item
		res = append(res, &clone)
	}
	sort.SliceStable(res, func(i, j int) bool {
		if res[i].CreatedAt.Equal(res[j].CreatedAt) {
			return res[i].ID > res[j].ID
		}
		return res[i].CreatedAt.After(res[j].CreatedAt)
	})
	if len(res) > limit {
		res = res[:limit]
	}
	return res, nil
}

func (r *InMemoryAccountRestrictionRepository) RevokeActiveAccountRestrictions(_ context.Context, userID int64, operator string, now time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var count int64
	for _, item := range r.restrictions[userID] {
		if item == nil || item.Status != models.AccountRestrictionStatusActive {
			continue
		}
		if item.ExpiresAt != nil && !item.ExpiresAt.After(now) {
			continue
		}
		item.Status = models.AccountRestrictionStatusRevoked
		item.UpdatedAt = now
		item.RevokedAt = &now
		count++
	}
	return count, nil
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

// ListByUserID lists collect records of a user in reverse chronological order.
// Order: created_at desc, track_id desc.
func (r *InMemoryCollectRepository) ListByUserID(_ context.Context, userID int64, cursor *models.TrackCollectCursor, limit int) ([]*models.TrackCollect, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if userID <= 0 || limit <= 0 {
		return []*models.TrackCollect{}, nil
	}
	tracks, ok := r.collects[userID]
	if !ok || len(tracks) == 0 {
		return []*models.TrackCollect{}, nil
	}

	items := make([]*models.TrackCollect, 0, len(tracks))
	for _, c := range tracks {
		clone := *c
		items = append(items, &clone)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].TrackID > items[j].TrackID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})

	res := make([]*models.TrackCollect, 0, limit)
	for _, c := range items {
		if cursor != nil && !cursor.CreatedAt.IsZero() && cursor.TrackID != "" {
			// Keep only those strictly less than cursor in (created_at desc, track_id desc).
			if c.CreatedAt.After(cursor.CreatedAt) {
				continue
			}
			if c.CreatedAt.Equal(cursor.CreatedAt) && c.TrackID >= cursor.TrackID {
				continue
			}
		}
		res = append(res, c)
		if len(res) >= limit {
			break
		}
	}
	return res, nil
}

func (r *InMemoryCollectRepository) RemoveByTrackID(_ context.Context, trackID string) error {
	if trackID == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for uid, tracks := range r.collects {
		delete(tracks, trackID)
		if len(tracks) == 0 {
			delete(r.collects, uid)
		}
	}
	return nil
}

func (r *InMemoryCollectRepository) CountByTrackIDs(_ context.Context, trackIDs []string) (map[string]int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res := make(map[string]int64, len(trackIDs))
	if len(trackIDs) == 0 {
		return res, nil
	}
	uniq := make(map[string]struct{}, len(trackIDs))
	for _, id := range trackIDs {
		if id == "" {
			continue
		}
		if _, ok := uniq[id]; ok {
			continue
		}
		uniq[id] = struct{}{}
		res[id] = 0
	}
	if len(uniq) == 0 {
		return res, nil
	}

	// userID -> trackID -> collect，保证每个 userID 对同一 trackID 最多计数 1。
	for _, tracks := range r.collects {
		for trackID := range tracks {
			if _, ok := uniq[trackID]; ok {
				res[trackID]++
			}
		}
	}
	return res, nil
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

// InMemoryFollowRepository is an in-memory implementation of FollowRepository.
type InMemoryFollowRepository struct {
	mu      sync.RWMutex
	follows map[int64]map[int64]*models.UserFollow // followerUserID -> followeeUserID -> follow
}

func NewInMemoryFollowRepository() *InMemoryFollowRepository {
	return &InMemoryFollowRepository{follows: make(map[int64]map[int64]*models.UserFollow)}
}

func (r *InMemoryFollowRepository) IsFollowing(_ context.Context, followerUserID int64, followeeUserID int64) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	following, ok := r.follows[followerUserID]
	if !ok {
		return false, nil
	}
	_, ok = following[followeeUserID]
	return ok, nil
}

func (r *InMemoryFollowRepository) AddFollow(_ context.Context, followerUserID int64, followeeUserID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.follows[followerUserID]; !ok {
		r.follows[followerUserID] = make(map[int64]*models.UserFollow)
	}
	if _, exists := r.follows[followerUserID][followeeUserID]; exists {
		return nil
	}
	r.follows[followerUserID][followeeUserID] = &models.UserFollow{
		FollowerUserID: followerUserID,
		FolloweeUserID: followeeUserID,
		CreatedAt:      time.Now(),
	}
	return nil
}

func (r *InMemoryFollowRepository) RemoveFollow(_ context.Context, followerUserID int64, followeeUserID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	following, ok := r.follows[followerUserID]
	if !ok {
		return nil
	}
	delete(following, followeeUserID)
	if len(following) == 0 {
		delete(r.follows, followerUserID)
	}
	return nil
}

func (r *InMemoryFollowRepository) ListFollowing(_ context.Context, userID int64, cursor *models.UserFollowCursor, limit int) ([]*models.UserFollow, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if userID <= 0 || limit <= 0 {
		return []*models.UserFollow{}, nil
	}
	following, ok := r.follows[userID]
	if !ok || len(following) == 0 {
		return []*models.UserFollow{}, nil
	}
	items := make([]*models.UserFollow, 0, len(following))
	for _, f := range following {
		clone := *f
		items = append(items, &clone)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].FolloweeUserID > items[j].FolloweeUserID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return pageUserFollows(items, cursor, limit, func(f *models.UserFollow) int64 { return f.FolloweeUserID }), nil
}

func (r *InMemoryFollowRepository) ListFollowers(_ context.Context, userID int64, cursor *models.UserFollowCursor, limit int) ([]*models.UserFollow, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if userID <= 0 || limit <= 0 {
		return []*models.UserFollow{}, nil
	}
	items := make([]*models.UserFollow, 0)
	for _, following := range r.follows {
		if f, ok := following[userID]; ok {
			clone := *f
			items = append(items, &clone)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].FollowerUserID > items[j].FollowerUserID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return pageUserFollows(items, cursor, limit, func(f *models.UserFollow) int64 { return f.FollowerUserID }), nil
}

func pageUserFollows(items []*models.UserFollow, cursor *models.UserFollowCursor, limit int, cursorUserID func(*models.UserFollow) int64) []*models.UserFollow {
	res := make([]*models.UserFollow, 0, limit)
	for _, f := range items {
		if cursor != nil && !cursor.CreatedAt.IsZero() && cursor.UserID > 0 {
			id := cursorUserID(f)
			if f.CreatedAt.After(cursor.CreatedAt) {
				continue
			}
			if f.CreatedAt.Equal(cursor.CreatedAt) && id >= cursor.UserID {
				continue
			}
		}
		res = append(res, f)
		if len(res) >= limit {
			break
		}
	}
	return res
}

func (r *InMemoryFollowRepository) CountFollowing(_ context.Context, userID int64) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return int64(len(r.follows[userID])), nil
}

func (r *InMemoryFollowRepository) CountFollowers(_ context.Context, userID int64) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var cnt int64
	for _, following := range r.follows {
		if _, ok := following[userID]; ok {
			cnt++
		}
	}
	return cnt, nil
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
func NewInMemoryRepositories() (TrackRepository, UserRepository, CollectRepository, LoginLogRepository, NavigationRepository, AppReleaseRepository, CompanionRepository) {
	return NewInMemoryTrackRepository(), NewInMemoryUserRepository(), NewInMemoryCollectRepository(), NewInMemoryLoginLogRepository(), NewInMemoryNavigationRepository(), NewInMemoryAppReleaseRepository(), NewInMemoryCompanionRepository()
}

func (r *InMemoryFeedbackRepository) Create(_ context.Context, feedback *models.Feedback) error {
	if feedback == nil || feedback.FeedbackID == "" {
		return errors.New("feedback id is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.feedbacks[feedback.FeedbackID]; exists {
		return ErrAlreadyExists
	}
	now := time.Now()
	clone := cloneFeedback(feedback)
	if clone.ID == 0 {
		clone.ID = r.nextID
		r.nextID++
	}
	if clone.CreatedAt.IsZero() {
		clone.CreatedAt = now
	}
	if clone.UpdatedAt.IsZero() {
		clone.UpdatedAt = clone.CreatedAt
	}
	r.feedbacks[clone.FeedbackID] = clone
	feedback.ID = clone.ID
	feedback.CreatedAt = clone.CreatedAt
	feedback.UpdatedAt = clone.UpdatedAt
	return nil
}

func (r *InMemoryFeedbackRepository) FindByFeedbackID(_ context.Context, feedbackID string) (*models.Feedback, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.feedbacks[feedbackID]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneFeedback(item), nil
}

func (r *InMemoryFeedbackRepository) List(_ context.Context, filter models.FeedbackListFilter) ([]*models.Feedback, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	res := make([]*models.Feedback, 0, limit)
	for _, item := range r.feedbacks {
		if item == nil {
			continue
		}
		if filter.UserID > 0 && item.UserID != filter.UserID {
			continue
		}
		if filter.Status != "" && item.Status != filter.Status {
			continue
		}
		if filter.AppVersion != "" && item.AppVersion != filter.AppVersion {
			continue
		}
		if filter.Cursor != nil {
			if item.CreatedAt.After(filter.Cursor.CreatedAt) {
				continue
			}
			if item.CreatedAt.Equal(filter.Cursor.CreatedAt) && item.FeedbackID >= filter.Cursor.FeedbackID {
				continue
			}
		}
		res = append(res, cloneFeedback(item))
	}
	sort.SliceStable(res, func(i, j int) bool {
		if res[i].CreatedAt.Equal(res[j].CreatedAt) {
			return res[i].FeedbackID > res[j].FeedbackID
		}
		return res[i].CreatedAt.After(res[j].CreatedAt)
	})
	if len(res) > limit {
		res = res[:limit]
	}
	return res, nil
}

func (r *InMemoryFeedbackRepository) CountByUserAndStatuses(_ context.Context, userID int64, statuses []models.FeedbackStatus) (int64, error) {
	if userID <= 0 || len(statuses) == 0 {
		return 0, nil
	}
	allowed := make(map[models.FeedbackStatus]struct{}, len(statuses))
	for _, status := range statuses {
		allowed[status] = struct{}{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var count int64
	for _, item := range r.feedbacks {
		if item == nil || item.UserID != userID {
			continue
		}
		if _, ok := allowed[item.Status]; ok {
			count++
		}
	}
	return count, nil
}

func (r *InMemoryFeedbackRepository) UpdateStatus(_ context.Context, feedbackID string, status models.FeedbackStatus, reply string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.feedbacks[feedbackID]
	if !ok {
		return ErrNotFound
	}
	item.Status = status
	item.Reply = reply
	item.UpdatedAt = time.Now()
	return nil
}

func cloneFeedback(in *models.Feedback) *models.Feedback {
	if in == nil {
		return nil
	}
	clone := *in
	if len(in.Images) > 0 {
		clone.Images = append([]models.FeedbackImage(nil), in.Images...)
	}
	return &clone
}

func (r *InMemoryNavigationRepository) AddNavigation(_ context.Context, navigatorUserID int64, trackID string) error {
	if trackID == "" || navigatorUserID <= 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byTrack[trackID] = append(r.byTrack[trackID], navigatorUserID)
	return nil
}

func (r *InMemoryNavigationRepository) CountByTrackIDs(_ context.Context, trackIDs []string) (map[string]int64, error) {
	res := make(map[string]int64, len(trackIDs))
	if len(trackIDs) == 0 {
		return res, nil
	}
	uniq := make(map[string]struct{}, len(trackIDs))
	uniqIDs := make([]string, 0, len(trackIDs))
	for _, id := range trackIDs {
		if id == "" {
			continue
		}
		if _, ok := uniq[id]; ok {
			continue
		}
		uniq[id] = struct{}{}
		uniqIDs = append(uniqIDs, id)
		res[id] = 0
	}
	if len(uniqIDs) == 0 {
		return res, nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, id := range uniqIDs {
		res[id] = int64(len(r.byTrack[id]))
	}
	return res, nil
}

func (r *InMemoryAchievementRepository) UpsertUserReward(_ context.Context, reward *models.UserAchievementReward) (bool, error) {
	if reward == nil || reward.UserID <= 0 || reward.RewardCode == "" {
		return false, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rewards[reward.UserID] == nil {
		r.rewards[reward.UserID] = make(map[string]*models.UserAchievementReward)
	}
	if _, exists := r.rewards[reward.UserID][reward.RewardCode]; exists {
		return false, nil
	}
	clone := *reward
	if clone.ID == 0 {
		clone.ID = r.nextID
		r.nextID++
	}
	if clone.EarnedAt.IsZero() {
		clone.EarnedAt = time.Now()
	}
	r.rewards[reward.UserID][reward.RewardCode] = &clone
	return true, nil
}

func (r *InMemoryAchievementRepository) ListUserRewards(_ context.Context, userID int64) ([]*models.UserAchievementReward, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]*models.UserAchievementReward, 0, len(r.rewards[userID]))
	for _, reward := range r.rewards[userID] {
		clone := *reward
		items = append(items, &clone)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].EarnedAt.Equal(items[j].EarnedAt) {
			return items[i].RewardCode > items[j].RewardCode
		}
		return items[i].EarnedAt.After(items[j].EarnedAt)
	})
	return items, nil
}

func (r *InMemoryAchievementRepository) ListRecentUserRewards(ctx context.Context, userID int64, limit int) ([]*models.UserAchievementReward, error) {
	items, err := r.ListUserRewards(ctx, userID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > len(items) {
		return items, nil
	}
	return items[:limit], nil
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

// InMemoryAppReleaseRepository is an in-memory implementation of AppReleaseRepository.
type InMemoryAppReleaseRepository struct {
	mu       sync.RWMutex
	nextID   int64
	releases map[int64]*models.AppRelease
}

// NewInMemoryAppReleaseRepository creates an in-memory app release repository.
func NewInMemoryAppReleaseRepository() *InMemoryAppReleaseRepository {
	return &InMemoryAppReleaseRepository{nextID: 1, releases: make(map[int64]*models.AppRelease)}
}

// Upsert inserts or updates a release record by (platform, version_code).
func (r *InMemoryAppReleaseRepository) Upsert(_ context.Context, release *models.AppRelease) error {
	if release == nil {
		return errors.New("release is nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	// Lookup existing by (platform, version_code).
	for _, existing := range r.releases {
		if existing.Platform == release.Platform && existing.VersionCode == release.VersionCode {
			release.ID = existing.ID
			release.CreatedAt = existing.CreatedAt
			release.UpdatedAt = now
			clone := *release
			r.releases[existing.ID] = &clone
			return nil
		}
	}
	if release.ID == 0 {
		release.ID = r.nextID
		r.nextID++
	} else if release.ID >= r.nextID {
		r.nextID = release.ID + 1
	}
	if release.CreatedAt.IsZero() {
		release.CreatedAt = now
	}
	release.UpdatedAt = now
	clone := *release
	r.releases[release.ID] = &clone
	return nil
}

// GetByID returns the release by id.
func (r *InMemoryAppReleaseRepository) GetByID(_ context.Context, id int64) (*models.AppRelease, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.releases[id]
	if !ok {
		return nil, ErrNotFound
	}
	clone := *item
	return &clone, nil
}

// GetByPlatformVersion returns the release by (platform, version_code).
func (r *InMemoryAppReleaseRepository) GetByPlatformVersion(_ context.Context, platform models.AppReleasePlatform, versionCode int64) (*models.AppRelease, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, item := range r.releases {
		if item.Platform == platform && item.VersionCode == versionCode {
			clone := *item
			return &clone, nil
		}
	}
	return nil, ErrNotFound
}

// List returns releases matching the filter, ordered by version_code desc.
func (r *InMemoryAppReleaseRepository) List(_ context.Context, filter models.AppReleaseListFilter) ([]*models.AppRelease, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]*models.AppRelease, 0)
	for _, item := range r.releases {
		if filter.Platform != "" && item.Platform != filter.Platform {
			continue
		}
		if filter.Status != "" && item.Status != filter.Status {
			continue
		}
		clone := *item
		res = append(res, &clone)
	}
	sort.SliceStable(res, func(i, j int) bool {
		if res[i].Platform == res[j].Platform {
			return res[i].VersionCode > res[j].VersionCode
		}
		return res[i].Platform < res[j].Platform
	})
	return res, nil
}

// GetLatestPublished returns the published release with the max version_code for the platform.
func (r *InMemoryAppReleaseRepository) GetLatestPublished(_ context.Context, platform models.AppReleasePlatform) (*models.AppRelease, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var latest *models.AppRelease
	for _, item := range r.releases {
		if item.Platform != platform {
			continue
		}
		if item.Status != models.AppReleaseStatusPublished {
			continue
		}
		if latest == nil || item.VersionCode > latest.VersionCode {
			latest = item
		}
	}
	if latest == nil {
		return nil, ErrNotFound
	}
	clone := *latest
	return &clone, nil
}

// Delete removes a release by id.
func (r *InMemoryAppReleaseRepository) Delete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.releases[id]; !ok {
		return ErrNotFound
	}
	delete(r.releases, id)
	return nil
}

func duplicateContainsIgnoreCaseShim() {}

func (r *InMemoryTrackMapRepository) EnqueueIndexJob(_ context.Context, trackID string, runAt time.Time) error {
	if trackID == "" {
		return errors.New("track id is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	if existing := r.jobs[trackID]; existing != nil {
		if existing.Status == models.TrackMapIndexJobSucceeded || existing.Status == models.TrackMapIndexJobProcessing {
			return nil
		}
		existing.Status = models.TrackMapIndexJobPending
		if existing.NextRunAt.IsZero() || runAt.Before(existing.NextRunAt) {
			existing.NextRunAt = runAt
		}
		existing.UpdatedAt = now
		return nil
	}
	r.jobs[trackID] = &models.TrackMapIndexJob{
		TrackID:   trackID,
		Status:    models.TrackMapIndexJobPending,
		NextRunAt: runAt,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return nil
}

func (r *InMemoryTrackMapRepository) ClaimPendingIndexJobs(_ context.Context, workerID string, now time.Time, limit int) ([]*models.TrackMapIndexJob, error) {
	if limit <= 0 {
		limit = 10
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	jobs := make([]*models.TrackMapIndexJob, 0, limit)
	for _, job := range r.jobs {
		if len(jobs) >= limit {
			break
		}
		staleProcessing := job != nil &&
			job.Status == models.TrackMapIndexJobProcessing &&
			!job.LockedAt.IsZero() &&
			job.LockedAt.Add(30*time.Minute).Before(now)
		if job == nil || (job.Status != models.TrackMapIndexJobPending && !staleProcessing) {
			continue
		}
		if job.Status == models.TrackMapIndexJobPending && !job.NextRunAt.IsZero() && job.NextRunAt.After(now) {
			continue
		}
		job.Status = models.TrackMapIndexJobProcessing
		job.LockedAt = now
		job.LockedBy = workerID
		job.UpdatedAt = now
		clone := *job
		jobs = append(jobs, &clone)
	}
	sort.SliceStable(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.Before(jobs[j].CreatedAt)
	})
	return jobs, nil
}

func (r *InMemoryTrackMapRepository) MarkIndexJobSucceeded(_ context.Context, trackID string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	job := r.jobs[trackID]
	if job == nil {
		return ErrNotFound
	}
	job.Status = models.TrackMapIndexJobSucceeded
	job.LastError = ""
	job.SucceededAt = now
	job.UpdatedAt = now
	return nil
}

func (r *InMemoryTrackMapRepository) MarkIndexJobFailed(_ context.Context, trackID, errMsg string, nextRunAt time.Time, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	job := r.jobs[trackID]
	if job == nil {
		return ErrNotFound
	}
	job.Status = models.TrackMapIndexJobPending
	job.Attempts++
	job.LastError = errMsg
	job.LastFailedAt = now
	job.NextRunAt = nextRunAt
	job.UpdatedAt = now
	return nil
}

func (r *InMemoryTrackMapRepository) UpsertTrackGeoIndex(_ context.Context, index *models.TrackGeoIndex) error {
	if index == nil || index.TrackID == "" {
		return errors.New("track geo index is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	clone := *index
	if clone.CreatedAt.IsZero() {
		clone.CreatedAt = now
	}
	clone.UpdatedAt = now
	r.indexes[index.TrackID] = &clone
	return nil
}

func (r *InMemoryTrackMapRepository) HasTrackGeoIndex(_ context.Context, trackID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.indexes[trackID]
	return ok, nil
}

// CleanupDeletedTrack removes map-index side records for a deleted track.
func (r *InMemoryTrackMapRepository) CleanupDeletedTrack(_ context.Context, trackID string) error {
	if trackID == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.jobs, trackID)
	delete(r.indexes, trackID)
	now := time.Now()
	for groupID, members := range r.routeMembers {
		if members == nil || members[trackID] == nil {
			continue
		}
		group := r.routeGroups[groupID]
		deletingRepresentative := group != nil && group.RepresentativeTrackID == trackID
		delete(members, trackID)
		if deletingRepresentative {
			delete(r.routeMembers, groupID)
			if group != nil {
				group.Status = models.TrackRouteGroupStatusArchived
				group.MemberCount = 0
				group.UpdatedAt = now
			}
			continue
		}
		if len(members) == 0 {
			delete(r.routeMembers, groupID)
		}
		if group != nil {
			group.MemberCount = int64(len(members))
			group.UpdatedAt = now
		}
	}
	return nil
}

func (r *InMemoryTrackMapRepository) ListCompletedTracksMissingGeoIndex(_ context.Context, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 100
	}
	if r.tracks == nil {
		return nil, nil
	}
	r.tracks.mu.RLock()
	defer r.tracks.mu.RUnlock()
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]*models.Track, 0, limit)
	for _, track := range r.tracks.tracks {
		if len(items) >= limit {
			break
		}
		if track == nil || track.IsRunning || track.Status != models.TrackStatusNormal || track.RawTrackURL == "" {
			continue
		}
		if _, ok := r.indexes[track.ID]; ok {
			continue
		}
		clone := *track
		items = append(items, &clone)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

func (r *InMemoryTrackMapRepository) FindTrackGeoIndex(_ context.Context, trackID string) (*models.TrackGeoIndex, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item := r.indexes[trackID]
	if item == nil {
		return nil, ErrNotFound
	}
	clone := cloneTrackGeoIndex(item)
	return clone, nil
}

func (r *InMemoryTrackMapRepository) ListAllTrackGeoIndexes(_ context.Context) ([]*models.TrackGeoIndex, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]*models.TrackGeoIndex, 0, len(r.indexes))
	for _, index := range r.indexes {
		if index == nil {
			continue
		}
		items = append(items, cloneTrackGeoIndex(index))
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].TrackType == items[j].TrackType {
			if items[i].CenterLat == items[j].CenterLat {
				if items[i].CenterLng == items[j].CenterLng {
					return items[i].TrackID < items[j].TrackID
				}
				return items[i].CenterLng < items[j].CenterLng
			}
			return items[i].CenterLat < items[j].CenterLat
		}
		return items[i].TrackType < items[j].TrackType
	})
	return items, nil
}

func (r *InMemoryTrackMapRepository) ListTrackGeoIndexes(_ context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackGeoIndex, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	limit := normalizeTrackMapQueryLimit(filter.Limit)
	items := make([]*models.TrackGeoIndex, 0, limit)
	for _, index := range r.indexes {
		if !trackGeoIndexMatchesFilter(index, filter) {
			continue
		}
		items = append(items, cloneTrackGeoIndex(index))
		if len(items) >= limit {
			break
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items, nil
}

func (r *InMemoryTrackMapRepository) CountTrackGeoIndexesByCity(_ context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackMapClusterItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	groups := make(map[string]*trackMapClusterAcc)
	for _, index := range r.indexes {
		if !trackGeoIndexMatchesFilter(index, filter) {
			continue
		}
		key := index.CityCode
		if key == "" {
			key = "unknown"
		}
		a := groups[key]
		if a == nil {
			a = &trackMapClusterAcc{}
			groups[key] = a
		}
		a.add(index)
	}
	items := make([]*models.TrackMapClusterItem, 0, len(groups))
	for cityCode, a := range groups {
		items = append(items, a.item("city_cluster", "", cityCode, filter.TrackType))
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].RouteCount > items[j].RouteCount
	})
	limit := normalizeTrackMapQueryLimit(filter.Limit)
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *InMemoryTrackMapRepository) CountTrackGeoIndexesByArea(_ context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackMapClusterItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	groups := make(map[string]*trackMapClusterAcc)
	for _, index := range r.indexes {
		if !trackGeoIndexMatchesFilter(index, filter) {
			continue
		}
		key := trackMapAreaClusterKey(index.CenterLat, index.CenterLng)
		a := groups[key]
		if a == nil {
			a = &trackMapClusterAcc{}
			groups[key] = a
		}
		a.add(index)
	}
	items := make([]*models.TrackMapClusterItem, 0, len(groups))
	for key, a := range groups {
		items = append(items, a.item("area_cluster", key, "", filter.TrackType))
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].RouteCount > items[j].RouteCount
	})
	limit := normalizeTrackMapQueryLimit(filter.Limit)
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func cloneTrackGeoIndex(index *models.TrackGeoIndex) *models.TrackGeoIndex {
	if index == nil {
		return nil
	}
	clone := *index
	clone.SimplifiedPolyline = append([]models.TrackPoint(nil), index.SimplifiedPolyline...)
	return &clone
}

func (r *InMemoryTrackMapRepository) FindRouteGroup(_ context.Context, groupID string) (*models.TrackRouteGroup, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	group := r.routeGroups[groupID]
	if group == nil || group.Status == models.TrackRouteGroupStatusArchived {
		return nil, ErrNotFound
	}
	return cloneTrackRouteGroup(group), nil
}

func (r *InMemoryTrackMapRepository) ListRouteGroups(_ context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackRouteGroup, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	limit := normalizeTrackMapQueryLimit(filter.Limit)
	items := make([]*models.TrackRouteGroup, 0, limit)
	for _, group := range r.routeGroups {
		if !routeGroupMatchesFilter(group, filter) {
			continue
		}
		items = append(items, cloneTrackRouteGroup(group))
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].MemberCount == items[j].MemberCount {
			return items[i].UpdatedAt.After(items[j].UpdatedAt)
		}
		return items[i].MemberCount > items[j].MemberCount
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *InMemoryTrackMapRepository) ListRouteGroupSummaries(ctx context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackRouteGroup, error) {
	return r.ListRouteGroups(ctx, filter)
}

func (r *InMemoryTrackMapRepository) ListRouteGroupCandidates(_ context.Context, index *models.TrackGeoIndex, limit int) ([]*models.TrackRouteGroupCandidate, error) {
	if index == nil {
		return nil, nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if limit <= 0 {
		limit = 20
	}
	items := make([]*models.TrackRouteGroupCandidate, 0, limit)
	filter := models.TrackMapQueryFilter{
		TrackType: index.TrackType,
		BBox: &models.TrackMapBBox{
			MinLatitude:  index.MinLat - 0.02,
			MinLongitude: index.MinLng - 0.02,
			MaxLatitude:  index.MaxLat + 0.02,
			MaxLongitude: index.MaxLng + 0.02,
		},
	}
	for _, group := range r.routeGroups {
		if !routeGroupMatchesFilter(group, filter) {
			continue
		}
		rep := r.indexes[group.RepresentativeTrackID]
		if rep == nil {
			continue
		}
		items = append(items, &models.TrackRouteGroupCandidate{Group: cloneTrackRouteGroup(group), Index: cloneTrackGeoIndex(rep)})
		if len(items) >= limit {
			break
		}
	}
	return items, nil
}

func (r *InMemoryTrackMapRepository) ListGeoIndexesWithoutRouteGroup(_ context.Context, limit int) ([]*models.TrackGeoIndex, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if limit <= 0 {
		limit = 100
	}
	items := make([]*models.TrackGeoIndex, 0, limit)
	for _, index := range r.indexes {
		if index == nil {
			continue
		}
		found := false
		for _, members := range r.routeMembers {
			if members[index.TrackID] != nil {
				found = true
				break
			}
		}
		if found {
			continue
		}
		items = append(items, cloneTrackGeoIndex(index))
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *InMemoryTrackMapRepository) UpsertRouteGroup(_ context.Context, group *models.TrackRouteGroup) error {
	if group == nil || group.GroupID == "" {
		return errors.New("route group is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	clone := cloneTrackRouteGroup(group)
	if clone.CreatedAt.IsZero() {
		clone.CreatedAt = now
	}
	clone.UpdatedAt = now
	r.routeGroups[clone.GroupID] = clone
	return nil
}

func (r *InMemoryTrackMapRepository) UpsertRouteGroupMember(_ context.Context, member *models.TrackRouteGroupMember) error {
	if member == nil || member.GroupID == "" || member.TrackID == "" {
		return errors.New("route group member is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	clone := *member
	if clone.CreatedAt.IsZero() {
		clone.CreatedAt = now
	}
	clone.UpdatedAt = now
	if r.routeMembers[clone.GroupID] == nil {
		r.routeMembers[clone.GroupID] = make(map[string]*models.TrackRouteGroupMember)
	}
	r.routeMembers[clone.GroupID][clone.TrackID] = &clone
	if group := r.routeGroups[clone.GroupID]; group != nil {
		group.MemberCount = int64(len(r.routeMembers[clone.GroupID]))
		group.UpdatedAt = now
	}
	return nil
}

func (r *InMemoryTrackMapRepository) ReplaceRouteGroups(_ context.Context, groups []*models.TrackRouteGroup, members []*models.TrackRouteGroupMember) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routeGroups = make(map[string]*models.TrackRouteGroup, len(groups))
	r.routeMembers = make(map[string]map[string]*models.TrackRouteGroupMember)
	for _, group := range groups {
		if group == nil || group.GroupID == "" {
			continue
		}
		clone := cloneTrackRouteGroup(group)
		r.routeGroups[clone.GroupID] = clone
	}
	for _, member := range members {
		if member == nil || member.GroupID == "" || member.TrackID == "" {
			continue
		}
		clone := *member
		if r.routeMembers[clone.GroupID] == nil {
			r.routeMembers[clone.GroupID] = make(map[string]*models.TrackRouteGroupMember)
		}
		r.routeMembers[clone.GroupID][clone.TrackID] = &clone
	}
	return nil
}

func (r *InMemoryTrackMapRepository) DeleteRouteGroupMember(_ context.Context, groupID, trackID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	members := r.routeMembers[groupID]
	if members == nil || members[trackID] == nil {
		return ErrNotFound
	}
	delete(members, trackID)
	if group := r.routeGroups[groupID]; group != nil {
		group.MemberCount = int64(len(members))
		group.UpdatedAt = time.Now()
	}
	return nil
}

func (r *InMemoryTrackMapRepository) ArchiveRouteGroup(_ context.Context, groupID string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	group := r.routeGroups[groupID]
	if group == nil {
		return ErrNotFound
	}
	group.Status = models.TrackRouteGroupStatusArchived
	group.UpdatedAt = now
	return nil
}

func (r *InMemoryTrackMapRepository) ListRouteGroupMembers(_ context.Context, groupID string, limit int) ([]*models.TrackRouteGroupMember, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if limit <= 0 {
		limit = 20
	}
	members := r.routeMembers[groupID]
	items := make([]*models.TrackRouteGroupMember, 0, len(members))
	for _, member := range members {
		if member == nil {
			continue
		}
		clone := *member
		items = append(items, &clone)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Role == items[j].Role {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].Role == models.TrackRouteGroupMemberRoleRepresentative
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *InMemoryTrackMapRepository) FindRouteGroupByTrackID(_ context.Context, trackID string) (*models.TrackRouteGroup, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for groupID, members := range r.routeMembers {
		if members[trackID] == nil {
			continue
		}
		group := r.routeGroups[groupID]
		if group == nil || group.Status == models.TrackRouteGroupStatusArchived {
			return nil, ErrNotFound
		}
		return cloneTrackRouteGroup(group), nil
	}
	return nil, ErrNotFound
}

func (r *InMemoryTrackMapRepository) CountRouteGroupsByCity(_ context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackMapClusterItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	groups := make(map[string]*trackMapClusterAcc)
	for _, group := range r.routeGroups {
		if !routeGroupMatchesFilter(group, filter) {
			continue
		}
		codes := group.CityCodes
		if len(codes) == 0 {
			codes = []string{""}
		}
		for _, cityCode := range codes {
			key := cityCode
			if key == "" {
				key = "unknown"
			}
			a := groups[key]
			if a == nil {
				a = &trackMapClusterAcc{}
				groups[key] = a
			}
			a.addRouteGroup(group)
		}
	}
	return sortedClusterItems(groups, "city_cluster", filter.TrackType, filter.Limit), nil
}

func (r *InMemoryTrackMapRepository) CountRouteGroupsByArea(_ context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackMapClusterItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	groups := make(map[string]*trackMapClusterAcc)
	for _, group := range r.routeGroups {
		if !routeGroupMatchesFilter(group, filter) {
			continue
		}
		key, areaID := trackMapRouteGroupAreaClusterKey(group)
		a := groups[key]
		if a == nil {
			a = &trackMapClusterAcc{}
			groups[key] = a
		}
		a.areaID = areaID
		a.addRouteGroup(group)
	}
	return sortedClusterItems(groups, "area_cluster", filter.TrackType, filter.Limit), nil
}

func cloneTrackRouteGroup(group *models.TrackRouteGroup) *models.TrackRouteGroup {
	if group == nil {
		return nil
	}
	clone := *group
	clone.CityCodes = append([]string(nil), group.CityCodes...)
	return &clone
}

func routeGroupMatchesFilter(group *models.TrackRouteGroup, filter models.TrackMapQueryFilter) bool {
	if group == nil || group.Status == models.TrackRouteGroupStatusArchived {
		return false
	}
	if filter.TrackType != "" && group.TrackType != filter.TrackType {
		return false
	}
	if filter.CityCode != "" && !routeGroupHasCity(group, filter.CityCode) {
		return false
	}
	if filter.BBox != nil && !routeGroupIntersectsBBox(group, *filter.BBox) {
		return false
	}
	if filter.Center != nil && filter.RadiusM > 0 {
		if haversineMeters(filter.Center.Latitude, filter.Center.Longitude, group.CenterLat, group.CenterLng) > float64(filter.RadiusM) {
			return false
		}
	}
	return true
}

func routeGroupHasCity(group *models.TrackRouteGroup, cityCode string) bool {
	for _, code := range group.CityCodes {
		if strings.TrimSpace(code) == cityCode {
			return true
		}
	}
	return false
}

func routeGroupIntersectsBBox(group *models.TrackRouteGroup, bbox models.TrackMapBBox) bool {
	return group.MaxLat >= bbox.MinLatitude &&
		group.MinLat <= bbox.MaxLatitude &&
		group.MaxLng >= bbox.MinLongitude &&
		group.MinLng <= bbox.MaxLongitude
}

func trackGeoIndexMatchesFilter(index *models.TrackGeoIndex, filter models.TrackMapQueryFilter) bool {
	if index == nil {
		return false
	}
	if filter.TrackType != "" && index.TrackType != filter.TrackType {
		return false
	}
	if filter.CityCode != "" && index.CityCode != filter.CityCode {
		return false
	}
	if filter.BBox != nil && !geoIndexIntersectsBBox(index, *filter.BBox) {
		return false
	}
	if filter.Center != nil && filter.RadiusM > 0 {
		if haversineMeters(filter.Center.Latitude, filter.Center.Longitude, index.CenterLat, index.CenterLng) > float64(filter.RadiusM) {
			return false
		}
	}
	return true
}

func geoIndexIntersectsBBox(index *models.TrackGeoIndex, bbox models.TrackMapBBox) bool {
	return index.MaxLat >= bbox.MinLatitude &&
		index.MinLat <= bbox.MaxLatitude &&
		index.MaxLng >= bbox.MinLongitude &&
		index.MinLng <= bbox.MaxLongitude
}

func trackMapAreaClusterKey(lat, lng float64) string {
	return fmt.Sprintf("cell_%.1f_%.1f", lat, lng)
}

func trackMapRouteGroupAreaClusterKey(group *models.TrackRouteGroup) (key, areaID string) {
	if group == nil {
		return "", ""
	}
	areaID = strings.TrimSpace(group.AreaID)
	if areaID != "" {
		return areaID, areaID
	}
	return trackMapAreaClusterKey(group.CenterLat, group.CenterLng), ""
}

type trackMapClusterAcc struct {
	areaID      string
	count       int64
	sumLat      float64
	sumLng      float64
	minLat      float64
	minLng      float64
	maxLat      float64
	maxLng      float64
	initialized bool
}

func (a *trackMapClusterAcc) add(index *models.TrackGeoIndex) {
	if index == nil {
		return
	}
	a.count++
	a.sumLat += index.CenterLat
	a.sumLng += index.CenterLng
	if !a.initialized {
		a.minLat, a.minLng, a.maxLat, a.maxLng = index.MinLat, index.MinLng, index.MaxLat, index.MaxLng
		a.initialized = true
		return
	}
	if index.MinLat < a.minLat {
		a.minLat = index.MinLat
	}
	if index.MinLng < a.minLng {
		a.minLng = index.MinLng
	}
	if index.MaxLat > a.maxLat {
		a.maxLat = index.MaxLat
	}
	if index.MaxLng > a.maxLng {
		a.maxLng = index.MaxLng
	}
}

func (a *trackMapClusterAcc) addRouteGroup(group *models.TrackRouteGroup) {
	if group == nil {
		return
	}
	a.count++
	a.sumLat += group.CenterLat
	a.sumLng += group.CenterLng
	if !a.initialized {
		a.minLat, a.minLng, a.maxLat, a.maxLng = group.MinLat, group.MinLng, group.MaxLat, group.MaxLng
		a.initialized = true
		return
	}
	if group.MinLat < a.minLat {
		a.minLat = group.MinLat
	}
	if group.MinLng < a.minLng {
		a.minLng = group.MinLng
	}
	if group.MaxLat > a.maxLat {
		a.maxLat = group.MaxLat
	}
	if group.MaxLng > a.maxLng {
		a.maxLng = group.MaxLng
	}
}

func (a *trackMapClusterAcc) item(kind, clusterID, cityCode, trackType string) *models.TrackMapClusterItem {
	item := &models.TrackMapClusterItem{
		Type:       kind,
		ClusterID:  clusterID,
		AreaID:     a.areaID,
		CityCode:   cityCode,
		TrackType:  trackType,
		RouteCount: a.count,
		BBox: models.TrackMapBBox{
			MinLatitude:  a.minLat,
			MinLongitude: a.minLng,
			MaxLatitude:  a.maxLat,
			MaxLongitude: a.maxLng,
		},
	}
	if a.count > 0 {
		item.Center = models.TrackMapPoint{Latitude: a.sumLat / float64(a.count), Longitude: a.sumLng / float64(a.count)}
	}
	return item
}

func sortedClusterItems(groups map[string]*trackMapClusterAcc, kind, trackType string, limit int) []*models.TrackMapClusterItem {
	items := make([]*models.TrackMapClusterItem, 0, len(groups))
	for key, a := range groups {
		cityCode := ""
		clusterID := key
		if kind == "area_cluster" && a.areaID != "" {
			clusterID = "area_" + a.areaID
		}
		if kind == "city_cluster" {
			cityCode = key
			if cityCode == "unknown" {
				cityCode = ""
			}
			clusterID = ""
		}
		items = append(items, a.item(kind, clusterID, cityCode, trackType))
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].RouteCount > items[j].RouteCount
	})
	max := normalizeTrackMapQueryLimit(limit)
	if len(items) > max {
		items = items[:max]
	}
	return items
}

func haversineMeters(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusM = 6371000
	toRad := func(v float64) float64 { return v * math.Pi / 180 }
	dLat := toRad(lat2 - lat1)
	dLng := toRad(lng2 - lng1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadiusM * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
