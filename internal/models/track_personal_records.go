package models

import "math"

type PersonalRecord struct {
	Metric string        `json:"metric"`
	Value  float64       `json:"value"`
	Track  *TrackSummary `json:"track"`
}

type PersonalRecordGroup struct {
	TrackType string           `json:"track_type"`
	Records   []PersonalRecord `json:"records"`
}

type PersonalRecords struct {
	Groups []PersonalRecordGroup `json:"groups"`
}

// PersonalRecordValue returns comparable base units; missing or invalid values cannot win a record.
func PersonalRecordValue(t *Track, metric string) float64 {
	var value float64
	switch metric {
	case "distance":
		value = t.Distance
	case "duration":
		value = float64(t.Duration)
	case "elevation":
		value = float64(t.ElevationGain)
	case "steps":
		if t.StepCount != nil {
			value = float64(*t.StepCount)
		}
	}
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}
