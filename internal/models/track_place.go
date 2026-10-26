package models

import "time"

type TrackPrimaryScenicSpot struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TrackPlace is shared by all tracks referencing the same provider identity.
// No polygon is inferred from a POI's center, area or display viewport.
type TrackPlace struct {
	ID               string    `json:"id"`
	Provider         string    `json:"provider"`
	ProviderID       string    `json:"provider_id"`
	Name             string    `json:"name"`
	Category         string    `json:"category"`
	ParentID         string    `json:"parent_id,omitempty"`
	CityCode         string    `json:"city_code,omitempty"`
	DistrictCode     string    `json:"district_code,omitempty"`
	Latitude         float64   `json:"latitude"`
	Longitude        float64   `json:"longitude"`
	CoordinateSystem string    `json:"coordinate_system"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type TrackPlaceRelation struct {
	PlaceID        string  `json:"place_id"`
	Kind           string  `json:"kind"` // sampled_inside or nearby; neither asserts full-route containment.
	MatchedSamples int     `json:"matched_samples"`
	SampleCount    int     `json:"sample_count"`
	Confidence     float64 `json:"confidence"`
}

// Match identities use track IDs so regrouping does not lose enrichment.
type TrackPlaceMatch struct {
	TrackID        string               `json:"track_id"`
	SourceVersion  string               `json:"source_version"`
	RuleVersion    int                  `json:"rule_version"`
	Provider       string               `json:"provider"`
	Status         string               `json:"status"`
	PrimaryPlaceID string               `json:"primary_place_id,omitempty"`
	Relations      []TrackPlaceRelation `json:"relations"`
	Attempts       int                  `json:"attempts"`
	ErrorCode      string               `json:"error_code,omitempty"`
	UpdatedAt      time.Time            `json:"updated_at"`
}

// TrackPlaceRecord persists task state, provider response cache or shared places.
type TrackPlaceRecord struct {
	Key        string    `bson:"_id"`
	Data       string    `bson:"data"`
	ExpiresAt  time.Time `bson:"expires_at"`
	LeaseUntil time.Time `bson:"lease_until"`
	LeaseOwner string    `bson:"lease_owner"`
}
