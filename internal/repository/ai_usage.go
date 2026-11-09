package repository

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

// AIUsageRepository is separate from business repositories: losing its durable ledger must not enable untracked billing.
type AIUsageRepository interface {
	GetAITask(context.Context, string) (*models.AITask, error)
	SaveAITask(context.Context, *models.AITask, int64) error
	ListAITasks(context.Context, models.AIFilter) (*models.AIPage, error)
	AIStatistics(context.Context, models.AIFilter) (*models.AIStatistics, error)
	LoadAIState(context.Context, string) (int64, []byte, error)
	SaveAIState(context.Context, string, int64, []byte) error
}

func aiSummary(t *models.AITask) *models.AITask {
	v := *t
	v.Calls = nil
	v.Result = ""
	v.InputSummary = ""
	return &v
}
func aiClone(t *models.AITask) *models.AITask {
	b, _ := json.Marshal(t)
	var v models.AITask
	_ = json.Unmarshal(b, &v)
	return &v
}
func aiMatches(t *models.AITask, f models.AIFilter) bool {
	if !f.From.IsZero() && t.CreatedAt.Before(f.From) || !f.To.IsZero() && !t.CreatedAt.Before(f.To) {
		return false
	}
	if f.Active && t.Status != "running" && t.Status != "queued" && t.Status != "cancelling" {
		return false
	}
	for _, p := range [][2]string{{f.ObjectID, t.ObjectID}, {f.InputVersion, t.InputVersion}, {f.Feature, t.Feature}, {f.Provider, t.Provider}, {f.Model, t.Model}, {f.Status, t.Status}, {f.Mode, t.Mode}, {f.Adoption, t.Adoption}} {
		if p[0] != "" && p[0] != p[1] {
			return false
		}
	}
	if f.Query != "" && !strings.Contains(strings.ToLower(t.ID+" "+t.ObjectID+" "+t.ObjectName+" "+t.Actor), strings.ToLower(f.Query)) {
		return false
	}
	return true
}
func aiLimit(f models.AIFilter) int {
	if f.Limit < 1 || f.Limit > 100 {
		return 30
	}
	return f.Limit
}
func aiAdd(v *models.AITotals, t *models.AITask) {
	v.Tasks++
	v.Requests += t.Requests
	v.UsageKnown += t.UsageKnown
	v.InputTokens += t.InputTokens
	v.OutputTokens += t.OutputTokens
	v.CostKnown += t.CostKnown
	v.EstimatedCNY += t.EstimatedCNY
	switch t.Status {
	case "running", "cancelling":
		v.Running++
	case "queued":
		v.Queued++
	case "success":
		v.Success++
	case "failed", "interrupted":
		v.Failed++
	case "cancelled":
		v.Cancelled++
	}
	if t.Adoption == "applied" {
		v.Adopted++
	}
	if t.FinishedAt != nil {
		v.Finished++
		v.DurationMS += t.DurationMS
	}
}

type aiAggregator struct {
	out                    models.AIStatistics
	features, models, days map[string]*models.AITotals
}

func newAIAggregator() *aiAggregator {
	return &aiAggregator{features: map[string]*models.AITotals{}, models: map[string]*models.AITotals{}, days: map[string]*models.AITotals{}}
}
func (a *aiAggregator) add(t *models.AITask) {
	aiAdd(&a.out.AITotals, t)
	for _, p := range []struct {
		m map[string]*models.AITotals
		k string
	}{{a.features, t.Feature}, {a.models, t.Provider + " / " + t.Model}, {a.days, t.CreatedAt.In(time.FixedZone("Asia/Shanghai", 28800)).Format("2006-01-02")}} {
		if p.m[p.k] == nil {
			p.m[p.k] = &models.AITotals{}
		}
		aiAdd(p.m[p.k], t)
	}
}
func (a *aiAggregator) result() *models.AIStatistics {
	for _, p := range []struct {
		m   map[string]*models.AITotals
		dst *[]models.AIGroup
	}{{a.features, &a.out.Features}, {a.models, &a.out.Models}, {a.days, &a.out.Days}} {
		*p.dst = []models.AIGroup{}
		for k, v := range p.m {
			*p.dst = append(*p.dst, models.AIGroup{Key: k, AITotals: *v})
		}
		sort.Slice(*p.dst, func(i, j int) bool { return (*p.dst)[i].Key < (*p.dst)[j].Key })
	}
	return &a.out
}

type memoryAIState struct {
	version int64
	data    []byte
}
type MemoryAIUsageRepository struct {
	mu     sync.RWMutex
	tasks  map[string]*models.AITask
	states map[string]memoryAIState
}

func NewMemoryAIUsageRepository() *MemoryAIUsageRepository {
	return &MemoryAIUsageRepository{tasks: map[string]*models.AITask{}, states: map[string]memoryAIState{}}
}
func (r *MemoryAIUsageRepository) GetAITask(ctx context.Context, id string) (*models.AITask, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	v := r.tasks[id]
	if v == nil {
		return nil, ErrNotFound
	}
	return aiClone(v), nil
}
func (r *MemoryAIUsageRepository) SaveAITask(ctx context.Context, t *models.AITask, expected int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	v := r.tasks[t.ID]
	if v == nil && expected != 0 || v != nil && v.Revision != expected {
		return ErrAlreadyExists
	}
	r.tasks[t.ID] = aiClone(t)
	return nil
}
func (r *MemoryAIUsageRepository) ListAITasks(ctx context.Context, f models.AIFilter) (*models.AIPage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := &models.AIPage{Items: []*models.AITask{}}
	for _, t := range r.tasks {
		if !aiMatches(t, f) {
			continue
		}
		out.Total++
		if f.Cursor == "" || t.ID < f.Cursor {
			out.Items = append(out.Items, aiSummary(t))
		}
	}
	sort.Slice(out.Items, func(i, j int) bool { return out.Items[i].ID > out.Items[j].ID })
	limit := aiLimit(f)
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = out.Items[limit-1].ID
	}
	return out, nil
}
func (r *MemoryAIUsageRepository) AIStatistics(ctx context.Context, f models.AIFilter) (*models.AIStatistics, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	a := newAIAggregator()
	for _, t := range r.tasks {
		if a.out.FirstRecordedAt == nil || t.CreatedAt.Before(*a.out.FirstRecordedAt) {
			v := t.CreatedAt
			a.out.FirstRecordedAt = &v
		}
		if aiMatches(t, f) {
			a.add(t)
		}
	}
	return a.result(), nil
}
func (r *MemoryAIUsageRepository) LoadAIState(ctx context.Context, key string) (int64, []byte, error) {
	if err := ctx.Err(); err != nil {
		return 0, nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.states[key]
	if !ok {
		return 0, nil, ErrNotFound
	}
	return v.version, append([]byte(nil), v.data...), nil
}
func (r *MemoryAIUsageRepository) SaveAIState(ctx context.Context, key string, expected int64, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	v := r.states[key]
	if v.version != expected {
		return ErrAlreadyExists
	}
	r.states[key] = memoryAIState{expected + 1, append([]byte(nil), data...)}
	return nil
}
