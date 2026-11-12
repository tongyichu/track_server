package repository

import (
	"context"
	"github.com/tongyichu/track_server/internal/models"
	"sort"
	"strconv"
	"sync"
	"time"
)

type MessageFilter struct {
	GroupByKind           bool
	Kinds                 []string
	UserID                int64
	EitherUser            bool
	Participants          []int64
	ThreadID              string
	ThreadsAfter          map[string]int64
	Before                time.Time
	BeforeID              string
	After                 time.Time
	AfterID               string
	MaxTime               time.Time
	BeforeSeq             int64
	AfterSeq              int64
	ActiveOnly            bool
	PendingOnly           bool
	UnreadOnly            bool
	HasMessages           bool
	BlockedBy             int64
	VisibleTo             int64
	Audience              string
	ExcludeNoticeReadUser int64
	NoticeRegisteredAt    time.Time
	NoticeUnreadOnly      bool
	NoticeReadAfter       time.Time
	Limit                 int
	Asc                   bool
}
type MessageRepository interface {
	GetMessageRecord(context.Context, string) (*models.MessageRecord, error)
	GetMessageRecords(context.Context, []string) (map[string]*models.MessageRecord, error)
	PutMessageRecord(context.Context, *models.MessageRecord, int64) error
	ListMessageRecords(context.Context, MessageFilter) ([]*models.MessageRecord, error)
	CountMessageRecords(context.Context, MessageFilter) (int64, error)
	GroupMessageCounts(context.Context, MessageFilter) (map[string]int64, error)
	MarkMessageDelivered(context.Context, string, int64) error
	PruneMessageCredentials(context.Context, time.Time) error
}

func MessageNoticeReadKey(user int64, id string) string { return messageReadKey(user, id) }
func MessageNoticeClientID(r *models.MessageRecord) string {
	if r.NoticeRound > 0 {
		return r.ID + "~" + strconv.FormatInt(r.NoticeRound, 10)
	}
	return r.ID
}

func MessageNoticeEligible(r *models.MessageRecord, registered time.Time) bool {
	if r.NoticeMode == "once" && r.CreatedAt.Before(registered) {
		return false
	}
	return r.NoticeMode != "persistent" || r.NoticeUntilMicros == 0 || registered.UnixMicro() < r.NoticeUntilMicros
}

func MessageNoticeUnread(r *models.MessageRecord, registered, readAfter, now time.Time) bool {
	if !MessageNoticeEligible(r, registered) {
		return false
	}
	if r.NoticeMode == "persistent" {
		return (r.NoticeUntilMicros == 0 || now.UnixMicro() < r.NoticeUntilMicros) && (r.CreatedAt.After(readAfter) || registered.After(readAfter))
	}
	return r.CreatedAt.After(readAfter) && r.CreatedAt.After(registered)
}
func messageReadKey(user int64, id string) string {
	return fmtMessageUser("read.notice.", user) + "." + id
}
func fmtMessageUser(prefix string, user int64) string { return prefix + strconv.FormatInt(user, 10) }

func messageClone(r *models.MessageRecord) *models.MessageRecord {
	v := *r
	v.Payload = append([]byte(nil), r.Payload...)
	v.Targets = append([]string(nil), r.Targets...)
	return &v
}
func messageLimit(f MessageFilter) int {
	if f.Limit < 1 || f.Limit > 500 {
		return 30
	}
	return f.Limit
}
func messageMatches(r *models.MessageRecord, f MessageFilter) bool {
	if len(f.Participants) > 0 {
		user, peer := false, false
		for _, id := range f.Participants {
			user = user || r.UserID == id
			peer = peer || r.PeerID == id
		}
		if !user || !peer {
			return false
		}
	}
	if !f.NoticeRegisteredAt.IsZero() && !MessageNoticeEligible(r, f.NoticeRegisteredAt) {
		return false
	}
	if f.NoticeUnreadOnly && !MessageNoticeUnread(r, f.NoticeRegisteredAt, f.NoticeReadAfter, f.MaxTime) {
		return false
	}
	if len(f.Kinds) > 0 {
		found := false
		for _, k := range f.Kinds {
			if r.Kind == k {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	if f.UserID > 0 && r.UserID != f.UserID && (!f.EitherUser || r.PeerID != f.UserID) {
		return false
	}
	if f.ThreadID != "" && r.ThreadID != f.ThreadID {
		return false
	}
	if len(f.ThreadsAfter) > 0 {
		seq, ok := f.ThreadsAfter[r.ThreadID]
		if !ok || r.Seq <= seq {
			return false
		}
	}
	if !f.Before.IsZero() && (r.CreatedAt.After(f.Before) || r.CreatedAt.Equal(f.Before) && r.ID >= f.BeforeID) {
		return false
	}
	if !f.After.IsZero() && (r.CreatedAt.Before(f.After) || r.CreatedAt.Equal(f.After) && (f.AfterID == "" || r.ID <= f.AfterID)) {
		return false
	}
	if !f.MaxTime.IsZero() && r.CreatedAt.After(f.MaxTime) {
		return false
	}
	if f.BeforeSeq > 0 && r.Seq >= f.BeforeSeq || f.AfterSeq > 0 && r.Seq <= f.AfterSeq {
		return false
	}
	if f.ActiveOnly && !r.Active || f.PendingOnly && !r.Pending || f.UnreadOnly && r.FlagA || f.HasMessages && r.Seq == 0 {
		return false
	}
	if f.BlockedBy > 0 && !(r.UserID == f.BlockedBy && r.FlagA || r.PeerID == f.BlockedBy && r.FlagB) {
		return false
	}
	if f.VisibleTo > 0 && !(r.UserID == f.VisibleTo && !r.FlagA && r.Seq > r.ClearA || r.PeerID == f.VisibleTo && !r.FlagB && r.Seq > r.ClearB) {
		return false
	}
	if f.Audience != "" && len(r.Targets) > 0 {
		found := false
		for _, id := range r.Targets {
			if id == f.Audience {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

type MemoryMessageRepository struct {
	mu   sync.RWMutex
	rows map[string]*models.MessageRecord
}

func NewMemoryMessageRepository() *MemoryMessageRepository {
	return &MemoryMessageRepository{rows: map[string]*models.MessageRecord{}}
}
func (r *MemoryMessageRepository) GetMessageRecord(ctx context.Context, id string) (*models.MessageRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	v := r.rows[id]
	if v == nil {
		return nil, ErrNotFound
	}
	return messageClone(v), nil
}
func (r *MemoryMessageRepository) GetMessageRecords(ctx context.Context, ids []string) (map[string]*models.MessageRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := map[string]*models.MessageRecord{}
	for _, id := range ids {
		if v := r.rows[id]; v != nil {
			out[id] = messageClone(v)
		}
	}
	return out, nil
}
func (r *MemoryMessageRepository) PutMessageRecord(ctx context.Context, v *models.MessageRecord, expected int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	old := r.rows[v.ID]
	if old == nil && expected != 0 || old != nil && old.Revision != expected || v.Revision != expected+1 {
		return ErrConflict
	}
	r.rows[v.ID] = messageClone(v)
	return nil
}
func (r *MemoryMessageRepository) matches(v *models.MessageRecord, f MessageFilter) bool {
	if !messageMatches(v, f) {
		return false
	}
	return f.ExcludeNoticeReadUser <= 0 || r.rows[messageReadKey(f.ExcludeNoticeReadUser, MessageNoticeClientID(v))] == nil
}
func (r *MemoryMessageRepository) ListMessageRecords(ctx context.Context, f MessageFilter) ([]*models.MessageRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []*models.MessageRecord{}
	for _, v := range r.rows {
		if r.matches(v, f) {
			out = append(out, messageClone(v))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if f.ThreadID != "" && len(f.Kinds) == 1 && f.Kinds[0] == "dm" {
			if f.Asc {
				return out[i].Seq < out[j].Seq
			}
			return out[i].Seq > out[j].Seq
		}
		less := out[i].CreatedAt.Before(out[j].CreatedAt) || out[i].CreatedAt.Equal(out[j].CreatedAt) && out[i].ID < out[j].ID
		if f.Asc {
			return less
		}
		return out[i].CreatedAt.After(out[j].CreatedAt) || out[i].CreatedAt.Equal(out[j].CreatedAt) && out[i].ID > out[j].ID
	})
	if len(out) > messageLimit(f) {
		out = out[:messageLimit(f)]
	}
	return out, nil
}
func (r *MemoryMessageRepository) CountMessageRecords(ctx context.Context, f MessageFilter) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var n int64
	for _, v := range r.rows {
		if r.matches(v, f) {
			n++
		}
	}
	return n, nil
}
func (r *MemoryMessageRepository) GroupMessageCounts(ctx context.Context, f MessageFilter) (map[string]int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := map[string]int64{}
	for _, v := range r.rows {
		if r.matches(v, f) {
			key := v.ThreadID
			if f.GroupByKind {
				key = v.Kind
			}
			out[key]++
		}
	}
	return out, nil
}
func (r *MemoryMessageRepository) MarkMessageDelivered(ctx context.Context, id string, revision int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.rows[id]
	if row == nil || row.Revision != revision {
		return ErrConflict
	}
	row.Pending = false
	return nil
}

func (r *MemoryMessageRepository) PruneMessageCredentials(ctx context.Context, cutoff time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, row := range r.rows {
		if row.Kind == "mqtt_session" && !row.CreatedAt.After(cutoff) {
			delete(r.rows, id)
		}
	}
	return nil
}
