package models

import "time"

// User represents a basic user profile of the track application.
type User struct {
	ID             string    `json:"id" bson:"_id,omitempty"`
	Nickname       string    `json:"nickname" bson:"nickname"`
	AvatarURL      string    `json:"avatar_url" bson:"avatar_url"`
	Signature      string    `json:"signature" bson:"signature"`
	ClientLanguage string    `json:"client_language" bson:"client_language"`
	CreatedAt      time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" bson:"updated_at"`
}

// TrackCollect represents a user-track collection relationship.
type TrackCollect struct {
	UserID    string    `json:"user_id" bson:"user_id"`
	TrackID   string    `json:"track_id" bson:"track_id"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
}

// RequestMeta keeps normalized header information from client.
type RequestMeta struct {
	UserID         string
	ClientType     string
	ClientVersion  string
	ClientLanguage string
	Location       string
}
