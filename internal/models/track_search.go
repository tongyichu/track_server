package models

// TrackDistanceRange uses meters, including the minimum and excluding the maximum.
type TrackDistanceRange struct {
	MinMeters *float64 `json:"min_distance_m,omitempty"`
	MaxMeters *float64 `json:"max_distance_m,omitempty"`
}

type TrackSearchOptions struct {
	DistanceFilterVersion int                             `json:"distance_filter_version"`
	DefaultRanges         []TrackDistanceRange            `json:"default_ranges"`
	RangesByType          map[string][]TrackDistanceRange `json:"ranges_by_type"`
}
