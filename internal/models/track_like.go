package models

import "time"

type TrackLike struct {
	UserID    int64     `json:"user_id" bson:"user_id"`
	TrackID   string    `json:"track_id" bson:"track_id"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
}

type TrackLikeCursor struct {
	CreatedAt time.Time `json:"created_at"`
	TrackID   string    `json:"track_id"`
	UserID    int64     `json:"user_id"`
}

type TrackLikeStats struct {
	Liked     bool  `json:"liked"`
	LikeCount int64 `json:"like_count"`
}

type ReceivedLikeTrackCursor struct {
	LastLikedAt time.Time `json:"last_liked_at"`
	TrackID     string    `json:"track_id"`
}

type ReceivedLikeTrack struct {
	TrackID     string       `bson:"_id"`
	LikeCount   int64        `bson:"received_like_count"`
	LastLikedAt time.Time    `bson:"last_liked_at"`
	RecentLikes []*TrackLike `bson:"recent_likes"`
}
