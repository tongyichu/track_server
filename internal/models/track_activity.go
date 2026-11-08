package models

// ActivityTypeTotal summarizes completed uploaded tracks without loading GPS data.
type ActivityTypeTotal struct {
	TrackType           string  `json:"track_type" bson:"_id"`
	TrackCount          int64   `json:"track_count" bson:"track_count"`
	DurationSeconds     int64   `json:"duration_seconds" bson:"duration_seconds"`
	DistanceMeters      float64 `json:"distance_meters" bson:"distance_meters"`
	ElevationGainMeters int64   `json:"elevation_gain_meters" bson:"elevation_gain_meters"`
}
