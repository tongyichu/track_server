package admin

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/robfig/cron/v3"
	"github.com/tongyichu/track_server/internal/models"
)

type analyticsHealthMonitor struct {
	serviceReady, enabled, uploaderReady, schedulerEnabled bool
	schedule                                               cron.Schedule
	startedAt                                              time.Time
}

// AnalyticsHealthConfig contains non-secret runtime flags for the archive health indicator.
type AnalyticsHealthConfig struct {
	ServiceReady, Enabled, UploaderReady, SchedulerEnabled bool
	SyncCron                                               string
}

// SetAnalyticsHealth injects runtime configuration without probing OSS or modifying the queue.
func (h *Handler) SetAnalyticsHealth(cfg AnalyticsHealthConfig) {
	schedule, _ := cron.ParseStandard(cfg.SyncCron)
	h.analyticsHealth = &analyticsHealthMonitor{
		serviceReady: cfg.ServiceReady, enabled: cfg.Enabled, uploaderReady: cfg.UploaderReady,
		schedulerEnabled: cfg.SchedulerEnabled, schedule: schedule, startedAt: time.Now(),
	}
}

type analyticsHealthData struct {
	State          string     `json:"state"`
	Label          string     `json:"label"`
	Message        string     `json:"message"`
	CheckedAt      time.Time  `json:"checked_at"`
	LastFinishedAt *time.Time `json:"last_finished_at,omitempty"`
	NextDueAt      *time.Time `json:"next_due_at,omitempty"`
}

// GetAnalyticsHealth summarizes archive health; it does not certify client event completeness.
func (h *Handler) GetAnalyticsHealth(ctx context.Context, c *app.RequestContext) {
	now := time.Now()
	data := analyticsHealthData{CheckedAt: now}
	set := func(state, label, message string) {
		data.State, data.Label, data.Message = state, label, message
	}
	m := h.analyticsHealth
	switch {
	case m == nil:
		set("unknown", "状态未知", "暂时无法获取埋点归档状态")
	case !m.enabled:
		set("disabled", "采集已停用", "埋点采集已在配置中停用")
	case !m.serviceReady:
		set("error", "采集不可用", "埋点服务初始化失败，请检查服务日志")
	case !m.schedulerEnabled:
		set("warning", "自动同步未启用", "当前实例未启用定时任务；如有独立任务实例，请在该实例确认同步状态")
	case !m.uploaderReady:
		set("warning", "归档未配置", "埋点正在本地保存，尚未配置可用的归档上传服务")
	case m.schedule == nil:
		set("warning", "同步计划无效", "埋点同步时间配置无效，请检查定时任务配置")
	case h.analyticsRepo == nil:
		set("unknown", "日志不可用", "暂时无法读取同步日志")
	default:
		readCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		items, err := h.analyticsRepo.ListSyncSummaries(readCtx, "", 1, 0)
		if err != nil {
			log.Printf("[admin] analytics_health: read summary failed: %v", err)
			set("unknown", "状态读取失败", "暂时无法读取同步日志，请刷新重试")
		} else {
			var latest *models.AnalyticsSyncSummary
			if len(items) > 0 {
				latest = items[0]
			}
			data = m.summarize(time.Now(), latest)
		}
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": data})
}

func (m *analyticsHealthMonitor) summarize(now time.Time, latest *models.AnalyticsSyncSummary) analyticsHealthData {
	data := analyticsHealthData{State: "pending", Label: "等待首次同步", Message: "尚无同步日志，等待计划任务首次执行", CheckedAt: now}
	baseline := m.startedAt
	if latest != nil {
		if latest.StartedAt.IsZero() || latest.EndedAt.IsZero() || latest.EndedAt.Before(latest.StartedAt) || latest.EndedAt.After(now) {
			data.State, data.Label, data.Message = "unknown", "日志时间异常", "最近同步日志的时间无效，请检查服务时间"
			return data
		}
		baseline = latest.StartedAt
		data.LastFinishedAt = &latest.EndedAt
		switch latest.Status {
		case models.AnalyticsSyncStatusFailed:
			data.State, data.Label, data.Message = "error", "最近同步失败", "最近一次归档失败，请查看同步日志"
			return data
		case models.AnalyticsSyncStatusPartial:
			data.State, data.Label, data.Message = "warning", "部分同步失败", "最近一次归档有文件失败，请查看同步日志"
			return data
		case models.AnalyticsSyncStatusSuccess:
			if latest.FailedFiles > 0 {
				data.State, data.Label, data.Message = "warning", "部分同步失败", "最近一次归档存在失败文件，请查看同步日志"
				return data
			}
			data.State, data.Label, data.Message = "normal", "归档正常", "最近一次同步成功；此状态不代表所有客户端埋点均完整上报"
		default:
			data.State, data.Label, data.Message = "unknown", "同步状态未知", "最近日志没有有效的同步结果，请查看同步日志"
			return data
		}
	}
	// Match the scheduler's process timezone and allow its 30-minute job timeout plus reporting grace.
	next := m.schedule.Next(baseline.In(time.Local))
	if next.IsZero() {
		data.State, data.Label, data.Message = "warning", "同步计划无效", "埋点同步计划没有后续执行时间"
		return data
	}
	data.NextDueAt = &next
	if now.After(next.Add(time.Hour)) {
		data.State, data.Label, data.Message = "warning", "同步逾期", "已超过计划同步时间一小时，仍未收到新的完成日志"
	}
	return data
}
