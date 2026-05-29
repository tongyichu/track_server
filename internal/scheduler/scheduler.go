// Package scheduler 提供进程内定时任务调度能力。
//
// 设计目标：
//   - 业务任务通过实现 Job 接口注册到 Scheduler，调度器只关心 Spec 与 Run，不感知具体业务；
//   - 是否启动由上层（main）按 cfg.SchedulerEnabled 控制，便于后续把「API 集群」与「定时任务集群」拆开部署；
//   - 单实例 cron，wrap 函数统一处理：任务超时上下文、panic recover、起止 / 错误日志。
package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/robfig/cron/v3"
)

// defaultJobTimeout 是单次任务执行的默认上限。
// 超过后传入 Run 的 ctx 会被 cancel，业务可主动检查 ctx.Err() 提前结束。
const defaultJobTimeout = 30 * time.Minute

// Job 抽象一次定时任务。
//
// - Name 用于日志区分，要求在同一个 Scheduler 内唯一；
// - Spec 使用 robfig/cron/v3 默认的 5 段表达式（分 时 日 月 周），也支持 "@every 30s" 等扩展语法；
// - Run 接收一个带超时的 Context，业务应当遵守取消语义。
type Job interface {
	Name() string
	Spec() string
	Run(ctx context.Context) error
}

// Scheduler 是一个轻量级 cron 调度器。
type Scheduler struct {
	cron   *cron.Cron
	logger *log.Logger
	jobs   []Job
}

// New 构造一个新的 Scheduler，使用标准库 log 作为日志后端。
func New() *Scheduler {
	logger := log.Default()
	return &Scheduler{
		cron:   cron.New(cron.WithLogger(cron.PrintfLogger(logger))),
		logger: logger,
	}
}

// Register 注册一组任务。
//
// 约束：
//   - 必须在 Start 之前调用；
//   - 任务名称必须唯一，重复名称将返回错误并跳过；
//   - cron 表达式无效时会返回错误。
func (s *Scheduler) Register(jobs ...Job) error {
	if s == nil {
		return fmt.Errorf("scheduler: nil receiver")
	}
	seen := make(map[string]struct{}, len(s.jobs)+len(jobs))
	for _, j := range s.jobs {
		seen[j.Name()] = struct{}{}
	}
	for _, j := range jobs {
		if j == nil {
			return fmt.Errorf("scheduler: nil job")
		}
		name := j.Name()
		if name == "" {
			return fmt.Errorf("scheduler: job name is empty")
		}
		if _, dup := seen[name]; dup {
			return fmt.Errorf("scheduler: duplicate job name %q", name)
		}
		if _, err := s.cron.AddFunc(j.Spec(), s.wrap(j)); err != nil {
			return fmt.Errorf("scheduler: add job %q: %w", name, err)
		}
		seen[name] = struct{}{}
		s.jobs = append(s.jobs, j)
	}
	return nil
}

// wrap 给一次任务执行附加超时上下文、panic recover 和起止日志。
func (s *Scheduler) wrap(j Job) func() {
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), defaultJobTimeout)
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				s.logger.Printf("[scheduler] job=%s panic recovered: %v", j.Name(), r)
			}
		}()
		start := time.Now()
		s.logger.Printf("[scheduler] job=%s start", j.Name())
		if err := j.Run(ctx); err != nil {
			s.logger.Printf("[scheduler] job=%s failed after %s: %v", j.Name(), time.Since(start), err)
			return
		}
		s.logger.Printf("[scheduler] job=%s done in %s", j.Name(), time.Since(start))
	}
}

// Start 启动 cron 主循环；非阻塞。
func (s *Scheduler) Start() {
	s.cron.Start()
}

// Stop 触发 cron 停止接收新调度，并等待执行中的任务完成。
//
// 受 ctx 控制：若 ctx 先超时，则不再等待 cron 返回，直接退出。
func (s *Scheduler) Stop(ctx context.Context) error {
	stopCtx := s.cron.Stop()
	select {
	case <-stopCtx.Done():
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Jobs 返回已注册任务的副本，供日志 / 调试使用。
func (s *Scheduler) Jobs() []Job {
	out := make([]Job, len(s.jobs))
	copy(out, s.jobs)
	return out
}
