package models

import "time"

type MusicTrack struct {
	ID           string    `json:"id" bson:"_id"`
	Revision     int64     `json:"revision" bson:"revision"`
	Name         string    `json:"name" bson:"name"`
	Artist       string    `json:"artist" bson:"artist"`
	DurationMS   int64     `json:"duration_ms" bson:"duration_ms"`
	AudioSHA     string    `json:"audio_sha256" bson:"audio_sha256"`
	AudioBytes   int64     `json:"audio_bytes" bson:"audio_bytes"`
	AudioLink    string    `json:"audio_link,omitempty" bson:"audio_link,omitempty"`
	AudioVersion string    `json:"audio_version,omitempty" bson:"audio_version,omitempty"`
	CoverSHA     string    `json:"cover_sha256" bson:"cover_sha256"`
	License      string    `json:"license" bson:"license"`
	Source       string    `json:"source" bson:"source"`
	Attribution  string    `json:"attribution" bson:"attribution"`
	Published    bool      `json:"published" bson:"published"`
	SortOrder    int       `json:"sort_order" bson:"sort_order"`
	UpdatedAt    time.Time `json:"updated_at" bson:"updated_at"`
}
