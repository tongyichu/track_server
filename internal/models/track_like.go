package models

import "time"

type TrackLike struct {
	UserID    int64     `json:"user_id" bson:"user_id"`
	TrackID   string    `json:"track_id" bson:"track_id"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
}

type TrackLikeStats struct {
	Liked     bool  `json:"liked"`
	LikeCount int64 `json:"like_count"`
}
