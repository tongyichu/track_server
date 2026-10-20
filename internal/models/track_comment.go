package models

import "time"

type TrackComment struct {
	ID           string         `json:"id" bson:"_id"`
	TrackID      string         `json:"track_id" bson:"track_id"`
	RootID       string         `json:"root_id" bson:"root_id"`
	ReplyToID    string         `json:"reply_to_id" bson:"reply_to_id"`
	UserID       int64          `json:"user_id" bson:"user_id"`
	ClientID     string         `json:"-" bson:"client_id"`
	RequestHash  string         `json:"-" bson:"request_hash"`
	Content      string         `json:"content" bson:"content"`
	Images       []CommentMedia `json:"images" bson:"images"`
	ImageIDs     []string       `json:"-" bson:"image_ids"`
	Deleted      bool           `json:"deleted" bson:"deleted"`
	ReplyCount   int64          `json:"reply_count" bson:"reply_count"`
	LikeCount    int64          `json:"like_count" bson:"like_count"`
	DislikeCount int64          `json:"dislike_count" bson:"dislike_count"`
	CreatedAt    time.Time      `json:"created_at" bson:"created_at"`
	UpdatedAt    time.Time      `json:"-" bson:"updated_at"`
}
type CommentImage struct {
	ID        string    `json:"id" bson:"_id"`
	TrackID   string    `json:"-" bson:"track_id"`
	UserID    int64     `json:"-" bson:"user_id"`
	CommentID string    `json:"-" bson:"comment_id"`
	ObjectKey string    `json:"-" bson:"object_key"`
	OSSURL    string    `json:"-" bson:"oss_url"`
	Width     int       `json:"width" bson:"width"`
	Height    int       `json:"height" bson:"height"`
	ByteSize  int64     `json:"-" bson:"byte_size"`
	CreatedAt time.Time `json:"-" bson:"created_at"`
}
type CommentCursor struct {
	TrackID   string    `json:"t"`
	RootID    string    `json:"r"`
	CreatedAt time.Time `json:"c"`
	ID        string    `json:"i"`
}
type CommentLikerCursor struct {
	TrackID   string `json:"t"`
	CommentID string `json:"c"`
	UserID    int64  `json:"u"`
}

type CommentMedia struct {
	ID     string `json:"id" bson:"id"`
	Width  int    `json:"width" bson:"width"`
	Height int    `json:"height" bson:"height"`
}
