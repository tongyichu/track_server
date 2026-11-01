package models

import "time"

// AppClientDevice 记录一台客户端设备的最近活跃信息。
//
// 以 device_id 为主键，匿名（未登录）升级检查和登录成功都会 upsert；
// user_id 只在登录后补绑，其余字段只在新值非空/非零时覆盖。
type AppClientDevice struct {
	DeviceID    string    `json:"device_id" bson:"device_id"`
	UserID      int64     `json:"user_id" bson:"user_id"`
	Platform    string    `json:"platform" bson:"platform"`
	AppVersion  string    `json:"app_version" bson:"app_version"`
	VersionCode int64     `json:"version_code" bson:"version_code"`
	DeviceModel string    `json:"device_model" bson:"device_model"`
	OSVersion   string    `json:"os_version" bson:"os_version"`
	FirstSeenAt time.Time `json:"first_seen_at" bson:"first_seen_at"`
	LastSeenAt  time.Time `json:"last_seen_at" bson:"last_seen_at"`
}

// AppClientDimensionStat 是一个维度（版本/机型/系统版本）的活跃设备计数。
type AppClientDimensionStat struct {
	Label       string `json:"label"`
	VersionCode int64  `json:"version_code,omitempty"`
	Devices     int64  `json:"devices"`
}

// AppClientDailyTrendItem 是一天内各版本的活跃设备数。
type AppClientDailyTrendItem struct {
	Date     string                   `json:"date"`
	Versions []AppClientDimensionStat `json:"versions"`
}

// AppClientStatistics 是后台 App 发布页的客户端统计聚合结果。
// 活跃设备口径：last_seen_at 在最近 days 天内的去重 device_id；
// 趋势按服务端本地自然日统计当天出现过的设备数（按版本分桶）。
type AppClientStatistics struct {
	Days                    int                       `json:"days"`
	ActiveDevices           int64                     `json:"active_devices"`
	VersionDistribution     []AppClientDimensionStat  `json:"version_distribution"`
	DeviceModelDistribution []AppClientDimensionStat  `json:"device_model_distribution"`
	OSVersionDistribution   []AppClientDimensionStat  `json:"os_version_distribution"`
	DailyTrend              []AppClientDailyTrendItem `json:"daily_trend"`
}

// AppClientLatestSummary 是最新发布版本的渗透概况，由 service 结合发布记录计算。
type AppClientLatestSummary struct {
	VersionName             string `json:"version_name"`
	VersionCode             int64  `json:"version_code"`
	MinSupportedVersionCode int64  `json:"min_supported_version_code"`
	ActiveDevices           int64  `json:"active_devices"`
	SharePercent            int    `json:"share_percent"`
	BelowMinSupported       int64  `json:"below_min_supported"`
}
