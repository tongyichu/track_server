package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/ai"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

func (h *Handler) SetAIManager(m *ai.Manager, trackModel, scenicModel string, legacy int) {
	h.ai = m
	h.aiLegacyBudget = legacy
	for _, name := range []string{trackModel, scenicModel} {
		if name != "" && (len(h.aiModels) == 0 || name != h.aiModels[0]) {
			h.aiModels = append(h.aiModels, name)
		}
	}
}
func (h *Handler) aiReady(c *app.RequestContext) bool {
	if h.ai == nil {
		c.JSON(503, utils.H{"error": "AI 使用管理暂不可用"})
		return false
	}
	return true
}
func aiError(c *app.RequestContext, err error) {
	status := http.StatusConflict
	if errors.Is(err, repository.ErrNotFound) {
		status = 404
	}
	message := err.Error()
	if status != 404 && !strings.Contains(message, "AI") && !strings.Contains(message, "任务") && !strings.Contains(message, "配置") && !strings.Contains(message, "参数") && !strings.Contains(message, "预算") && !strings.Contains(message, "模型") && !strings.Contains(message, "价格") && !strings.Contains(message, "重试") && !strings.Contains(message, "资料") {
		message = "操作未完成，请刷新后重试"
	}
	c.JSON(status, utils.H{"error": message})
}
func aiFilter(c *app.RequestContext) (models.AIFilter, error) {
	f := models.AIFilter{Feature: strings.TrimSpace(string(c.Query("feature"))), Provider: strings.TrimSpace(string(c.Query("provider"))), Model: strings.TrimSpace(string(c.Query("model"))), Status: strings.TrimSpace(string(c.Query("status"))), Mode: strings.TrimSpace(string(c.Query("mode"))), Adoption: strings.TrimSpace(string(c.Query("adoption"))), Query: strings.TrimSpace(string(c.Query("q"))), Cursor: strings.TrimSpace(string(c.Query("cursor"))), Active: string(c.Query("active")) == "true", Limit: parseAdminListLimit(string(c.Query("limit")))}
	if len(f.Query) > 200 || len(f.Cursor) > 80 || len(f.Model) > 100 || len(f.Status) > 24 || len(f.Adoption) > 24 || len(f.Mode) > 16 || len(f.Provider) > 40 || len(f.Feature) > 48 {
		return f, errors.New("筛选参数过长")
	}
	zone := time.FixedZone("Asia/Shanghai", 28800)
	today := time.Now().In(zone)
	f.From = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, zone)
	f.To = f.From.AddDate(0, 0, 1)
	if v := string(c.Query("from")); v != "" {
		parsed, err := time.ParseInLocation("2006-01-02", v, zone)
		if err != nil {
			return f, errors.New("起始日期参数无效")
		}
		f.From = parsed
	}
	if v := string(c.Query("to")); v != "" {
		parsed, err := time.ParseInLocation("2006-01-02", v, zone)
		if err != nil {
			return f, errors.New("结束日期参数无效")
		}
		f.To = parsed.AddDate(0, 0, 1)
	}
	if !f.To.After(f.From) || f.To.Sub(f.From) > 366*24*time.Hour {
		return f, errors.New("查询日期范围需为 1～366 天")
	}
	if f.Active {
		f.From = time.Time{}
		f.To = time.Time{}
	}
	return f, nil
}
func (h *Handler) ListAITasks(ctx context.Context, c *app.RequestContext) {
	if !h.aiReady(c) {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	f, err := aiFilter(c)
	if err != nil {
		c.JSON(400, utils.H{"error": err.Error()})
		return
	}

	page, err := h.ai.Repo.ListAITasks(ctx, f)
	if err != nil {
		aiError(c, err)
		return
	}
	c.JSON(200, utils.H{"data": page})
}
func (h *Handler) AIStatistics(ctx context.Context, c *app.RequestContext) {
	if !h.aiReady(c) {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	f, err := aiFilter(c)
	if err != nil {
		c.JSON(400, utils.H{"error": err.Error()})
		return
	}
	out, err := h.ai.Repo.AIStatistics(ctx, f)
	if err != nil {
		aiError(c, err)
		return
	}
	c.JSON(200, utils.H{"data": out})
}
func (h *Handler) GetAITask(ctx context.Context, c *app.RequestContext) {
	if !h.aiReady(c) {
		return
	}
	t, err := h.ai.Repo.GetAITask(ctx, c.Param("task_id"))
	if err != nil {
		aiError(c, err)
		return
	}
	c.JSON(200, utils.H{"data": t})
}
func (h *Handler) CancelAITask(ctx context.Context, c *app.RequestContext) {
	if !h.aiReady(c) {
		return
	}
	if err := h.ai.Cancel(ctx, c.Param("task_id")); err != nil {
		aiError(c, err)
		return
	}
	c.JSON(202, utils.H{"data": utils.H{"cancel_requested": true}})
}
func (h *Handler) RetryAITask(ctx context.Context, c *app.RequestContext) {
	if !h.aiReady(c) {
		return
	}
	t, err := h.ai.Repo.GetAITask(ctx, c.Param("task_id"))
	if err != nil {
		aiError(c, err)
		return
	}
	session := h.auth.SessionFromRequest(c)
	source := "tracks"
	if t.Feature == "scenic_description" {
		source = "scenic_spots"
	}
	if session == nil || session.Account == nil || !session.Account.allows(source, true) {
		c.JSON(403, utils.H{"error": "重试还需要对应业务的操作权限"})
		return
	}
	id, err := h.ai.Retry(ctx, t.ID, h.adminOperator(c))
	if err != nil {
		aiError(c, err)
		return
	}
	c.JSON(202, utils.H{"data": utils.H{"task_id": id}})
}
func (h *Handler) AISettings(ctx context.Context, c *app.RequestContext) {
	if !h.aiReady(c) {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	policies, err := h.ai.Policies(ctx)
	if err != nil {
		aiError(c, err)
		return
	}
	prices, err := h.ai.Prices(ctx)
	if err != nil {
		aiError(c, err)
		return
	}
	zone := time.FixedZone("Asia/Shanghai", 28800)
	now := time.Now().In(zone)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, zone)
	todayStats, err := h.ai.Repo.AIStatistics(ctx, models.AIFilter{From: start, To: now.Add(time.Second)})
	if err != nil {
		aiError(c, err)
		return
	}
	features := []utils.H{}
	for _, f := range ai.Features {
		features = append(features, utils.H{"id": f.ID, "name": f.Name})
	}
	c.JSON(200, utils.H{"data": utils.H{"today_usage": todayStats, "features": features, "models": h.aiModels, "policies": policies, "prices": prices, "durable": h.ai.Durable, "legacy_narrative_daily_budget": h.aiLegacyBudget}})
}
func (h *Handler) SaveAIPolicy(ctx context.Context, c *app.RequestContext) {
	if !h.aiReady(c) {
		return
	}
	var p models.AIPolicy
	if len(c.Request.Body()) > 16384 || json.Unmarshal(c.Request.Body(), &p) != nil || p.Feature != c.Param("feature") {
		c.JSON(400, utils.H{"error": "配置参数无效"})
		return
	}
	if err := h.ai.SavePolicy(ctx, p, h.adminOperator(c)); err != nil {
		aiError(c, err)
		return
	}
	c.JSON(200, utils.H{"data": utils.H{"saved": true}})
}
func (h *Handler) SaveAIPrices(ctx context.Context, c *app.RequestContext) {
	if !h.aiReady(c) {
		return
	}
	var book ai.PriceBook
	if len(c.Request.Body()) > 32768 || json.Unmarshal(c.Request.Body(), &book) != nil {
		c.JSON(400, utils.H{"error": "价格配置参数无效"})
		return
	}
	if err := h.ai.SavePrices(ctx, book, h.adminOperator(c)); err != nil {
		aiError(c, err)
		return
	}
	c.JSON(200, utils.H{"data": utils.H{"saved": true}})
}
