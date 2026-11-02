package service

import (
	"context"
	"errors"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

// AppReleaseService 负责 App 发布与升级检查相关的业务逻辑。
type AppReleaseService struct {
	repo          repository.AppReleaseRepository
	statsRepo     repository.AppClientStatsRepository
	changelogRepo repository.AppReleaseChangelogRepository
}

// NewAppReleaseService 构造 AppReleaseService。
func NewAppReleaseService(repo repository.AppReleaseRepository) *AppReleaseService {
	return &AppReleaseService{repo: repo}
}

// SetClientStatsRepository 注入客户端活跃统计仓储（可选，未注入时跳过采集与统计）。
func (s *AppReleaseService) SetClientStatsRepository(statsRepo repository.AppClientStatsRepository) {
	s.statsRepo = statsRepo
}

// SetChangelogRepository 注入版本更新日志仓储（可选，未注入时跳过 changelog 留存）。
func (s *AppReleaseService) SetChangelogRepository(changelogRepo repository.AppReleaseChangelogRepository) {
	s.changelogRepo = changelogRepo
}

// ListChangelogs 返回独立留存的版本更新日志（按版本号倒序，不受发布记录删除影响）。
func (s *AppReleaseService) ListChangelogs(ctx context.Context, platform models.AppReleasePlatform) ([]*models.AppReleaseChangelog, error) {
	if s == nil || s.changelogRepo == nil {
		return nil, errors.New("app release changelog not configured")
	}
	return s.changelogRepo.List(ctx, platform)
}

// RecordClientActivity 幂等记录一台设备的最近活跃信息。
// 由升级检查（匿名，userID=0）和登录成功（绑定 userID）调用；
// 采集失败只记录日志，不影响主流程，调用方无需处理返回错误之外的逻辑。
func (s *AppReleaseService) RecordClientActivity(ctx context.Context, meta *models.RequestMeta, userID int64, fallbackVersionCode int64) {
	if s == nil || s.statsRepo == nil || meta == nil {
		return
	}
	deviceID := strings.TrimSpace(meta.DeviceID)
	if deviceID == "" {
		return
	}
	versionCode := meta.VersionCode
	if versionCode <= 0 {
		versionCode = fallbackVersionCode
	}
	platform := strings.TrimSpace(meta.Platform)
	if platform != string(models.AppReleasePlatformAndroid) && platform != string(models.AppReleasePlatformIOS) {
		platform = ""
	}
	device := &models.AppClientDevice{
		DeviceID:    deviceID,
		UserID:      userID,
		Platform:    platform,
		AppVersion:  strings.TrimSpace(meta.ClientVersion),
		VersionCode: versionCode,
		DeviceModel: strings.TrimSpace(meta.DeviceModel),
		OSVersion:   strings.TrimSpace(meta.OSVersion),
		LastSeenAt:  time.Now(),
	}
	if err := s.statsRepo.TouchClientDevice(ctx, device); err != nil {
		log.Printf("record client activity failed: device=%s err=%v", deviceID, err)
	}
}

// AppClientStatisticsResult 是后台客户端统计的完整响应。
type AppClientStatisticsResult struct {
	*models.AppClientStatistics
	Latest *models.AppClientLatestSummary `json:"latest,omitempty"`
}

// GetClientStatistics 返回最近 days 天的客户端统计，并结合最新发布版本计算渗透概况。
func (s *AppReleaseService) GetClientStatistics(ctx context.Context, platform models.AppReleasePlatform, days int) (*AppClientStatisticsResult, error) {
	if s == nil || s.statsRepo == nil {
		return nil, errors.New("app client stats not configured")
	}
	if days != 7 && days != 30 {
		days = 30
	}
	stats, err := s.statsRepo.GetClientStatistics(ctx, days)
	if err != nil {
		return nil, err
	}
	result := &AppClientStatisticsResult{AppClientStatistics: stats}

	latest, err := s.repo.GetLatestPublished(ctx, platform)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return result, nil
		}
		return nil, err
	}
	summary := &models.AppClientLatestSummary{
		VersionName:             latest.VersionName,
		VersionCode:             latest.VersionCode,
		MinSupportedVersionCode: latest.MinSupportedVersionCode,
	}
	for _, item := range stats.VersionDistribution {
		if item.VersionCode == latest.VersionCode {
			summary.ActiveDevices += item.Devices
		}
		if item.VersionCode > 0 && item.VersionCode < latest.MinSupportedVersionCode {
			summary.BelowMinSupported += item.Devices
		}
	}
	if stats.ActiveDevices > 0 {
		summary.SharePercent = int(summary.ActiveDevices * 100 / stats.ActiveDevices)
	}
	result.Latest = summary
	return result, nil
}

// PublishReleaseInput 是管理后台发布版本的入参。
type PublishReleaseInput struct {
	Platform                models.AppReleasePlatform
	VersionName             string
	VersionCode             int64
	MinSupportedVersionCode int64
	PackageURL              string
	PackageSize             int64
	PackageMD5              string
	ReleaseNotes            string
	ForceUpdate             bool
	Status                  models.AppReleaseStatus
	OperatorUserID          int64
	OperatorName            string
}

// Publish 发布或更新一个版本。同 (platform, version_code) 视为同一个版本，反复提交会覆盖。
func (s *AppReleaseService) Publish(ctx context.Context, in PublishReleaseInput) (*models.AppRelease, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("app release service not configured")
	}
	if in.Platform != models.AppReleasePlatformAndroid && in.Platform != models.AppReleasePlatformIOS {
		return nil, invalidArg("platform must be android or ios")
	}
	if strings.TrimSpace(in.VersionName) == "" {
		return nil, invalidArg("version_name is required")
	}
	if in.VersionCode <= 0 {
		return nil, invalidArg("version_code must be > 0")
	}
	if in.MinSupportedVersionCode < 0 {
		return nil, invalidArg("min_supported_version_code must be >= 0")
	}
	if in.MinSupportedVersionCode > in.VersionCode {
		return nil, invalidArg("min_supported_version_code must be <= version_code")
	}
	if strings.TrimSpace(in.PackageURL) == "" {
		return nil, invalidArg("package_url is required")
	}
	status := in.Status
	if status == "" {
		status = models.AppReleaseStatusPublished
	}
	if status != models.AppReleaseStatusDraft &&
		status != models.AppReleaseStatusPublished &&
		status != models.AppReleaseStatusArchived {
		return nil, invalidArg("invalid status")
	}

	release := &models.AppRelease{
		Platform:                in.Platform,
		VersionName:             strings.TrimSpace(in.VersionName),
		VersionCode:             in.VersionCode,
		MinSupportedVersionCode: in.MinSupportedVersionCode,
		PackageURL:              strings.TrimSpace(in.PackageURL),
		PackageSize:             in.PackageSize,
		PackageMD5:              strings.TrimSpace(in.PackageMD5),
		ReleaseNotes:            in.ReleaseNotes,
		ForceUpdate:             in.ForceUpdate,
		Status:                  status,
		OperatorUserID:          in.OperatorUserID,
		OperatorName:            in.OperatorName,
	}
	if err := s.repo.Upsert(ctx, release); err != nil {
		return nil, err
	}
	// changelog 独立留存：记录失败仅记日志，不影响发布主流程。
	if s.changelogRepo != nil {
		entry := &models.AppReleaseChangelog{
			Platform:     release.Platform,
			VersionName:  release.VersionName,
			VersionCode:  release.VersionCode,
			ReleaseNotes: release.ReleaseNotes,
			OperatorName: release.OperatorName,
		}
		if err := s.changelogRepo.Upsert(ctx, entry); err != nil {
			log.Printf("record release changelog failed: platform=%s version_code=%d err=%v",
				release.Platform, release.VersionCode, err)
		}
	}
	return release, nil
}

// List 返回发布列表（管理后台用）。
func (s *AppReleaseService) List(ctx context.Context, filter models.AppReleaseListFilter) ([]*models.AppRelease, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("app release service not configured")
	}
	return s.repo.List(ctx, filter)
}

// GetByID 返回指定 id 的发布记录。
func (s *AppReleaseService) GetByID(ctx context.Context, id int64) (*models.AppRelease, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("app release service not configured")
	}
	if id <= 0 {
		return nil, invalidArg("id must be > 0")
	}
	return s.repo.GetByID(ctx, id)
}

// Delete 删除一条发布记录。
func (s *AppReleaseService) Delete(ctx context.Context, id int64) error {
	if s == nil || s.repo == nil {
		return errors.New("app release service not configured")
	}
	if id <= 0 {
		return invalidArg("id must be > 0")
	}
	return s.repo.Delete(ctx, id)
}

// UpgradeChangelogItem 是跨版本升级时单个版本的更新说明。
type UpgradeChangelogItem struct {
	VersionName string `json:"version_name"`
	VersionCode int64  `json:"version_code"`
	ReleaseNotes string `json:"release_notes"`
}

// UpgradeCheckResult 是客户端升级检查的结果。
type UpgradeCheckResult struct {
	HasUpdate               bool                   `json:"has_update"`
	ForceUpdate             bool                   `json:"force_update"`
	LatestVersionName       string                 `json:"latest_version_name"`
	LatestVersionCode       int64                  `json:"latest_version_code"`
	MinSupportedVersionCode int64                  `json:"min_supported_version_code"`
	PackageURL              string                 `json:"package_url"`
	PackageSize             int64                  `json:"package_size"`
	PackageMD5              string                 `json:"package_md5"`
	ReleaseNotes            string                 `json:"release_notes"`
	Changelogs              []UpgradeChangelogItem `json:"changelogs,omitempty"`
}

// CheckUpgrade 用于客户端升级检查。
//
// 强升判定：currentVersionCode < latest.MinSupportedVersionCode 或 latest.ForceUpdate=true。
// 当 currentVersionCode >= latest.VersionCode 时返回 has_update=false。
func (s *AppReleaseService) CheckUpgrade(ctx context.Context, platform models.AppReleasePlatform, currentVersionCode int64) (*UpgradeCheckResult, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("app release service not configured")
	}
	if platform != models.AppReleasePlatformAndroid && platform != models.AppReleasePlatformIOS {
		return nil, invalidArg("platform must be android or ios")
	}
	if currentVersionCode < 0 {
		return nil, invalidArg("current_version_code must be >= 0")
	}

	latest, err := s.repo.GetLatestPublished(ctx, platform)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return &UpgradeCheckResult{HasUpdate: false}, nil
		}
		return nil, err
	}

	if currentVersionCode >= latest.VersionCode {
		return &UpgradeCheckResult{HasUpdate: false}, nil
	}

	force := latest.ForceUpdate || currentVersionCode < latest.MinSupportedVersionCode
	result := &UpgradeCheckResult{
		HasUpdate:               true,
		ForceUpdate:             force,
		LatestVersionName:       latest.VersionName,
		LatestVersionCode:       latest.VersionCode,
		MinSupportedVersionCode: latest.MinSupportedVersionCode,
		PackageURL:              latest.PackageURL,
		PackageSize:             latest.PackageSize,
		PackageMD5:              latest.PackageMD5,
		ReleaseNotes:            latest.ReleaseNotes,
	}

	// 跨版本升级聚合：收集所有比当前版本新的 published 版本的说明，
	// 按版本号倒序，跳过空说明；失败时降级为只返回最新版本说明。
	releases, err := s.repo.List(ctx, models.AppReleaseListFilter{
		Platform: platform,
		Status:   models.AppReleaseStatusPublished,
	})
	if err == nil {
		var changelogs []UpgradeChangelogItem
		for _, r := range releases {
			if r == nil || r.VersionCode <= currentVersionCode {
				continue
			}
			notes := strings.TrimSpace(r.ReleaseNotes)
			if notes == "" {
				continue
			}
			changelogs = append(changelogs, UpgradeChangelogItem{
				VersionName:  r.VersionName,
				VersionCode:  r.VersionCode,
				ReleaseNotes: notes,
			})
		}
		sort.Slice(changelogs, func(i, j int) bool {
			return changelogs[i].VersionCode > changelogs[j].VersionCode
		})
		result.Changelogs = changelogs
	} else {
		log.Printf("list changelogs for upgrade check failed: platform=%s err=%v", platform, err)
	}
	return result, nil
}
