package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"math"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

// TrackService provides business logic around track lifecycle and statistics.
type TrackService struct {
	tracks          repository.TrackRepository
	collects        repository.CollectRepository
	navigations     repository.NavigationRepository
	users           repository.UserRepository
	screenshotCache *AssetCacheService
	rawTrackCache   *AssetCacheService
}

const (
	defaultTrackPageSize = 20
	maxTrackPageSize     = 50
)

type ListRecommendInput struct {
	Cursor string
	Limit  int
}

type ListMyTracksInput struct {
	Cursor string
	Limit  int
}

type SearchTracksInput struct {
	Keyword string
	Cursor  string
	Limit   int
}

func (s *TrackService) decorateTrackAssets(ctx context.Context, track *models.Track) {
	if track == nil {
		return
	}
	// 把 OSS 地址字段覆盖为服务端本地可下载 URL，客户端无需处理 OSS 鉴权。
	// 未命中缓存时同步兜底拉取（带超时），失败则字段返回空串。
	// STS 下载凭证使用轨迹归属用户的 userID 申请。
	if s.screenshotCache != nil && track.TrackScreenshotURL != "" {
		cacheCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		track.TrackScreenshotURL = s.screenshotCache.EnsureCached(cacheCtx, track.UserID, track.ID, track.TrackScreenshotURL)
		cancel()
	}
	if s.rawTrackCache != nil && track.RawTrackURL != "" {
		cacheCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		track.RawTrackURL = s.rawTrackCache.EnsureCached(cacheCtx, track.UserID, track.ID, track.RawTrackURL)
		cancel()
	}
}

// ErrForbidden indicates current user has no permission to operate the resource.
var ErrForbidden = errors.New("forbidden")

// InvalidArgumentError represents a client-side request error.
type InvalidArgumentError struct{ Msg string }

func (e *InvalidArgumentError) Error() string { return e.Msg }

func invalidArg(msg string) error { return &InvalidArgumentError{Msg: msg} }

// CreateTrackInput describes optional fields accepted by track creation.
type CreateTrackInput struct {
	Title                     *string    `json:"title"`
	CityCode                  *string    `json:"city_code"`
	TrackType                 *string    `json:"track_type"`
	StartTime                 *time.Time `json:"start_time"`
	EndTime                   *time.Time `json:"end_time"`
	Distance                  *float64   `json:"distance"`
	Duration                  *uint32    `json:"duration"`
	ElevationGain             *int       `json:"elevation_gain"`
	RawTrackURL               *string    `json:"raw_track_url"`
	TrackScreenshotURL        *string    `json:"track_screenshot_url"`
	TrackNoMapBgScreenshotURL *string    `json:"track_no_map_bg_screenshot_url"`
	LegacyScreenshotURL       *string    `json:"screenshot_url,omitempty"`
	IsRunning                 *bool      `json:"is_running"`
	AvgSpeedKmh               *float64   `json:"avg_speed_kmh"`
}

func (in *CreateTrackInput) normalize() {
	if in.TrackScreenshotURL == nil && in.LegacyScreenshotURL != nil {
		in.TrackScreenshotURL = in.LegacyScreenshotURL
	}
}

func (in CreateTrackInput) validate() error {
	if in.Distance != nil && *in.Distance < 0 {
		return invalidArg("distance must be >= 0")
	}
	if in.ElevationGain != nil && *in.ElevationGain < 0 {
		return invalidArg("elevation_gain must be >= 0")
	}
	if in.AvgSpeedKmh != nil && *in.AvgSpeedKmh < 0 {
		return invalidArg("avg_speed_kmh must be >= 0")
	}
	if in.StartTime != nil && in.EndTime != nil && in.EndTime.Before(*in.StartTime) {
		return invalidArg("end_time must be >= start_time")
	}
	return nil
}

// TrackInfoPatch describes which track summary fields should be updated.
// Nil means the field is not provided and should remain unchanged.
type TrackInfoPatch struct {
	Distance           *float64 `json:"distance"`
	Duration           *uint32  `json:"duration"`
	ElevationGain      *int     `json:"elevation_gain"`
	RawTrackURL        *string  `json:"raw_track_url"`
	TrackScreenshotURL *string  `json:"screenshot_url"`
	IsRunning          *bool    `json:"is_running"`
	AvgSpeedKmh        *float64 `json:"avg_speed_kmh"`
}

func (p TrackInfoPatch) empty() bool {
	return p.Distance == nil &&
		p.Duration == nil &&
		p.ElevationGain == nil &&
		p.RawTrackURL == nil &&
		p.TrackScreenshotURL == nil &&
		p.IsRunning == nil &&
		p.AvgSpeedKmh == nil
}

// NewTrackService constructs a new TrackService instance.
func NewTrackService(tracks repository.TrackRepository, collects repository.CollectRepository) *TrackService {
	return &TrackService{tracks: tracks, collects: collects}
}

// SetUserRepository 注入用户仓储，用于在列表等场景补充用户头像等信息。
// 独立于构造函数以避免破坏既有调用方/单测。
func (s *TrackService) SetUserRepository(users repository.UserRepository) {
	s.users = users
}

// SetNavigationRepository 注入轨迹导航使用记录仓储，用于统计 navigate_count。
// 独立于构造函数以避免破坏既有调用方/单测。
func (s *TrackService) SetNavigationRepository(navigations repository.NavigationRepository) {
	s.navigations = navigations
}

// SetScreenshotCache 设置截图本地缓存服务。
// 独立于构造函数是为了避免破坏既有单测/调用方；在未设置时，相关逻辑会直接跳过。
func (s *TrackService) SetScreenshotCache(cache *AssetCacheService) {
	s.screenshotCache = cache
}

// SetRawTrackCache 设置原始轨迹文件本地缓存服务。
// 与截图类似，CreateTrack 会异步预热，GetTrackDetail/ListRecommend 响应时会把
// raw_track_url 覆盖为服务端本地可下载的 URL。
func (s *TrackService) SetRawTrackCache(cache *AssetCacheService) {
	s.rawTrackCache = cache
}

// CreateTrack creates a new track for a user.
func (s *TrackService) CreateTrack(ctx context.Context, userID int64, input CreateTrackInput) (*models.Track, error) {
	if userID <= 0 {
		return nil, errors.New("userID is required")
	}
	input.normalize()
	if err := input.validate(); err != nil {
		return nil, err
	}
	// 轨迹 ID 在创建轨迹之前先由仓储层统一分配。
	// 这样 service 只负责组装业务对象，不关心底层是 MySQL 自增序列、Mongo 兼容实现，
	// 还是内存仓储中的测试序列，避免把 ID 生成细节散落在业务层。
	trackID, err := s.tracks.NextTrackID(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	startTime := now
	if input.StartTime != nil {
		startTime = *input.StartTime
	}
	isRunning := true
	if input.IsRunning != nil {
		isRunning = *input.IsRunning
	}
	track := &models.Track{
		ID:        trackID,
		UserID:    userID,
		CityCode:  "",
		TrackType: "",
		Title:     "新的轨迹",
		StartTime: startTime,
		IsRunning: isRunning,
		Status:    models.TrackStatusNormal,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if input.Title != nil {
		track.Title = *input.Title
	}
	if input.CityCode != nil {
		track.CityCode = *input.CityCode
	}
	if input.TrackType != nil {
		track.TrackType = *input.TrackType
	}
	if input.EndTime != nil {
		track.EndTime = *input.EndTime
	}
	if input.Distance != nil {
		track.Distance = *input.Distance
	}
	if input.Duration != nil {
		track.Duration = *input.Duration
	}
	if input.ElevationGain != nil {
		track.ElevationGain = *input.ElevationGain
	}
	if input.RawTrackURL != nil {
		track.RawTrackURL = *input.RawTrackURL
	}
	if input.TrackScreenshotURL != nil {
		track.TrackScreenshotURL = *input.TrackScreenshotURL
	}
	if input.TrackNoMapBgScreenshotURL != nil {
		track.TrackNoMapBgScreenshotURL = *input.TrackNoMapBgScreenshotURL
	}
	if input.AvgSpeedKmh != nil {
		track.AvgSpeedKmh = *input.AvgSpeedKmh
	}
	// 到这里 track.ID 已经是最终可落库的业务 ID，格式固定为 `NO.` + 8 位 base36 编码。
	// Create 只负责持久化，不再承担“冲突重试生成 ID”的职责，唯一性保证前移到 NextTrackID。
	if err := s.tracks.Create(ctx, track); err != nil {
		return nil, err
	}
	// 异步预热截图到服务端本地缓存目录，供 ListRecommend 等接口后续直接下发本地 URL。
	// 失败不影响主流程，且仅在配置了缓存服务时触发。
	if s.screenshotCache != nil && track.TrackScreenshotURL != "" {
		src := track.TrackScreenshotURL
		if err := s.screenshotCache.RemoveTempCached(track.ID); err != nil {
			return nil, err
		}
		s.screenshotCache.PrefetchAsync(userID, track.ID, src)
		track.TrackScreenshotURL = s.screenshotCache.GuessLocalURL(track.ID, src)
	}
	// 同理异步预热原始轨迹文件。
	if s.rawTrackCache != nil && track.RawTrackURL != "" {
		src := track.RawTrackURL
		if err := s.rawTrackCache.RemoveTempCached(track.ID); err != nil {
			return nil, err
		}
		s.rawTrackCache.PrefetchAsync(userID, track.ID, src)
		track.RawTrackURL = s.rawTrackCache.GuessLocalURL(track.ID, src)
	}
	return track, nil
}

// GetTrackDetail returns detailed information of a track.
func (s *TrackService) GetTrackDetail(ctx context.Context, trackID string) (*models.Track, error) {
	if trackID == "" {
		return nil, errors.New("trackID is required")
	}
	track, err := s.tracks.FindByID(ctx, trackID)
	if err != nil {
		return nil, err
	}
	// Recompute summary metrics from points if available.
	updateTrackMetrics(track)
	s.decorateTrackAssets(ctx, track)
	return track, nil
}

// GetTrackMap returns map polyline for a track.
func (s *TrackService) GetTrackMap(ctx context.Context, trackID string) (*models.TrackMap, error) {
	track, err := s.tracks.FindByID(ctx, trackID)
	if err != nil {
		return nil, err
	}
	return &models.TrackMap{TrackID: track.ID, Points: track.Points}, nil
}

// ReportTrackNavigation records one navigation usage for a track.
// 仅统计“其他用户”使用该轨迹导航的次数（自己导航自己的轨迹不计入）。
func (s *TrackService) ReportTrackNavigation(ctx context.Context, navigatorUserID int64, trackID string) error {
	if navigatorUserID <= 0 {
		return invalidArg("userID is required")
	}
	if trackID == "" {
		return invalidArg("trackID is required")
	}
	if s.navigations == nil {
		return nil
	}
	track, err := s.tracks.FindByID(ctx, trackID)
	if err != nil {
		return err
	}
	if track.UserID == navigatorUserID {
		return invalidArg("cannot report navigation for your own track")
	}
	return s.navigations.AddNavigation(ctx, navigatorUserID, track.ID)
}

// GetRunningTrack returns currently running track of the user if exists.
func (s *TrackService) GetRunningTrack(ctx context.Context, userID int64) (*models.Track, error) {
	track, err := s.tracks.FindRunningByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	s.decorateTrackAssets(ctx, track)
	return track, nil
}

func normalizeTrackPageLimit(limit int) int {
	if limit <= 0 {
		return defaultTrackPageSize
	}
	if limit > maxTrackPageSize {
		return maxTrackPageSize
	}
	return limit
}

func decodeTrackListCursor(raw string) (*models.TrackListCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	buf, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, invalidArg("invalid cursor")
	}
	var cursor models.TrackListCursor
	if err := json.Unmarshal(buf, &cursor); err != nil {
		return nil, invalidArg("invalid cursor")
	}
	if cursor.ID == "" || cursor.StartTime.IsZero() {
		return nil, invalidArg("invalid cursor")
	}
	return &cursor, nil
}

func encodeTrackListCursor(startTime time.Time, id string) (string, error) {
	buf, err := json.Marshal(models.TrackListCursor{StartTime: startTime, ID: id})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func buildTrackSummaryPage(summaries []*models.TrackSummary, hasMore bool) (*models.TrackSummaryPage, error) {
	page := &models.TrackSummaryPage{Items: summaries, HasMore: hasMore}
	if hasMore && len(summaries) > 0 {
		nextCursor, err := encodeTrackListCursor(summaries[len(summaries)-1].StartTime, summaries[len(summaries)-1].ID)
		if err != nil {
			return nil, err
		}
		page.NextCursor = nextCursor
	}
	return page, nil
}

func buildMyTrackSummaryPage(summaries []*models.MyTrackSummary, hasMore bool) (*models.MyTrackSummaryPage, error) {
	page := &models.MyTrackSummaryPage{Items: summaries, HasMore: hasMore}
	if hasMore && len(summaries) > 0 {
		nextCursor, err := encodeTrackListCursor(summaries[len(summaries)-1].StartTime, summaries[len(summaries)-1].ID)
		if err != nil {
			return nil, err
		}
		page.NextCursor = nextCursor
	}
	return page, nil
}

// ListRecommend returns recommended tracks for the user.
func (s *TrackService) ListRecommend(ctx context.Context, userID int64, input ListRecommendInput) (*models.TrackSummaryPage, error) {
	limit := normalizeTrackPageLimit(input.Limit)
	cursor, err := decodeTrackListCursor(input.Cursor)
	if err != nil {
		return nil, err
	}
	tracks, err := s.tracks.ListRecommend(ctx, userID, cursor, limit+1)
	if err != nil {
		return nil, err
	}
	hasMore := len(tracks) > limit
	if hasMore {
		tracks = tracks[:limit]
	}
	summaries := toSummaries(tracks)
	// 填充服务器本地可下载截图 URL：
	// - 命中本地缓存则直接返回本地 URL
	// - 未命中则兜底拉取一次（同步，带 5 秒超时），失败则返回空串，不阻塞列表返回其它字段
	// - userID<=0（未登录游客）时无法申请 STS 读凭证，只尝试本地命中，不主动下载
	// 排查日志（轻量）：若列表返回的 track_screenshot_url/raw_track_url 为空，
	// 输出汇总计数，避免逐条日志刷屏。
	var (
		srcEmptySS, resEmptySS   int
		srcEmptyRaw, resEmptyRaw int
	)
	if s.screenshotCache == nil && s.rawTrackCache == nil {
		// cache 服务未启用时，两个字段会一直为空。
		if len(tracks) > 0 {
			log.Printf("[ListRecommend][asset] cache_disabled user_id=%d tracks=%d", userID, len(tracks))
		}
	} else {
		for i, t := range tracks {
			cacheCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if s.screenshotCache != nil {
				if t.TrackScreenshotURL == "" {
					srcEmptySS++
				} else {
					local := s.screenshotCache.EnsureCached(cacheCtx, userID, t.ID, t.TrackScreenshotURL)
					summaries[i].TrackScreenshotURL = local
					if local == "" {
						resEmptySS++
					}
				}
				// 无地图背景轨迹截图：使用不同 cache key，避免与 track_screenshot_url 文件名冲突。
				if t.TrackNoMapBgScreenshotURL != "" {
					local := s.screenshotCache.EnsureCached(cacheCtx, userID, t.ID+"_no_map_bg", t.TrackNoMapBgScreenshotURL)
					summaries[i].TrackNoMapBgScreenshotURL = local
				}
			}
			if s.rawTrackCache != nil {
				if t.RawTrackURL == "" {
					srcEmptyRaw++
				} else {
					local := s.rawTrackCache.EnsureCached(cacheCtx, userID, t.ID, t.RawTrackURL)
					summaries[i].RawTrackURL = local
					if local == "" {
						resEmptyRaw++
					}
				}
			}
			cancel()
		}
		// 仅在出现“源为空 / 结果为空”时打汇总日志，避免正常请求刷屏。
		if srcEmptySS > 0 || resEmptySS > 0 || srcEmptyRaw > 0 || resEmptyRaw > 0 {
			log.Printf("[ListRecommend][asset] summary user_id=%d tracks=%d screenshot_src_empty=%d screenshot_result_empty=%d raw_src_empty=%d raw_result_empty=%d", userID, len(tracks), srcEmptySS, resEmptySS, srcEmptyRaw, resEmptyRaw)
		}
	}
	if err := s.fillTrackSummaryExtras(ctx, userID, summaries); err != nil {
		return nil, err
	}
	return buildTrackSummaryPage(summaries, hasMore)
}

// ListMyTracks returns tracks that belong to the given user.
// It is used by the "我的轨迹" list API and intentionally omits user brief and collected fields.
func (s *TrackService) ListMyTracks(ctx context.Context, userID int64, input ListMyTracksInput) (*models.MyTrackSummaryPage, error) {
	if userID <= 0 {
		return nil, invalidArg("userID is required")
	}
	limit := normalizeTrackPageLimit(input.Limit)
	cursor, err := decodeTrackListCursor(input.Cursor)
	if err != nil {
		return nil, err
	}
	tracks, err := s.tracks.ListByUserID(ctx, userID, cursor, limit+1)
	if err != nil {
		return nil, err
	}
	hasMore := len(tracks) > limit
	if hasMore {
		tracks = tracks[:limit]
	}
	summaries := toMySummaries(tracks)
	// 与推荐/搜索列表保持一致：填充资源本地 URL（截图/原始轨迹）。
	if s.screenshotCache != nil || s.rawTrackCache != nil {
		for i, t := range tracks {
			cacheCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if s.screenshotCache != nil {
				summaries[i].TrackScreenshotURL = s.screenshotCache.EnsureCached(cacheCtx, userID, t.ID, t.TrackScreenshotURL)
			}
			if s.rawTrackCache != nil {
				summaries[i].RawTrackURL = s.rawTrackCache.EnsureCached(cacheCtx, userID, t.ID, t.RawTrackURL)
			}
			cancel()
		}
	}
	if err := s.fillMyTrackSummaryExtras(ctx, summaries); err != nil {
		return nil, err
	}
	return buildMyTrackSummaryPage(summaries, hasMore)
}

// SearchTracks searches tracks globally by keyword.
func (s *TrackService) SearchTracks(ctx context.Context, userID int64, input SearchTracksInput) (*models.TrackSummaryPage, error) {
	limit := normalizeTrackPageLimit(input.Limit)
	cursor, err := decodeTrackListCursor(input.Cursor)
	if err != nil {
		return nil, err
	}
	tracks, err := s.tracks.Search(ctx, input.Keyword, cursor, limit+1)
	if err != nil {
		return nil, err
	}
	hasMore := len(tracks) > limit
	if hasMore {
		tracks = tracks[:limit]
	}
	summaries := toSummaries(tracks)
	// 与推荐列表保持一致：填充资源本地 URL / 收藏状态与总数 / 用户昵称和头像。
	if s.screenshotCache != nil || s.rawTrackCache != nil {
		for i, t := range tracks {
			cacheCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if s.screenshotCache != nil {
				summaries[i].TrackScreenshotURL = s.screenshotCache.EnsureCached(cacheCtx, userID, t.ID, t.TrackScreenshotURL)
				if t.TrackNoMapBgScreenshotURL != "" {
					summaries[i].TrackNoMapBgScreenshotURL = s.screenshotCache.EnsureCached(cacheCtx, userID, t.ID+"_no_map_bg", t.TrackNoMapBgScreenshotURL)
				}
			}
			if s.rawTrackCache != nil {
				summaries[i].RawTrackURL = s.rawTrackCache.EnsureCached(cacheCtx, userID, t.ID, t.RawTrackURL)
			}
			cancel()
		}
	}
	if err := s.fillTrackSummaryExtras(ctx, userID, summaries); err != nil {
		return nil, err
	}
	return buildTrackSummaryPage(summaries, hasMore)
}

// MarkUploadedToCloud marks a finished track as uploaded to cloud.
func (s *TrackService) MarkUploadedToCloud(ctx context.Context, trackID string) error {
	track, err := s.tracks.FindByID(ctx, trackID)
	if err != nil {
		return err
	}
	// For demo purposes we only normalize record status and end time.
	if track.Status != models.TrackStatusNormal {
		track.Status = models.TrackStatusNormal
	}
	track.IsRunning = false
	if track.EndTime.Before(track.StartTime) {
		track.EndTime = time.Now()
	}
	return s.tracks.Update(ctx, track)
}

// UpdateTrackInfo updates a track with partially provided fields.
func (s *TrackService) UpdateTrackInfo(ctx context.Context, userID int64, trackID string, patch TrackInfoPatch) (*models.Track, error) {
	if userID <= 0 {
		return nil, invalidArg("userID is required")
	}
	if trackID == "" {
		return nil, invalidArg("trackID is required")
	}
	if patch.empty() {
		return nil, invalidArg("no fields to update")
	}
	if patch.Distance != nil && *patch.Distance < 0 {
		return nil, invalidArg("distance must be >= 0")
	}
	if patch.ElevationGain != nil && *patch.ElevationGain < 0 {
		return nil, invalidArg("elevation_gain must be >= 0")
	}
	if patch.AvgSpeedKmh != nil && *patch.AvgSpeedKmh < 0 {
		return nil, invalidArg("avg_speed_kmh must be >= 0")
	}

	track, err := s.tracks.FindByID(ctx, trackID)
	if err != nil {
		return nil, err
	}
	if track.UserID != userID {
		return nil, ErrForbidden
	}

	if patch.Distance != nil {
		track.Distance = *patch.Distance
	}
	if patch.Duration != nil {
		track.Duration = *patch.Duration
	}
	if patch.ElevationGain != nil {
		track.ElevationGain = *patch.ElevationGain
	}
	if patch.RawTrackURL != nil {
		track.RawTrackURL = *patch.RawTrackURL
	}
	if patch.TrackScreenshotURL != nil {
		track.TrackScreenshotURL = *patch.TrackScreenshotURL
	}
	if patch.IsRunning != nil {
		track.IsRunning = *patch.IsRunning
	}
	if patch.AvgSpeedKmh != nil {
		track.AvgSpeedKmh = *patch.AvgSpeedKmh
	}

	if err := s.tracks.Update(ctx, track); err != nil {
		return nil, err
	}
	// 轨迹资源链接字段对客户端下发本地可下载 URL（并触发异步预热）。
	if s.screenshotCache != nil && patch.TrackScreenshotURL != nil {
		src := track.TrackScreenshotURL
		s.screenshotCache.PrefetchAsync(userID, track.ID, src)
		track.TrackScreenshotURL = s.screenshotCache.GuessLocalURL(track.ID, src)
	}
	if s.rawTrackCache != nil && patch.RawTrackURL != nil {
		src := track.RawTrackURL
		s.rawTrackCache.PrefetchAsync(userID, track.ID, src)
		track.RawTrackURL = s.rawTrackCache.GuessLocalURL(track.ID, src)
	}
	return track, nil
}

// IsCollected reports whether a track is collected by user.
func (s *TrackService) IsCollected(ctx context.Context, userID int64, trackID string) (bool, error) {
	return s.collects.IsCollected(ctx, userID, trackID)
}

// CollectTrack adds a collection for user-track pair.
func (s *TrackService) CollectTrack(ctx context.Context, userID int64, trackID string) error {
	// Check track existence.
	if _, err := s.tracks.FindByID(ctx, trackID); err != nil {
		return err
	}
	return s.collects.AddCollect(ctx, userID, trackID)
}

// UncollectTrack removes a collection for user-track pair.
func (s *TrackService) UncollectTrack(ctx context.Context, userID int64, trackID string) error {
	return s.collects.RemoveCollect(ctx, userID, trackID)
}

// updateTrackMetrics recomputes distance, duration, ascent and average speed based on points.
func updateTrackMetrics(t *models.Track) {
	if len(t.Points) < 2 {
		return
	}
	var distance float64
	var ascent float64
	for i := 1; i < len(t.Points); i++ {
		d := haversineDistance(t.Points[i-1].Latitude, t.Points[i-1].Longitude, t.Points[i].Latitude, t.Points[i].Longitude)
		distance += d
		deltaAlt := t.Points[i].Elevation - t.Points[i-1].Elevation
		if deltaAlt > 0 {
			ascent += deltaAlt
		}
	}
	duration := t.Points[len(t.Points)-1].Timestamp.Sub(t.Points[0].Timestamp).Seconds()
	if duration < 0 {
		duration = 0
	}
	t.Distance = distance
	t.ElevationGain = int(math.Round(ascent))
	t.Duration = uint32(duration)
	if t.StartTime.IsZero() {
		t.StartTime = t.Points[0].Timestamp
	}
	t.EndTime = t.Points[len(t.Points)-1].Timestamp
	if duration > 0 {
		// m/s to km/h
		t.AvgSpeedKmh = (distance / duration) * 3.6
	}
}

func toSummaries(tracks []*models.Track) []*models.TrackSummary {
	res := make([]*models.TrackSummary, 0, len(tracks))
	for _, t := range tracks {
		// city_name 由 city_code 通过本地配置映射得到：
		// - 若 city_code 为空/未配置，则 city_name 返回空字符串；
		// - 映射解析失败会被内部吞掉并兜底为空（不影响列表其它字段返回）。
		res = append(res, &models.TrackSummary{
			ID:            t.ID,
			UserID:        t.UserID,
			CityCode:      t.CityCode,
			TrackType:     t.TrackType,
			StartTime:     t.StartTime,
			EndTime:       t.EndTime,
			CityName:      config.CityNameByCode(t.CityCode),
			Title:         t.Title,
			Distance:      t.Distance,
			Duration:      t.Duration,
			AvgSpeedKmh:   t.AvgSpeedKmh,
			ElevationGain: t.ElevationGain,
		})
	}
	return res
}

func toMySummaries(tracks []*models.Track) []*models.MyTrackSummary {
	res := make([]*models.MyTrackSummary, 0, len(tracks))
	for _, t := range tracks {
		res = append(res, &models.MyTrackSummary{
			ID:            t.ID,
			UserID:        t.UserID,
			CityCode:      t.CityCode,
			TrackType:     t.TrackType,
			StartTime:     t.StartTime,
			EndTime:       t.EndTime,
			CityName:      config.CityNameByCode(t.CityCode),
			Title:         t.Title,
			Distance:      t.Distance,
			Duration:      t.Duration,
			AvgSpeedKmh:   t.AvgSpeedKmh,
			ElevationGain: t.ElevationGain,
		})
	}
	return res
}

// fillMyTrackSummaryExtras fills only counters needed by MyTrackSummary.
// It intentionally does NOT fill nickname/avatar/collected.
func (s *TrackService) fillMyTrackSummaryExtras(ctx context.Context, summaries []*models.MyTrackSummary) error {
	if len(summaries) == 0 {
		return nil
	}
	trackIDs := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		trackIDs = append(trackIDs, summary.ID)
	}

	counts := map[string]int64{}
	if s.collects != nil {
		m, err := s.collects.CountByTrackIDs(ctx, trackIDs)
		if err != nil {
			return err
		}
		counts = m
	}

	navCounts := map[string]int64{}
	if s.navigations != nil {
		m, err := s.navigations.CountByTrackIDs(ctx, trackIDs)
		if err != nil {
			return err
		}
		navCounts = m
	}

	for _, summary := range summaries {
		if s.collects != nil {
			summary.CollectCount = counts[summary.ID]
		}
		if s.navigations != nil {
			summary.NavigateCount = navCounts[summary.ID]
		}
	}
	return nil
}

// fillTrackSummaryExtras 负责为 TrackSummary 列表补齐“跨表/跨仓储”的扩展字段。
//
// 为什么需要它：
// - TrackSummary 的核心字段来自 track_records，但 nickname/avatar_url、收藏数/收藏状态来自其他表。
// - 推荐列表与搜索列表都需要同一套补齐逻辑，集中在 service 里更易维护。
//
// 补齐的字段：
// - 用户信息：summary.Nickname / summary.UserAvatarURL（来自 users 表）
// - 收藏信息：summary.CollectCount（来自 track_collects 的聚合统计）、summary.Collected（当前鉴权用户是否收藏）
//
// 性能与一致性说明：
// - collect_count 通过 CountByTrackIDs 批量聚合，避免逐条 COUNT；
// - 用户信息按 user_id 去重后查询（仍是逐个 FindByID；若需要进一步减少 SQL 次数，可在 UserRepository 增加 FindByIDs 批量接口）；
// - collected 目前仍是按轨迹逐条 IsCollected（典型 N 次查询），后续可通过 CollectRepository 增加批量接口优化；
// - 该函数只负责“组装返回”，不保证强一致（例如用户刚改昵称/头像、刚收藏/取消收藏时可能存在短暂延迟）。
func (s *TrackService) fillTrackSummaryExtras(ctx context.Context, userID int64, summaries []*models.TrackSummary) error {
	if len(summaries) == 0 {
		return nil
	}

	// 1) 一次遍历收集所需的 trackIDs 和去重后的 userIDs。
	// 这样后续无论是聚合查询还是用户信息查询，都不需要再扫描 summaries 来构造入参。
	trackIDs := make([]string, 0, len(summaries))
	userIDs := make([]int64, 0, len(summaries))
	seenUserIDs := make(map[int64]struct{}, 8)
	for _, summary := range summaries {
		trackIDs = append(trackIDs, summary.ID)
		if summary.UserID > 0 {
			if _, ok := seenUserIDs[summary.UserID]; !ok {
				seenUserIDs[summary.UserID] = struct{}{}
				userIDs = append(userIDs, summary.UserID)
			}
		}
	}

	// 2) 先准备好“批量/去重”的数据源，再在最后一次遍历里一次性写回 summaries。
	// 这么做的好处是：
	// - 写回逻辑只需要一轮 for；
	// - 数据源（counts/users）都以 map 形式存在，查找是 O(1)。
	counts := map[string]int64{}
	if s.collects != nil {
		m, err := s.collects.CountByTrackIDs(ctx, trackIDs)
		if err != nil {
			return err
		}
		counts = m
	}

	navCounts := map[string]int64{}
	if s.navigations != nil {
		m, err := s.navigations.CountByTrackIDs(ctx, trackIDs)
		if err != nil {
			return err
		}
		navCounts = m
	}

	type userBrief struct {
		avatar string
		nick   string
	}
	users := make(map[int64]userBrief, len(userIDs))
	if s.users != nil {
		for _, uid := range userIDs {
			u, err := s.users.FindByID(ctx, uid)
			switch {
			case err == nil && u != nil:
				users[uid] = userBrief{avatar: fallbackAvatarURL(uid, u.AvatarURL), nick: u.Nickname}
			case errors.Is(err, repository.ErrNotFound):
				users[uid] = userBrief{avatar: defaultAvatarURL(uid)}
			case err != nil:
				return err
			}
		}
	}

	// 3) 最后一轮遍历：把准备好的扩展字段写回到每个 summary。
	for _, summary := range summaries {
		// 导航使用次数
		if s.navigations != nil {
			summary.NavigateCount = navCounts[summary.ID]
		}

		// 收藏总数
		if s.collects != nil {
			summary.CollectCount = counts[summary.ID]
			// 当前用户是否收藏（需要 userID）
			if userID > 0 {
				collected, err := s.collects.IsCollected(ctx, userID, summary.ID)
				if err != nil {
					return err
				}
				summary.Collected = collected
			}
		}

		// 用户昵称/头像
		if s.users != nil && summary.UserID > 0 {
			if brief, ok := users[summary.UserID]; ok {
				summary.UserAvatarURL = brief.avatar
				summary.Nickname = brief.nick
			}
		}
	}
	return nil
}

// haversineDistance computes distance between two lat/lng points in meters.
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000.0
	toRad := func(d float64) float64 { return d * math.Pi / 180 }
	phi1 := toRad(lat1)
	phi2 := toRad(lat2)
	deltaPhi := toRad(lat2 - lat1)
	deltaLambda := toRad(lon2 - lon1)

	a := math.Sin(deltaPhi/2)*math.Sin(deltaPhi/2) +
		math.Cos(phi1)*math.Cos(phi2)*math.Sin(deltaLambda/2)*math.Sin(deltaLambda/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadius * c
}
