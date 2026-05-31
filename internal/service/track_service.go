package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/url"
	"strconv"
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
	companions      repository.CompanionRepository
	achievements    *AchievementService
	screenshotCache *AssetCacheService
	avatarCache     *AssetCacheService
	rawTrackCache   *AssetCacheService
	trackTypes      []string
}

const (
	defaultTrackPageSize   = 20
	maxTrackPageSize       = 50
	trackTypeIconURLPrefix = "/api/v1/static/track_type_icon/"
	trackTypeAnimURLPrefix = "/api/v1/static/track_type_icon_anim/"
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

type ListCollectedTracksInput struct {
	Cursor string
	Limit  int
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
	SessionID                 *string    `json:"session_id"`
	CityCode                  *string    `json:"city_code"`
	LocateAddr                *string    `json:"locate_addr"`
	TrackType                 *string    `json:"track_type"`
	CoordinateSystem          *string    `json:"coordinate_system"`
	StartTime                 *time.Time `json:"start_time"`
	EndTime                   *time.Time `json:"end_time"`
	Distance                  *float64   `json:"distance"`
	Duration                  *uint32    `json:"duration"`
	CaloriesBurned            *float64   `json:"calories_burned"`
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
	if in.SessionID != nil {
		v := strings.TrimSpace(*in.SessionID)
		in.SessionID = &v
	}
}

func (in CreateTrackInput) validate() error {
	if in.SessionID != nil {
		if *in.SessionID == "" {
			return invalidArg("session_id is required")
		}
		if len(*in.SessionID) > 64 {
			return invalidArg("session_id is too long")
		}
	}
	if in.Distance != nil && *in.Distance < 0 {
		return invalidArg("distance must be >= 0")
	}
	if in.LocateAddr != nil && len(*in.LocateAddr) > 128 {
		return invalidArg("locate_addr is too long")
	}
	if in.ElevationGain != nil && *in.ElevationGain < 0 {
		return invalidArg("elevation_gain must be >= 0")
	}
	if in.AvgSpeedKmh != nil && *in.AvgSpeedKmh < 0 {
		return invalidArg("avg_speed_kmh must be >= 0")
	}
	if in.CaloriesBurned != nil && *in.CaloriesBurned < 0 {
		return invalidArg("calories_burned must be >= 0")
	}
	if in.StartTime != nil && in.EndTime != nil && in.EndTime.Before(*in.StartTime) {
		return invalidArg("end_time must be >= start_time")
	}
	return nil
}

// TrackInfoPatch describes which track summary fields should be updated.
// Nil means the field is not provided and should remain unchanged.
type TrackInfoPatch struct {
	SessionID                 *string  `json:"session_id"`
	CityCode                  *string  `json:"city_code"`
	LocateAddr                *string  `json:"locate_addr"`
	CoordinateSystem          *string  `json:"coordinate_system"`
	Distance                  *float64 `json:"distance"`
	Duration                  *uint32  `json:"duration"`
	ElevationGain             *int     `json:"elevation_gain"`
	RawTrackURL               *string  `json:"raw_track_url"`
	TrackScreenshotURL        *string  `json:"track_screenshot_url"`
	TrackNoMapBgScreenshotURL *string  `json:"track_no_map_bg_screenshot_url"`
	LegacyScreenshotURL       *string  `json:"screenshot_url,omitempty"`
	AvgSpeedKmh               *float64 `json:"avg_speed_kmh"`
}

func (p *TrackInfoPatch) normalize() {
	if p == nil {
		return
	}
	if p.TrackScreenshotURL == nil && p.LegacyScreenshotURL != nil {
		p.TrackScreenshotURL = p.LegacyScreenshotURL
	}
	if p.SessionID != nil {
		v := strings.TrimSpace(*p.SessionID)
		p.SessionID = &v
	}
}

func (p TrackInfoPatch) empty() bool {
	return p.SessionID == nil &&
		p.CityCode == nil &&
		p.LocateAddr == nil &&
		p.CoordinateSystem == nil &&
		p.Distance == nil &&
		p.Duration == nil &&
		p.ElevationGain == nil &&
		p.RawTrackURL == nil &&
		p.TrackScreenshotURL == nil &&
		p.TrackNoMapBgScreenshotURL == nil &&
		p.LegacyScreenshotURL == nil &&
		p.AvgSpeedKmh == nil
}

// NewTrackService constructs a new TrackService instance.
func NewTrackService(tracks repository.TrackRepository, collects repository.CollectRepository) *TrackService {
	return &TrackService{tracks: tracks, collects: collects, trackTypes: config.ParseTrackTypes("")}
}

// SetTrackTypes configures the list of track types returned to clients.
func (s *TrackService) SetTrackTypes(trackTypes []string) {
	s.trackTypes = config.ParseTrackTypes(strings.Join(trackTypes, ","))
}

// ListTrackTypes returns configured track types for clients to choose from.
func (s *TrackService) ListTrackTypes() []string {
	if len(s.trackTypes) == 0 {
		return config.ParseTrackTypes("")
	}
	return append([]string(nil), s.trackTypes...)
}

// ListTrackTypeOptions returns configured track types with icon URLs.
func (s *TrackService) ListTrackTypeOptions() []models.TrackTypeOption {
	types := s.ListTrackTypes()
	items := make([]models.TrackTypeOption, 0, len(types))
	for _, trackType := range types {
		meta, ok := config.TrackTypeConfigByName(trackType)
		typeCode := trackType
		themeColor := ""
		iconAnimURL := ""
		if ok {
			typeCode = meta.Type
			themeColor = meta.ThemeColor
			if meta.IconAnimFile != "" {
				iconAnimURL = trackTypeAnimURLPrefix + meta.IconAnimFile
			}
		}
		items = append(items, models.TrackTypeOption{
			Type:        typeCode,
			Name:        trackType,
			ThemeColor:  themeColor,
			IconURL:     trackTypeIconURL(trackType),
			IconAnimURL: iconAnimURL,
		})
	}
	return items
}

// ListTrackTypeOptionsWithStats returns configured track types plus current user's month/year stats.
func (s *TrackService) ListTrackTypeOptionsWithStats(ctx context.Context, userID int64) ([]models.TrackTypeOption, error) {
	if userID <= 0 {
		return nil, invalidArg("userID is required")
	}
	items := s.ListTrackTypeOptions()
	if s.tracks == nil || len(items) == 0 {
		return items, nil
	}

	statsByType := make(map[string]*models.TrackTypeMilestone, len(items))
	for i := range items {
		statsByType[items[i].Name] = &items[i].Milestone
	}

	now := time.Now()
	monthSince := now.AddDate(0, -1, 0)
	yearSince := now.AddDate(-1, 0, 0)

	var cursor *models.TrackListCursor
	for {
		tracks, err := s.tracks.ListByUserID(ctx, userID, cursor, maxTrackPageSize)
		if err != nil {
			return nil, err
		}
		if len(tracks) == 0 {
			break
		}
		for _, track := range tracks {
			milestone := statsByType[track.TrackType]
			if milestone == nil {
				continue
			}
			trackTime := track.StartTime
			if trackTime.IsZero() {
				trackTime = track.CreatedAt
			}
			if !trackTime.Before(monthSince) {
				accumulateTrackMilestone(&milestone.Month, track)
			}
			if !trackTime.Before(yearSince) {
				accumulateTrackMilestone(&milestone.Year, track)
			}
		}
		if len(tracks) < maxTrackPageSize {
			break
		}
		last := tracks[len(tracks)-1]
		cursor = &models.TrackListCursor{StartTime: last.StartTime, ID: last.ID}
	}
	return items, nil
}

func accumulateTrackMilestone(dst *models.TrackTypeMilestoneStats, track *models.Track) {
	if dst == nil || track == nil {
		return
	}
	dst.Distance += track.Distance
	dst.TrackCount++
	dst.Duration += int64(track.Duration)
	dst.Calories += track.CaloriesBurned
}

func trackTypeIconURL(trackType string) string {
	if file, ok := config.TrackTypeIconFile(trackType); ok {
		return trackTypeIconURLPrefix + file
	}
	return trackTypeIconURLPrefix + url.PathEscape(trackType) + ".svg"
}

// GetUserTrackStats returns per-user track statistics.
func (s *TrackService) GetUserTrackStats(ctx context.Context, userID int64) (*models.TrackUserStats, error) {
	stats := &models.TrackUserStats{}
	if userID <= 0 {
		return stats, invalidArg("userID is required")
	}
	if s.tracks == nil {
		return stats, nil
	}
	return s.tracks.StatsSummaryByUserID(ctx, userID)
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

// SetCompanionRepository 注入同行仓储，用于约束普通轨迹与同行同时只能参与一个。
func (s *TrackService) SetCompanionRepository(companions repository.CompanionRepository) {
	s.companions = companions
}

// SetAchievementService injects achievement settlement service.
func (s *TrackService) SetAchievementService(achievements *AchievementService) {
	s.achievements = achievements
}

// SetScreenshotCache 设置截图本地缓存服务。
// 独立于构造函数是为了避免破坏既有单测/调用方；在未设置时，相关逻辑会直接跳过。
func (s *TrackService) SetScreenshotCache(cache *AssetCacheService) {
	s.screenshotCache = cache
}

// SetAvatarCache 设置头像本地缓存服务。
// 仅用于需要把 OSS 头像地址改写为服务端静态资源地址的列表场景。
func (s *TrackService) SetAvatarCache(cache *AssetCacheService) {
	s.avatarCache = cache
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
	if isRunning {
		if err := s.ensureNoActiveCompanionSession(ctx, userID); err != nil {
			return nil, err
		}
	}
	track := &models.Track{
		ID:        trackID,
		UserID:    userID,
		SessionID: "",
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
	if input.SessionID != nil {
		track.SessionID = *input.SessionID
	}
	if input.CityCode != nil {
		track.CityCode = *input.CityCode
	}
	if input.LocateAddr != nil {
		track.LocateAddr = *input.LocateAddr
	}
	if input.TrackType != nil {
		track.TrackType = *input.TrackType
	}
	if input.CoordinateSystem != nil {
		track.CoordinateSystem = *input.CoordinateSystem
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
	if input.CaloriesBurned != nil {
		track.CaloriesBurned = *input.CaloriesBurned
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
	if !track.IsRunning && s.achievements != nil {
		if _, err := s.achievements.SettleTrackCompleted(ctx, track); err != nil {
			return nil, err
		}
	}
	return track, nil
}

func (s *TrackService) ensureNoActiveCompanionSession(ctx context.Context, userID int64) error {
	if s == nil || s.companions == nil {
		return nil
	}
	session, err := s.companions.FindActiveSessionByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return err
	}
	title := strings.TrimSpace(session.Title)
	if title == "" {
		title = defaultCompanionTitle
	}
	return invalidArg(fmt.Sprintf("you already joined an active companion session: %s", title))
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

func decodeTrackCollectCursor(raw string) (*models.TrackCollectCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	buf, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, invalidArg("invalid cursor")
	}
	var cursor models.TrackCollectCursor
	if err := json.Unmarshal(buf, &cursor); err != nil {
		return nil, invalidArg("invalid cursor")
	}
	if cursor.TrackID == "" || cursor.CreatedAt.IsZero() {
		return nil, invalidArg("invalid cursor")
	}
	return &cursor, nil
}

func encodeTrackCollectCursor(createdAt time.Time, trackID string) (string, error) {
	buf, err := json.Marshal(models.TrackCollectCursor{CreatedAt: createdAt, TrackID: trackID})
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

func buildMyTrackSummaryPage(summaries []*models.MyTrackSummary, hasMore bool, totalCount int64) (*models.MyTrackSummaryPage, error) {
	page := &models.MyTrackSummaryPage{Items: summaries, HasMore: hasMore, TotalCount: totalCount}
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
	tracks, hasMore, err := s.listTracksWithNonEmptyRawTrackURL(ctx, cursor, limit, func(cur *models.TrackListCursor, n int) ([]*models.Track, error) {
		return s.tracks.ListRecommend(ctx, userID, cur, n)
	})
	if err != nil {
		return nil, err
	}
	summaries := toSummaries(tracks)
	// 填充服务器本地可下载截图 URL：
	// - 命中本地缓存则直接返回本地 URL
	// - 未命中则兜底拉取一次（同步，带 5 秒超时），失败则返回空串，不阻塞列表返回其它字段
	// - userID<=0（未登录游客）时无法申请 STS 读凭证，只尝试本地命中，不主动下载
	// 排查日志（轻量）：若列表返回的 track_screenshot_url 为空，输出汇总计数，避免逐条日志刷屏。
	var (
		srcEmptySS, resEmptySS int
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
				if t.RawTrackURL != "" {
					local := s.rawTrackCache.EnsureCached(cacheCtx, userID, t.ID, t.RawTrackURL)
					summaries[i].RawTrackURL = local
				}
			}
			cancel()
		}
		// 仅在出现“源为空 / 结果为空”时打汇总日志，避免正常请求刷屏。
		if srcEmptySS > 0 || resEmptySS > 0 {
			log.Printf("[ListRecommend][asset] summary user_id=%d tracks=%d screenshot_src_empty=%d screenshot_result_empty=%d", userID, len(tracks), srcEmptySS, resEmptySS)
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
	totalCount, err := s.countMyTracksTotalCount(ctx, userID)
	if err != nil {
		return nil, err
	}
	tracks, hasMore, err := s.listTracksWithNonEmptyRawTrackURL(ctx, cursor, limit, func(cur *models.TrackListCursor, n int) ([]*models.Track, error) {
		return s.tracks.ListByUserID(ctx, userID, cur, n)
	})
	if err != nil {
		return nil, err
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
	return buildMyTrackSummaryPage(summaries, hasMore, totalCount)
}

func (s *TrackService) countMyTracksTotalCount(ctx context.Context, userID int64) (int64, error) {
	if userID <= 0 {
		return 0, nil
	}
	if s.tracks == nil {
		return 0, nil
	}
	// 优先走仓储侧聚合（仅统计 raw_track_url 非空的轨迹）。
	if p, ok := s.tracks.(interface {
		CountByUserIDWithNonEmptyRawTrackURL(ctx context.Context, userID int64) (int64, error)
	}); ok {
		return p.CountByUserIDWithNonEmptyRawTrackURL(ctx, userID)
	}

	// fallback：逐页扫描（用于非 SQL/测试实现）。
	var (
		cursor *models.TrackListCursor
		limit  = 200
		count  int64
	)
	for page := 0; page < 1000; page++ { // guard: 避免异常实现导致死循环
		items, err := s.tracks.ListByUserID(ctx, userID, cursor, limit)
		if err != nil {
			return 0, err
		}
		if len(items) == 0 {
			break
		}
		for _, t := range items {
			if t == nil {
				continue
			}
			if t.RawTrackURL == "" {
				continue
			}
			count++
		}
		last := items[len(items)-1]
		if last == nil || last.ID == "" || last.StartTime.IsZero() {
			break
		}
		cursor = &models.TrackListCursor{StartTime: last.StartTime, ID: last.ID}
		if len(items) < limit {
			break
		}
	}
	return count, nil
}

type collectVisibleCounter interface {
	CountVisibleByUserID(ctx context.Context, userID int64) (int64, error)
}

func (s *TrackService) countVisibleCollectedTotalCount(ctx context.Context, userID int64) (int64, error) {
	if userID <= 0 {
		return 0, nil
	}
	if s.collects == nil || s.tracks == nil {
		return 0, nil
	}
	if c, ok := s.collects.(collectVisibleCounter); ok {
		return c.CountVisibleByUserID(ctx, userID)
	}

	// fallback：逐页扫描收藏记录 + FindByID 过滤（用于非 SQL/测试实现）。
	var (
		cursor *models.TrackCollectCursor
		limit  = 200
		count  int64
	)
	for page := 0; page < 10000; page++ { // guard
		collects, err := s.collects.ListByUserID(ctx, userID, cursor, limit)
		if err != nil {
			return 0, err
		}
		if len(collects) == 0 {
			break
		}
		for _, c := range collects {
			if c == nil || c.TrackID == "" {
				continue
			}
			t, err := s.tracks.FindByID(ctx, c.TrackID)
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					continue
				}
				return 0, err
			}
			if t != nil && t.Status == models.TrackStatusNormal && !t.IsRunning {
				count++
			}
		}
		last := collects[len(collects)-1]
		if last == nil || last.TrackID == "" || last.CreatedAt.IsZero() {
			break
		}
		cursor = &models.TrackCollectCursor{CreatedAt: last.CreatedAt, TrackID: last.TrackID}
		if len(collects) < limit {
			break
		}
	}
	return count, nil
}

// SearchTracks searches tracks globally by keyword.
func (s *TrackService) SearchTracks(ctx context.Context, userID int64, input SearchTracksInput) (*models.TrackSummaryPage, error) {
	limit := normalizeTrackPageLimit(input.Limit)
	cursor, err := decodeTrackListCursor(input.Cursor)
	if err != nil {
		return nil, err
	}
	tracks, hasMore, err := s.listTracksWithNonEmptyRawTrackURL(ctx, cursor, limit, func(cur *models.TrackListCursor, n int) ([]*models.Track, error) {
		return s.tracks.Search(ctx, input.Keyword, cur, n)
	})
	if err != nil {
		return nil, err
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

// listTracksWithNonEmptyRawTrackURL scans pages from repository and only keeps tracks with non-empty RawTrackURL.
//
// This is used by ListRecommend/SearchTracks to filter out tracks that do not have raw_track_url.
// It makes a bounded number of additional repository calls to compensate for filtered items.
func (s *TrackService) listTracksWithNonEmptyRawTrackURL(
	ctx context.Context,
	cursor *models.TrackListCursor,
	limit int,
	fetch func(cur *models.TrackListCursor, n int) ([]*models.Track, error),
) ([]*models.Track, bool, error) {
	if limit <= 0 {
		return []*models.Track{}, false, nil
	}

	// 过滤后仍尽量凑满一页：多取一些并允许最多多轮扫描。
	const maxRounds = 5
	batch := limit*5 + 1
	if batch < limit+1 {
		batch = limit + 1
	}
	if batch > 200 {
		batch = 200
	}

	scanCursor := cursor
	res := make([]*models.Track, 0, limit+1)
	for round := 0; round < maxRounds && len(res) < limit+1; round++ {
		items, err := fetch(scanCursor, batch)
		if err != nil {
			return nil, false, err
		}
		if len(items) == 0 {
			break
		}
		for _, t := range items {
			if t == nil {
				continue
			}
			if t.RawTrackURL == "" {
				continue
			}
			res = append(res, t)
			if len(res) >= limit+1 {
				break
			}
		}
		last := items[len(items)-1]
		if last == nil || last.ID == "" || last.StartTime.IsZero() {
			break
		}
		scanCursor = &models.TrackListCursor{StartTime: last.StartTime, ID: last.ID}
		if len(items) < batch {
			break
		}
	}

	hasMore := len(res) > limit
	if hasMore {
		res = res[:limit]
	}
	return res, hasMore, nil
}

// ListCollectedTracks 返回“当前用户已收藏的轨迹列表”。
//
// 返回结构约定：
// - 外层分页结构与 ListRecommend 保持一致（items/next_cursor/has_more）。
// - items 字段与 TrackSummary 基本一致，但**不返回** `collected` 字段：因为该列表内的轨迹天然已收藏。
//
// 排序/翻页约定：
// - 按收藏记录的 created_at 倒序（created_at desc, track_id desc）翻页。
// - cursor 是 TrackCollectCursor 的 base64(JSON) 结果，客户端应原样透传。
func (s *TrackService) ListCollectedTracks(ctx context.Context, userID int64, input ListCollectedTracksInput) (*models.CollectedTrackSummaryPage, error) {
	if userID <= 0 {
		return nil, invalidArg("userID is required")
	}
	if s.collects == nil {
		return nil, errors.New("collect repository not configured")
	}
	limit := normalizeTrackPageLimit(input.Limit)
	cur, err := decodeTrackCollectCursor(input.Cursor)
	if err != nil {
		return nil, err
	}
	totalCount, err := s.countVisibleCollectedTotalCount(ctx, userID)
	if err != nil {
		return nil, err
	}

	// 先拉收藏记录，再逐条通过 TrackRepository.FindByID 解析到 track_records。
	// NOTE: CollectRepository.ListByUserID 不做 join（保持仓储职责单一），因此这里需要二次查询解析轨迹。
	//
	// 约束/假设：
	// - 当轨迹被删除时，DeleteTrack 会同步调用 CollectRepository.RemoveByTrackID 清理收藏记录；
	//   因此这里不需要通过“多轮补齐”来对抗大量无效收藏记录。
	batch := limit + 1 // 取多 1 条用于判断 has_more

	recs, err := s.collects.ListByUserID(ctx, userID, cur, batch)
	if err != nil {
		return nil, err
	}

	tracks := make([]*models.Track, 0, len(recs))
	includedCursors := make([]models.TrackCollectCursor, 0, len(recs))
	for _, rec := range recs {
		t, err := s.tracks.FindByID(ctx, rec.TrackID)
		switch {
		case err == nil && t != nil:
			// 与推荐/搜索列表保持一致：排除删除/私密/进行中轨迹。
			// 理论上：删除轨迹会同步清理收藏记录，这里更多是兜底保护。
			if t.Status != models.TrackStatusNormal || t.IsRunning {
				continue
			}
			tracks = append(tracks, t)
			includedCursors = append(includedCursors, models.TrackCollectCursor{CreatedAt: rec.CreatedAt, TrackID: rec.TrackID})
		case errors.Is(err, repository.ErrNotFound):
			continue
		case err != nil:
			return nil, err
		}
	}

	hasMore := len(tracks) > limit
	var nextCursorSrc *models.TrackCollectCursor
	if hasMore {
		// Use the cursor of the last returned item (the limit-th), not the extra one.
		c := includedCursors[limit-1]
		nextCursorSrc = &c
		tracks = tracks[:limit]
	} else if len(tracks) > 0 {
		c := includedCursors[len(includedCursors)-1]
		nextCursorSrc = &c
	}

	summaries := toSummaries(tracks)
	// Fill asset local URLs (same as recommend/search).
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
	// Reuse TrackSummary filling logic but skip per-item `collected` check.
	if err := s.fillTrackSummaryExtras(ctx, 0, summaries); err != nil {
		return nil, err
	}

	items := make([]*models.CollectedTrackSummary, 0, len(summaries))
	for _, s := range summaries {
		items = append(items, &models.CollectedTrackSummary{
			ID:                        s.ID,
			UserID:                    s.UserID,
			SessionID:                 s.SessionID,
			CityCode:                  s.CityCode,
			LocateAddr:                s.LocateAddr,
			TrackType:                 s.TrackType,
			StartTime:                 s.StartTime,
			EndTime:                   s.EndTime,
			CityName:                  s.CityName,
			Nickname:                  s.Nickname,
			UserAvatarURL:             s.UserAvatarURL,
			Title:                     s.Title,
			Distance:                  s.Distance,
			Duration:                  s.Duration,
			AvgSpeedKmh:               s.AvgSpeedKmh,
			CaloriesBurned:            s.CaloriesBurned,
			ElevationGain:             s.ElevationGain,
			CollectCount:              s.CollectCount,
			NavigateCount:             s.NavigateCount,
			TrackScreenshotURL:        s.TrackScreenshotURL,
			TrackNoMapBgScreenshotURL: s.TrackNoMapBgScreenshotURL,
			RawTrackURL:               s.RawTrackURL,
		})
	}

	page := &models.CollectedTrackSummaryPage{Items: items, HasMore: hasMore, TotalCount: totalCount}
	if hasMore && nextCursorSrc != nil {
		nextCursor, err := encodeTrackCollectCursor(nextCursorSrc.CreatedAt, nextCursorSrc.TrackID)
		if err != nil {
			return nil, err
		}
		page.NextCursor = nextCursor
	}
	return page, nil
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
	if err := s.tracks.Update(ctx, track); err != nil {
		return err
	}
	if s.achievements != nil {
		_, err := s.achievements.SettleTrackCompleted(ctx, track)
		return err
	}
	return nil
}

// UpdateTrackInfo updates a track with partially provided fields.
func (s *TrackService) UpdateTrackInfo(ctx context.Context, userID int64, trackID string, patch TrackInfoPatch) (*models.Track, error) {
	if userID <= 0 {
		return nil, invalidArg("userID is required")
	}
	if trackID == "" {
		return nil, invalidArg("trackID is required")
	}
	patch.normalize()
	if patch.empty() {
		return nil, invalidArg("no fields to update")
	}
	if patch.Distance != nil && *patch.Distance < 0 {
		return nil, invalidArg("distance must be >= 0")
	}
	if patch.LocateAddr != nil && len(*patch.LocateAddr) > 128 {
		return nil, invalidArg("locate_addr is too long")
	}
	if patch.ElevationGain != nil && *patch.ElevationGain < 0 {
		return nil, invalidArg("elevation_gain must be >= 0")
	}
	if patch.AvgSpeedKmh != nil && *patch.AvgSpeedKmh < 0 {
		return nil, invalidArg("avg_speed_kmh must be >= 0")
	}
	if patch.SessionID != nil {
		if *patch.SessionID == "" {
			return nil, invalidArg("session_id is required")
		}
		if len(*patch.SessionID) > 64 {
			return nil, invalidArg("session_id is too long")
		}
	}
	track, err := s.tracks.FindByID(ctx, trackID)
	if err != nil {
		return nil, err
	}
	if track.UserID != userID {
		return nil, ErrForbidden
	}

	updated := false
	updatedRaw := false
	updatedScreenshot := false
	updatedNoMapBgScreenshot := false

	// 只允许“补全”空字段：若数据库已有值，则忽略该字段更新。
	if patch.SessionID != nil && track.SessionID == "" {
		track.SessionID = *patch.SessionID
		updated = true
	}
	if patch.CityCode != nil && track.CityCode == "" {
		track.CityCode = *patch.CityCode
		updated = true
	}
	if patch.LocateAddr != nil && track.LocateAddr == "" {
		track.LocateAddr = *patch.LocateAddr
		updated = true
	}
	if patch.CoordinateSystem != nil && track.CoordinateSystem == "" {
		track.CoordinateSystem = *patch.CoordinateSystem
		updated = true
	}
	if patch.Distance != nil && track.Distance == 0 {
		track.Distance = *patch.Distance
		updated = true
	}
	if patch.Duration != nil && track.Duration == 0 {
		track.Duration = *patch.Duration
		updated = true
	}
	if patch.ElevationGain != nil && track.ElevationGain == 0 {
		track.ElevationGain = *patch.ElevationGain
		updated = true
	}
	if patch.AvgSpeedKmh != nil && track.AvgSpeedKmh == 0 {
		track.AvgSpeedKmh = *patch.AvgSpeedKmh
		updated = true
	}
	if patch.RawTrackURL != nil && track.RawTrackURL == "" {
		track.RawTrackURL = *patch.RawTrackURL
		updated = true
		updatedRaw = true
	}
	if patch.TrackScreenshotURL != nil && track.TrackScreenshotURL == "" {
		track.TrackScreenshotURL = *patch.TrackScreenshotURL
		updated = true
		updatedScreenshot = true
	}
	if patch.TrackNoMapBgScreenshotURL != nil && track.TrackNoMapBgScreenshotURL == "" {
		track.TrackNoMapBgScreenshotURL = *patch.TrackNoMapBgScreenshotURL
		updated = true
		updatedNoMapBgScreenshot = true
	}

	if updated {
		if err := s.tracks.Update(ctx, track); err != nil {
			return nil, err
		}
		if !track.IsRunning && s.achievements != nil {
			if _, err := s.achievements.SettleTrackCompleted(ctx, track); err != nil {
				return nil, err
			}
		}
	}

	// 轨迹资源链接字段对客户端下发本地可下载 URL（并触发异步预热）。
	if s.screenshotCache != nil {
		if updatedScreenshot {
			src := track.TrackScreenshotURL
			s.screenshotCache.PrefetchAsync(userID, track.ID, src)
			track.TrackScreenshotURL = s.screenshotCache.GuessLocalURL(track.ID, src)
		}
		if updatedNoMapBgScreenshot {
			key := track.ID + "_no_map_bg"
			src := track.TrackNoMapBgScreenshotURL
			s.screenshotCache.PrefetchAsync(userID, key, src)
			track.TrackNoMapBgScreenshotURL = s.screenshotCache.GuessLocalURL(key, src)
		}
	}
	if s.rawTrackCache != nil && updatedRaw {
		src := track.RawTrackURL
		s.rawTrackCache.PrefetchAsync(userID, track.ID, src)
		track.RawTrackURL = s.rawTrackCache.GuessLocalURL(track.ID, src)
	}
	return track, nil
}

// DeleteTrack performs a soft delete for a track.
// It only marks status as deleted and records deleted_at.
func (s *TrackService) DeleteTrack(ctx context.Context, userID int64, trackID string) error {
	if userID <= 0 {
		return invalidArg("userID is required")
	}
	if trackID == "" {
		return invalidArg("trackID is required")
	}
	// MySQL：把“软删除轨迹 + 清理收藏记录”放到同一个事务中，保证一致性。
	if txDeleter, ok := s.tracks.(interface {
		SoftDeleteAndCleanupCollectsTx(ctx context.Context, userID int64, trackID string) error
	}); ok {
		// 注意：走到这里意味着底层仓储（MySQL）会在事务里同时完成
		// - track_records 的软删除
		// - track_collects 的清理
		// 因此此分支成功后需要直接 return，不能再走下面的“非事务兜底逻辑”（否则会重复 Update/重复删除收藏）。
		if err := txDeleter.SoftDeleteAndCleanupCollectsTx(ctx, userID, trackID); err != nil {
			if errors.Is(err, repository.ErrForbidden) {
				return ErrForbidden
			}
			return err
		}
		return nil
	}

	track, err := s.tracks.FindByID(ctx, trackID)
	if err != nil {
		return err
	}
	if track.UserID != userID {
		return ErrForbidden
	}
	// 软删除：仅标记 status=0，并记录删除时间。
	if track.Status != models.TrackStatusDeleted {
		track.Status = models.TrackStatusDeleted
	}
	if track.DeletedAt.IsZero() {
		track.DeletedAt = time.Now()
	}
	// 删除后不应再被视为进行中。
	track.IsRunning = false
	if err := s.tracks.Update(ctx, track); err != nil {
		return err
	}
	// 同步清理收藏记录：用户删除轨迹后，track_collects 中不应继续保留该轨迹的收藏关系。
	if s.collects != nil {
		if err := s.collects.RemoveByTrackID(ctx, trackID); err != nil {
			return err
		}
	}
	return nil
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
			ID:             t.ID,
			UserID:         t.UserID,
			SessionID:      t.SessionID,
			CityCode:       t.CityCode,
			LocateAddr:     t.LocateAddr,
			TrackType:      t.TrackType,
			StartTime:      t.StartTime,
			EndTime:        t.EndTime,
			CityName:       config.CityNameByCode(t.CityCode),
			Title:          t.Title,
			Distance:       t.Distance,
			Duration:       t.Duration,
			AvgSpeedKmh:    t.AvgSpeedKmh,
			CaloriesBurned: t.CaloriesBurned,
			ElevationGain:  t.ElevationGain,
		})
	}
	return res
}

func toMySummaries(tracks []*models.Track) []*models.MyTrackSummary {
	res := make([]*models.MyTrackSummary, 0, len(tracks))
	for _, t := range tracks {
		res = append(res, &models.MyTrackSummary{
			ID:             t.ID,
			UserID:         t.UserID,
			SessionID:      t.SessionID,
			CityCode:       t.CityCode,
			LocateAddr:     t.LocateAddr,
			TrackType:      t.TrackType,
			StartTime:      t.StartTime,
			EndTime:        t.EndTime,
			CityName:       config.CityNameByCode(t.CityCode),
			Title:          t.Title,
			Distance:       t.Distance,
			Duration:       t.Duration,
			AvgSpeedKmh:    t.AvgSpeedKmh,
			CaloriesBurned: t.CaloriesBurned,
			ElevationGain:  t.ElevationGain,
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
				avatar := fallbackAvatarURL(uid, u.AvatarURL)
				if s.avatarCache != nil && shouldRewriteAvatarURL(avatar) {
					cacheCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
					if local := s.avatarCache.EnsureCached(cacheCtx, uid, formatUserAvatarCacheKey(uid), avatar); local != "" {
						avatar = local
					}
					cancel()
				}
				users[uid] = userBrief{avatar: avatar, nick: u.Nickname}
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

func shouldRewriteAvatarURL(avatarURL string) bool {
	if avatarURL == "" || strings.HasPrefix(avatarURL, "/api/v1/static/") {
		return false
	}
	u, err := url.Parse(avatarURL)
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return strings.Contains(host, ".aliyuncs.com") || strings.Contains(host, ".aliyun-inc.com")
}

func formatUserAvatarCacheKey(userID int64) string {
	return strings.TrimSpace(strconv.FormatInt(userID, 10))
}
