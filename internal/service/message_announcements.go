package service

import (
	"context"
	"encoding/json"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

func (s *MessageService) ListAdminAnnouncements(ctx context.Context, cursor string, limit int) (*models.MessagePage[*models.Announcement], error) {
	limit = messagePageLimit(limit)
	f := repository.MessageFilter{Kinds: []string{"notice"}, Limit: limit + 1}
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
		out.Items = append(out.Items, &v)
	}
	return out, nil
}
func (s *MessageService) GetAdminAnnouncement(ctx context.Context, id string) (*models.Announcement, error) {
	row, err := s.repo.GetMessageRecord(ctx, id)
	if err != nil {
		return nil, err
	}
	if row.Kind != "notice" {
		return nil, repository.ErrNotFound
	}
	var v models.Announcement
	err = json.Unmarshal(row.Payload, &v)
	v.Revision = row.Revision
	return &v, err
}
func (s *MessageService) SaveAnnouncement(ctx context.Context, actor, id string, expected int64, in models.Announcement) (*models.Announcement, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Body = strings.TrimSpace(in.Body)
	in.H5URL = strings.TrimSpace(in.H5URL)
	if in.Kind != "system" && in.Kind != "activity" || in.Status != "draft" && in.Status != "published" && in.Status != "withdrawn" || in.Title == "" || utf8.RuneCountInString(in.Title) > 100 || in.Body == "" || utf8.RuneCountInString(in.Body) > 8000 || len(in.TargetUserIDs) > 1000 {
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
	if id == "" {
		if expected != 0 {
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
		if row.Kind != "notice" || row.Revision != expected {
			return nil, repository.ErrConflict
		}
		var old models.Announcement
		if err = json.Unmarshal(row.Payload, &old); err != nil {
			return nil, err
		}
		in.ID = id
		in.CreatedAt = old.CreatedAt
		if old.Status != "draft" {
			if !in.PublishAt.Equal(old.PublishAt) {
				return nil, invalidArg("已发布通知的发布时间不能修改")
			}
			if in.Status == "draft" {
				return nil, invalidArg("已发布通知只能撤回，不能退回草稿")
			}
			if old.Status == "withdrawn" && in.Status != "withdrawn" {
				return nil, invalidArg("撤回的通知需要新建后重新发布")
			}
			if !sameMessageTargets(in.TargetUserIDs, old.TargetUserIDs) {
				return nil, invalidArg("已发布通知不能修改接收范围")
			}
		}
	}
	if in.PublishAt.IsZero() {
		in.PublishAt = now
	}
	if in.Status == "published" && !row.Active && in.PublishAt.Before(now) {
		in.PublishAt = now
	}
	if in.ExpiresAt != nil && !in.ExpiresAt.After(in.PublishAt) {
		return nil, invalidArg("结束时间须晚于发布时间")
	}
	ids := []int64{}
	seen := map[string]bool{}
	targets := []string{}
	for _, raw := range in.TargetUserIDs {
		id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil || id <= 0 {
			return nil, invalidArg("接收用户ID无效")
		}
		key := strconv.FormatInt(id, 10)
		if !seen[key] {
			seen[key] = true
			targets = append(targets, key)
			ids = append(ids, id)
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
	in.Revision = expected + 1
	in.UpdatedAt = now
	in.UpdatedBy = actor
	row.ID = in.ID
	row.Revision = in.Revision
	row.Active = in.Status == "published"
	row.Pending = in.Status != "draft"
	row.Targets = targets
	row.CreatedAt = in.PublishAt
	row.Payload = messageJSON(in)
	if err := s.repo.PutMessageRecord(ctx, row, expected); err != nil {
		return nil, err
	}
	return &in, nil
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
