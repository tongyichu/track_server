package models

// Index zero is the immutable completion manifest; positive indexes contain point batches.
type CompanionTrackChunk struct {
	SessionID  string `json:"session_id" bson:"session_id"`
	UserID     int64  `json:"user_id" bson:"user_id"`
	Index      int    `json:"index" bson:"index"`
	PointCount int    `json:"point_count" bson:"point_count"`
	Digest     string `json:"-" bson:"digest"`
	Data       string `json:"data" bson:"data"`
}
