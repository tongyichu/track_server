package repository

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

// InMemoryAppClientStatsRepository is an in-memory implementation of AppClientStatsRepository.
type InMemoryAppClientStatsRepository struct {
	mu      sync.RWMutex
	devices map[string]*models.AppClientDevice
	// daily key: date + "\x00" + platform + "\x00" + version_code + "\x00" + app_version
	daily map[string]*memoryClientDailyActive
}

type memoryClientDailyActive struct {
	Date        string
	Platform    string
	VersionCode int64
	AppVersion  string
	Devices     int64
}

// NewInMemoryAppClientStatsRepository creates an in-memory stats repository.
func NewInMemoryAppClientStatsRepository() *InMemoryAppClientStatsRepository {
	return &InMemoryAppClientStatsRepository{
		devices: make(map[string]*models.AppClientDevice),
		daily:   make(map[string]*memoryClientDailyActive),
	}
}

func memoryClientDailyKey(date, platform string, versionCode int64, appVersion string) string {
	return date + "\x00" + platform + "\x00" + appVersion + "\x00" + strconv.FormatInt(versionCode, 10)
}

// TouchClientDevice upserts the device and counts it once per local date.
func (r *InMemoryAppClientStatsRepository) TouchClientDevice(_ context.Context, device *models.AppClientDevice) error {
	if device == nil || strings.TrimSpace(device.DeviceID) == "" {
		return errors.New("device id is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := device.LastSeenAt
	if now.IsZero() {
		now = time.Now()
	}
	today := now.Format("2006-01-02")

	existing := r.devices[device.DeviceID]
	firstToday := false
	if existing == nil {
		clone := *device
		clone.FirstSeenAt = now
		clone.LastSeenAt = now
		r.devices[device.DeviceID] = &clone
		firstToday = true
	} else {
		firstToday = existing.LastSeenAt.Format("2006-01-02") != today
		if device.UserID > 0 {
			existing.UserID = device.UserID
		}
		if device.Platform != "" {
			existing.Platform = device.Platform
		}
		if device.AppVersion != "" {
			existing.AppVersion = device.AppVersion
		}
		if device.VersionCode > 0 {
			existing.VersionCode = device.VersionCode
		}
		if device.DeviceModel != "" {
			existing.DeviceModel = device.DeviceModel
		}
		if device.OSVersion != "" {
			existing.OSVersion = device.OSVersion
		}
		existing.LastSeenAt = now
	}
	if firstToday {
		key := memoryClientDailyKey(today, device.Platform, device.VersionCode, device.AppVersion)
		entry := r.daily[key]
		if entry == nil {
			entry = &memoryClientDailyActive{Date: today, Platform: device.Platform, VersionCode: device.VersionCode, AppVersion: device.AppVersion}
			r.daily[key] = entry
		}
		entry.Devices++
	}
	return nil
}

// GetClientStatistics aggregates active devices and daily trend for the last days days.
func (r *InMemoryAppClientStatsRepository) GetClientStatistics(_ context.Context, days int) (*models.AppClientStatistics, error) {
	if days <= 0 {
		days = 30
	}
	now := time.Now()
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(days - 1))
	cutoffDate := cutoff.Format("2006-01-02")
	stats := &models.AppClientStatistics{Days: days}

	r.mu.RLock()
	defer r.mu.RUnlock()

	versionDist := make(map[string]*models.AppClientDimensionStat)
	modelDist := make(map[string]*models.AppClientDimensionStat)
	osDist := make(map[string]*models.AppClientDimensionStat)
	for _, device := range r.devices {
		if device.LastSeenAt.Before(cutoff) {
			continue
		}
		stats.ActiveDevices++
		versionLabel := device.AppVersion
		if versionLabel == "" {
			versionLabel = "未知"
		}
		versionKey := versionLabel + "\x00" + strconv.FormatInt(device.VersionCode, 10)
		item := versionDist[versionKey]
		if item == nil {
			item = &models.AppClientDimensionStat{Label: versionLabel, VersionCode: device.VersionCode}
			versionDist[versionKey] = item
		}
		item.Devices++
		memoryBumpDimension(modelDist, device.DeviceModel)
		memoryBumpDimension(osDist, device.OSVersion)
	}
	stats.VersionDistribution = memorySortDimensions(versionDist)
	stats.DeviceModelDistribution = memorySortDimensions(modelDist)
	stats.OSVersionDistribution = memorySortDimensions(osDist)

	trendIndex := make(map[string]int)
	for _, entry := range r.daily {
		if entry.Date < cutoffDate {
			continue
		}
		idx, ok := trendIndex[entry.Date]
		if !ok {
			stats.DailyTrend = append(stats.DailyTrend, models.AppClientDailyTrendItem{Date: entry.Date})
			idx = len(stats.DailyTrend) - 1
			trendIndex[entry.Date] = idx
		}
		label := entry.AppVersion
		if label == "" {
			label = "未知"
		}
		stats.DailyTrend[idx].Versions = append(stats.DailyTrend[idx].Versions, models.AppClientDimensionStat{
			Label: label, VersionCode: entry.VersionCode, Devices: entry.Devices,
		})
	}
	sort.SliceStable(stats.DailyTrend, func(i, j int) bool { return stats.DailyTrend[i].Date < stats.DailyTrend[j].Date })
	for i := range stats.DailyTrend {
		versions := stats.DailyTrend[i].Versions
		sort.SliceStable(versions, func(a, b int) bool { return versions[a].Devices > versions[b].Devices })
	}
	return stats, nil
}

func memoryBumpDimension(dist map[string]*models.AppClientDimensionStat, value string) {
	if value == "" {
		value = "未知"
	}
	item := dist[value]
	if item == nil {
		item = &models.AppClientDimensionStat{Label: value}
		dist[value] = item
	}
	item.Devices++
}

func memorySortDimensions(dist map[string]*models.AppClientDimensionStat) []models.AppClientDimensionStat {
	result := make([]models.AppClientDimensionStat, 0, len(dist))
	for _, item := range dist {
		result = append(result, *item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Devices == result[j].Devices {
			return result[i].VersionCode > result[j].VersionCode
		}
		return result[i].Devices > result[j].Devices
	})
	return result
}
