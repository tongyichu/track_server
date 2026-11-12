package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type RealtimeMessage struct {
	ID         string    `json:"id"`
	Seq        string    `json:"seq,omitempty"`
	SenderID   string    `json:"sender_id"`
	ReceiverID string    `json:"receiver_id,omitempty"`
	Content    string    `json:"content"`
	TrackID    string    `json:"track_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type RealtimeConversation struct {
	PeerID    string    `json:"peer_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type RealtimeChatPage struct {
	models.MessagePage[RealtimeMessage]
	LatestSeq    string `json:"latest_seq"`
	LatestCursor string `json:"latest_cursor,omitempty"`
}

func (s *MessageService) RealtimeAccountMessages(ctx context.Context, user int64, cursor, after string) (*RealtimeChatPage, error) {
	if !s.Enabled() || !s.durable || user <= 0 {
		return nil, repository.ErrForbidden
	}
	if cursor != "" && after != "" {
		return nil, invalidArg("历史和增量游标不能同时使用")
	}
	f := repository.MessageFilter{Kinds: []string{"dm"}, UserID: user, EitherUser: true, ActiveOnly: true, Limit: 31, Asc: after != ""}
	if err := messageApplyCursor(&f, cursor); err != nil {
		return nil, err
	}
	if after != "" && after != "origin" {
		var boundary repository.MessageFilter
		if err := messageApplyCursor(&boundary, after); err != nil {
			return nil, err
		}
		f.After, f.AfterID = boundary.Before, boundary.BeforeID
	}
	rows, err := s.repo.ListMessageRecords(ctx, f)
	if err != nil {
		return nil, err
	}
	out := &RealtimeChatPage{MessagePage: models.MessagePage[RealtimeMessage]{Items: []RealtimeMessage{}, HasMore: len(rows) > 30}, LatestCursor: after}
	if len(rows) > 30 {
		rows = rows[:30]
	}
	if !f.Asc {
		if len(rows) > 0 {
			out.LatestCursor = messageEncodeCursor(rows[0])
			if out.HasMore {
				out.NextCursor = messageEncodeCursor(rows[len(rows)-1])
			}
		} else {
			out.LatestCursor = "origin"
		}
		for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
			rows[left], rows[right] = rows[right], rows[left]
		}
	} else if len(rows) > 0 {
		out.LatestCursor = messageEncodeCursor(rows[len(rows)-1])
	}
	for _, row := range rows {
		var stored models.StoredDirectMessage
		if err := json.Unmarshal(row.Payload, &stored); err != nil {
			return nil, err
		}
		v := stored.Message
		out.Items = append(out.Items, RealtimeMessage{ID: row.ID, Seq: strconv.FormatInt(row.Seq, 10), SenderID: strconv.FormatInt(v.SenderID, 10), ReceiverID: strconv.FormatInt(v.ReceiverID, 10), Content: v.Text, TrackID: v.TrackID, CreatedAt: row.CreatedAt})
	}
	return out, nil
}

// Administrative reads must not flush pending journals or update read state.
func (s *MessageService) RealtimeConversations(ctx context.Context, user int64, cursor string) (*models.MessagePage[RealtimeConversation], error) {
	if !s.Enabled() || !s.durable || user <= 0 {
		return nil, repository.ErrForbidden
	}
	f := repository.MessageFilter{Kinds: []string{"thread"}, UserID: user, EitherUser: true, HasMessages: true, ActiveOnly: true, Limit: 31}
	if err := messageApplyCursor(&f, cursor); err != nil {
		return nil, err
	}
	rows, err := s.repo.ListMessageRecords(ctx, f)
	if err != nil {
		return nil, err
	}
	rows, more, next := messagePageRows(rows, 30)
	out := &models.MessagePage[RealtimeConversation]{Items: []RealtimeConversation{}, HasMore: more, NextCursor: next}
	for _, row := range rows {
		peer := row.UserID
		if peer == user {
			peer = row.PeerID
		}
		var thread models.MessageThread
		if err := json.Unmarshal(row.Payload, &thread); err != nil {
			return nil, err
		}
		text := ""
		if thread.Last != nil {
			text = thread.Last.Text
			if text == "" && thread.Last.TrackID != "" {
				text = "[分享轨迹]"
			}
		}
		letters := []rune(text)
		if len(letters) > 80 {
			text = string(letters[:80]) + "…"
		}
		out.Items = append(out.Items, RealtimeConversation{PeerID: strconv.FormatInt(peer, 10), Content: text, CreatedAt: row.CreatedAt})
	}
	return out, nil
}

func (s *MessageService) RealtimeDirectMessages(ctx context.Context, user, peer int64, cursor, after string) (*RealtimeChatPage, error) {
	if !s.Enabled() || !s.durable || user <= 0 {
		return nil, repository.ErrForbidden
	}
	if peer <= 0 || peer == user || cursor != "" && after != "" {
		return nil, invalidArg("对话参数无效")
	}
	f := repository.MessageFilter{Kinds: []string{"dm"}, UserID: user, EitherUser: true, ThreadID: messageThreadID(user, peer), ActiveOnly: true, Limit: 31}
	if cursor != "" {
		seq, err := strconv.ParseInt(strings.TrimPrefix(cursor, "seq:"), 10, 64)
		if err != nil || seq <= 0 {
			return nil, invalidArg("历史消息游标无效")
		}
		f.BeforeSeq = seq
	}
	if after != "" {
		seq, err := strconv.ParseInt(after, 10, 64)
		if err != nil || seq < 0 {
			return nil, invalidArg("增量消息游标无效")
		}
		f.AfterSeq, f.Asc = seq, true
	}
	thread, err := s.repo.GetMessageRecord(ctx, f.ThreadID)
	if err != nil {
		return nil, err
	}
	if thread.Kind != "thread" || !(thread.UserID == user && thread.PeerID == peer || thread.UserID == peer && thread.PeerID == user) {
		return nil, repository.ErrNotFound
	}
	rows, err := s.repo.ListMessageRecords(ctx, f)
	if err != nil {
		return nil, err
	}
	out := &RealtimeChatPage{MessagePage: models.MessagePage[RealtimeMessage]{Items: []RealtimeMessage{}, HasMore: len(rows) > 30}, LatestSeq: after}
	if len(rows) > 30 {
		rows = rows[:30]
	}
	if f.Asc {
		for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
			rows[left], rows[right] = rows[right], rows[left]
		}
	}
	if len(rows) > 0 {
		out.LatestSeq = strconv.FormatInt(rows[0].Seq, 10)
		if out.HasMore && after == "" {
			out.NextCursor = "seq:" + strconv.FormatInt(rows[len(rows)-1].Seq, 10)
		}
	}
	for i := len(rows) - 1; i >= 0; i-- {
		row := rows[i]
		var stored models.StoredDirectMessage
		if err := json.Unmarshal(row.Payload, &stored); err != nil {
			return nil, err
		}
		v := stored.Message
		out.Items = append(out.Items, RealtimeMessage{ID: row.ID, Seq: strconv.FormatInt(row.Seq, 10), SenderID: strconv.FormatInt(v.SenderID, 10), ReceiverID: strconv.FormatInt(v.ReceiverID, 10), Content: v.Text, TrackID: v.TrackID, CreatedAt: row.CreatedAt})
	}
	return out, nil
}

// Use existing local avatars only; opening a conversation never triggers remote cache fills.
func (s *MessageService) RealtimePeople(ctx context.Context, ids []int64) (map[string]models.MessageUser, error) {
	unique := []int64{}
	seen := map[int64]bool{}
	for _, id := range ids {
		if id > 0 && !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	people, err := s.users.FindByIDs(ctx, unique)
	if err != nil {
		return nil, err
	}
	out := map[string]models.MessageUser{}
	for _, id := range unique {
		v := models.MessageUser{ID: id, Nickname: "用户已注销", AvatarURL: defaultAvatarURL(id)}
		if person := people[id]; person != nil {
			v.Nickname = person.Nickname
			if v.Nickname == "" {
				v.Nickname = "用户 " + strconv.FormatInt(id, 10)
			}
			if strings.HasPrefix(person.AvatarURL, "/api/v1/static/avatars/") || strings.HasPrefix(person.AvatarURL, "/api/v1/static/default_avatars/") {
				v.AvatarURL = person.AvatarURL
			} else if s.avatarCache != nil {
				if ext, exists := s.avatarCache.Exists(formatUserAvatarCacheKey(id)); exists {
					v.AvatarURL = s.avatarCache.LocalURL(formatUserAvatarCacheKey(id), ext)
				}
			}
		}
		out[strconv.FormatInt(id, 10)] = v
	}
	return out, nil
}
