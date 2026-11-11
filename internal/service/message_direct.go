package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

func (s *MessageService) getThread(ctx context.Context, user, peer int64) (*models.MessageRecord, *models.MessageThread, error) {
	if user <= 0 || peer <= 0 || user == peer {
		return nil, nil, invalidArg("invalid message peer")
	}
	id := messageThreadID(user, peer)
	row, err := s.repo.GetMessageRecord(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		a, b := user, peer
		if a > b {
			a, b = b, a
		}
		return &models.MessageRecord{ID: id, Kind: "thread", UserID: a, PeerID: b, ThreadID: id, CreatedAt: messageNow(), Active: true}, &models.MessageThread{A: a, B: b}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var thread models.MessageThread
	if err = json.Unmarshal(row.Payload, &thread); err != nil {
		return nil, nil, err
	}
	if thread.A != user && thread.B != user {
		return nil, nil, repository.ErrForbidden
	}
	return row, &thread, nil
}
func (s *MessageService) saveThread(ctx context.Context, row *models.MessageRecord, thread *models.MessageThread) error {
	expected := row.Revision
	row.Revision++
	row.Pending = thread.Pending != nil
	row.FlagA = thread.BlockedA != nil
	row.FlagB = thread.BlockedB != nil
	row.Payload = messageJSON(thread)
	return s.repo.PutMessageRecord(ctx, row, expected)
}
func messageOwnClear(row *models.MessageRecord, user int64) int64 {
	if row.UserID == user {
		return row.ClearA
	}
	return row.ClearB
}

func (s *MessageService) ClearConversation(ctx context.Context, user, peer int64) (*models.MessageClearResult, error) {
	for n := 0; n < 12; n++ {
		row, thread, err := s.getThread(ctx, user, peer)
		if err != nil {
			return nil, err
		}
		if thread.Pending != nil {
			if err := s.flushThread(ctx, row, thread); err != nil {
				return nil, err
			}
			continue
		}
		through := row.Seq
		if messageOwnClear(row, user) >= through {
			return &models.MessageClearResult{ClearedThrough: through}, nil
		}
		if thread.A == user {
			row.ClearA = through
			thread.ReadA = max(thread.ReadA, through)
		} else {
			row.ClearB = through
			thread.ReadB = max(thread.ReadB, through)
		}
		if err := s.saveThread(ctx, row, thread); errors.Is(err, repository.ErrConflict) {
			continue
		} else if err != nil {
			return nil, err
		}
		s.queueUserRefresh(ctx, user)
		return &models.MessageClearResult{ClearedThrough: through}, nil
	}
	return nil, repository.ErrConflict
}

func (s *MessageService) mutual(ctx context.Context, a, b int64) (bool, error) {
	if s.follows == nil {
		return false, nil
	}
	first, err := s.follows.IsFollowing(ctx, a, b)
	if err != nil || !first {
		return false, err
	}
	return s.follows.IsFollowing(ctx, b, a)
}
func (s *MessageService) conversationView(ctx context.Context, user int64, row *models.MessageRecord, thread *models.MessageThread, peer *models.User, unread int64, checkQuota bool) (*models.MessageConversation, error) {
	peerID := thread.A
	if peerID == user {
		peerID = thread.B
	}
	v := &models.MessageConversation{Peer: s.messageUserView(ctx, peerID, peer), Last: thread.Last, Unread: unread, Blocked: messageOwnBlocked(thread, user) != nil, BlockedAt: messageOwnBlocked(thread, user), CanSend: true, ClearedThrough: messageOwnClear(row, user)}
	if v.Last != nil && v.Last.Seq <= v.ClearedThrough {
		v.Last = nil
	}
	if v.Last != nil {
		copy := *v.Last
		letters := []rune(copy.Text)
		if len(letters) > 120 {
			copy.Text = string(letters[:120]) + "…"
		}
		v.Last = &copy
	}
	v.ReadSeq = max(messageOwnRead(thread, user), v.ClearedThrough)
	if checkQuota {
		state, err := s.userState(ctx, user)
		if err != nil {
			return nil, err
		}
		if !state.AllReadAt.IsZero() {
			records, err := s.repo.ListMessageRecords(ctx, repository.MessageFilter{Kinds: []string{"dm"}, UserID: user, ThreadID: row.ID, MaxTime: state.AllReadAt, Limit: 1})
			if err != nil {
				return nil, err
			}
			if len(records) > 0 && records[0].Seq > v.ReadSeq {
				v.ReadSeq = records[0].Seq
			}
		}
	}
	if peer == nil {
		v.CanSend = false
		v.SendHint = "用户已注销"
		return v, nil
	}
	if thread.BlockedA != nil || thread.BlockedB != nil {
		v.CanSend = false
		if v.Blocked {
			v.SendHint = "已拉黑对方，解除后可继续私信"
		} else {
			v.SendHint = "暂时无法向对方发送消息"
		}
	} else if checkQuota && !thread.Replied && thread.Initiator == user && row.Seq > 0 {
		mutual, err := s.mutual(ctx, user, peerID)
		if err != nil {
			return nil, err
		}
		if !mutual {
			v.CanSend = false
			v.SendHint = "对方回复后可继续发送"
		}
	}
	return v, nil
}
func (s *MessageService) threadUnread(ctx context.Context, user int64, rows []*models.MessageRecord) (map[string]int64, error) {
	after := map[string]int64{}
	for _, row := range rows {
		var t models.MessageThread
		if err := json.Unmarshal(row.Payload, &t); err != nil {
			return nil, err
		}
		if t.Pending != nil {
			if err := s.flushThread(ctx, row, &t); err != nil {
				return nil, err
			}
		}
		after[row.ID] = max(messageOwnRead(&t, user), messageOwnClear(row, user))
	}
	if len(after) == 0 {
		return map[string]int64{}, nil
	}
	state, err := s.userState(ctx, user)
	if err != nil {
		return nil, err
	}
	return s.repo.GroupMessageCounts(ctx, repository.MessageFilter{Kinds: []string{"dm"}, UserID: user, ThreadsAfter: after, After: state.AllReadAt, ActiveOnly: true})
}
func (s *MessageService) ListConversations(ctx context.Context, user int64, cursor string, limit int, blocked bool) (*models.MessagePage[*models.MessageConversation], error) {
	f := repository.MessageFilter{Kinds: []string{"thread"}, UserID: user, EitherUser: true, HasMessages: !blocked}
	if blocked {
		f.BlockedBy = user
	} else {
		f.VisibleTo = user
	}
	if err := messageApplyCursor(&f, cursor); err != nil {
		return nil, err
	}
	limit = messagePageLimit(limit)
	f.Limit = limit + 1
	rows, err := s.repo.ListMessageRecords(ctx, f)
	if err != nil {
		return nil, err
	}
	rows, more, next := messagePageRows(rows, limit)
	var ids []int64
	threads := make([]*models.MessageThread, len(rows))
	for i, row := range rows {
		var t models.MessageThread
		if err = json.Unmarshal(row.Payload, &t); err != nil {
			return nil, err
		}
		threads[i] = &t
		if t.A == user {
			ids = append(ids, t.B)
		} else {
			ids = append(ids, t.A)
		}
	}
	people, err := s.users.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	counts, err := s.threadUnread(ctx, user, rows)
	if err != nil {
		return nil, err
	}
	out := &models.MessagePage[*models.MessageConversation]{Items: []*models.MessageConversation{}, HasMore: more, NextCursor: next}
	for i, row := range rows {
		peerID := threads[i].A
		if peerID == user {
			peerID = threads[i].B
		}
		v, err := s.conversationView(ctx, user, row, threads[i], people[peerID], counts[row.ID], false)
		if err != nil {
			return nil, err
		}
		out.Items = append(out.Items, v)
	}
	return out, nil
}
func (s *MessageService) GetConversation(ctx context.Context, user, peer int64) (*models.MessageConversation, error) {
	row, t, err := s.getThread(ctx, user, peer)
	if err != nil {
		return nil, err
	}
	profile, err := s.users.FindByID(ctx, peer)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	var count int64
	if row.Revision > 0 {
		counts, err := s.threadUnread(ctx, user, []*models.MessageRecord{row})
		if err != nil {
			return nil, err
		}
		count = counts[row.ID]
	}
	return s.conversationView(ctx, user, row, t, profile, count, true)
}
func (s *MessageService) SetBlocked(ctx context.Context, user, peer int64, blocked bool) error {
	if blocked {
		if _, err := s.users.FindByID(ctx, peer); err != nil {
			return err
		}
	}
	for n := 0; n < 12; n++ {
		row, t, err := s.getThread(ctx, user, peer)
		if err != nil {
			return err
		}
		current := messageOwnBlocked(t, user)
		if (current != nil) == blocked {
			return nil
		}
		var at *time.Time
		if blocked {
			now := messageNow()
			at = &now
		}
		if t.A == user {
			t.BlockedA = at
			if blocked {
				t.ReadA = max(t.ReadA, row.Seq)
			}
		} else {
			t.BlockedB = at
			if blocked {
				t.ReadB = max(t.ReadB, row.Seq)
			}
		}
		err = s.saveThread(ctx, row, t)
		if !errors.Is(err, repository.ErrConflict) {
			if err == nil {
				s.queueUserRefresh(ctx, user)
				s.queueUserRefresh(ctx, peer)
			}
			return err
		}
	}
	return repository.ErrConflict
}

type SendDirectMessageInput struct {
	ClientID string `json:"client_id"`
	Text     string `json:"text"`
	TrackID  string `json:"track_id"`
}

func (s *MessageService) SendDirect(ctx context.Context, user, peer int64, in SendDirectMessageInput) (*models.DirectMessage, error) {
	if s.restrictions != nil {
		if err := s.restrictions.EnsureAllowed(ctx, user, "账号已被限制，暂时不能发送私信"); err != nil {
			return nil, err
		}
	}
	in.ClientID = strings.TrimSpace(in.ClientID)
	in.Text = strings.TrimSpace(in.Text)
	in.TrackID = strings.TrimSpace(in.TrackID)
	if len(in.ClientID) < 8 || len(in.ClientID) > 80 || utf8.RuneCountInString(in.Text) > 1000 || in.Text == "" && in.TrackID == "" {
		return nil, invalidArg("私信内容无效（文字最多1000字）")
	}
	id := "D." + messageHash(fmt.Sprintf("%d:%d:%s", user, peer, in.ClientID))
	fingerprint := messageHash(fmt.Sprintf("%d:%s:%s", peer, in.Text, in.TrackID))
	if old, err := s.repo.GetMessageRecord(ctx, id); err == nil {
		var stored models.StoredDirectMessage
		if json.Unmarshal(old.Payload, &stored) != nil {
			return nil, errors.New("invalid stored message")
		}
		if stored.RequestHash != fingerprint || stored.Message.SenderID != user {
			return nil, repository.ErrConflict
		}
		threadRow, _, threadErr := s.getThread(ctx, user, peer)
		if threadErr != nil {
			return nil, threadErr
		}
		if stored.Message.Seq <= messageOwnClear(threadRow, user) {
			return nil, repository.ErrConflict
		}
		if err := s.decorateDirect(ctx, user, []*models.DirectMessage{&stored.Message}); err != nil {
			return nil, err
		}
		return &stored.Message, nil
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	if _, err := s.users.FindByID(ctx, peer); err != nil {
		return nil, err
	}
	if in.TrackID != "" {
		track, err := s.tracks.FindByID(ctx, in.TrackID)
		if err != nil {
			return nil, err
		}
		if !likeEligible(track) {
			return nil, invalidArg("仅支持分享已公开且已结束的轨迹")
		}
	}
	for n := 0; n < 16; n++ {
		row, t, err := s.getThread(ctx, user, peer)
		if err != nil {
			return nil, err
		}
		if old, lookupErr := s.repo.GetMessageRecord(ctx, id); lookupErr == nil {
			var stored models.StoredDirectMessage
			if json.Unmarshal(old.Payload, &stored) != nil {
				return nil, errors.New("invalid stored message")
			}
			if stored.RequestHash != fingerprint {
				return nil, repository.ErrConflict
			}
			if stored.Message.Seq <= messageOwnClear(row, user) {
				return nil, repository.ErrConflict
			}
			if err := s.decorateDirect(ctx, user, []*models.DirectMessage{&stored.Message}); err != nil {
				return nil, err
			}
			return &stored.Message, nil
		} else if !errors.Is(lookupErr, repository.ErrNotFound) {
			return nil, lookupErr
		}
		if t.Pending != nil {
			if t.Pending.Message.ID == id {
				if t.Pending.RequestHash != fingerprint {
					return nil, repository.ErrConflict
				}
				if err = s.flushThread(ctx, row, t); err != nil {
					return nil, err
				}
				msg := t.Pending.Message
				if err := s.decorateDirect(ctx, user, []*models.DirectMessage{&msg}); err != nil {
					return nil, err
				}
				return &msg, nil
			}
			if err = s.flushThread(ctx, row, t); err != nil {
				return nil, err
			}
			continue
		}
		if t.BlockedA != nil || t.BlockedB != nil {
			return nil, invalidArg("暂时无法向对方发送消息")
		}
		if t.Last != nil && t.Last.SenderID == user && time.Since(t.Last.CreatedAt) < time.Second {
			return nil, invalidArg("发送太快了，请稍后再试")
		}
		if !t.Replied && t.Initiator == user && row.Seq > 0 {
			mutual, err := s.mutual(ctx, user, peer)
			if err != nil {
				return nil, err
			}
			if !mutual {
				return nil, invalidArg("对方回复后可继续发送")
			}
		}
		now := messageNow()
		if t.Last != nil && !now.After(t.Last.CreatedAt) {
			now = t.Last.CreatedAt.Add(time.Microsecond)
		}
		msg := models.DirectMessage{ID: id, ClientID: in.ClientID, ThreadID: row.ID, Seq: row.Seq + 1, SenderID: user, ReceiverID: peer, Text: in.Text, TrackID: in.TrackID, CreatedAt: now}
		if t.Initiator == 0 {
			t.Initiator = user
		} else if t.Initiator != user {
			t.Replied = true
		}
		t.Last = &msg
		t.Pending = &models.StoredDirectMessage{Message: msg, RequestHash: fingerprint}
		row.Seq = msg.Seq
		row.CreatedAt = now
		if err = s.saveThread(ctx, row, t); errors.Is(err, repository.ErrConflict) {
			continue
		} else if err != nil {
			return nil, err
		}
		if err = s.flushThread(ctx, row, t); err != nil {
			return nil, err
		}
		if err := s.decorateDirect(ctx, user, []*models.DirectMessage{&msg}); err != nil {
			return nil, err
		}
		return &msg, nil
	}
	return nil, repository.ErrConflict
}
func (s *MessageService) flushThread(ctx context.Context, row *models.MessageRecord, t *models.MessageThread) error {
	if t.Pending == nil {
		return nil
	}
	pending := t.Pending
	msg := pending.Message
	stored := &models.MessageRecord{ID: msg.ID, Kind: "dm", UserID: msg.ReceiverID, PeerID: msg.SenderID, ThreadID: row.ID, Seq: msg.Seq, CreatedAt: msg.CreatedAt, Active: true, Pending: true, Revision: 1, Payload: messageJSON(pending)}
	if err := s.repo.PutMessageRecord(ctx, stored, 0); err != nil {
		if !errors.Is(err, repository.ErrConflict) {
			return err
		}
		existing, err := s.repo.GetMessageRecord(ctx, msg.ID)
		if err != nil {
			return err
		}
		var prior models.StoredDirectMessage
		if json.Unmarshal(existing.Payload, &prior) != nil || existing.Kind != "dm" || existing.ThreadID != row.ID || prior.RequestHash != pending.RequestHash || prior.Message.Seq != msg.Seq {
			return repository.ErrConflict
		}
	}
	copyThread := *t
	copyThread.Pending = nil
	err := s.saveThread(ctx, row, &copyThread)
	if errors.Is(err, repository.ErrConflict) {
		return nil
	}
	return err
}
func (s *MessageService) ListDirect(ctx context.Context, user, peer int64, cursor string, afterSeq int64, limit int) (*models.MessagePage[*models.DirectMessage], error) {
	row, t, err := s.getThread(ctx, user, peer)
	if err != nil {
		return nil, err
	}
	if t.Pending != nil {
		if err = s.flushThread(ctx, row, t); err != nil {
			return nil, err
		}
	}
	limit = messagePageLimit(limit)
	visibleAfter := max(afterSeq, messageOwnClear(row, user))
	f := repository.MessageFilter{Kinds: []string{"dm"}, ThreadID: row.ID, ActiveOnly: true, Limit: limit + 1, AfterSeq: visibleAfter, Asc: afterSeq >= 0}
	if cursor != "" {
		before, e := strconv.ParseInt(strings.TrimPrefix(cursor, "seq:"), 10, 64)
		if e != nil || before < 1 {
			return nil, invalidArg("invalid chat cursor")
		}
		f.BeforeSeq = before
	}
	rows, err := s.repo.ListMessageRecords(ctx, f)
	if err != nil {
		return nil, err
	}
	var more bool
	var next string
	if afterSeq >= 0 {
		if len(rows) > limit {
			rows = rows[:limit]
			more = true
		}
	} else {
		if len(rows) > limit {
			rows = rows[:limit]
			more = true
			next = "seq:" + strconv.FormatInt(rows[limit-1].Seq, 10)
		}
	}
	out := &models.MessagePage[*models.DirectMessage]{Items: []*models.DirectMessage{}, HasMore: more, NextCursor: next}
	for _, record := range rows {
		var stored models.StoredDirectMessage
		if err = json.Unmarshal(record.Payload, &stored); err != nil {
			return nil, err
		}
		out.Items = append(out.Items, &stored.Message)
	}
	sort.Slice(out.Items, func(i, j int) bool { return out.Items[i].Seq < out.Items[j].Seq })
	if err = s.decorateDirect(ctx, user, out.Items); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *MessageService) decorateDirect(ctx context.Context, user int64, items []*models.DirectMessage) error {
	var ids []string
	for _, v := range items {
		if v.TrackID != "" {
			ids = append(ids, v.TrackID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	tracks, err := s.messageTrackViews(ctx, user, ids)
	if err != nil {
		return err
	}
	for _, v := range items {
		if v.TrackID != "" {
			v.Track = tracks[v.TrackID]
		}
	}
	return nil
}
func (s *MessageService) ReadConversation(ctx context.Context, user, peer, throughSeq int64) error {
	if throughSeq < 1 {
		return nil
	}
	initial, _, err := s.getThread(ctx, user, peer)
	if err != nil {
		return err
	}
	if throughSeq > initial.Seq {
		throughSeq = initial.Seq
	}
	if throughSeq < 1 {
		return nil
	}
	f := repository.MessageFilter{Kinds: []string{"dm"}, UserID: user, ThreadID: messageThreadID(user, peer), BeforeSeq: throughSeq + 1, ActiveOnly: true, Limit: 1}
	messages, err := s.repo.ListMessageRecords(ctx, f)
	if err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}
	seen := messages[0].Seq
	for n := 0; n < 12; n++ {
		row, t, err := s.getThread(ctx, user, peer)
		if err != nil {
			return err
		}
		if seen <= messageOwnRead(t, user) {
			return nil
		}
		if t.A == user {
			t.ReadA = seen
		} else {
			t.ReadB = seen
		}
		err = s.saveThread(ctx, row, t)
		if !errors.Is(err, repository.ErrConflict) {
			if err == nil {
				s.queueUserRefresh(ctx, user)
			}
			return err
		}
	}
	return repository.ErrConflict
}
