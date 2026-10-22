package models

import (
	"math"
	"strings"
)

const defaultTrackCaloriesWeightKG = 65.0

// CalculateTrackCalories keeps the server fallback aligned with the Android
// recorder: calories = MET * 65kg * duration(hours), truncated to whole kcal.
func CalculateTrackCalories(trackType string, durationSeconds uint32) float64 {
	if durationSeconds == 0 {
		return 0
	}

	met := 9.8 // Android falls back unknown activity names to running.
	switch strings.ToLower(strings.TrimSpace(trackType)) {
	case "hiking", "hike", "徒步":
		met = 6.0
	case "walking", "walk", "散步":
		met = 3.5
	case "climbing", "climb", "mountain", "mountaineering", "爬山":
		met = 8.0
	case "riding", "ride", "cycling", "cycle", "bike", "biking", "骑行", "骑车":
		met = 6.8
	case "driving", "drive", "自驾":
		met = 1.5
	case "running", "run", "跑步":
		met = 9.8
	}

	return math.Trunc(met * defaultTrackCaloriesWeightKG * float64(durationSeconds) / 3600.0)
}
