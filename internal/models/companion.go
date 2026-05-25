package models

import "time"

// CompanionSessionStatus 表示一场“同行”会话的状态。
type CompanionSessionStatus string

const (
	CompanionSessionStatusActive CompanionSessionStatus = "active"
	CompanionSessionStatusEnded  CompanionSessionStatus = "ended"
)

// CompanionMemberRole 表示会话成员角色。
type CompanionMemberRole string

const (
	CompanionMemberRoleOwner  CompanionMemberRole = "owner"
	CompanionMemberRoleMember CompanionMemberRole = "member"
)

// CompanionMemberStatus 表示成员是否仍具有该会话的业务资格。
type CompanionMemberStatus string

const (
	CompanionMemberStatusJoined CompanionMemberStatus = "joined"
	CompanionMemberStatusLeft   CompanionMemberStatus = "left"
	CompanionMemberStatusKicked CompanionMemberStatus = "kicked"
	CompanionMemberStatusEnded  CompanionMemberStatus = "ended"
)

// CompanionPresenceStatus 表示成员当前连接状态。
type CompanionPresenceStatus string

const (
	CompanionPresenceStatusOnline  CompanionPresenceStatus = "online"
	CompanionPresenceStatusOffline CompanionPresenceStatus = "offline"
)

// CompanionSession 是一场“同行”控制面的核心会话对象。
//
// 设计说明：
// - session_id 是对外暴露的唯一业务 ID；
// - join_token 用于其他用户加入同行；
// - status 只区分 active / ended；
// - owner_user_id 是会话发起人，也是默认的结束权限拥有者。
type CompanionSession struct {
	SessionID          string                 `json:"session_id" bson:"session_id"`
	OwnerUserID        int64                  `json:"owner_user_id" bson:"owner_user_id"`
	Status             CompanionSessionStatus `json:"status" bson:"status"`
	JoinToken          string                 `json:"-" bson:"join_token"`
	JoinTokenExpireAt  time.Time              `json:"-" bson:"join_token_expire_at"`
	Title              string                 `json:"title" bson:"title"`
	MaxMembers         int                    `json:"max_members" bson:"max_members"`
	StartedAt          time.Time              `json:"started_at" bson:"started_at"`
	EndedAt            time.Time              `json:"ended_at,omitempty" bson:"ended_at,omitempty"`
	CreatedAt          time.Time              `json:"created_at" bson:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at" bson:"updated_at"`
}

// CompanionSessionMember 表示用户在某个“同行”会话中的成员资格与连接状态。
type CompanionSessionMember struct {
	SessionID      string                  `json:"session_id" bson:"session_id"`
	UserID         int64                   `json:"user_id" bson:"user_id"`
	Role           CompanionMemberRole     `json:"role" bson:"role"`
	MemberStatus   CompanionMemberStatus   `json:"member_status" bson:"member_status"`
	PresenceStatus CompanionPresenceStatus `json:"presence_status" bson:"presence_status"`
	JoinedAt       time.Time               `json:"joined_at" bson:"joined_at"`
	LeftAt         time.Time               `json:"left_at,omitempty" bson:"left_at,omitempty"`
	LastSeenAt     time.Time               `json:"last_seen_at,omitempty" bson:"last_seen_at,omitempty"`
	MQTTClientID   string                  `json:"-" bson:"mqtt_client_id"`
	MQTTPrincipal  string                  `json:"-" bson:"mqtt_principal"`
}

// CompanionLivePosition 保存某个成员在会话内的最新位置快照。
//
// 设计说明：
// - 一条 (session_id, user_id) 仅保留一份最新快照；
// - seq 由客户端单调递增，便于去重和乱序保护；
// - source 标记快照来自 http / mqtt 等来源，便于后续接入 EMQX rule engine 时复用。
type CompanionLivePosition struct {
	SessionID        string    `json:"session_id" bson:"session_id"`
	UserID           int64     `json:"user_id" bson:"user_id"`
	TrackID          string    `json:"track_id,omitempty" bson:"track_id,omitempty"`
	Latitude         float64   `json:"latitude" bson:"latitude"`
	Longitude        float64   `json:"longitude" bson:"longitude"`
	CoordinateSystem string    `json:"coordinate_system" bson:"coordinate_system"`
	SpeedKmh         float64   `json:"speed_kmh" bson:"speed_kmh"`
	Heading          float64   `json:"heading" bson:"heading"`
	AccuracyM        float64   `json:"accuracy_m" bson:"accuracy_m"`
	Altitude         float64   `json:"altitude" bson:"altitude"`
	RecordedAt       time.Time `json:"recorded_at" bson:"recorded_at"`
	Seq              int64     `json:"seq" bson:"seq"`
	Source           string    `json:"source" bson:"source"`
	CreatedAt        time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" bson:"updated_at"`
}

// CompanionMemberSnapshot 是会话快照里的成员展示信息。
type CompanionMemberSnapshot struct {
	UserID         int64                   `json:"user_id"`
	Role           CompanionMemberRole     `json:"role"`
	MemberStatus   CompanionMemberStatus   `json:"member_status"`
	PresenceStatus CompanionPresenceStatus `json:"presence_status"`
	Nickname       string                  `json:"nickname"`
	AvatarURL      string                  `json:"avatar_url"`
	JoinedAt       time.Time               `json:"joined_at"`
	LastSeenAt     time.Time               `json:"last_seen_at,omitempty"`
}

// CompanionSnapshot 是“同行”会话的当前快照。
type CompanionSnapshot struct {
	SnapshotAt time.Time                   `json:"snapshot_at"`
	Members    []CompanionMemberSnapshot   `json:"members"`
	Positions  []*CompanionLivePosition    `json:"positions"`
}

