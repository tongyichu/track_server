package models

import "time"

// TrackStatus represents the visibility status persisted in track_records.
type TrackStatus int8

const (
	// TrackStatusDeleted indicates the track has been deleted.
	TrackStatusDeleted TrackStatus = 0
	// TrackStatusNormal indicates the track is visible normally.
	TrackStatusNormal TrackStatus = 1
	// TrackStatusPrivate indicates the track is private.
	TrackStatusPrivate TrackStatus = 2
)

// TrackPoint represents a single sampled GPS point on a track.
type TrackPoint struct {
	Index     int       `json:"index" bson:"index"`         // Index 是轨迹点在序列中的顺序。
	Latitude  float64   `json:"latitude" bson:"latitude"`   // Latitude 是纬度。
	Longitude float64   `json:"longitude" bson:"longitude"` // Longitude 是经度。
	Elevation float64   `json:"elevation" bson:"elevation"` // Elevation 是海拔高度，单位米。
	Timestamp time.Time `json:"timestamp" bson:"timestamp"` // Timestamp 是轨迹点采样时间。
}

// Track aggregates persisted fields of track_records.
// Points is transient field kept for existing service logic.
type Track struct {
	ID                 string       `json:"id" bson:"_id,omitempty"`                          // ID 是轨迹记录唯一标识，由 generateTrackID 生成。
	UserID             int64        `json:"user_id" bson:"user_id"`                           // UserID 是轨迹所属用户 ID。
	CityCode           string       `json:"city_code" bson:"city_code"`                       // CityCode 是轨迹所属的城市 Code（城市/省份映射由配置文件维护）。
	TrackType          string       `json:"track_type" bson:"track_type"`                     // TrackType 是轨迹类型，例如徒步、跑步、骑车、自驾。
	Title              string       `json:"title" bson:"title"`                               // Title 是轨迹名称。
	StartTime          time.Time    `json:"start_time" bson:"start_time"`                     // StartTime 是运动开始时间。
	EndTime            time.Time    `json:"end_time" bson:"end_time"`                         // EndTime 是运动结束时间。
	Distance           float64      `json:"distance" bson:"distance"`                         // Distance 是总距离，单位米。
	Duration           uint32       `json:"duration" bson:"duration"`                         // Duration 是运动耗时，单位秒。
	ElevationGain      int          `json:"elevation_gain" bson:"elevation_gain"`             // ElevationGain 是累计爬升，单位米。
	RawTrackURL        string       `json:"raw_track_url" bson:"raw_track_url"`               // RawTrackURL 是原始轨迹点文件在对象存储中的地址。
	TrackScreenshotURL string       `json:"track_screenshot_url" bson:"track_screenshot_url"` // TrackScreenshotURL 是轨迹截图文件在对象存储中的地址。
	IsRunning          bool         `json:"is_running" bson:"is_running"`                     // IsRunning 表示轨迹是否仍处于进行中。
	Status             TrackStatus  `json:"status" bson:"status"`                             // Status 是轨迹状态：0-删除，1-正常，2-私密。
	CreatedAt          time.Time    `json:"created_at" bson:"created_at"`                     // CreatedAt 是记录创建时间。
	UpdatedAt          time.Time    `json:"updated_at" bson:"updated_at"`                     // UpdatedAt 是记录更新时间。
	Points             []TrackPoint `json:"points,omitempty" bson:"-"`                        // Points 是内存中的轨迹点集合，不直接持久化到 track_records。
	AvgSpeedKmh        float64      `json:"avg_speed_kmh,omitempty" bson:"avg_speed_kmh"`     // AvgSpeedKmh 是平均速度，单位 km/h。
}

// TrackSummary 是轨迹列表接口使用的轻量返回模型。
//
// 设计说明：
// - 推荐列表（/track/recommend/list）与搜索列表（/track/search/list）均返回该结构，保证字段口径一致；
// - 除 track_records 中的基础字段外，还会补充“轨迹所属用户信息（nickname/avatar）”与“收藏相关信息（collected/collect_count）”；
// - 这些补充字段来源于其他表/仓储：users、track_collects，服务层会在组装列表时统一填充；
// - 若用户信息不存在/查询失败（not found），nickname/avatar_url 将返回空字符串；
// - collect_count 为该轨迹被收藏的总数；collected 为“当前鉴权用户”是否收藏。
type TrackSummary struct {
	ID                 string    `json:"id"`                   // ID 是轨迹记录唯一标识。
	UserID             int64     `json:"user_id"`              // UserID 是轨迹所属用户 ID。
	CityCode           string    `json:"city_code"`            // CityCode 是轨迹所属的城市 Code。
	TrackType          string    `json:"track_type"`           // TrackType 是轨迹类型，例如徒步、跑步、骑车、自驾。
	StartTime          time.Time `json:"start_time"`           // StartTime 是运动开始时间。
	CityName           string    `json:"city_name"`            // CityName 是城市名称（由 city_code 映射得到；映射关系由配置文件维护）。
	Nickname           string    `json:"nickname"`             // Nickname 是轨迹所属用户的昵称（字段定义与 User struct 保持一致）。
	UserAvatarURL      string    `json:"user_avatar_url"`      // UserAvatarURL 是轨迹所属用户的头像 URI。
	Title              string    `json:"title"`                // Title 是轨迹名称。
	Distance           float64   `json:"distance"`             // Distance 是总距离，单位米。
	Duration           uint32    `json:"duration"`             // Duration 是运动耗时，单位秒。
	ElevationGain      int       `json:"elevation_gain"`       // ElevationGain 是累计爬升，单位米。
	Collected          bool      `json:"collected"`            // Collected 表示当前鉴权用户是否已收藏该轨迹。
	CollectCount       int64     `json:"collect_count"`        // CollectCount 是轨迹被收藏的总数。
	NavigateCount      int64     `json:"navigate_count"`       // NavigateCount 是该轨迹被其他用户用于导航的次数。
	TrackScreenshotURL string    `json:"track_screenshot_url"` // TrackScreenshotURL 是服务器本地缓存的轨迹截图可下载 URL。
	RawTrackURL        string    `json:"raw_track_url"`        // RawTrackURL 是服务器本地缓存的原始轨迹文件可下载 URL。
}

// TrackMap represents data needed for rendering a track polyline on map.
type TrackMap struct {
	TrackID string       `json:"track_id"` // TrackID 是轨迹记录唯一标识。
	Points  []TrackPoint `json:"points"`   // Points 是用于地图绘制的轨迹点集合。
}

// TrackWaypointMediaType represents the media type of a waypoint node.
type TrackWaypointMediaType int8

const (
	TrackWaypointMediaTypeText  TrackWaypointMediaType = 1
	TrackWaypointMediaTypeImage TrackWaypointMediaType = 2
	TrackWaypointMediaTypeAudio TrackWaypointMediaType = 3
	TrackWaypointMediaTypeMixed TrackWaypointMediaType = 4
)

// TrackWaypoint represents a media waypoint bound to a specific track position.
type TrackWaypoint struct {
	ID        uint64                 `json:"id" bson:"id"`                           // ID 是航点记录唯一标识。
	TrackID   string                 `json:"track_id" bson:"track_id"`               // TrackID 是关联的轨迹主表 ID。
	UserID    int64                  `json:"user_id" bson:"user_id"`                 // UserID 是用户 ID，冗余用于权限控制。
	Lat       float64                `json:"lat" bson:"lat"`                         // Lat 是纬度。
	Lng       float64                `json:"lng" bson:"lng"`                         // Lng 是经度。
	Elevation int                    `json:"elevation" bson:"elevation"`             // Elevation 是海拔，单位米。
	NodeTime  time.Time              `json:"node_time" bson:"node_time"`             // NodeTime 是该节点对应的时间戳。
	MediaType TrackWaypointMediaType `json:"media_type" bson:"media_type"`           // MediaType 是媒体类型：1-纯文本，2-图片，3-语音，4-图文+语音。
	Content   string                 `json:"content,omitempty" bson:"content"`       // Content 是文字描述内容。
	MediaURLs []string               `json:"media_urls,omitempty" bson:"media_urls"` // MediaURLs 是媒体文件地址数组。
	CreatedAt time.Time              `json:"created_at" bson:"created_at"`           // CreatedAt 是记录创建时间。
}
