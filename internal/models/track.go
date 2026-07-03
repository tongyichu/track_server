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
	ID                        string                   `json:"id" bson:"_id,omitempty"`                                              // ID 是轨迹记录唯一标识，由 generateTrackID 生成。
	UserID                    int64                    `json:"user_id" bson:"user_id"`                                               // UserID 是轨迹所属用户 ID。
	SessionID                 string                   `json:"session_id" bson:"session_id"`                                         // SessionID 是关联的同行会话 ID，可为空。
	CityCode                  string                   `json:"city_code" bson:"city_code"`                                           // CityCode 是轨迹所属的城市 Code（城市/省份映射由配置文件维护）。
	LocateAddr                string                   `json:"locate_addr" bson:"locate_addr"`                                       // LocateAddr 是轨迹的具体位置信息。
	TrackType                 string                   `json:"track_type" bson:"track_type"`                                         // TrackType 是轨迹类型，例如徒步、跑步、骑车、自驾。
	SourceTag                 string                   `json:"source_tag" bson:"source_tag"`                                         // SourceTag 是轨迹来源/运营标签，例如人工录入的冷启动轨迹。
	CoordinateSystem          string                   `json:"coordinate_system" bson:"coordinate_system"`                           // CoordinateSystem 是轨迹坐标系，例如 WGS84/GCJ02/BD09。
	Title                     string                   `json:"title" bson:"title"`                                                   // Title 是轨迹名称。
	StartTime                 time.Time                `json:"start_time" bson:"start_time"`                                         // StartTime 是运动开始时间。
	EndTime                   time.Time                `json:"end_time" bson:"end_time"`                                             // EndTime 是运动结束时间。
	Distance                  float64                  `json:"distance" bson:"distance"`                                             // Distance 是总距离，单位米。
	Duration                  uint32                   `json:"duration" bson:"duration"`                                             // Duration 是运动耗时，单位秒。
	CaloriesBurned            float64                  `json:"calories_burned" bson:"calories_burned"`                               // CaloriesBurned 是热量消耗，单位千卡。
	ElevationGain             int                      `json:"elevation_gain" bson:"elevation_gain"`                                 // ElevationGain 是累计爬升，单位米。
	RawTrackURL               string                   `json:"raw_track_url" bson:"raw_track_url"`                                   // RawTrackURL 是原始轨迹点文件在对象存储中的地址。
	TrackScreenshotURL        string                   `json:"track_screenshot_url" bson:"track_screenshot_url"`                     // TrackScreenshotURL 是轨迹截图文件在对象存储中的地址。
	TrackNoMapBgScreenshotURL string                   `json:"track_no_map_bg_screenshot_url" bson:"track_no_map_bg_screenshot_url"` // TrackNoMapBgScreenshotURL 是“无地图背景的轨迹路线截图”文件在对象存储中的地址。
	IsRunning                 bool                     `json:"is_running" bson:"is_running"`                                         // IsRunning 表示轨迹是否仍处于进行中。
	Status                    TrackStatus              `json:"status" bson:"status"`                                                 // Status 是轨迹状态：0-删除，1-正常，2-私密。
	CreatedAt                 time.Time                `json:"created_at" bson:"created_at"`                                         // CreatedAt 是记录创建时间。
	UpdatedAt                 time.Time                `json:"updated_at" bson:"updated_at"`                                         // UpdatedAt 是记录更新时间。
	DeletedAt                 time.Time                `json:"deleted_at,omitempty" bson:"deleted_at"`                               // DeletedAt 是删除时间（软删除）。
	Points                    []TrackPoint             `json:"points,omitempty" bson:"-"`                                            // Points 是内存中的轨迹点集合，不直接持久化到 track_records。
	AvgSpeedKmh               float64                  `json:"avg_speed_kmh,omitempty" bson:"avg_speed_kmh"`                         // AvgSpeedKmh 是平均速度，单位 km/h。
	EarnedRewards             []*AchievementRewardView `json:"earned_rewards,omitempty" bson:"-"`                                    // EarnedRewards 是本次轨迹完成即时获得的成就奖励，不持久化到 track_records。
	Submission                *TrackSubmission         `json:"submission,omitempty" bson:"-"`                                        // Submission 是审核通过的公开投稿详情，或轨迹所有者查看时的当前投稿。
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
	ID                        string    `json:"id"`                             // ID 是轨迹记录唯一标识。
	UserID                    int64     `json:"user_id"`                        // UserID 是轨迹所属用户 ID。
	SessionID                 string    `json:"session_id"`                     // SessionID 是关联的同行会话 ID，可为空。
	CityCode                  string    `json:"city_code"`                      // CityCode 是轨迹所属的城市 Code。
	LocateAddr                string    `json:"locate_addr"`                    // LocateAddr 是轨迹的具体位置信息。
	TrackType                 string    `json:"track_type"`                     // TrackType 是轨迹类型，例如徒步、跑步、骑车、自驾。
	StartTime                 time.Time `json:"start_time"`                     // StartTime 是运动开始时间。
	EndTime                   time.Time `json:"end_time"`                       // EndTime 是运动结束时间。
	CityName                  string    `json:"city_name"`                      // CityName 是城市名称（由 city_code 映射得到；映射关系由配置文件维护）。
	Nickname                  string    `json:"nickname"`                       // Nickname 是轨迹所属用户的昵称（字段定义与 User struct 保持一致）。
	UserAvatarURL             string    `json:"user_avatar_url"`                // UserAvatarURL 是轨迹所属用户的头像 URI。
	Title                     string    `json:"title"`                          // Title 是轨迹名称。
	Distance                  float64   `json:"distance"`                       // Distance 是总距离，单位米。
	Duration                  uint32    `json:"duration"`                       // Duration 是运动耗时，单位秒。
	AvgSpeedKmh               float64   `json:"avg_speed_kmh"`                  // AvgSpeedKmh 是平均速度，单位 km/h。
	CaloriesBurned            float64   `json:"calories_burned"`                // CaloriesBurned 是热量消耗，单位千卡。
	ElevationGain             int       `json:"elevation_gain"`                 // ElevationGain 是累计爬升，单位米。
	Collected                 bool      `json:"collected"`                      // Collected 表示当前鉴权用户是否已收藏该轨迹。
	CollectCount              int64     `json:"collect_count"`                  // CollectCount 是轨迹被收藏的总数。
	NavigateCount             int64     `json:"navigate_count"`                 // NavigateCount 是该轨迹被其他用户用于导航的次数。
	TrackScreenshotURL        string    `json:"track_screenshot_url"`           // TrackScreenshotURL 是服务器本地缓存的轨迹截图可下载 URL。
	TrackNoMapBgScreenshotURL string    `json:"track_no_map_bg_screenshot_url"` // TrackNoMapBgScreenshotURL 是服务器本地缓存的“无地图背景轨迹截图”可下载 URL。
	RawTrackURL               string    `json:"raw_track_url"`                  // RawTrackURL 是服务器本地缓存的原始轨迹文件可下载 URL。
	IsFeatured                bool      `json:"is_featured"`                    // IsFeatured 表示轨迹投稿已审核通过。
	FeaturedDescription       string    `json:"featured_description,omitempty"` // FeaturedDescription 是审核通过的投稿简介。
	FeaturedCoverURL          string    `json:"featured_cover_url,omitempty"`   // FeaturedCoverURL 优先使用投稿首图，无投稿图片时回退轨迹截图。
}

// CollectedTrackSummary is used by "user collected tracks list".
// It is consistent with TrackSummary but intentionally omits the `collected` field.
type CollectedTrackSummary struct {
	ID                        string    `json:"id"`
	UserID                    int64     `json:"user_id"`
	SessionID                 string    `json:"session_id"`
	CityCode                  string    `json:"city_code"`
	LocateAddr                string    `json:"locate_addr"`
	TrackType                 string    `json:"track_type"`
	StartTime                 time.Time `json:"start_time"`
	EndTime                   time.Time `json:"end_time"`
	CityName                  string    `json:"city_name"`
	Nickname                  string    `json:"nickname"`
	UserAvatarURL             string    `json:"user_avatar_url"`
	Title                     string    `json:"title"`
	Distance                  float64   `json:"distance"`
	Duration                  uint32    `json:"duration"`
	AvgSpeedKmh               float64   `json:"avg_speed_kmh"`
	CaloriesBurned            float64   `json:"calories_burned"`
	ElevationGain             int       `json:"elevation_gain"`
	CollectCount              int64     `json:"collect_count"`
	NavigateCount             int64     `json:"navigate_count"`
	TrackScreenshotURL        string    `json:"track_screenshot_url"`
	TrackNoMapBgScreenshotURL string    `json:"track_no_map_bg_screenshot_url"`
	RawTrackURL               string    `json:"raw_track_url"`
}

// TrackListCursor 表示按时间倒序翻页时使用的游标锚点。
//
// 约定：
// - start_time 与 id 组合后可稳定定位上一页最后一条记录；
// - 下一页查询条件为“(start_time, id) 严格小于该游标”；
// - id 作为同一 start_time 下的稳定次排序键，避免重复/漏数据。
type TrackListCursor struct {
	StartTime time.Time `json:"start_time"`
	ID        string    `json:"id"`
}

// TrackSummaryPage 是推荐轨迹列表的分页返回模型。
type TrackSummaryPage struct {
	Items      []*TrackSummary `json:"items"`
	NextCursor string          `json:"next_cursor,omitempty"`
	HasMore    bool            `json:"has_more"`
}

// CollectedTrackSummaryPage is the paging response of collected track list.
// Field names keep consistent with TrackSummaryPage.
type CollectedTrackSummaryPage struct {
	Items      []*CollectedTrackSummary `json:"items"`
	TotalCount int64                    `json:"total_count"`
	NextCursor string                   `json:"next_cursor,omitempty"`
	HasMore    bool                     `json:"has_more"`
}

// MyTrackSummary 是“我的轨迹”列表接口使用的返回模型。
//
// 与 TrackSummary 相比：
// - 不返回 nickname/user_avatar_url/collected（这些字段对“我的轨迹”列表不需要）。
// - 仍保留 collect_count/navigate_count 等统计字段，便于客户端展示。
type MyTrackSummary struct {
	ID                        string                  `json:"id"`                             // ID 是轨迹记录唯一标识。
	UserID                    int64                   `json:"user_id"`                        // UserID 是轨迹所属用户 ID。
	SessionID                 string                  `json:"session_id"`                     // SessionID 是关联的同行会话 ID，可为空。
	CityCode                  string                  `json:"city_code"`                      // CityCode 是轨迹所属的城市 Code。
	LocateAddr                string                  `json:"locate_addr"`                    // LocateAddr 是轨迹的具体位置信息。
	TrackType                 string                  `json:"track_type"`                     // TrackType 是轨迹类型，例如徒步、跑步、骑车、自驾。
	StartTime                 time.Time               `json:"start_time"`                     // StartTime 是运动开始时间。
	EndTime                   time.Time               `json:"end_time"`                       // EndTime 是运动结束时间。
	CityName                  string                  `json:"city_name"`                      // CityName 是城市名称（由 city_code 映射得到）。
	Title                     string                  `json:"title"`                          // Title 是轨迹名称。
	Distance                  float64                 `json:"distance"`                       // Distance 是总距离，单位米。
	Duration                  uint32                  `json:"duration"`                       // Duration 是运动耗时，单位秒。
	AvgSpeedKmh               float64                 `json:"avg_speed_kmh"`                  // AvgSpeedKmh 是平均速度，单位 km/h。
	CaloriesBurned            float64                 `json:"calories_burned"`                // CaloriesBurned 是热量消耗，单位千卡。
	ElevationGain             int                     `json:"elevation_gain"`                 // ElevationGain 是累计爬升，单位米。
	CollectCount              int64                   `json:"collect_count"`                  // CollectCount 是轨迹被收藏的总数。
	NavigateCount             int64                   `json:"navigate_count"`                 // NavigateCount 是该轨迹被其他用户用于导航的次数。
	TrackScreenshotURL        string                  `json:"track_screenshot_url"`           // TrackScreenshotURL 是服务器本地缓存的轨迹截图可下载 URL。
	TrackNoMapBgScreenshotURL string                  `json:"track_no_map_bg_screenshot_url"` // TrackNoMapBgScreenshotURL 是服务器本地缓存的“无地图背景轨迹截图”可下载 URL。
	RawTrackURL               string                  `json:"raw_track_url"`                  // RawTrackURL 是服务器本地缓存的原始轨迹文件可下载 URL。
	Submission                *TrackSubmissionSummary `json:"submission,omitempty"`           // Submission 是当前用户自己的投稿状态摘要。
}

// MyTrackSummaryPage 是“我的轨迹”列表的分页返回模型。
type MyTrackSummaryPage struct {
	Items      []*MyTrackSummary `json:"items"`
	TotalCount int64             `json:"total_count"`
	NextCursor string            `json:"next_cursor,omitempty"`
	HasMore    bool              `json:"has_more"`
}

// TrackMap represents data needed for rendering a track polyline on map.
type TrackMap struct {
	TrackID string       `json:"track_id"` // TrackID 是轨迹记录唯一标识。
	Points  []TrackPoint `json:"points"`   // Points 是用于地图绘制的轨迹点集合。
}

// TrackMapIndexJobStatus represents the lifecycle of an async map index job.
type TrackMapIndexJobStatus string

const (
	TrackMapIndexJobPending    TrackMapIndexJobStatus = "pending"
	TrackMapIndexJobProcessing TrackMapIndexJobStatus = "processing"
	TrackMapIndexJobSucceeded  TrackMapIndexJobStatus = "succeeded"
	TrackMapIndexJobFailed     TrackMapIndexJobStatus = "failed"
)

// TrackMapIndexJob records async work needed to build map indexes for a track.
type TrackMapIndexJob struct {
	TrackID      string                 `json:"track_id" bson:"track_id"`
	Status       TrackMapIndexJobStatus `json:"status" bson:"status"`
	Attempts     int                    `json:"attempts" bson:"attempts"`
	LastError    string                 `json:"last_error,omitempty" bson:"last_error"`
	NextRunAt    time.Time              `json:"next_run_at" bson:"next_run_at"`
	LockedAt     time.Time              `json:"locked_at,omitempty" bson:"locked_at"`
	LockedBy     string                 `json:"locked_by,omitempty" bson:"locked_by"`
	CreatedAt    time.Time              `json:"created_at" bson:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at" bson:"updated_at"`
	SucceededAt  time.Time              `json:"succeeded_at,omitempty" bson:"succeeded_at"`
	LastFailedAt time.Time              `json:"last_failed_at,omitempty" bson:"last_failed_at"`
}

// TrackGeoIndex stores the server-side spatial index derived from raw track points.
type TrackGeoIndex struct {
	TrackID                string       `json:"track_id" bson:"track_id"`
	UserID                 int64        `json:"user_id" bson:"user_id"`
	CityCode               string       `json:"city_code" bson:"city_code"`
	TrackType              string       `json:"track_type" bson:"track_type"`
	CoordinateSystem       string       `json:"coordinate_system" bson:"coordinate_system"`
	StartLat               float64      `json:"start_lat" bson:"start_lat"`
	StartLng               float64      `json:"start_lng" bson:"start_lng"`
	EndLat                 float64      `json:"end_lat" bson:"end_lat"`
	EndLng                 float64      `json:"end_lng" bson:"end_lng"`
	CenterLat              float64      `json:"center_lat" bson:"center_lat"`
	CenterLng              float64      `json:"center_lng" bson:"center_lng"`
	MinLat                 float64      `json:"min_lat" bson:"min_lat"`
	MinLng                 float64      `json:"min_lng" bson:"min_lng"`
	MaxLat                 float64      `json:"max_lat" bson:"max_lat"`
	MaxLng                 float64      `json:"max_lng" bson:"max_lng"`
	Distance               float64      `json:"distance" bson:"distance"`
	PointCount             int          `json:"point_count" bson:"point_count"`
	SimplifiedPolyline     []TrackPoint `json:"simplified_polyline,omitempty" bson:"simplified_polyline"`
	SimplifiedPolylineJSON string       `json:"-" bson:"simplified_polyline_json"`
	CreatedAt              time.Time    `json:"created_at" bson:"created_at"`
	UpdatedAt              time.Time    `json:"updated_at" bson:"updated_at"`
}

// TrackRouteGroupSource marks how a route group or membership was produced.
type TrackRouteGroupSource string

const (
	TrackRouteGroupSourceAuto       TrackRouteGroupSource = "auto"
	TrackRouteGroupSourceManual     TrackRouteGroupSource = "manual"
	TrackRouteGroupSourceMixed      TrackRouteGroupSource = "mixed"
	TrackRouteGroupSourceSubmission TrackRouteGroupSource = "submission"
)

// TrackRouteGroupStatus marks whether a route group is visible to clients.
type TrackRouteGroupStatus string

const (
	TrackRouteGroupStatusActive   TrackRouteGroupStatus = "active"
	TrackRouteGroupStatusArchived TrackRouteGroupStatus = "archived"
)

// TrackRouteGroupMemberRole identifies the representative member of a group.
type TrackRouteGroupMemberRole string

const (
	TrackRouteGroupMemberRoleRepresentative TrackRouteGroupMemberRole = "representative"
	TrackRouteGroupMemberRoleMember         TrackRouteGroupMemberRole = "member"
)

// TrackRouteGroupMemberDirection records whether a track matches the group forward or reversed.
type TrackRouteGroupMemberDirection string

const (
	TrackRouteGroupMemberDirectionForward TrackRouteGroupMemberDirection = "forward"
	TrackRouteGroupMemberDirectionReverse TrackRouteGroupMemberDirection = "reverse"
)

// TrackRouteGroup is the persistent spatial aggregation of public completed tracks.
type TrackRouteGroup struct {
	GroupID               string                `json:"group_id" bson:"_id,omitempty"`
	Name                  string                `json:"name" bson:"name"`
	TrackType             string                `json:"track_type" bson:"track_type"`
	Status                TrackRouteGroupStatus `json:"status" bson:"status"`
	CityCodes             []string              `json:"city_codes" bson:"city_codes"`
	CityCodesJSON         string                `json:"-" bson:"city_codes_json"`
	AreaID                string                `json:"area_id,omitempty" bson:"area_id,omitempty"`
	CoordinateSystem      string                `json:"coordinate_system" bson:"coordinate_system"`
	CenterLat             float64               `json:"center_lat" bson:"center_lat"`
	CenterLng             float64               `json:"center_lng" bson:"center_lng"`
	RadiusM               float64               `json:"radius_m" bson:"radius_m"`
	MinLat                float64               `json:"min_lat" bson:"min_lat"`
	MinLng                float64               `json:"min_lng" bson:"min_lng"`
	MaxLat                float64               `json:"max_lat" bson:"max_lat"`
	MaxLng                float64               `json:"max_lng" bson:"max_lng"`
	Distance              float64               `json:"distance" bson:"distance"`
	RepresentativeTrackID string                `json:"representative_track_id" bson:"representative_track_id"`
	MemberCount           int64                 `json:"member_count" bson:"member_count"`
	Source                TrackRouteGroupSource `json:"source" bson:"source"`
	CreatedAt             time.Time             `json:"created_at" bson:"created_at"`
	UpdatedAt             time.Time             `json:"updated_at" bson:"updated_at"`
}

const (
	TrackRouteIntroductionStatusDraft     = "draft"
	TrackRouteIntroductionStatusPublished = "published"
	TrackRouteIntroductionStatusArchived  = "archived"
)

// TrackRouteIntroductionContent is localized, structured copy rendered by the route introduction H5.
type TrackRouteIntroductionContent struct {
	Name        string   `json:"name" bson:"name"`
	Summary     string   `json:"summary" bson:"summary"`
	Description []string `json:"description,omitempty" bson:"description,omitempty"`
	Highlights  []string `json:"highlights,omitempty" bson:"highlights,omitempty"`
	Tips        []string `json:"tips,omitempty" bson:"tips,omitempty"`
}

// TrackRouteIntroduction stores editorial copy separately from the rebuilt route-group tables.
// AnchorTrackID is used to bind the copy to the new group after every offline regrouping.
type TrackRouteIntroduction struct {
	ID                   int64                         `json:"id" bson:"id"`
	AnchorTrackID        string                        `json:"anchor_track_id" bson:"anchor_track_id"`
	CurrentGroupID       string                        `json:"current_group_id" bson:"current_group_id"`
	Status               string                        `json:"status" bson:"status"`
	Chinese              TrackRouteIntroductionContent `json:"zh" bson:"zh"`
	English              TrackRouteIntroductionContent `json:"en" bson:"en"`
	Difficulty           string                        `json:"difficulty,omitempty" bson:"difficulty,omitempty"`
	EstimatedDurationMin int                           `json:"estimated_duration_min,omitempty" bson:"estimated_duration_min,omitempty"`
	EstimatedDurationMax int                           `json:"estimated_duration_max,omitempty" bson:"estimated_duration_max,omitempty"`
	BestSeasons          []string                      `json:"best_seasons,omitempty" bson:"best_seasons,omitempty"`
	ContentVersion       int64                         `json:"content_version" bson:"content_version"`
	CreatedAt            time.Time                     `json:"created_at" bson:"created_at"`
	UpdatedAt            time.Time                     `json:"updated_at" bson:"updated_at"`
	PublishedAt          *time.Time                    `json:"published_at,omitempty" bson:"published_at,omitempty"`
}

// TrackRouteGroupMember stores the many-to-many membership between groups and tracks.
type TrackRouteGroupMember struct {
	GroupID         string                         `json:"group_id" bson:"group_id"`
	TrackID         string                         `json:"track_id" bson:"track_id"`
	SimilarityScore float64                        `json:"similarity_score" bson:"similarity_score"`
	MatchDirection  TrackRouteGroupMemberDirection `json:"match_direction" bson:"match_direction"`
	Role            TrackRouteGroupMemberRole      `json:"role" bson:"role"`
	Source          TrackRouteGroupSource          `json:"source" bson:"source"`
	CreatedAt       time.Time                      `json:"created_at" bson:"created_at"`
	UpdatedAt       time.Time                      `json:"updated_at" bson:"updated_at"`
}

// TrackRouteGroupCandidate is used by offline aggregation candidate recall.
type TrackRouteGroupCandidate struct {
	Group *TrackRouteGroup
	Index *TrackGeoIndex
}

// TrackMapBBox represents a map bounding box.
type TrackMapBBox struct {
	MinLatitude  float64 `json:"min_latitude" bson:"min_latitude"`
	MinLongitude float64 `json:"min_longitude" bson:"min_longitude"`
	MaxLatitude  float64 `json:"max_latitude" bson:"max_latitude"`
	MaxLongitude float64 `json:"max_longitude" bson:"max_longitude"`
}

// TrackMapPoint represents a map point.
type TrackMapPoint struct {
	Latitude  float64 `json:"latitude" bson:"latitude"`
	Longitude float64 `json:"longitude" bson:"longitude"`
}

// TrackMapQueryFilter is used by repository map queries.
type TrackMapQueryFilter struct {
	TrackType string
	CityCode  string
	BBox      *TrackMapBBox
	Center    *TrackMapPoint
	RadiusM   int
	Limit     int
}

// TrackMapCoverTrack is a lightweight representative track payload.
type TrackMapCoverTrack struct {
	TrackID            string `json:"track_id"`
	TrackScreenshotURL string `json:"track_screenshot_url"`
}

// TrackMapRouteGroupItem is the route-level item returned to map clients.
type TrackMapRouteGroupItem struct {
	Type             string              `json:"type"`
	GroupID          string              `json:"group_id"`
	Name             string              `json:"name"`
	IntroductionURL  string              `json:"introduction_url,omitempty"`
	CityCode         string              `json:"city_code"`
	CityName         string              `json:"city_name"`
	TrackType        string              `json:"track_type"`
	CoordinateSystem string              `json:"coordinate_system"`
	Center           TrackMapPoint       `json:"center"`
	RadiusM          float64             `json:"radius_m"`
	BBox             TrackMapBBox        `json:"bbox"`
	CoverTrack       *TrackMapCoverTrack `json:"cover_track,omitempty"`
	RawTrackID       string              `json:"-"`
	SourceGeoIndex   *TrackGeoIndex      `json:"-"`
	Track            *Track              `json:"-"`
}

// TrackMapAreaReference identifies an area and its optional public introduction page.
type TrackMapAreaReference struct {
	ID              string `json:"id"`
	IntroductionURL string `json:"introduction_url,omitempty"`
}

// TrackMapClusterItem is the area/city aggregate item returned to map clients.
type TrackMapClusterItem struct {
	Type       string                 `json:"type"`
	ClusterID  string                 `json:"cluster_id,omitempty"`
	AreaID     string                 `json:"-"`
	Name       string                 `json:"name,omitempty"`
	AreaType   string                 `json:"area_type,omitempty"`
	Area       *TrackMapAreaReference `json:"area,omitempty"`
	CityCode   string                 `json:"city_code,omitempty"`
	CityName   string                 `json:"city_name,omitempty"`
	TrackType  string                 `json:"track_type"`
	Center     TrackMapPoint          `json:"center"`
	BBox       TrackMapBBox           `json:"bbox"`
	RouteCount int64                  `json:"route_count"`
}

// TrackMapViewResponse is the main map viewport response.
type TrackMapViewResponse struct {
	ViewLevel        string      `json:"view_level"`
	CoordinateSystem string      `json:"coordinate_system"`
	Items            interface{} `json:"items"`
}

// TrackMapRouteGroupList is the response for direct route group listing.
type TrackMapRouteGroupList struct {
	Items []*TrackMapRouteGroupItem `json:"items"`
}

// TrackUserStats 是用户维度的轨迹统计数据。
type TrackUserStats struct {
	TotalDistance float64 `json:"total_distance" bson:"total_distance"` // TotalDistance 是总里程，单位米。
	TrackCount    int64   `json:"track_count" bson:"track_count"`       // TrackCount 是轨迹次数。
	TotalDuration int64   `json:"total_duration" bson:"total_duration"` // TotalDuration 是总耗时，单位秒。
	TotalCalories float64 `json:"total_calories" bson:"total_calories"` // TotalCalories 是总热量，单位千卡。
}

// TrackTypeOption 是运动类型选项。
type TrackTypeOption struct {
	Type        string             `json:"type"`          // Type 是运动类型英文标识，例如 hiking/running。
	Name        string             `json:"name"`          // Name 是运动类型名称。
	ThemeColor  string             `json:"theme_color"`   // ThemeColor 是运动类型主题色。
	IconURL     string             `json:"icon_url"`      // IconURL 是运动类型图标静态资源链接。
	IconAnimURL string             `json:"icon_anim_url"` // IconAnimURL 是运动类型 Lottie 动画资源链接。
	Milestone   TrackTypeMilestone `json:"milestone"`     // Milestone 是当前用户在该运动类型下的里程碑统计。
}

// TrackTypeMilestoneStats 是按时间窗口聚合的运动统计数据。
type TrackTypeMilestoneStats struct {
	Distance   float64 `json:"distance"`    // Distance 是总里程，单位米。
	TrackCount int64   `json:"track_count"` // TrackCount 是轨迹次数。
	Duration   int64   `json:"duration"`    // Duration 是总耗时，单位秒。
	Calories   float64 `json:"calories"`    // Calories 是总热量，单位千卡。
}

// TrackTypeMilestone 是运动类型在不同时间窗口下的统计数据。
type TrackTypeMilestone struct {
	Month TrackTypeMilestoneStats `json:"month"` // Month 是最近一个月统计。
	Year  TrackTypeMilestoneStats `json:"year"`  // Year 是最近一年统计。
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
