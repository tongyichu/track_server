package models

import "time"

// UserListCursor 表示按 created_at 倒序翻页用户列表时使用的游标。
//
// 约定：
// - 排序：created_at desc, id desc；
// - 下一页查询条件为 "(created_at, id) 严格小于该游标"；
// - 仅供管理后台等"全量列表"场景使用，不影响业务接口。
type UserListCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        int64     `json:"id"`
}

// User represents a basic user profile of the track application.
type User struct {
	ID             int64     `json:"id" bson:"_id,omitempty"`
	Nickname       string    `json:"nickname" bson:"nickname"`
	AvatarURL      string    `json:"avatar_url" bson:"avatar_url"`
	Signature      string    `json:"signature" bson:"signature"`
	Phone          string    `json:"phone" bson:"phone"`
	ClientLanguage string    `json:"client_language" bson:"client_language"`
	TokenVersion   int64     `json:"-" bson:"token_version"`
	CreatedAt      time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" bson:"updated_at"`
}

const (
	AccountRestrictionStatusActive  = "active"
	AccountRestrictionStatusRevoked = "revoked"
)

// AccountRestriction records a management-center account restriction.
// ExpiresAt == nil means the restriction is permanent.
type AccountRestriction struct {
	ID        int64      `json:"id" bson:"_id,omitempty"`
	UserID    int64      `json:"user_id" bson:"user_id"`
	Status    string     `json:"status" bson:"status"`
	Reason    string     `json:"reason" bson:"reason"`
	Operator  string     `json:"operator" bson:"operator"`
	ExpiresAt *time.Time `json:"expires_at,omitempty" bson:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" bson:"updated_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty" bson:"revoked_at,omitempty"`
}

// TrackCollect represents a user-track collection relationship.
type TrackCollect struct {
	UserID    int64     `json:"user_id" bson:"user_id"`
	TrackID   string    `json:"track_id" bson:"track_id"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
}

// TrackCollectCursor is the cursor used for paging a user's collected track list.
//
// Order: created_at desc, track_id desc.
// Next page condition: (created_at, track_id) strictly less than cursor.
type TrackCollectCursor struct {
	CreatedAt time.Time `json:"created_at"`
	TrackID   string    `json:"track_id"`
}

// UserFollow represents a one-way follow relationship between two users.
type UserFollow struct {
	FollowerUserID int64     `json:"follower_user_id" bson:"follower_user_id"`
	FolloweeUserID int64     `json:"followee_user_id" bson:"followee_user_id"`
	CreatedAt      time.Time `json:"created_at" bson:"created_at"`
}

// UserFollowCursor is the cursor used for paging follow/follower lists.
//
// Order: created_at desc, user_id desc.
// Next page condition: (created_at, user_id) strictly less than cursor.
type UserFollowCursor struct {
	CreatedAt time.Time `json:"created_at"`
	UserID    int64     `json:"user_id"`
}

// LoginLog records a user's login event for audit and statistics.
type LoginLog struct {
	ID        int64     `json:"id" bson:"_id,omitempty"`
	UserID    int64     `json:"user_id" bson:"user_id"`
	LoginType string    `json:"login_type" bson:"login_type"`
	IP        string    `json:"ip,omitempty" bson:"ip,omitempty"`
	DeviceID  string    `json:"device_id,omitempty" bson:"device_id,omitempty"`
	Platform  string    `json:"platform,omitempty" bson:"platform,omitempty"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
}

// RequestMeta keeps normalized header information from client.
type RequestMeta struct {
	UserID         string // header中 X-User-ID 的值
	RawUserID      string // 权限token对应的user_id转为string的值
	AuthUserID     int64  // 权限token对应的user_id值
	ClientVersion  string // 客户端版本
	ClientLanguage string // 客户端语言
	Location       string // 地理位置
	Platform       string // ios or android
	DeviceID       string // 设备ID
}
