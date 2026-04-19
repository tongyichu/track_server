package models

import "time"

// User represents a basic user profile of the track application.
type User struct {
	ID             int64     `json:"id" bson:"_id,omitempty"`
	Nickname       string    `json:"nickname" bson:"nickname"`
	AvatarURL      string    `json:"avatar_url" bson:"avatar_url"`
	Signature      string    `json:"signature" bson:"signature"`
	Phone          string    `json:"phone" bson:"phone"`
	ClientLanguage string    `json:"client_language" bson:"client_language"`
	CreatedAt      time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" bson:"updated_at"`
}

// TrackCollect represents a user-track collection relationship.
type TrackCollect struct {
	UserID    int64     `json:"user_id" bson:"user_id"`
	TrackID   string    `json:"track_id" bson:"track_id"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
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
	UserID         int64
	RawUserID      string
	ClientType     string
	ClientVersion  string
	ClientLanguage string
	Location       string
}
