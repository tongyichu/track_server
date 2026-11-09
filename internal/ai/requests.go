package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

// Observe records only endpoint kind, timings, safe error categories and usage counters.
// Request bodies and provider error messages can contain private inputs, so are never logged here.
func Observe(ctx context.Context, client *http.Client, req *http.Request, kind string) (*http.Response, error) {
	r, _ := ctx.Value(runKey{}).(*Run)
	if r == nil {
		return client.Do(req)
	}
	if err := r.Check(); err != nil {
		return nil, err
	}
	task, err := r.m.Repo.GetAITask(ctx, r.id)
	if err != nil {
		return nil, ErrUnavailable
	}
	var price models.AIPrice
	price, err = r.m.price(ctx, task.Provider, task.Model)
	if err != nil {
		return nil, ErrUnavailable
	}
	day := time.Now().In(time.FixedZone("Asia/Shanghai", 28800)).Format("2006-01-02")
	err = r.m.control(ctx, r.feature, func(c *models.AIControl) error {
		if task.Mode == "auto" && c.Policy.PauseAutomatic {
			return ErrPaused
		}
		if c.Day != day {
			c.Day = day
			c.Requests = 0
		}
		if c.Requests >= c.Policy.DailyRequests {
			return ErrBudget
		}
		if !c.Leases[r.id].After(time.Now()) {
			return ErrCancelled
		}
		c.Requests++
		return nil
	})
	if err != nil {
		return nil, err
	}
	callID, err := newID()
	if err != nil {
		return nil, err
	}
	call := models.AICall{ID: callID, Kind: kind, StartedAt: time.Now().UTC(), Status: "running", PriceVersion: price.Version}
	if price.Version > 0 {
		snapshot := price
		call.PriceSnapshot = &snapshot
	}
	err = r.m.edit(ctx, r.id, func(t *models.AITask) error {
		if t.CancelRequested || !active(t.Status) {
			return ErrCancelled
		}
		if len(t.Calls) >= 20 {
			return errors.New("单任务请求次数已达上限")
		}
		t.Stage = kind
		t.Calls = append(t.Calls, call)
		t.Requests++
		return nil
	})
	if err != nil {
		return nil, ErrUnavailable
	}
	// Recheck after durable reservation: cancellation must prevent any subsequent provider step.
	if err = r.Check(); err != nil {
		finishCall(r, call, nil, nil, err, price, false)
		return nil, err
	}
	resp, requestErr := client.Do(req)
	var raw []byte
	if resp != nil && resp.Body != nil {
		raw, err = io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(raw))
		if err != nil && requestErr == nil {
			requestErr = err
		}
		if len(raw) > 2<<20 {
			requestErr = errors.New("provider response size limit")
		}
	}
	if err = finishCall(r, call, resp, raw, requestErr, price, true); err != nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return nil, ErrUnavailable
	}
	return resp, requestErr
}
func finishCall(r *Run, call models.AICall, resp *http.Response, raw []byte, cause error, price models.AIPrice, sent bool) error {
	call.DurationMS = time.Since(call.StartedAt).Milliseconds()
	call.Status = "success"
	if cause != nil {
		call.Status = "failed"
		call.ErrorCode = "network_or_response_error"
		if r.ctx.Err() != nil {
			call.Status = "cancelled"
			call.ErrorCode = "cancelled_or_timeout"
		}
	}
	if resp != nil {
		call.HTTPStatus = resp.StatusCode
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			call.Status = "failed"
			call.ErrorCode = httpErrorCategory(resp.StatusCode, raw)
		}
	}
	if call.Status == "success" && call.Kind == "generation" {
		var data struct {
			Usage *struct {
				Prompt        *int64 `json:"prompt_tokens"`
				Completion    *int64 `json:"completion_tokens"`
				Cached        *int64 `json:"cached_tokens"`
				PromptDetails *struct {
					Cached *int64 `json:"cached_tokens"`
				} `json:"prompt_tokens_details"`
			} `json:"usage"`
		}
		if json.Unmarshal(raw, &data) == nil && data.Usage != nil {
			u := data.Usage
			if u.Prompt != nil && u.Completion != nil && *u.Prompt >= 0 && *u.Completion >= 0 && *u.Prompt <= 100000000 && *u.Completion <= 100000000 {
				call.InputTokens = u.Prompt
				call.OutputTokens = u.Completion
				call.CachedTokens = u.Cached
				if call.CachedTokens == nil && u.PromptDetails != nil {
					call.CachedTokens = u.PromptDetails.Cached
				}
			}
		}
	}
	if call.Status == "success" {
		if call.Kind == "search" && price.SearchPerCall != nil {
			v := *price.SearchPerCall
			call.EstimatedCNY = &v
		}
		if call.Kind == "generation" && call.InputTokens != nil && call.OutputTokens != nil && price.InputPerMillion != nil && price.OutputPerMillion != nil {
			cached := int64(0)
			if call.CachedTokens != nil {
				cached = *call.CachedTokens
			}
			if cached >= 0 && cached <= *call.InputTokens && (cached == 0 || price.CachedPerMillion != nil) {
				v := float64(*call.InputTokens-cached)*(*price.InputPerMillion)/1e6 + float64(*call.OutputTokens)*(*price.OutputPerMillion)/1e6
				if cached > 0 {
					v += float64(cached) * (*price.CachedPerMillion) / 1e6
				}
				call.EstimatedCNY = &v
			}
		}
	}
	if !sent {
		call.Status = "not_sent"
		call.ErrorCode = "not_sent"
	}
	ctx, cancel := freshContext()
	defer cancel()
	return r.m.edit(ctx, r.id, func(t *models.AITask) error {
		if !sent {
			t.Requests--
		}
		for i := range t.Calls {
			if t.Calls[i].ID == call.ID {
				t.Calls[i] = call
				break
			}
		}
		t.UsageKnown = 0
		t.InputTokens = 0
		t.OutputTokens = 0
		t.CostKnown = 0
		t.EstimatedCNY = 0
		for _, c := range t.Calls {
			if c.InputTokens != nil && c.OutputTokens != nil {
				t.UsageKnown++
				t.InputTokens += *c.InputTokens
				t.OutputTokens += *c.OutputTokens
			}
			if c.EstimatedCNY != nil {
				t.CostKnown++
				t.EstimatedCNY += *c.EstimatedCNY
			}
		}
		return nil
	})
}
func httpErrorCategory(status int, raw []byte) string {
	var data struct {
		Error struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &data)
	text := strings.ToLower(data.Error.Type + " " + data.Error.Code + " " + data.Error.Message)
	if strings.Contains(text, "exceeded_current_quota") || strings.Contains(text, "insufficient") || strings.Contains(text, "balance") {
		return "insufficient_balance"
	}
	switch status {
	case 400:
		return "invalid_parameters"
	case 401:
		return "authentication_failed"
	case 403:
		return "provider_denied"
	case 429:
		return "rate_limited"
	}
	if status >= 500 {
		return "provider_unavailable"
	}
	return "provider_request_failed"
}
func (m *Manager) price(ctx context.Context, provider, model string) (models.AIPrice, error) {
	book, err := m.Prices(ctx)
	if err != nil {
		return models.AIPrice{}, err
	}
	for _, p := range book.Items {
		if p.Provider == provider && p.Model == model {
			return p, nil
		}
	}
	return models.AIPrice{Provider: provider, Model: model}, nil
}

type PriceBook struct {
	Revision int64            `json:"revision"`
	Items    []models.AIPrice `json:"items"`
}

func (m *Manager) Prices(ctx context.Context) (PriceBook, error) {
	v, b, err := m.Repo.LoadAIState(ctx, "prices")
	out := PriceBook{Revision: v, Items: []models.AIPrice{}}
	if errors.Is(err, repository.ErrNotFound) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(b, &out.Items); err != nil {
		return out, err
	}
	return out, nil
}
func (m *Manager) SavePrices(ctx context.Context, book PriceBook, actor string) error {
	if len(book.Items) > 30 {
		return errors.New("模型价格配置过多")
	}
	seen := map[string]bool{}
	for i := range book.Items {
		p := &book.Items[i]
		if p.Provider != "moonshot" || len(p.Model) < 1 || len(p.Model) > 100 || strings.TrimSpace(p.Model) != p.Model || seen[p.Provider+":"+p.Model] {
			return errors.New("模型价格配置无效或重复")
		}
		seen[p.Provider+":"+p.Model] = true
		for _, v := range []*float64{p.InputPerMillion, p.CachedPerMillion, p.OutputPerMillion, p.SearchPerCall} {
			if v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0 || *v > 1e6) {
				return errors.New("模型单价无效")
			}
		}
		p.Version = book.Revision + 1
		p.UpdatedAt = time.Now().UTC()
		p.UpdatedBy = actor
	}
	b, err := json.Marshal(book.Items)
	if err != nil {
		return err
	}
	return m.Repo.SaveAIState(ctx, "prices", book.Revision, b)
}

// MarkAdopted links explicit confirmation to the candidate; it cannot change task results or business data.
func (m *Manager) MarkAdopted(ctx context.Context, id, feature, object string) error {
	if m == nil || id == "" {
		return nil
	}
	return m.edit(ctx, id, func(t *models.AITask) error {
		if t.Feature != feature || t.ObjectID != object || t.Status != "success" {
			return errors.New("AI 任务与保存对象不匹配")
		}
		t.Adoption = "applied"
		return nil
	})
}
func (m *Manager) ValidateCandidate(ctx context.Context, id, feature, object, version string) error {
	if m == nil || id == "" {
		return nil
	}
	t, err := m.Repo.GetAITask(ctx, id)
	if err != nil {
		return err
	}
	if t.Feature != feature || t.ObjectID != object || t.Status != "success" || t.InputVersion != version {
		return errors.New("AI 候选与当前资料不匹配")
	}
	return nil
}

func (m *Manager) ValidateScenicCandidate(ctx context.Context, id, object, actor string) error {
	if m == nil || id == "" {
		return nil
	}
	t, err := m.Repo.GetAITask(ctx, id)
	if err != nil {
		return err
	}
	if t.Feature != "scenic_description" || t.Status != "success" || (t.ObjectID != "" && t.ObjectID != object) || t.Actor != actor {
		return errors.New("AI 景点建议与当前对象不匹配")
	}
	return nil
}
func (m *Manager) BindScenicAdoption(ctx context.Context, id, object string) error {
	if m == nil || id == "" {
		return nil
	}
	return m.edit(ctx, id, func(t *models.AITask) error {
		if t.Feature != "scenic_description" || t.Status != "success" || (t.ObjectID != "" && t.ObjectID != object) {
			return errors.New("AI 景点建议与当前对象不匹配")
		}
		t.ObjectID = object
		t.Adoption = "applied"
		return nil
	})
}
