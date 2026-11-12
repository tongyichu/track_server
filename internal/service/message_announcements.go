package service

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func (s *MessageService) ListAdminAnnouncements(ctx context.Context, cursor string, limit int) (*models.MessagePage[*models.Announcement], error) {
	limit = messagePageLimit(limit)
	f := repository.MessageFilter{Kinds: []string{"notice", "notice_schedule"}, Limit: limit + 1}
	if err := messageApplyCursor(&f, cursor); err != nil {
		return nil, err
	}
	rows, err := s.repo.ListMessageRecords(ctx, f)
	if err != nil {
		return nil, err
	}
	rows, more, next := messagePageRows(rows, limit)
	out := &models.MessagePage[*models.Announcement]{Items: []*models.Announcement{}, HasMore: more, NextCursor: next}
	for _, row := range rows {
		var v models.Announcement
		if err = json.Unmarshal(row.Payload, &v); err != nil {
			return nil, err
		}
		v.Revision = row.Revision
		v.History = nil
		out.Items = append(out.Items, &v)
	}
	return out, nil
}
func (s *MessageService) GetAdminAnnouncement(ctx context.Context, id string) (*models.Announcement, error) {
	row, err := s.repo.GetMessageRecord(ctx, id)
	if err != nil {
		return nil, err
	}
	if row.Kind != "notice" && row.Kind != "notice_schedule" {
		return nil, repository.ErrNotFound
	}
	var v models.Announcement
	err = json.Unmarshal(row.Payload, &v)
	v.Revision = row.Revision
	if len(v.History) > 50 {
		v.History = v.History[len(v.History)-50:]
	}
	return &v, err
}
func (s *MessageService) SaveAnnouncement(ctx context.Context, actor, id string, expected int64, in models.Announcement) (*models.Announcement, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Body = strings.TrimSpace(in.Body)
	in.H5URL = strings.TrimSpace(in.H5URL)
	if in.Kind != "system" && in.Kind != "activity" || in.Status != "draft" && in.Status != "scheduled" && in.Status != "published" && in.Status != "withdrawn" || in.Title == "" || utf8.RuneCountInString(in.Title) > 100 || in.Body == "" || utf8.RuneCountInString(in.Body) > 8000 || len(in.TargetUserIDs) > 1000 {
		return nil, invalidArg("请填写有效的通知类型、状态、标题和正文")
	}
	if in.H5URL != "" {
		u, err := url.Parse(in.H5URL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || len(in.H5URL) > 2048 {
			return nil, invalidArg("H5链接须为有效的HTTPS地址")
		}
	}
	now := messageNow()
	row := &models.MessageRecord{Kind: "notice", CreatedAt: now}
	old := models.Announcement{}
	if id == "" {
		if expected != 0 || in.Status == "withdrawn" {
			return nil, repository.ErrConflict
		}
		in.ID = newScenicID("N.")
		in.CreatedAt = now
	} else {
		var err error
		row, err = s.repo.GetMessageRecord(ctx, id)
		if err != nil {
			return nil, err
		}
		if row.Kind != "notice" && row.Kind != "notice_schedule" || row.Revision != expected {
			return nil, repository.ErrConflict
		}
		if err = json.Unmarshal(row.Payload, &old); err != nil {
			return nil, err
		}
		// Old scheduled notices used published + a future timestamp.
		if old.Status == "published" && old.PublishAt.After(now) && old.PublishedAt == nil {
			old.Status = "scheduled"
		}
		in.ID, in.CreatedAt = id, old.CreatedAt
	}
	targets := []string{}
	ids := []int64{}
	seen := map[string]bool{}
	for _, raw := range in.TargetUserIDs {
		target, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil || target <= 0 {
			return nil, invalidArg("接收用户ID无效")
		}
		key := strconv.FormatInt(target, 10)
		if !seen[key] {
			seen[key] = true
			targets = append(targets, key)
			ids = append(ids, target)
		}
	}
	if len(ids) > 0 {
		users, err := s.users.FindByIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		if len(users) != len(ids) {
			return nil, invalidArg("部分接收用户不存在")
		}
	}
	in.TargetUserIDs = targets
	live := old.Status == "published"
	if live {
		if in.Status != "published" && in.Status != "withdrawn" {
			return nil, invalidArg("已上线通知请先撤回，再修改发布安排")
		}
		if in.DeliveryMode == "legacy" {
			in.DeliveryMode = ""
		}
		if !in.PublishAt.Equal(old.PublishAt) || !sameMessageTargets(targets, old.TargetUserIDs) || in.DeliveryMode != old.DeliveryMode || !sameNoticeTime(in.DeliveryEndsAt, old.DeliveryEndsAt) {
			return nil, invalidArg("已上线通知的时间、覆盖方式和接收范围不能修改，请先撤回")
		}
		in.PublishMode = old.PublishMode
	} else {
		if in.DeliveryMode == "" || in.DeliveryMode == "legacy" {
			in.DeliveryMode = "once"
		}
		if in.DeliveryMode != "once" && in.DeliveryMode != "persistent" {
			return nil, invalidArg("请选择一次性通知或持续公告")
		}
		if in.PublishMode == "" {
			in.PublishMode = "immediate"
			if in.Status == "scheduled" || in.PublishAt.After(now) {
				in.PublishMode = "scheduled"
			}
		}
		if in.PublishMode != "immediate" && in.PublishMode != "scheduled" {
			return nil, invalidArg("请选择立即发布或定时发布")
		}
		if in.Status == "published" || in.Status == "scheduled" {
			if in.PublishMode == "scheduled" {
				if !in.PublishAt.After(now) {
					return nil, invalidArg("定时发布时间已过去，请重新选择")
				}
				in.Status = "scheduled"
			} else {
				in.Status = "published"
				in.PublishAt = now
			}
		}
	}
	if in.PublishAt.IsZero() {
		in.PublishAt = now
	}
	if in.DeliveryMode != "persistent" {
		in.DeliveryEndsAt = nil
	}
	if in.Status == "published" || in.Status == "scheduled" {
		if in.ExpiresAt != nil && !in.ExpiresAt.After(in.PublishAt) {
			return nil, invalidArg("活动结束时间须晚于发布时间")
		}
		if in.DeliveryEndsAt != nil && !in.DeliveryEndsAt.After(in.PublishAt) {
			return nil, invalidArg("停止发放时间须晚于发布时间")
		}
	}
	in.PublishRound = row.NoticeRound
	in.PublishedAt = old.PublishedAt
	newRound := (in.Status == "published" || in.Status == "scheduled") && old.Status != "published" && old.Status != "scheduled"
	if newRound || !live && (in.Status == "scheduled" || in.Status == "published") && in.PublishRound == 0 {
		in.PublishRound++
		in.PublishedAt = nil
	}
	if in.Status == "published" && !live {
		in.PublishedAt = &now
	}
	in.History = append([]models.AnnouncementHistory(nil), old.History...)
	action := "edited"
	if in.Status != old.Status {
		action = in.Status
	}
	if old.Status == "scheduled" && in.Status == "draft" {
		action = "cancelled"
	}
	in.Revision, in.UpdatedAt, in.UpdatedBy = expected+1, now, actor
	in.History = append(in.History, models.AnnouncementHistory{Action: action, At: now, Actor: actor, Round: in.PublishRound, Revision: in.Revision, PublishAt: in.PublishAt, Title: in.Title})
	row.ID, row.Kind, row.Revision = in.ID, "notice", in.Revision
	if in.Status == "scheduled" {
		row.Kind = "notice_schedule"
	}
	row.Active = in.Status == "published"
	row.Pending = in.Status == "published" || in.Status == "scheduled" || in.Status == "withdrawn"
	row.Targets, row.CreatedAt = targets, in.PublishAt
	if in.Status == "draft" || in.Status == "withdrawn" {
		row.CreatedAt = now
	}
	row.NoticeMode, row.NoticeRound, row.NoticeUntilMicros = in.DeliveryMode, in.PublishRound, 0
	if in.DeliveryEndsAt != nil {
		row.NoticeUntilMicros = in.DeliveryEndsAt.UnixMicro()
	}
	row.Payload = messageJSON(in)
	if err := s.repo.PutMessageRecord(ctx, row, expected); err != nil {
		return nil, err
	}
	if len(in.History) > 50 {
		in.History = in.History[len(in.History)-50:]
	}
	return &in, nil
}

func sameNoticeTime(a, b *time.Time) bool {
	return a == nil && b == nil || a != nil && b != nil && a.Equal(*b)
}

func (s *MessageService) activateScheduledAnnouncements(ctx context.Context) {
	now := messageNow()
	rows, err := s.repo.ListMessageRecords(ctx, repository.MessageFilter{Kinds: []string{"notice_schedule"}, MaxTime: now, Limit: 100, Asc: true})
	if err != nil {
		log.Printf("notice schedule scan failed: %v", err)
		return
	}
	for _, row := range rows {
		if ctx.Err() != nil {
			return
		}
		var notice models.Announcement
		if err = json.Unmarshal(row.Payload, &notice); err != nil {
			log.Printf("invalid scheduled notice %s", row.ID)
			continue
		}
		if notice.Status != "scheduled" {
			continue
		}
		notice.Status = "published"
		// A missed publication window must never briefly go live after restart.
		if notice.DeliveryEndsAt != nil && !now.Before(*notice.DeliveryEndsAt) || notice.ExpiresAt != nil && !now.Before(*notice.ExpiresAt) {
			notice.Status = "expired"
		}
		expected := row.Revision
		row.Revision++
		row.Kind, row.Active, row.Pending = "notice", notice.Status == "published", notice.Status == "published"
		notice.Revision, notice.UpdatedAt, notice.UpdatedBy = row.Revision, now, "scheduler"
		if row.Active {
			notice.PublishedAt = &now
		}
		notice.History = append(notice.History, models.AnnouncementHistory{Action: notice.Status, At: now, Actor: "scheduler", Round: notice.PublishRound, Revision: row.Revision, PublishAt: notice.PublishAt, Title: notice.Title})
		row.Payload = messageJSON(notice)
		if err = s.repo.PutMessageRecord(ctx, row, expected); err != nil && !errors.Is(err, repository.ErrConflict) {
			log.Printf("notice activation deferred: %v", err)
		}
	}
}

func sameMessageTargets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]int{}
	for _, v := range a {
		m[v]++
	}
	for _, v := range b {
		m[v]--
	}
	for _, n := range m {
		if n != 0 {
			return false
		}
	}
	return true
}
