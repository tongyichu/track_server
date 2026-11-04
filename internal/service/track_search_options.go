package service

import (
	"math"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
)

func validateSearchDistance(r models.TrackDistanceRange) error {
	valid := func(v *float64) bool { return v == nil || (!math.IsNaN(*v) && !math.IsInf(*v, 0) && *v >= 0) }
	if !valid(r.MinMeters) || !valid(r.MaxMeters) ||
		(r.MaxMeters != nil && *r.MaxMeters == 0) ||
		(r.MinMeters != nil && r.MaxMeters != nil && *r.MinMeters >= *r.MaxMeters) {
		return invalidArg("invalid distance range")
	}
	return nil
}

func (s *TrackService) SearchOptions() models.TrackSearchOptions {
	presets := config.SearchDistanceBreakpoints()
	ranges := func(edges []float64) []models.TrackDistanceRange {
		out := make([]models.TrackDistanceRange, 0, len(edges)+1)
		for i := 0; i <= len(edges); i++ {
			r := models.TrackDistanceRange{}
			if i > 0 {
				v := edges[i-1]
				r.MinMeters = &v
			}
			if i < len(edges) {
				v := edges[i]
				r.MaxMeters = &v
			}
			out = append(out, r)
		}
		return out
	}
	out := models.TrackSearchOptions{DistanceFilterVersion: 1, DefaultRanges: ranges(presets[""]), RangesByType: map[string][]models.TrackDistanceRange{}}
	for _, code := range s.ListTrackTypes() {
		if edges, ok := presets[code]; ok {
			out.RangesByType[code] = ranges(edges)
		}
	}
	return out
}
