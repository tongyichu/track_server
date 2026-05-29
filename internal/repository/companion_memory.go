package repository

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

// InMemoryCompanionRepository 是 CompanionRepository 的内存实现。
type InMemoryCompanionRepository struct {
	mu               sync.RWMutex
	sessions         map[string]*models.CompanionSession
	sessionByToken   map[string]string
	members          map[string]map[int64]*models.CompanionSessionMember
	positions        map[string]map[int64]*models.CompanionLivePosition
	danmakus         []*models.CompanionDanmaku
	danmakuNextID    int64
}

// NewInMemoryCompanionRepository creates an in-memory companion repository.
func NewInMemoryCompanionRepository() *InMemoryCompanionRepository {
	return &InMemoryCompanionRepository{
		sessions:       make(map[string]*models.CompanionSession),
		sessionByToken: make(map[string]string),
		members:        make(map[string]map[int64]*models.CompanionSessionMember),
		positions:      make(map[string]map[int64]*models.CompanionLivePosition),
		danmakus:       make([]*models.CompanionDanmaku, 0),
		danmakuNextID:  0,
	}
}

func (r *InMemoryCompanionRepository) CreateSession(_ context.Context, session *models.CompanionSession) error {
	if session == nil || session.SessionID == "" {
		return ErrNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[session.SessionID]; ok {
		return ErrAlreadyExists
	}
	clone := *session
	if clone.CreatedAt.IsZero() {
		clone.CreatedAt = time.Now()
	}
	if clone.StartedAt.IsZero() {
		clone.StartedAt = clone.CreatedAt
	}
	if clone.Visibility == "" {
		clone.Visibility = models.CompanionSessionVisibilityPrivate
	}
	clone.UpdatedAt = clone.CreatedAt
	r.sessions[clone.SessionID] = &clone
	if clone.JoinToken != "" {
		r.sessionByToken[clone.JoinToken] = clone.SessionID
	}
	return nil
}

func (r *InMemoryCompanionRepository) UpdateSession(_ context.Context, session *models.CompanionSession) error {
	if session == nil || session.SessionID == "" {
		return ErrNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[session.SessionID]; !ok {
		return ErrNotFound
	}
	clone := *session
	clone.UpdatedAt = time.Now()
	r.sessions[clone.SessionID] = &clone
	if clone.JoinToken != "" {
		r.sessionByToken[clone.JoinToken] = clone.SessionID
	}
	return nil
}

func (r *InMemoryCompanionRepository) FindSessionByID(_ context.Context, sessionID string) (*models.CompanionSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	session, ok := r.sessions[sessionID]
	if !ok {
		return nil, ErrNotFound
	}
	clone := *session
	return &clone, nil
}

func (r *InMemoryCompanionRepository) FindSessionByJoinToken(_ context.Context, joinToken string) (*models.CompanionSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sessionID, ok := r.sessionByToken[joinToken]
	if !ok {
		return nil, ErrNotFound
	}
	session, ok := r.sessions[sessionID]
	if !ok {
		return nil, ErrNotFound
	}
	clone := *session
	return &clone, nil
}

func (r *InMemoryCompanionRepository) FindActiveSessionByUserID(_ context.Context, userID int64) (*models.CompanionSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for sessionID, members := range r.members {
		member, ok := members[userID]
		if !ok || member.MemberStatus != models.CompanionMemberStatusJoined {
			continue
		}
		session, ok := r.sessions[sessionID]
		if !ok || session.Status != models.CompanionSessionStatusActive {
			continue
		}
		clone := *session
		return &clone, nil
	}
	return nil, ErrNotFound
}

func (r *InMemoryCompanionRepository) ListSessionsByUserID(_ context.Context, userID int64, cursor *models.CompanionSessionListCursor, limit int) ([]*models.CompanionSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]*models.CompanionSession, 0)
	for sessionID, members := range r.members {
		if _, ok := members[userID]; !ok {
			continue
		}
		session, ok := r.sessions[sessionID]
		if !ok {
			continue
		}
		if cursor != nil {
			if session.StartedAt.After(cursor.StartedAt) {
				continue
			}
			if session.StartedAt.Equal(cursor.StartedAt) && session.SessionID >= cursor.SessionID {
				continue
			}
		}
		clone := *session
		items = append(items, &clone)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].StartedAt.Equal(items[j].StartedAt) {
			return items[i].SessionID > items[j].SessionID
		}
		return items[i].StartedAt.After(items[j].StartedAt)
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *InMemoryCompanionRepository) ListActiveSessions(_ context.Context, limit int) ([]*models.CompanionSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]*models.CompanionSession, 0)
	for _, session := range r.sessions {
		if session == nil || session.Status != models.CompanionSessionStatusActive {
			continue
		}
		clone := *session
		items = append(items, &clone)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].StartedAt.Equal(items[j].StartedAt) {
			return items[i].SessionID > items[j].SessionID
		}
		return items[i].StartedAt.After(items[j].StartedAt)
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *InMemoryCompanionRepository) CountSessionsByUserID(_ context.Context, userID int64) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var count int64
	for _, members := range r.members {
		if _, ok := members[userID]; ok {
			count++
		}
	}
	return count, nil
}

// ListAllSessions 返回全量会话（按 started_at desc, session_id desc）。仅供管理后台使用。
func (r *InMemoryCompanionRepository) ListAllSessions(_ context.Context, cursor *models.CompanionSessionListCursor, limit int) ([]*models.CompanionSession, error) {
	if limit <= 0 {
		limit = 20
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]*models.CompanionSession, 0, len(r.sessions))
	for _, session := range r.sessions {
		if session == nil {
			continue
		}
		if cursor != nil && !cursor.StartedAt.IsZero() {
			if session.StartedAt.After(cursor.StartedAt) {
				continue
			}
			if session.StartedAt.Equal(cursor.StartedAt) && session.SessionID >= cursor.SessionID {
				continue
			}
		}
		clone := *session
		items = append(items, &clone)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].StartedAt.Equal(items[j].StartedAt) {
			return items[i].SessionID > items[j].SessionID
		}
		return items[i].StartedAt.After(items[j].StartedAt)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

// CountAllSessions 返回全量会话数量。仅供管理后台使用。
func (r *InMemoryCompanionRepository) CountAllSessions(_ context.Context) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return int64(len(r.sessions)), nil
}

func (r *InMemoryCompanionRepository) UpsertMember(_ context.Context, member *models.CompanionSessionMember) error {
	if member == nil || member.SessionID == "" || member.UserID <= 0 {
		return ErrNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.members[member.SessionID]; !ok {
		r.members[member.SessionID] = make(map[int64]*models.CompanionSessionMember)
	}
	clone := *member
	if clone.JoinedAt.IsZero() {
		clone.JoinedAt = time.Now()
	}
	if clone.MemberStatus == models.CompanionMemberStatusJoined && clone.PresenceStatus == "" {
		clone.PresenceStatus = models.CompanionPresenceStatusOffline
	}
	r.members[member.SessionID][member.UserID] = &clone
	return nil
}

func (r *InMemoryCompanionRepository) FindMember(_ context.Context, sessionID string, userID int64) (*models.CompanionSessionMember, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	members, ok := r.members[sessionID]
	if !ok {
		return nil, ErrNotFound
	}
	member, ok := members[userID]
	if !ok {
		return nil, ErrNotFound
	}
	clone := *member
	return &clone, nil
}

func (r *InMemoryCompanionRepository) ListMembers(_ context.Context, sessionID string) ([]*models.CompanionSessionMember, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	members := r.members[sessionID]
	items := make([]*models.CompanionSessionMember, 0, len(members))
	for _, member := range members {
		clone := *member
		items = append(items, &clone)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Role != items[j].Role {
			return items[i].Role == models.CompanionMemberRoleOwner
		}
		if items[i].JoinedAt.Equal(items[j].JoinedAt) {
			return items[i].UserID < items[j].UserID
		}
		return items[i].JoinedAt.Before(items[j].JoinedAt)
	})
	return items, nil
}

func (r *InMemoryCompanionRepository) CountMembersByStatus(_ context.Context, sessionID string, status models.CompanionMemberStatus) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var count int64
	for _, member := range r.members[sessionID] {
		if member.MemberStatus == status {
			count++
		}
	}
	return count, nil
}

func (r *InMemoryCompanionRepository) UpsertPosition(_ context.Context, position *models.CompanionLivePosition) error {
	if position == nil || position.SessionID == "" || position.UserID <= 0 {
		return ErrNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.positions[position.SessionID]; !ok {
		r.positions[position.SessionID] = make(map[int64]*models.CompanionLivePosition)
	}
	clone := *position
	now := time.Now()
	if clone.CreatedAt.IsZero() {
		clone.CreatedAt = now
	}
	clone.UpdatedAt = now
	r.positions[position.SessionID][position.UserID] = &clone
	return nil
}

func (r *InMemoryCompanionRepository) ListPositions(_ context.Context, sessionID string) ([]*models.CompanionLivePosition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	positions := r.positions[sessionID]
	items := make([]*models.CompanionLivePosition, 0, len(positions))
	for _, position := range positions {
		clone := *position
		items = append(items, &clone)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].RecordedAt.Equal(items[j].RecordedAt) {
			return items[i].UserID < items[j].UserID
		}
		return items[i].RecordedAt.After(items[j].RecordedAt)
	})
	return items, nil
}

func (r *InMemoryCompanionRepository) DeletePositions(_ context.Context, sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.positions, sessionID)
	return nil
}

func (r *InMemoryCompanionRepository) InsertDanmaku(_ context.Context, d *models.CompanionDanmaku) error {
	if d == nil || d.SessionID == "" || d.UserID <= 0 {
		return ErrNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.danmakuNextID++
	clone := *d
	clone.ID = r.danmakuNextID
	if clone.CreatedAt.IsZero() {
		clone.CreatedAt = time.Now()
	}
	r.danmakus = append(r.danmakus, &clone)
	d.ID = clone.ID
	d.CreatedAt = clone.CreatedAt
	return nil
}

func (r *InMemoryCompanionRepository) CountDanmakuByMemberSince(_ context.Context, sessionID string, userID int64, since time.Time) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var count int64
	for _, d := range r.danmakus {
		if d == nil {
			continue
		}
		if d.SessionID != sessionID || d.UserID != userID {
			continue
		}
		if !since.IsZero() && d.CreatedAt.Before(since) {
			continue
		}
		count++
	}
	return count, nil
}

func (r *InMemoryCompanionRepository) CountDanmakuBySessionSince(_ context.Context, sessionID string, since time.Time) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var count int64
	for _, d := range r.danmakus {
		if d == nil {
			continue
		}
		if d.SessionID != sessionID {
			continue
		}
		if !since.IsZero() && d.CreatedAt.Before(since) {
			continue
		}
		count++
	}
	return count, nil
}

func (r *InMemoryCompanionRepository) DeleteDanmakusBySessionEndedBefore(_ context.Context, deadline time.Time) (int64, error) {
	if deadline.IsZero() {
		return 0, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	targets := make(map[string]struct{})
	for sid, s := range r.sessions {
		if s == nil {
			continue
		}
		if s.Status != models.CompanionSessionStatusEnded {
			continue
		}
		if s.EndedAt.IsZero() || !s.EndedAt.Before(deadline) {
			continue
		}
		targets[sid] = struct{}{}
	}
	if len(targets) == 0 {
		return 0, nil
	}
	filtered := r.danmakus[:0]
	var affected int64
	for _, d := range r.danmakus {
		if d == nil {
			continue
		}
		if _, hit := targets[d.SessionID]; hit {
			affected++
			continue
		}
		filtered = append(filtered, d)
	}
	r.danmakus = filtered
	return affected, nil
}
