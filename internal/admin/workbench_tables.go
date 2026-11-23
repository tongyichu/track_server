package admin

import (
	"context"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

func (h *Handler) GetWorkbenchTable(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.auth == nil {
		c.JSON(http.StatusForbidden, utils.H{"error": "没有查看统计的权限"})
		return
	}
	session := h.auth.SessionFromRequest(c)
	if session == nil || session.Account == nil {
		c.JSON(http.StatusForbidden, utils.H{"error": "没有查看统计的权限"})
		return
	}
	can := func(module string) bool { return session.Account.allows(module, false) }
	table := string(c.Query("table"))
	required := map[string][]string{
		"cities":     {"tracks"},
		"cohorts":    {"users", "tracks"},
		"versions":   {"releases"},
		"companions": {"companions", "tracks"},
		"backlog":    {},
	}
	modules, known := required[table]
	if !known {
		c.JSON(http.StatusBadRequest, utils.H{"error": "未知的工作台表格"})
		return
	}
	for _, module := range modules {
		if !can(module) {
			c.JSON(http.StatusForbidden, utils.H{"error": "没有查看统计的权限"})
			return
		}
	}
	repo, ok := h.dashboardStatsRepo.(repository.AdminWorkbenchRepository)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "工作台表格暂不可用"})
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	now := time.Now().In(time.FixedZone("Asia/Shanghai", 8*60*60))
	start, previous, previousEnd, _, period := dashboardPeriodBounds(now, strings.TrimSpace(string(c.Query("period"))))
	q := models.AdminDashboardQuery{Start: start, End: now, PreviousStart: previous, PreviousEnd: previousEnd, Now: now}

	data := utils.H{"period": period, "range": utils.H{"start": start, "end": now}}
	var rows any
	var err error
	switch table {
	case "cities":
		page := 1
		if raw := string(c.Query("page")); raw != "" {
			page, err = strconv.Atoi(raw)
			if err != nil || page < 1 {
				c.JSON(http.StatusBadRequest, utils.H{"error": "页码必须是正整数"})
				return
			}
		}
		var cities []models.WorkbenchCity
		cities, err = repo.WorkbenchCities(ctx, q)
		if err == nil {
			const pageSize = 10
			total := len(cities)
			pages := 1 + max(0, total-1)/pageSize
			page = min(page, pages)
			offset := (page - 1) * pageSize
			rows = cities[offset:min(offset+pageSize, total)]
			data["pagination"] = utils.H{"page": page, "pages": pages, "page_size": pageSize, "total": total}
		}
	case "cohorts":
		rows, err = repo.WorkbenchCohorts(ctx, now)
	case "versions":
		since := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -6)
		rows, err = h.workbenchVersions(ctx, repo, since, can("feedbacks"))
	case "companions":
		rows, err = repo.WorkbenchCompanions(ctx, q)
	case "backlog":
		var scopes []string
		for _, pair := range [][2]string{{"track_submissions", "review"}, {"feedbacks", "feedback"}, {"route_groups", "index"}, {"users", "deletion"}} {
			if can(pair[0]) {
				scopes = append(scopes, pair[1])
			}
		}
		rows, err = repo.WorkbenchBacklogs(ctx, now, scopes)
	}
	if err != nil {
		log.Printf("admin workbench %s: %v", table, err)
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "工作台表格暂不可用"})
		return
	}
	c.Header("Cache-Control", "no-store")
	data["rows"] = rows
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": data})
}

type workbenchVersion struct {
	Version           string     `json:"version"`
	ReleaseRecordedAt *time.Time `json:"release_recorded_at"`
	Devices           *int64     `json:"devices"`
	Share             *float64   `json:"share"`
	Feedback          *int64     `json:"feedback"`
}

func (h *Handler) workbenchVersions(ctx context.Context, repo repository.AdminWorkbenchRepository, since time.Time, includeFeedback bool) ([]workbenchVersion, error) {
	counts, err := repo.WorkbenchVersions(ctx, since, includeFeedback)
	if err != nil {
		return nil, err
	}
	published := make(map[string]time.Time)
	if h.releaseSvc != nil {
		releases, releaseErr := h.releaseSvc.List(ctx, models.AppReleaseListFilter{
			Platform: models.AppReleasePlatformAndroid, Status: models.AppReleaseStatusPublished,
		})
		if releaseErr != nil {
			log.Print("admin workbench release dates unavailable")
		}
		for _, item := range releases {
			if item == nil || item.CreatedAt.IsZero() {
				continue
			}
			name := strings.TrimSpace(item.VersionName)
			if name != "" && (published[name].IsZero() || item.CreatedAt.Before(published[name])) {
				published[name] = item.CreatedAt
			}
		}
	}

	names := make(map[string]struct{}, len(counts.Devices)+len(counts.Feedback)+len(published))
	var total int64
	for name, count := range counts.Devices {
		names[name] = struct{}{}
		total += count
	}
	for name := range counts.Feedback {
		names[name] = struct{}{}
	}
	for name := range published {
		names[name] = struct{}{}
	}
	rows := make([]workbenchVersion, 0, len(names))
	for name := range names {
		row := workbenchVersion{Version: name}
		if name == "" {
			row.Version = "未知版本"
		}
		if at, found := published[name]; found {
			row.ReleaseRecordedAt = &at
		}
		if counts.Devices != nil {
			count := counts.Devices[name]
			row.Devices = &count
			if total > 0 {
				share := float64(count) / float64(total)
				row.Share = &share
			}
		}
		if counts.Feedback != nil {
			count := counts.Feedback[name]
			row.Feedback = &count
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Devices != nil && b.Devices != nil && *a.Devices != *b.Devices {
			return *a.Devices > *b.Devices
		}
		if a.Feedback != nil && b.Feedback != nil && *a.Feedback != *b.Feedback {
			return *a.Feedback > *b.Feedback
		}
		if a.ReleaseRecordedAt != nil && (b.ReleaseRecordedAt == nil || !a.ReleaseRecordedAt.Equal(*b.ReleaseRecordedAt)) {
			return b.ReleaseRecordedAt == nil || a.ReleaseRecordedAt.After(*b.ReleaseRecordedAt)
		}
		if b.ReleaseRecordedAt != nil && a.ReleaseRecordedAt == nil {
			return false
		}
		return a.Version < b.Version
	})
	return rows[:min(len(rows), 12)], nil
}
