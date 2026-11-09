// Package ai coordinates durable AI tasks, provider requests and operational controls.
package ai

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

var ErrCancelled = errors.New("AI 任务已中断，未继续写入结果")
var ErrPaused = errors.New("该功能的自动 AI 任务已暂停")
var ErrBudget = errors.New("该功能今日 AI 请求预算已用完")
var ErrSuppressed = errors.New("此版本的自动 AI 任务已被中断，请手动重试或等待资料变化")
var ErrUnavailable = errors.New("AI 使用记录暂不可用，已阻止新的付费调用")
var ErrBusy = errors.New("AI 任务繁忙，请稍后重试")

var Features = []struct{ ID, Name string }{
	{"track_auto", "自动路线说明与图案"}, {"track_description", "路线说明联网填写"}, {"track_shape", "手动轨迹图案识别"}, {"shape_learning", "人工改标学习复核"}, {"scenic_description", "景点介绍"},
}

func IsControlError(err error) bool {
	return errors.Is(err, ErrCancelled) || errors.Is(err, ErrPaused) || errors.Is(err, ErrBudget) || errors.Is(err, ErrSuppressed) || errors.Is(err, ErrUnavailable)
}

func ValidFeature(v string) bool {
	for _, f := range Features {
		if f.ID == v {
			return true
		}
	}
	return false
}

type metaKey struct{}
type runKey struct{}
type metadata struct{ Actor, Mode, Reason, RetryOf, ID string }

func WithActor(ctx context.Context, actor string) context.Context {
	m, _ := ctx.Value(metaKey{}).(metadata)
	m.Actor = actor
	m.Mode = "manual"
	return context.WithValue(ctx, metaKey{}, m)
}
func WithReason(ctx context.Context, mode, reason string) context.Context {
	m, _ := ctx.Value(metaKey{}).(metadata)
	m.Mode = mode
	m.Reason = reason
	return context.WithValue(ctx, metaKey{}, m)
}
func TaskID(ctx context.Context) string {
	r, _ := ctx.Value(runKey{}).(*Run)
	if r == nil {
		return ""
	}
	return r.id
}

type Spec struct{ Feature, Provider, Model, ObjectID, ObjectName, Mode, Reason, InputVersion, RuleVersion, InputSummary, RetryOf string }
type RetryFunc func(context.Context, *models.AITask) error

type Manager struct {
	Repo     repository.AIUsageRepository
	Durable  bool
	mu       sync.Mutex
	local    map[string]context.CancelFunc
	retries  map[string]RetryFunc
	workers  chan struct{}
	shutdown context.Context
	stop     context.CancelFunc
	wg       sync.WaitGroup
}

func New(repo repository.AIUsageRepository, durable bool) *Manager {
	ctx, stop := context.WithCancel(context.Background())
	m := &Manager{Repo: repo, Durable: durable, local: map[string]context.CancelFunc{}, retries: map[string]RetryFunc{}, workers: make(chan struct{}, 4), shutdown: ctx, stop: stop}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cleanup, cancel := context.WithTimeout(ctx, 10*time.Second)
				if err := m.Reconcile(cleanup); err != nil && ctx.Err() == nil {
					log.Printf("AI task recovery temporarily unavailable")
				}
				cancel()
			}
		}
	}()
	return m
}
func (m *Manager) Close() {
	if m == nil {
		return
	}
	m.stop()
	m.mu.Lock()
	for _, cancel := range m.local {
		cancel()
	}
	m.mu.Unlock()
	m.wg.Wait()
}
func newID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", ErrUnavailable
	}
	return fmt.Sprintf("AI-%016x-%s", time.Now().UnixNano(), hex.EncodeToString(b)), nil
}
func safeText(s string, n int) string {
	v := []rune(strings.TrimSpace(s))
	if len(v) > n {
		v = v[:n]
	}
	return string(v)
}
func freshContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}
func (m *Manager) edit(ctx context.Context, id string, fn func(*models.AITask) error) error {
	for i := 0; i < 20; i++ {
		t, err := m.Repo.GetAITask(ctx, id)
		if err != nil {
			return err
		}
		expected := t.Revision
		if err = fn(t); err != nil {
			return err
		}
		t.Revision++
		t.UpdatedAt = time.Now().UTC()
		err = m.Repo.SaveAITask(ctx, t, expected)
		if !errors.Is(err, repository.ErrAlreadyExists) {
			return err
		}
	}
	return ErrBusy
}
func defaultControl(feature string) models.AIControl {
	return models.AIControl{Policy: models.AIPolicy{Feature: feature, DailyRequests: 100, MaxConcurrent: 1}, Leases: map[string]time.Time{}}
}
func (m *Manager) control(ctx context.Context, feature string, fn func(*models.AIControl) error) error {
	if m == nil || m.Repo == nil {
		return ErrUnavailable
	}
	for i := 0; i < 30; i++ {
		v, b, err := m.Repo.LoadAIState(ctx, "feature:"+feature)
		c := defaultControl(feature)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return err
		}
		if err == nil {
			if err = json.Unmarshal(b, &c); err != nil {
				return err
			}
		}
		if c.Leases == nil {
			c.Leases = map[string]time.Time{}
		}
		if err = fn(&c); err != nil {
			return err
		}
		b, err = json.Marshal(c)
		if err != nil {
			return err
		}
		err = m.Repo.SaveAIState(ctx, "feature:"+feature, v, b)
		if !errors.Is(err, repository.ErrAlreadyExists) {
			return err
		}
	}
	return ErrBusy
}
func (m *Manager) Policies(ctx context.Context) ([]models.AIControl, error) {
	out := []models.AIControl{}
	for _, f := range Features {
		_, b, err := m.Repo.LoadAIState(ctx, "feature:"+f.ID)
		c := defaultControl(f.ID)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return nil, err
		}
		if err == nil {
			if err = json.Unmarshal(b, &c); err != nil {
				return nil, err
			}
		}
		for id, until := range c.Leases {
			if until.Before(time.Now()) {
				delete(c.Leases, id)
			}
		}
		out = append(out, c)
	}
	return out, nil
}
func (m *Manager) SavePolicy(ctx context.Context, p models.AIPolicy, actor string) error {
	if !ValidFeature(p.Feature) || p.DailyRequests < 1 || p.DailyRequests > 100000 || p.MaxConcurrent < 1 || p.MaxConcurrent > 16 || p.TokenAlert < 0 || p.TokenAlert > 1000000000 || math.IsNaN(p.CostAlertCNY) || math.IsInf(p.CostAlertCNY, 0) || p.CostAlertCNY < 0 || p.CostAlertCNY > 100000 {
		return errors.New("预算或并发参数无效")
	}
	return m.control(ctx, p.Feature, func(c *models.AIControl) error {
		if c.Policy.Revision != p.Revision {
			return errors.New("配置已变化，请刷新后重试")
		}
		p.Revision++
		p.UpdatedBy = actor
		p.UpdatedAt = time.Now().UTC()
		c.Policy = p
		return nil
	})
}
func (m *Manager) AutomaticAllowed(ctx context.Context, feature, id, version string) error {
	if m == nil {
		return nil
	}
	meta, _ := ctx.Value(metaKey{}).(metadata)
	if meta.Mode == "manual" {
		return nil
	}
	if !m.Durable {
		return ErrUnavailable
	}
	_, b, err := m.Repo.LoadAIState(ctx, "feature:"+feature)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return ErrUnavailable
	}
	if err == nil {
		var c models.AIControl
		if json.Unmarshal(b, &c) != nil {
			return ErrUnavailable
		}
		if c.Policy.PauseAutomatic {
			return ErrPaused
		}
		day := time.Now().In(time.FixedZone("Asia/Shanghai", 28800)).Format("2006-01-02")
		if c.Day == day && c.Requests >= c.Policy.DailyRequests {
			return ErrBudget
		}
	}
	page, err := m.Repo.ListAITasks(ctx, models.AIFilter{Feature: feature, ObjectID: id, InputVersion: version, Status: "cancelled", Limit: 1})
	if err != nil {
		return ErrUnavailable
	}
	if page.Total > 0 {
		return ErrSuppressed
	}
	return nil
}

type Run struct {
	m           *Manager
	id, feature string
	ctx         context.Context
	cancel      context.CancelFunc
	done        chan struct{}
	once        sync.Once
	stopMu      sync.Mutex
	stopReason  error
}

func (m *Manager) Begin(parent context.Context, s Spec) (context.Context, *Run, error) {
	if m == nil {
		return parent, nil, nil
	}
	if !m.Durable || m.Repo == nil {
		return parent, nil, ErrUnavailable
	}
	if !ValidFeature(s.Feature) {
		return parent, nil, ErrUnavailable
	}
	meta, _ := parent.Value(metaKey{}).(metadata)
	mode := s.Mode
	if mode == "" {
		mode = "auto"
	}
	if meta.Mode != "" {
		mode = meta.Mode
	}
	reason := s.Reason
	if meta.Reason != "" {
		reason = meta.Reason
	}
	parentID := meta.RetryOf
	if parentID == "" {
		parentID = s.RetryOf
	}
	actor := meta.Actor
	if actor == "" {
		actor = "system"
	}
	if mode == "auto" {
		if err := m.AutomaticAllowed(parent, s.Feature, s.ObjectID, s.InputVersion); err != nil {
			return parent, nil, err
		}
	}
	id := meta.ID
	if id == "" {
		var err error
		id, err = newID()
		if err != nil {
			return parent, nil, err
		}
	}
	now := time.Now().UTC()
	t := &models.AITask{ID: id, Revision: 1, Feature: s.Feature, Provider: s.Provider, Model: s.Model, ObjectID: safeText(s.ObjectID, 100), ObjectName: safeText(s.ObjectName, 200), Mode: mode, Reason: safeText(reason, 200), Actor: safeText(actor, 100), RetryOf: parentID, InputVersion: s.InputVersion, RuleVersion: s.RuleVersion, InputSummary: safeText(s.InputSummary, 2000), Status: "queued", Stage: "waiting", Adoption: "not_generated", CreatedAt: now, UpdatedAt: now, LeaseUntil: now.Add(30 * time.Second)}
	if meta.ID == "" {
		if err := m.Repo.SaveAITask(parent, t, 0); err != nil {
			return parent, nil, ErrUnavailable
		}
	} else {
		if err := m.edit(parent, id, func(v *models.AITask) error {
			if v.Status != "queued" || v.CancelRequested {
				return ErrCancelled
			}
			t.CreatedAt = v.CreatedAt
			t.Revision = v.Revision
			*v = *t
			return nil
		}); err != nil {
			return parent, nil, err
		}
	}
	if s.RetryOf != "" && meta.RetryOf == "" {
		if err := m.edit(parent, s.RetryOf, func(v *models.AITask) error {
			if v.RetryTaskID == "" {
				v.RetryTaskID = id
			}
			return nil
		}); err != nil {
			log.Printf("AI automatic retry parent link unavailable: task=%s", id)
		}
	}
	ctx, cancel := context.WithCancel(parent)
	r := &Run{m: m, id: id, feature: s.Feature, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	ctx = context.WithValue(ctx, runKey{}, r)
	r.ctx = ctx
	m.mu.Lock()
	m.local[id] = cancel
	m.mu.Unlock()
	m.wg.Add(1)
	go r.watch()
	// Acquire a durable feature slot. Waiting is cancellable and visible as queued.
	for {
		err := m.control(ctx, s.Feature, func(c *models.AIControl) error {
			if mode == "auto" && c.Policy.PauseAutomatic {
				return ErrPaused
			}
			now := time.Now()
			for id, until := range c.Leases {
				if until.Before(now) {
					delete(c.Leases, id)
				}
			}
			if len(c.Leases) >= c.Policy.MaxConcurrent {
				return ErrBusy
			}
			c.Leases[id] = now.Add(30 * time.Second)
			return nil
		})
		if err == nil {
			break
		}
		if !errors.Is(err, ErrBusy) {
			r.Finish(err)
			return parent, nil, err
		}
		select {
		case <-ctx.Done():
			r.Finish(ErrCancelled)
			return parent, nil, ErrCancelled
		case <-time.After(500 * time.Millisecond):
		}
	}
	if err := m.edit(ctx, id, func(v *models.AITask) error {
		if v.CancelRequested {
			return ErrCancelled
		}
		v.Status = "running"
		v.Stage = "preparing"
		started := time.Now().UTC()
		v.StartedAt = &started
		return nil
	}); err != nil {
		r.Finish(err)
		return parent, nil, err
	}
	return ctx, r, nil
}
func (r *Run) watch() {
	defer r.m.wg.Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.done:
			return
		case <-r.m.shutdown.Done():
			r.cancel()
			return
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := freshContext()
			t, err := r.m.Repo.GetAITask(ctx, r.id)
			if err != nil || t.CancelRequested || !active(t.Status) {
				cancel()
				r.cancel()
				return
			}
			err = r.m.control(ctx, r.feature, func(c *models.AIControl) error {
				if t.Mode == "auto" && c.Policy.PauseAutomatic && t.Stage != "saving" {
					return ErrPaused
				}
				if until, ok := c.Leases[r.id]; ok && until.After(time.Now()) {
					c.Leases[r.id] = time.Now().Add(30 * time.Second)
				} else if t.Status != "queued" {
					return ErrCancelled
				}
				return nil
			})
			if err == nil {
				err = r.m.edit(ctx, r.id, func(v *models.AITask) error {
					if !active(v.Status) {
						return ErrCancelled
					}
					v.LeaseUntil = time.Now().UTC().Add(30 * time.Second)
					return nil
				})
			}
			cancel()
			if err != nil {
				r.stopMu.Lock()
				r.stopReason = err
				r.stopMu.Unlock()
				r.cancel()
				return
			}
		}
	}
}
func active(s string) bool { return s == "queued" || s == "running" || s == "cancelling" }
func (r *Run) Check() error {
	if r == nil {
		return nil
	}
	if err := r.ctx.Err(); err != nil {
		r.stopMu.Lock()
		reason := r.stopReason
		r.stopMu.Unlock()
		if reason != nil {
			return reason
		}
		return ErrCancelled
	}
	t, err := r.m.Repo.GetAITask(r.ctx, r.id)
	if err != nil {
		return ErrUnavailable
	}
	if t.CancelRequested || !active(t.Status) || !t.LeaseUntil.After(time.Now()) {
		return ErrCancelled
	}
	return nil
}

// BeforeWrite forms the cancellation boundary: once entered, cancel refuses to claim it can undo an in-flight database save.
func (r *Run) BeforeWrite() error {
	if r == nil {
		return nil
	}
	if err := r.Check(); err != nil {
		return err
	}
	if err := r.m.control(r.ctx, r.feature, func(c *models.AIControl) error {
		if !c.Leases[r.id].After(time.Now()) {
			return ErrCancelled
		}
		t, err := r.m.Repo.GetAITask(r.ctx, r.id)
		if err != nil {
			return ErrUnavailable
		}
		if t.Mode == "auto" && c.Policy.PauseAutomatic && t.Stage != "saving" {
			return ErrPaused
		}
		return nil
	}); err != nil {
		return err
	}
	return r.m.edit(r.ctx, r.id, func(v *models.AITask) error {
		if v.CancelRequested || !active(v.Status) {
			return ErrCancelled
		}
		v.Stage = "saving"
		return nil
	})
}
func (r *Run) Result(value any, adoption string) error {
	if r == nil {
		return nil
	}
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(b) > 32768 {
		return errors.New("AI 结果记录过大")
	}
	return r.m.edit(r.ctx, r.id, func(t *models.AITask) error {
		if t.CancelRequested || !active(t.Status) {
			return ErrCancelled
		}
		t.Result = string(b)
		t.Adoption = adoption
		return nil
	})
}
func (r *Run) Adoption(value string) error {
	if r == nil {
		return nil
	}
	ctx, cancel := freshContext()
	defer cancel()
	return r.m.edit(ctx, r.id, func(t *models.AITask) error { t.Adoption = value; return nil })
}
func (r *Run) Finish(cause error) {
	if r == nil {
		return
	}
	r.once.Do(func() {
		close(r.done)
		defer r.cancel()
		ctx, cancel := freshContext()
		defer cancel()
		err := r.m.edit(ctx, r.id, func(t *models.AITask) error {
			if !active(t.Status) {
				return nil
			}
			now := time.Now().UTC()
			t.FinishedAt = &now
			t.DurationMS = now.Sub(t.CreatedAt).Milliseconds()
			t.LeaseUntil = now
			t.Status = "success"
			t.Stage = "finished"
			finalizeUnknownCalls(t, now)
			r.stopMu.Lock()
			stopReason := r.stopReason
			r.stopMu.Unlock()
			if t.CancelRequested {
				t.Status = "cancelled"
				t.Error = ErrCancelled.Error()
				t.Adoption = "not_applied"
			} else if errors.Is(stopReason, ErrPaused) || errors.Is(cause, ErrPaused) {
				t.Status = "paused"
				t.Error = ErrPaused.Error()
			} else if errors.Is(cause, ErrBudget) {
				t.Status = "deferred"
				t.Error = ErrBudget.Error()
			} else if r.ctx.Err() != nil {
				t.Status = "interrupted"
				t.Error = "执行已中断或超时，供应商消耗以已记录数据及账单为准"
			} else if cause != nil {
				t.Status = "failed"
				t.Error = safeText(cause.Error(), 400)
			}
			return nil
		})
		if err != nil {
			log.Printf("AI task final record unavailable: id=%s", r.id)
		}
		if err = r.m.control(ctx, r.feature, func(c *models.AIControl) error { delete(c.Leases, r.id); return nil }); err != nil {
			log.Printf("AI lease release unavailable: id=%s", r.id)
		}
		r.m.mu.Lock()
		delete(r.m.local, r.id)
		r.m.mu.Unlock()
	})
}
func (m *Manager) Cancel(ctx context.Context, id string) error {
	err := m.edit(ctx, id, func(t *models.AITask) error {
		if !active(t.Status) {
			return errors.New("任务已结束，请刷新")
		}
		if t.Stage == "saving" {
			return errors.New("结果正在保存，已无法中断，请刷新查看结果")
		}
		t.CancelRequested = true
		t.Status = "cancelling"
		return nil
	})
	if err != nil {
		return err
	}
	m.mu.Lock()
	cancel := m.local[id]
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}
func (m *Manager) Reconcile(ctx context.Context) error {
	cursor := ""
	for i := 0; i < 20; i++ {
		page, err := m.Repo.ListAITasks(ctx, models.AIFilter{Active: true, Cursor: cursor, Limit: 100})
		if err != nil {
			return err
		}
		for _, t := range page.Items {
			if t.LeaseUntil.After(time.Now()) {
				continue
			}
			err = m.edit(ctx, t.ID, func(v *models.AITask) error {
				if !active(v.Status) || v.LeaseUntil.After(time.Now()) {
					return nil
				}
				now := time.Now().UTC()
				v.FinishedAt = &now
				v.DurationMS = now.Sub(v.CreatedAt).Milliseconds()
				if v.CancelRequested {
					v.Status = "cancelled"
					v.Error = ErrCancelled.Error()
				} else {
					v.Status = "interrupted"
					v.Error = "执行进程中断或租约过期；供应商消耗可能未知，请核对后手动重试"
				}
				v.Stage = "finished"
				finalizeUnknownCalls(v, now)
				return nil
			})
			if err != nil {
				return err
			}
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	return nil
}

func (m *Manager) RegisterRetry(feature string, fn RetryFunc) { m.retries[feature] = fn }
func (m *Manager) Retry(ctx context.Context, id, actor string) (string, error) {
	old, err := m.Repo.GetAITask(ctx, id)
	if err != nil {
		return "", err
	}
	fn := m.retries[old.Feature]
	if fn == nil {
		return "", errors.New("该功能暂不可重试")
	}
	if old.Status != "failed" && old.Status != "cancelled" && old.Status != "interrupted" {
		return "", errors.New("仅失败、中断任务可以重试")
	}
	select {
	case m.workers <- struct{}{}:
	default:
		return "", ErrBusy
	}
	queued := false
	defer func() {
		if !queued {
			<-m.workers
		}
	}()
	next, err := newID()
	if err != nil {
		return "", err
	}
	if err = m.edit(ctx, id, func(t *models.AITask) error {
		if t.RetryTaskID != "" {
			return errors.New("已创建重试任务，请查看关联任务")
		}
		t.RetryTaskID = next
		return nil
	}); err != nil {
		return "", err
	}
	now := time.Now().UTC()
	t := *old
	t.ID = next
	t.Revision = 1
	t.Status = "queued"
	t.Stage = "waiting"
	t.Mode = "manual"
	t.Actor = actor
	t.Reason = "后台手动重试，重新读取当前资料"
	t.RetryOf = id
	t.RetryTaskID = ""
	t.CancelRequested = false
	t.Calls = nil
	t.Result = ""
	t.Adoption = "not_generated"
	t.Error = ""
	t.CreatedAt = now
	t.UpdatedAt = now
	t.StartedAt = nil
	t.FinishedAt = nil
	t.LeaseUntil = now.Add(30 * time.Second)
	t.Requests = 0
	t.UsageKnown = 0
	t.InputTokens = 0
	t.OutputTokens = 0
	t.CostKnown = 0
	t.EstimatedCNY = 0
	t.DurationMS = 0
	if err = m.Repo.SaveAITask(ctx, &t, 0); err != nil {
		_ = m.edit(ctx, id, func(v *models.AITask) error {
			if v.RetryTaskID == next {
				v.RetryTaskID = ""
			}
			return nil
		})
		return "", err
	}
	queued = true
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer func() { <-m.workers }()
		parent, cancel := context.WithTimeout(m.shutdown, 6*time.Minute)
		defer cancel()
		parent = context.WithValue(parent, metaKey{}, metadata{Actor: actor, Mode: "manual", Reason: t.Reason, RetryOf: id, ID: next})
		err := fn(parent, old)
		cleanup, stop := freshContext()
		defer stop()
		_ = m.edit(cleanup, next, func(v *models.AITask) error {
			if !active(v.Status) {
				return nil
			}
			now := time.Now().UTC()
			v.Status = "failed"
			v.Stage = "finished"
			v.Error = "重试未进入模型调用，请检查当前资料或任务占用"
			if err != nil {
				v.Error = safeText(err.Error(), 400)
			}
			if v.CancelRequested {
				v.Status = "cancelled"
			}
			v.FinishedAt = &now
			return nil
		})
	}()
	return next, nil
}

func finalizeUnknownCalls(t *models.AITask, now time.Time) {
	for i := range t.Calls {
		if t.Calls[i].Status == "running" {
			t.Calls[i].Status = "unknown"
			t.Calls[i].ErrorCode = "result_not_recorded"
			t.Calls[i].DurationMS = now.Sub(t.Calls[i].StartedAt).Milliseconds()
		}
	}
}

// CancellationRequested reports an explicit operator cancellation, distinct from a timeout or a feature pause.
func (r *Run) CancellationRequested() bool {
	if r == nil {
		return false
	}
	ctx, cancel := freshContext()
	defer cancel()
	task, err := r.m.Repo.GetAITask(ctx, r.id)
	return err == nil && task.CancelRequested
}
