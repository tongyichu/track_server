package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

var interactionKinds = []string{"comment", "reply", "like", "collect", "comment_like", "follow"}

type MessageService struct {
	repo         repository.MessageRepository
	users        repository.UserRepository
	tracks       repository.TrackRepository
	follows      repository.FollowRepository
	restrictions *AccountRestrictionService
	companion    *CompanionService
	avatarCache  *AssetCacheService
	durable      bool
}

func NewMessageService(repo repository.MessageRepository, users repository.UserRepository, tracks repository.TrackRepository, follows repository.FollowRepository, restrictions *AccountRestrictionService) *MessageService {
	return &MessageService{repo: repo, users: users, tracks: tracks, follows: follows, restrictions: restrictions}
}
func (s *MessageService) Enabled() bool { return s != nil && s.repo != nil }
func messageNow() time.Time             { return time.Now().UTC().Truncate(time.Microsecond) }
func messageJSON(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
func messageHash(value string) string {
	b := sha256.Sum256([]byte(value))
	return hex.EncodeToString(b[:])
}
func messageThreadID(a, b int64) string {
	if a > b {
		a, b = b, a
	}
	return fmt.Sprintf("C.%d.%d", a, b)
}
func messageUserStateID(user int64) string { return fmt.Sprintf("state.%d", user) }
func messageOwnRead(t *models.MessageThread, user int64) int64 {
	if t.A == user {
		return t.ReadA
	}
	return t.ReadB
}
func messageOwnBlocked(t *models.MessageThread, user int64) *time.Time {
	if t.A == user {
		return t.BlockedA
	}
	return t.BlockedB
}
func (s *MessageService) userState(ctx context.Context, user int64) (*models.MessageUserState, error) {
	row, err := s.repo.GetMessageRecord(ctx, messageUserStateID(user))
	if errors.Is(err, repository.ErrNotFound) {
		return &models.MessageUserState{}, nil
	}
	if err != nil {
		return nil, err
	}
	var state models.MessageUserState
	err = json.Unmarshal(row.Payload, &state)
	return &state, err
}
func (s *MessageService) ReadAll(ctx context.Context, user int64, cutoff time.Time) error {
	now := messageNow()
	if cutoff.IsZero() || cutoff.After(now) {
		return invalidArg("invalid message cutoff")
	}
	for n := 0; n < 12; n++ {
		row, err := s.repo.GetMessageRecord(ctx, messageUserStateID(user))
		if errors.Is(err, repository.ErrNotFound) {
			row = &models.MessageRecord{ID: messageUserStateID(user), Kind: "user_state", UserID: user, CreatedAt: now, Active: true}
		} else if err != nil {
			return err
		}
		var state models.MessageUserState
		if len(row.Payload) > 0 {
			if err := json.Unmarshal(row.Payload, &state); err != nil {
				return err
			}
		}
		if !cutoff.After(state.AllReadAt) {
			return nil
		}
		state.AllReadAt = cutoff
		expected := row.Revision
		row.Revision++
		row.Pending = true
		row.Payload = messageJSON(state)
		if err = s.repo.PutMessageRecord(ctx, row, expected); !errors.Is(err, repository.ErrConflict) {
			return err
		}
	}
	return repository.ErrConflict
}

type messageCursor struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

func messageEncodeCursor(row *models.MessageRecord) string {
	return base64.RawURLEncoding.EncodeToString(messageJSON(messageCursor{row.CreatedAt, row.ID}))
}
func messageApplyCursor(f *repository.MessageFilter, cursor string) error {
	if cursor == "" {
		return nil
	}
	if len(cursor) > 512 {
		return invalidArg("invalid message cursor")
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	var v messageCursor
	if err != nil || json.Unmarshal(raw, &v) != nil || v.At.IsZero() || v.ID == "" {
		return invalidArg("invalid message cursor")
	}
	f.Before, f.BeforeID = v.At, v.ID
	return nil
}
func messagePageRows(rows []*models.MessageRecord, limit int) ([]*models.MessageRecord, bool, string) {
	if len(rows) > limit {
		return rows[:limit], true, messageEncodeCursor(rows[limit-1])
	}
	return rows, false, ""
}
func messagePageLimit(limit int) int {
	if limit < 1 || limit > 100 {
		return 30
	}
	return limit
}

func (s *MessageService) noticeFilter(ctx context.Context, user int64, unread bool) (repository.MessageFilter, error) {
	f := repository.MessageFilter{Kinds: []string{"notice"}, ActiveOnly: true, Audience: strconv.FormatInt(user, 10), MaxTime: messageNow()}
	if unread {
		state, err := s.userState(ctx, user)
		if err != nil {
			return f, err
		}
		profile, err := s.users.FindByID(ctx, user)
		if err != nil {
			return f, err
		}
		f.After = state.AllReadAt
		if profile.CreatedAt.After(f.After) {
			f.After = profile.CreatedAt
		}
		f.ExcludeNoticeReadUser = user
	}
	return f, nil
}
func (s *MessageService) ListNotices(ctx context.Context, user int64, cursor string, limit int) (*models.MessagePage[*models.AnnouncementView], error) {
	limit = messagePageLimit(limit)
	f, err := s.noticeFilter(ctx, user, false)
	if err != nil {
		return nil, err
	}
	if err = messageApplyCursor(&f, cursor); err != nil {
		return nil, err
	}
	f.Limit = limit + 1
	rows, err := s.repo.ListMessageRecords(ctx, f)
	if err != nil {
		return nil, err
	}
	rows, more, next := messagePageRows(rows, limit)
	keys := make([]string, len(rows))
	for i, row := range rows {
		keys[i] = repository.MessageNoticeReadKey(user, row.ID)
	}
	marks, err := s.repo.GetMessageRecords(ctx, keys)
	if err != nil {
		return nil, err
	}
	state, err := s.userState(ctx, user)
	if err != nil {
		return nil, err
	}
	profile, err := s.users.FindByID(ctx, user)
	if err != nil {
		return nil, err
	}
	out := &models.MessagePage[*models.AnnouncementView]{Items: []*models.AnnouncementView{}, HasMore: more, NextCursor: next}
	for _, row := range rows {
		var v models.Announcement
		if err := json.Unmarshal(row.Payload, &v); err != nil {
			return nil, err
		}
		v.Revision = row.Revision
		v.Unread = row.CreatedAt.After(state.AllReadAt) && row.CreatedAt.After(profile.CreatedAt) && marks[repository.MessageNoticeReadKey(user, row.ID)] == nil
		out.Items = append(out.Items, &models.AnnouncementView{ID: v.ID, Kind: v.Kind, Title: v.Title, Body: v.Body, H5URL: v.H5URL, PublishAt: v.PublishAt, ExpiresAt: v.ExpiresAt, Unread: v.Unread})
	}
	return out, nil
}
func (s *MessageService) ReadNotices(ctx context.Context, user int64, ids []string) error {
	if len(ids) > 100 {
		return invalidArg("too many notice IDs")
	}
	rows, err := s.repo.GetMessageRecords(ctx, ids)
	if err != nil {
		return err
	}
	now := messageNow()
	for _, id := range ids {
		row := rows[id]
		if row == nil || row.Kind != "notice" || !row.Active || row.CreatedAt.After(now) {
			continue
		}
		allowed := len(row.Targets) == 0
		for _, target := range row.Targets {
			if target == strconv.FormatInt(user, 10) {
				allowed = true
			}
		}
		if !allowed {
			continue
		}
		mark := &models.MessageRecord{ID: repository.MessageNoticeReadKey(user, id), Kind: "notice_read", UserID: user, ThreadID: id, CreatedAt: now, Active: true, Pending: true, Revision: 1, Payload: json.RawMessage(`{}`)}
		if err := s.repo.PutMessageRecord(ctx, mark, 0); err != nil && !errors.Is(err, repository.ErrConflict) {
			return err
		}
	}
	return nil
}

func messageInteractionKinds(category string) ([]string, error) {
	switch category {
	case "", "all":
		return interactionKinds, nil
	case "comments":
		return []string{"comment", "reply"}, nil
	case "reactions":
		return []string{"like", "collect", "comment_like"}, nil
	default:
		return nil, invalidArg("invalid message category")
	}
}
func (s *MessageService) interactionFilter(ctx context.Context, user int64, category string, unread bool) (repository.MessageFilter, error) {
	kinds, err := messageInteractionKinds(category)
	f := repository.MessageFilter{Kinds: kinds, UserID: user, ActiveOnly: true}
	if err != nil {
		return f, err
	}
	if unread {
		state, err := s.userState(ctx, user)
		if err != nil {
			return f, err
		}
		f.UnreadOnly = true
		f.After = state.AllReadAt
	}
	return f, nil
}
func (s *MessageService) ListInteractions(ctx context.Context, user int64, category, cursor string, limit int) (*models.MessagePage[*models.InboxInteraction], error) {
	f, err := s.interactionFilter(ctx, user, category, false)
	if err != nil {
		return nil, err
	}
	if err = messageApplyCursor(&f, cursor); err != nil {
		return nil, err
	}
	limit = messagePageLimit(limit)
	f.Limit = limit + 1
	rows, err := s.repo.ListMessageRecords(ctx, f)
	if err != nil {
		return nil, err
	}
	rows, more, next := messagePageRows(rows, limit)
	state, err := s.userState(ctx, user)
	if err != nil {
		return nil, err
	}
	out := &models.MessagePage[*models.InboxInteraction]{Items: []*models.InboxInteraction{}, HasMore: more, NextCursor: next}
	var actorIDs []int64
	var trackIDs []string
	for _, row := range rows {
		var v models.InboxInteraction
		if err := json.Unmarshal(row.Payload, &v); err != nil {
			return nil, err
		}
		v.Unread = !row.FlagA && row.CreatedAt.After(state.AllReadAt)
		actorIDs = append(actorIDs, v.ActorID)
		if v.TrackID != "" {
			trackIDs = append(trackIDs, v.TrackID)
		}
		out.Items = append(out.Items, &v)
	}
	people, err := s.users.FindByIDs(ctx, actorIDs)
	if err != nil {
		return nil, err
	}
	tracks, err := s.tracks.FindByIDs(ctx, trackIDs)
	if err != nil {
		return nil, err
	}
	for _, v := range out.Items {
		v.Actor = s.messageUserView(ctx, v.ActorID, people[v.ActorID])
		if v.TrackID != "" {
			v.Track = messageTrackView(user, v.TrackID, tracks[v.TrackID])
		}
	}
	return out, nil
}
func (s *MessageService) ReadInteractions(ctx context.Context, user int64, ids []string) error {
	if len(ids) > 100 {
		return invalidArg("too many message IDs")
	}
	for _, id := range ids {
		done := false
		for n := 0; n < 8; n++ {
			row, err := s.repo.GetMessageRecord(ctx, id)
			if errors.Is(err, repository.ErrNotFound) {
				done = true
				break
			}
			if err != nil {
				return err
			}
			if row.UserID != user {
				return repository.ErrForbidden
			}
			valid := false
			for _, kind := range interactionKinds {
				if row.Kind == kind {
					valid = true
				}
			}
			if !valid {
				return repository.ErrForbidden
			}
			if row.FlagA {
				done = true
				break
			}
			expected := row.Revision
			row.Revision++
			row.FlagA = true
			row.Pending = true
			err = s.repo.PutMessageRecord(ctx, row, expected)
			if errors.Is(err, repository.ErrConflict) {
				continue
			}
			if err != nil {
				return err
			}
			done = true
			break
		}
		if !done {
			return repository.ErrConflict
		}
	}
	return nil
}

// EmitInteraction is idempotent per business action; toggling a relation never creates unread spam.
func (s *MessageService) EmitInteraction(ctx context.Context, kind string, actor, recipient int64, trackID, commentID, text, key string, active bool) error {
	if !s.Enabled() || actor <= 0 || recipient <= 0 || actor == recipient {
		return nil
	}
	id := "E." + messageHash(kind+":"+key+":"+strconv.FormatInt(recipient, 10))
	for n := 0; n < 8; n++ {
		row, err := s.repo.GetMessageRecord(ctx, id)
		if errors.Is(err, repository.ErrNotFound) {
			if !active {
				return nil
			}
			now := messageNow()
			v := models.InboxInteraction{ID: id, Kind: kind, ActorID: actor, TrackID: trackID, CommentID: commentID, Text: text, CreatedAt: now}
			row = &models.MessageRecord{ID: id, Kind: kind, UserID: recipient, PeerID: actor, ThreadID: trackID, CreatedAt: now, Active: true, Pending: true, Payload: messageJSON(v)}
		} else if err != nil {
			return err
		} else if row.Active == active {
			return nil
		} else {
			row.Active = active
			row.Pending = true
		}
		expected := row.Revision
		row.Revision++
		err = s.repo.PutMessageRecord(ctx, row, expected)
		if !errors.Is(err, repository.ErrConflict) {
			return err
		}
	}
	return repository.ErrConflict
}
func messageTrackView(user int64, id string, t *models.Track) *models.MessageTrack {
	v := &models.MessageTrack{ID: id, Title: "轨迹已不可用"}
	if t != nil && t.Status != models.TrackStatusDeleted && (t.Status == models.TrackStatusNormal || t.UserID == user) {
		v.Available = true
		v.Title = t.Title
		v.PreviewURL = t.PreviewImages.LandscapeDayURL
	}
	return v
}

func (s *MessageService) Summary(ctx context.Context, user int64) (*models.InboxSummary, error) {
	out := &models.InboxSummary{Cutoff: messageNow()}
	filter, err := s.interactionFilter(ctx, user, "all", true)
	if err != nil {
		return nil, err
	}
	filter.GroupByKind = true
	counts, err := s.repo.GroupMessageCounts(ctx, filter)
	if err != nil {
		return nil, err
	}
	for _, n := range counts {
		out.Interactions += n
	}
	out.Comments = counts["comment"] + counts["reply"]
	out.Reactions = counts["like"] + counts["collect"] + counts["comment_like"]
	nf, err := s.noticeFilter(ctx, user, true)
	if err != nil {
		return nil, err
	}
	out.System, err = s.repo.CountMessageRecords(ctx, nf)
	if err != nil {
		return nil, err
	}
	notices, err := s.ListNotices(ctx, user, "", 1)
	if err != nil {
		return nil, err
	}
	if len(notices.Items) > 0 {
		notice := notices.Items[0]
		out.SystemLatest = &models.AnnouncementPreview{ID: notice.ID, Title: notice.Title, PublishAt: notice.PublishAt}
	}
	var cursor string
	for {
		f := repository.MessageFilter{Kinds: []string{"thread"}, UserID: user, EitherUser: true, HasMessages: true, Limit: 200}
		if err = messageApplyCursor(&f, cursor); err != nil {
			return nil, err
		}
		rows, err := s.repo.ListMessageRecords(ctx, f)
		if err != nil {
			return nil, err
		}
		counts, err := s.threadUnread(ctx, user, rows)
		if err != nil {
			return nil, err
		}
		for _, n := range counts {
			out.Private += n
		}
		if len(rows) < 200 {
			break
		}
		cursor = messageEncodeCursor(rows[len(rows)-1])
	}
	conversations, err := s.ListConversations(ctx, user, "", 1, false)
	if err != nil {
		return nil, err
	}
	if len(conversations.Items) > 0 {
		out.PrivateLatest = conversations.Items[0]
	}
	out.Total = out.System + out.Private + out.Interactions
	return out, nil
}

func (s *MessageService) repairPending(ctx context.Context) {
	rows, err := s.repo.ListMessageRecords(ctx, repository.MessageFilter{Kinds: []string{"thread"}, PendingOnly: true, Limit: 100, Asc: true})
	if err != nil {
		log.Printf("message journal scan failed: %v", err)
		return
	}
	for _, row := range rows {
		var t models.MessageThread
		if json.Unmarshal(row.Payload, &t) == nil {
			if err := s.flushThread(ctx, row, &t); err != nil {
				log.Printf("message journal flush failed: %v", err)
			}
		}
	}
}

func (s *MessageService) queueUserRefresh(ctx context.Context, user int64) {
	for n := 0; n < 5; n++ {
		row, err := s.repo.GetMessageRecord(ctx, messageUserStateID(user))
		if errors.Is(err, repository.ErrNotFound) {
			row = &models.MessageRecord{ID: messageUserStateID(user), Kind: "user_state", UserID: user, CreatedAt: messageNow(), Active: true, Payload: messageJSON(models.MessageUserState{})}
		} else if err != nil {
			log.Printf("inbox refresh deferred: %v", err)
			return
		}
		expected := row.Revision
		row.Revision++
		row.Pending = true
		err = s.repo.PutMessageRecord(ctx, row, expected)
		if errors.Is(err, repository.ErrConflict) {
			continue
		}
		if err != nil {
			log.Printf("inbox refresh deferred: %v", err)
		}
		return
	}
}
