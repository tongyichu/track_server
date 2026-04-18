package models

import "time"

// TrackStatus represents the lifecycle status of a track.
type TrackStatus string

const (
	// TrackStatusRunning indicates the track is currently being recorded.
	TrackStatusRunning TrackStatus = "running"
	// TrackStatusPaused indicates the track is paused.
	TrackStatusPaused TrackStatus = "paused"
	// TrackStatusFinished indicates the track recording is finished.
	TrackStatusFinished TrackStatus = "finished"
)

// TrackPoint represents a single sampled GPS point on a track.
type TrackPoint struct {
	Index     int       `json:"index" bson:"index"`
	Latitude  float64   `json:"latitude" bson:"latitude"`
	Longitude float64   `json:"longitude" bson:"longitude"`
	Elevation float64   `json:"elevation" bson:"elevation"`
	Timestamp time.Time `json:"timestamp" bson:"timestamp"`
}

// Track aggregates points and statistics of a single outdoor activity.
type Track struct {
	ID             string       `json:"id" bson:"_id,omitempty"`
	UserID         string       `json:"user_id" bson:"user_id"`
	Name           string       `json:"name" bson:"name"`
	Status         TrackStatus  `json:"status" bson:"status"`
	Points         []TrackPoint `json:"points" bson:"points"`
	DistanceMeters float64      `json:"distance_meters" bson:"distance_meters"`
	DurationSec    int64        `json:"duration_sec" bson:"duration_sec"`
	AscentMeters   float64      `json:"ascent_meters" bson:"ascent_meters"`
	AvgSpeedKmh    float64      `json:"avg_speed_kmh" bson:"avg_speed_kmh"`
	StartedAt      time.Time    `json:"started_at" bson:"started_at"`
	EndedAt        *time.Time   `json:"ended_at,omitempty" bson:"ended_at,omitempty"`
	CreatedAt      time.Time    `json:"created_at" bson:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at" bson:"updated_at"`
}

// TrackSummary is a lightweight view used for recommend/search lists.
type TrackSummary struct {
	ID             string  `json:"id"`
	UserID         string  `json:"user_id"`
	Name           string  `json:"name"`
	DistanceMeters float64 `json:"distance_meters"`
	DurationSec    int64   `json:"duration_sec"`
	AscentMeters   float64 `json:"ascent_meters"`
	AvgSpeedKmh    float64 `json:"avg_speed_kmh"`
}

// TrackMap represents data needed for rendering a track polyline on map.
type TrackMap struct {
	TrackID string        `json:"track_id"`
	Points  []TrackPoint  `json:"points"`
}
