package models

import "time"

const (
	ScenicSpotDraft     = "draft"
	ScenicSpotPublished = "published"
	ScenicSpotArchived  = "archived"
	ScenicSpotMerged    = "merged"
)

type ScenicSpot struct {
	ID                        string             `json:"id" bson:"_id"`
	Name                      string             `json:"name" bson:"name"`
	Aliases                   []string           `json:"aliases,omitempty" bson:"aliases,omitempty"`
	Category                  string             `json:"category,omitempty" bson:"category,omitempty"`
	CityCode                  string             `json:"city_code,omitempty" bson:"city_code,omitempty"`
	CityName                  string             `json:"city_name,omitempty" bson:"city_name,omitempty"`
	Latitude                  float64            `json:"latitude,omitempty" bson:"latitude,omitempty"`
	Longitude                 float64            `json:"longitude,omitempty" bson:"longitude,omitempty"`
	Description               string             `json:"description,omitempty" bson:"description,omitempty"`
	Address                   string             `json:"address,omitempty" bson:"address,omitempty"`
	CoverURL                  string             `json:"cover_url,omitempty" bson:"cover_url,omitempty"`
	Photos                    []ScenicSpotPhoto  `json:"photos,omitempty" bson:"photos,omitempty"`
	Sources                   []ScenicSpotSource `json:"sources,omitempty" bson:"sources,omitempty"`
	Status                    string             `json:"status" bson:"status"`
	ManualEdited              bool               `json:"manual_edited,omitempty" bson:"manual_edited,omitempty"`
	ProviderDataStatus        string             `json:"provider_data_status,omitempty" bson:"provider_data_status,omitempty"`
	ProviderDataFetchedAt     *time.Time         `json:"provider_data_fetched_at,omitempty" bson:"provider_data_fetched_at,omitempty"`
	CoordinateSystem          string             `json:"coordinate_system,omitempty" bson:"coordinate_system,omitempty"`
	ProviderCoordinatesStatus string             `json:"provider_coordinates_status,omitempty" bson:"provider_coordinates_status,omitempty"`
	RelatedCities             []string           `json:"related_cities,omitempty" bson:"-"`
	MergedInto                string             `json:"merged_into,omitempty" bson:"merged_into,omitempty"`
	Version                   int64              `json:"version" bson:"version"`
	CreatedAt                 time.Time          `json:"created_at" bson:"created_at"`
	UpdatedAt                 time.Time          `json:"updated_at" bson:"updated_at"`
	PublishedAt               *time.Time         `json:"published_at,omitempty" bson:"published_at,omitempty"`
	CreatedBy                 string             `json:"created_by" bson:"created_by"`
	UpdatedBy                 string             `json:"updated_by" bson:"updated_by"`
}

type ScenicSpotPhoto struct {
	ID             string `json:"id" bson:"id"`
	URL            string `json:"url" bson:"url"`
	Caption        string `json:"caption,omitempty" bson:"caption,omitempty"`
	AltText        string `json:"alt_text,omitempty" bson:"alt_text,omitempty"`
	Attribution    string `json:"attribution,omitempty" bson:"attribution,omitempty"`
	AttributionURL string `json:"attribution_url,omitempty" bson:"attribution_url,omitempty"`
	SourceURL      string `json:"source_url,omitempty" bson:"source_url,omitempty"`
	Provider       string `json:"provider,omitempty" bson:"provider,omitempty"`
}

// Provider IDs identify upstream records.
type ScenicSpotSource struct {
	Provider       string     `json:"provider" bson:"provider"`
	ProviderID     string     `json:"provider_id" bson:"provider_id"`
	Status         string     `json:"status" bson:"status"`
	LastVerifiedAt *time.Time `json:"last_verified_at,omitempty" bson:"last_verified_at,omitempty"`
	RefreshAfter   *time.Time `json:"refresh_after,omitempty" bson:"refresh_after,omitempty"`
}

type TrackScenicSpotLink struct {
	TrackID    string    `json:"track_id" bson:"_id"`
	SpotID     string    `json:"scenic_spot_id" bson:"scenic_spot_id"`
	Origin     string    `json:"origin" bson:"origin"` // auto, manual, merge, backfill
	Provider   string    `json:"provider,omitempty" bson:"provider,omitempty"`
	Confidence float64   `json:"confidence,omitempty" bson:"confidence,omitempty"`
	UpdatedAt  time.Time `json:"updated_at" bson:"updated_at"`
	UpdatedBy  string    `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
}

type ScenicSpotAudit struct {
	ID        string    `json:"id" bson:"_id"`
	SpotID    string    `json:"scenic_spot_id" bson:"scenic_spot_id"`
	Action    string    `json:"action" bson:"action"`
	Operator  string    `json:"operator" bson:"operator"`
	Details   string    `json:"details,omitempty" bson:"details,omitempty"`
	Before    string    `json:"before,omitempty" bson:"before,omitempty"`
	After     string    `json:"after,omitempty" bson:"after,omitempty"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
}

// ScenicSpotProviderUsage stores operational counters only; no provider content or photo references.
type ScenicSpotProviderUsage struct {
	SpotID    string `json:"scenic_spot_id" bson:"scenic_spot_id"`
	Provider  string `json:"provider" bson:"provider"`
	UsageDay  string `json:"usage_day" bson:"usage_day"`
	Operation string `json:"operation" bson:"operation"`
	Requests  int64  `json:"requests" bson:"requests"`
	Successes int64  `json:"successes" bson:"successes"`
	Failures  int64  `json:"failures" bson:"failures"`
}
