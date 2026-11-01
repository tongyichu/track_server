package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

const appClientDeviceSchema = `CREATE TABLE IF NOT EXISTS app_client_devices (
	device_id VARCHAR(128) NOT NULL COMMENT '设备唯一标识（客户端上报）',
	user_id BIGINT NOT NULL DEFAULT 0 COMMENT '最近登录用户ID，未登录为0',
	platform VARCHAR(16) NOT NULL DEFAULT '' COMMENT 'android / ios',
	app_version VARCHAR(32) NOT NULL DEFAULT '' COMMENT 'App 版本名',
	version_code BIGINT NOT NULL DEFAULT 0 COMMENT 'App 数字版本号',
	device_model VARCHAR(128) NOT NULL DEFAULT '' COMMENT '设备型号',
	os_version VARCHAR(64) NOT NULL DEFAULT '' COMMENT '系统版本',
	first_seen_at DATETIME(6) NOT NULL,
	last_seen_at DATETIME(6) NOT NULL,
	PRIMARY KEY (device_id),
	KEY idx_app_client_devices_last_seen (last_seen_at),
	KEY idx_app_client_devices_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='客户端设备活跃信息表';`

const appClientActiveDailySchema = `CREATE TABLE IF NOT EXISTS app_client_active_daily (
	stat_date DATE NOT NULL COMMENT '统计日期（服务端本地自然日）',
	platform VARCHAR(16) NOT NULL DEFAULT '',
	version_code BIGINT NOT NULL DEFAULT 0,
	app_version VARCHAR(32) NOT NULL DEFAULT '',
	active_devices BIGINT NOT NULL DEFAULT 0 COMMENT '当天出现过的去重设备数',
	PRIMARY KEY (stat_date, platform, version_code, app_version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='客户端按日版本活跃统计表';`

// MySQLAppClientStatsRepository implements AppClientStatsRepository on top of MySQL.
type MySQLAppClientStatsRepository struct{ db *sql.DB }

// NewMySQLAppClientStatsRepository constructs the repository and ensures its tables exist.
func NewMySQLAppClientStatsRepository(ctx context.Context, db *sql.DB) (*MySQLAppClientStatsRepository, error) {
	for _, stmt := range []string{appClientDeviceSchema, appClientActiveDailySchema} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return nil, err
		}
	}
	return &MySQLAppClientStatsRepository{db: db}, nil
}

// TouchClientDevice upserts the device's latest info and increments the daily
// active counter when the device appears for the first time on the local date.
func (r *MySQLAppClientStatsRepository) TouchClientDevice(ctx context.Context, device *models.AppClientDevice) error {
	if device == nil || strings.TrimSpace(device.DeviceID) == "" {
		return errors.New("device id is required")
	}
	now := device.LastSeenAt
	if now.IsZero() {
		now = time.Now()
	}
	today := now.Format("2006-01-02")

	var prevLastSeen time.Time
	err := r.db.QueryRowContext(ctx,
		`SELECT last_seen_at FROM app_client_devices WHERE device_id=?`, device.DeviceID,
	).Scan(&prevLastSeen)
	notFound := errors.Is(err, sql.ErrNoRows)
	if err != nil && !notFound {
		return err
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO app_client_devices (
			device_id, user_id, platform, app_version, version_code, device_model, os_version, first_seen_at, last_seen_at
		) VALUES (?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
			user_id = IF(VALUES(user_id) > 0, VALUES(user_id), user_id),
			platform = IF(VALUES(platform) <> '', VALUES(platform), platform),
			app_version = IF(VALUES(app_version) <> '', VALUES(app_version), app_version),
			version_code = IF(VALUES(version_code) > 0, VALUES(version_code), version_code),
			device_model = IF(VALUES(device_model) <> '', VALUES(device_model), device_model),
			os_version = IF(VALUES(os_version) <> '', VALUES(os_version), os_version),
			last_seen_at = VALUES(last_seen_at)`,
		device.DeviceID, device.UserID, device.Platform, device.AppVersion, device.VersionCode,
		device.DeviceModel, device.OSVersion, now, now,
	)
	if err != nil {
		return err
	}

	if notFound || prevLastSeen.Format("2006-01-02") != today {
		if _, err := r.db.ExecContext(ctx, `
			INSERT INTO app_client_active_daily (stat_date, platform, version_code, app_version, active_devices)
			VALUES (?,?,?,?,1)
			ON DUPLICATE KEY UPDATE active_devices = active_devices + 1`,
			today, device.Platform, device.VersionCode, device.AppVersion,
		); err != nil {
			return err
		}
	}
	return nil
}

// GetClientStatistics aggregates active devices and daily trend for the last days days.
func (r *MySQLAppClientStatsRepository) GetClientStatistics(ctx context.Context, days int) (*models.AppClientStatistics, error) {
	if days <= 0 {
		days = 30
	}
	now := time.Now()
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(days - 1))
	stats := &models.AppClientStatistics{Days: days}

	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM app_client_devices WHERE last_seen_at >= ?`, cutoff,
	).Scan(&stats.ActiveDevices); err != nil {
		return nil, err
	}

	versionDist, err := r.queryDeviceDimension(ctx, cutoff,
		`SELECT COALESCE(NULLIF(app_version, ''), '未知'), version_code, COUNT(*)
		 FROM app_client_devices WHERE last_seen_at >= ?
		 GROUP BY COALESCE(NULLIF(app_version, ''), '未知'), version_code
		 ORDER BY COUNT(*) DESC, version_code DESC`, true)
	if err != nil {
		return nil, err
	}
	stats.VersionDistribution = versionDist

	modelDist, err := r.queryDeviceDimension(ctx, cutoff,
		`SELECT COALESCE(NULLIF(device_model, ''), '未知'), 0, COUNT(*)
		 FROM app_client_devices WHERE last_seen_at >= ?
		 GROUP BY COALESCE(NULLIF(device_model, ''), '未知')
		 ORDER BY COUNT(*) DESC`, false)
	if err != nil {
		return nil, err
	}
	stats.DeviceModelDistribution = modelDist

	osDist, err := r.queryDeviceDimension(ctx, cutoff,
		`SELECT COALESCE(NULLIF(os_version, ''), '未知'), 0, COUNT(*)
		 FROM app_client_devices WHERE last_seen_at >= ?
		 GROUP BY COALESCE(NULLIF(os_version, ''), '未知')
		 ORDER BY COUNT(*) DESC`, false)
	if err != nil {
		return nil, err
	}
	stats.OSVersionDistribution = osDist

	rows, err := r.db.QueryContext(ctx, `
		SELECT stat_date, COALESCE(NULLIF(app_version, ''), '未知'), version_code, active_devices
		FROM app_client_active_daily
		WHERE stat_date >= ?
		ORDER BY stat_date ASC, active_devices DESC`, cutoff.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	trendIndex := make(map[string]int)
	for rows.Next() {
		var (
			date        time.Time
			label       string
			versionCode int64
			devices     int64
		)
		if err := rows.Scan(&date, &label, &versionCode, &devices); err != nil {
			return nil, err
		}
		dateKey := date.Format("2006-01-02")
		idx, ok := trendIndex[dateKey]
		if !ok {
			stats.DailyTrend = append(stats.DailyTrend, models.AppClientDailyTrendItem{Date: dateKey})
			idx = len(stats.DailyTrend) - 1
			trendIndex[dateKey] = idx
		}
		stats.DailyTrend[idx].Versions = append(stats.DailyTrend[idx].Versions, models.AppClientDimensionStat{
			Label: label, VersionCode: versionCode, Devices: devices,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return stats, nil
}

func (r *MySQLAppClientStatsRepository) queryDeviceDimension(ctx context.Context, cutoff time.Time, query string, withVersionCode bool) ([]models.AppClientDimensionStat, error) {
	rows, err := r.db.QueryContext(ctx, query, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]models.AppClientDimensionStat, 0)
	for rows.Next() {
		var item models.AppClientDimensionStat
		if err := rows.Scan(&item.Label, &item.VersionCode, &item.Devices); err != nil {
			return nil, err
		}
		if !withVersionCode {
			item.VersionCode = 0
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
