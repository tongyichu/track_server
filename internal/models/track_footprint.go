package models

import "time"

// FootprintTrack is an owner-only projection; no GPS or asset payload is loaded.
type FootprintTrack struct {
	ID        string    `bson:"_id"`
	CityCode  string    `bson:"city_code"`
	TrackType string    `bson:"track_type"`
	Distance  float64   `bson:"distance"`
	StartTime time.Time `bson:"start_time"`
}

type FootprintRegion struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	NameEN          string   `json:"name_en,omitempty"`
	Aliases         []string `json:"aliases"`
	ProvinceID      string   `json:"province_id,omitempty"`
	MapCode         string   `json:"map_code,omitempty"`
	Latitude        *float64 `json:"latitude,omitempty"`
	Longitude       *float64 `json:"longitude,omitempty"`
	TrackCount      int64    `json:"track_count"`
	DistanceMeters  float64  `json:"distance_meters"`
	CityCount       int      `json:"city_count"`
	ScenicSpotCount int      `json:"scenic_spot_count"`
}

type ActivityFootprint struct {
	Provinces              []FootprintRegion `json:"provinces"`
	Cities                 []FootprintRegion `json:"cities"`
	ScenicSpotCount        int               `json:"scenic_spot_count"`
	UnlocatedTrackCount    int64             `json:"unlocated_track_count"`
	OutsideChinaTrackCount int64             `json:"outside_china_track_count"`
}
