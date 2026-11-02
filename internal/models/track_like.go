package models

import "time"

type TrackLike struct {
	UserID    int64     `json:"user_id" bson:"user_id"`
	TrackID   string    `json:"track_id" bson:"track_id"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
}

// TrackLikeCursor 复用收藏游标结构：created_at desc, track_id desc，下一页取严格小于游标的记录。
type TrackLikeCursor = TrackCollectCursor

// LikedTrackSummaryPage 与收藏轨迹列表同构（协议字段完全一致），供「喜欢」列表复用。
type LikedTrackSummaryPage = CollectedTrackSummaryPage

type TrackLikeStats struct {
	Liked     bool  `json:"liked"`
	LikeCount int64 `json:"like_count"`
}
