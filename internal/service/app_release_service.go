package service

import (
	"context"
	"errors"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

// AppReleaseService 负责 App 发布与升级检查相关的业务逻辑。
type AppReleaseService struct {
	repo repository.AppReleaseRepository
}

// NewAppReleaseService 构造 AppReleaseService。
func NewAppReleaseService(repo repository.AppReleaseRepository) *AppReleaseService {
	return &AppReleaseService{repo: repo}
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

// UpgradeCheckResult 是客户端升级检查的结果。
type UpgradeCheckResult struct {
	HasUpdate               bool   `json:"has_update"`
	ForceUpdate             bool   `json:"force_update"`
	LatestVersionName       string `json:"latest_version_name"`
	LatestVersionCode       int64  `json:"latest_version_code"`
	MinSupportedVersionCode int64  `json:"min_supported_version_code"`
	PackageURL              string `json:"package_url"`
	PackageSize             int64  `json:"package_size"`
	PackageMD5              string `json:"package_md5"`
	ReleaseNotes            string `json:"release_notes"`
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
	return &UpgradeCheckResult{
		HasUpdate:               true,
		ForceUpdate:             force,
		LatestVersionName:       latest.VersionName,
		LatestVersionCode:       latest.VersionCode,
		MinSupportedVersionCode: latest.MinSupportedVersionCode,
		PackageURL:              latest.PackageURL,
		PackageSize:             latest.PackageSize,
		PackageMD5:              latest.PackageMD5,
		ReleaseNotes:            latest.ReleaseNotes,
	}, nil
}
