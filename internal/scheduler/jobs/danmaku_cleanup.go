// Package jobs 收纳 scheduler 下挂载的具体业务任务。
package jobs

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/repository"
)

// 默认调度参数：每天 03:00 执行一次，保留 7 天。
const (
	defaultDanmakuCleanupSpec     = "0 3 * * *"
	defaultDanmakuRetentionDays   = 7
	defaultDanmakuRetentionPolicy = time.Duration(defaultDanmakuRetentionDays) * 24 * time.Hour
)

// DanmakuCleanup 是「弹幕过期清理」定时任务。
//
// 业务约束：
//   - 仅清理「同行 session 已结束」且 ended_at < now - retention 的弹幕；
//   - active session 的弹幕不会被清理；
//   - 默认保留 7 天，调度时间默认每天 03:00。
type DanmakuCleanup struct {
	repo      repository.CompanionRepository
	retention time.Duration
	spec      string
	nowFunc   func() time.Time
}

// DanmakuCleanupOption 用于构造期注入可选参数。
type DanmakuCleanupOption func(*DanmakuCleanup)

// WithDanmakuCleanupNowFunc 注入自定义 now 函数，主要用于测试。
func WithDanmakuCleanupNowFunc(fn func() time.Time) DanmakuCleanupOption {
	return func(j *DanmakuCleanup) {
		if fn != nil {
			j.nowFunc = fn
		}
	}
}

// NewDanmakuCleanup 构造一个 DanmakuCleanup 任务。
//
// retentionDays <= 0 时回退到默认 7 天；spec 为空时回退到默认每日 03:00。
func NewDanmakuCleanup(repo repository.CompanionRepository, retentionDays int, spec string, opts ...DanmakuCleanupOption) *DanmakuCleanup {
	retention := defaultDanmakuRetentionPolicy
	if retentionDays > 0 {
		retention = time.Duration(retentionDays) * 24 * time.Hour
	}
	if strings.TrimSpace(spec) == "" {
		spec = defaultDanmakuCleanupSpec
	}
	job := &DanmakuCleanup{
		repo:      repo,
		retention: retention,
		spec:      spec,
		nowFunc:   time.Now,
	}
	for _, opt := range opts {
		opt(job)
	}
	return job
}

// Name 实现 scheduler.Job。
func (j *DanmakuCleanup) Name() string { return "danmaku_cleanup" }

// Spec 实现 scheduler.Job。
func (j *DanmakuCleanup) Spec() string { return j.spec }

// Run 实现 scheduler.Job：把所有已结束 session 中超过保留期的弹幕清理掉。
func (j *DanmakuCleanup) Run(ctx context.Context) error {
	if j.repo == nil {
		return fmt.Errorf("danmaku_cleanup: repo is nil")
	}
	deadline := j.nowFunc().Add(-j.retention)
	affected, err := j.repo.DeleteDanmakusBySessionEndedBefore(ctx, deadline)
	if err != nil {
		return err
	}
	log.Printf("[scheduler] danmaku_cleanup: deadline=%s deleted=%d", deadline.Format(time.RFC3339), affected)
	return nil
}
