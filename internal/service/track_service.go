package service

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

// TrackService provides business logic around track lifecycle and statistics.
type TrackService struct {
	tracks          repository.TrackRepository
	collects        repository.CollectRepository
	users           repository.UserRepository
	screenshotCache *AssetCacheService
	rawTrackCache   *AssetCacheService
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
	Title               *string    `json:"title"`
	CityCode            *string    `json:"city_code"`
	StartTime           *time.Time `json:"start_time"`
	EndTime             *time.Time `json:"end_time"`
	Distance            *float64   `json:"distance"`
	Duration            *uint32    `json:"duration"`
	ElevationGain       *int       `json:"elevation_gain"`
	RawTrackURL         *string    `json:"raw_track_url"`
	TrackScreenshotURL  *string    `json:"track_screenshot_url"`
	LegacyScreenshotURL *string    `json:"screenshot_url,omitempty"`
	IsRunning           *bool      `json:"is_running"`
	AvgSpeedKmh         *float64   `json:"avg_speed_kmh"`
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

// GetRunningTrack returns currently running track of the user if exists.
func (s *TrackService) GetRunningTrack(ctx context.Context, userID int64) (*models.Track, error) {
	track, err := s.tracks.FindRunningByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	s.decorateTrackAssets(ctx, track)
	return track, nil
}

// ListRecommend returns recommended tracks for the user.
func (s *TrackService) ListRecommend(ctx context.Context, userID int64, limit int) ([]*models.TrackSummary, error) {
	tracks, err := s.tracks.ListRecommend(ctx, userID, limit)
	if err != nil {
		return nil, err
	}
	summaries := toSummaries(tracks)
	// 填充服务器本地可下载截图 URL：
	// - 命中本地缓存则直接返回本地 URL
	// - 未命中则兜底拉取一次（同步，带 5 秒超时），失败则返回空串，不阻塞列表返回其它字段
	// - userID<=0（未登录游客）时无法申请 STS 读凭证，只尝试本地命中，不主动下载
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
	if err := s.fillTrackSummaryExtras(ctx, userID, summaries); err != nil {
		return nil, err
	}
	return summaries, nil
}

// SearchTracks searches tracks globally by keyword.
func (s *TrackService) SearchTracks(ctx context.Context, userID int64, keyword string, limit int) ([]*models.TrackSummary, error) {
	tracks, err := s.tracks.Search(ctx, keyword, limit)
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
	return summaries, nil
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
			CityName:      config.CityNameByCode(t.CityCode),
			Title:         t.Title,
			Distance:      t.Distance,
			Duration:      t.Duration,
			ElevationGain: t.ElevationGain,
		})
	}
	return res
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
				users[uid] = userBrief{avatar: u.AvatarURL, nick: u.Nickname}
			case errors.Is(err, repository.ErrNotFound):
				users[uid] = userBrief{}
			case err != nil:
				return err
			}
		}
	}

	// 3) 最后一轮遍历：把准备好的扩展字段写回到每个 summary。
	for _, summary := range summaries {
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
