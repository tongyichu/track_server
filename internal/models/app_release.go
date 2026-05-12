package models

import "time"

// AppReleasePlatform 表示客户端平台。
type AppReleasePlatform string

const (
	AppReleasePlatformAndroid AppReleasePlatform = "android"
	AppReleasePlatformIOS     AppReleasePlatform = "ios"
)

// AppReleaseStatus 表示发布状态。
type AppReleaseStatus string

const (
	AppReleaseStatusDraft     AppReleaseStatus = "draft"
	AppReleaseStatusPublished AppReleaseStatus = "published"
	AppReleaseStatusArchived  AppReleaseStatus = "archived"
)

// AppRelease 是一条 App 发布记录。
//
// 设计要点：
// - (platform, version_code) 唯一，用于幂等更新；
// - PackageURL 对于 Android 通常是 OSS 公开下载链接，iOS 通常是 AppStore 跳转链接；
// - MinSupportedVersionCode 表示低于该版本必须强升；与 ForceUpdate 配合使用，二者任意为真即触发强更。
type AppRelease struct {
	ID                      int64              `json:"id" bson:"_id,omitempty"`
	Platform                AppReleasePlatform `json:"platform" bson:"platform"`
	VersionName             string             `json:"version_name" bson:"version_name"`
	VersionCode             int64              `json:"version_code" bson:"version_code"`
	MinSupportedVersionCode int64              `json:"min_supported_version_code" bson:"min_supported_version_code"`
	PackageURL              string             `json:"package_url" bson:"package_url"`
	PackageSize             int64              `json:"package_size" bson:"package_size"`
	PackageMD5              string             `json:"package_md5" bson:"package_md5"`
	ReleaseNotes            string             `json:"release_notes" bson:"release_notes"`
	ForceUpdate             bool               `json:"force_update" bson:"force_update"`
	Status                  AppReleaseStatus   `json:"status" bson:"status"`
	OperatorUserID          int64              `json:"operator_user_id" bson:"operator_user_id"`
	OperatorName            string             `json:"operator_name" bson:"operator_name"`
	CreatedAt               time.Time          `json:"created_at" bson:"created_at"`
	UpdatedAt               time.Time          `json:"updated_at" bson:"updated_at"`
}

// AppReleaseListFilter 是发布列表查询条件。
type AppReleaseListFilter struct {
	Platform AppReleasePlatform
	Status   AppReleaseStatus
}
