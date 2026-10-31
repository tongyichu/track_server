package service

import (
	"context"
	"errors"
	"math"

	"github.com/tongyichu/track_server/internal/models"
)

type trackPlaceRing struct {
	points                         []models.TrackPoint
	minLat, minLng, maxLat, maxLng float64
}

func (s *TrackPlaceService) loadTrackPlaceRing(ctx context.Context, track *models.Track, index *models.TrackGeoIndex) (*trackPlaceRing, error) {
	// Cheaply reject non-loops before reading GPS. The index is only a prefilter:
	// its point subsampling has no error bound for precise containment.
	if newTrackPlaceRing(index) == nil {
		return nil, nil
	}
	if s.indexer == nil || s.indexer.rawTrackCache == nil {
		return nil, errors.New("place_ring_raw_unavailable")
	}
	path, err := s.indexer.rawTrackCache.EnsureCachedFile(ctx, track.UserID, track.ID, track.RawTrackURL)
	if err != nil {
		return nil, errors.New("place_ring_raw_unavailable")
	}
	points, err := parseTrackPointsFile(path)
	if err != nil {
		return nil, errors.New("place_ring_raw_invalid")
	}
	for i, p := range points {
		points[i], err = mapPointToGCJ02(p, track.CoordinateSystem)
		if err != nil {
			return nil, err
		}
	}
	full := *index
	full.SimplifiedPolyline = points
	return newTrackPlaceRing(&full), nil
}

func newTrackPlaceRing(index *models.TrackGeoIndex) *trackPlaceRing {
	if index.CoordinateSystem != "GCJ02" || len(index.SimplifiedPolyline) < 4 {
		return nil
	}
	points := index.SimplifiedPolyline
	cumulative := make([]float64, len(points))
	grid := make(map[[2]int][]int, len(points))
	for i, p := range points {
		if !validMapLatLng(p.Latitude, p.Longitude) {
			return nil
		}
		if i > 0 {
			cumulative[i] = cumulative[i-1] + haversineMeters(points[i-1].Latitude, points[i-1].Longitude, p.Latitude, p.Longitude)
		}
		grid[trackPlaceRingGridCell(p)] = append(grid[trackPlaceRingGridCell(p)], i)
	}
	var best *trackPlaceRing
	var bestArea float64
	for end, p := range points {
		cell := trackPlaceRingGridCell(p)
		for dx := -1; dx <= 1; dx++ {
			for dy := -1; dy <= 1; dy++ {
				for _, start := range grid[[2]int{cell[0] + dx, cell[1] + dy}] {
					length := cumulative[end] - cumulative[start]
					if length < 100 {
						continue
					}
					closure := haversineMeters(points[start].Latitude, points[start].Longitude, p.Latitude, p.Longitude)
					if closure > math.Max(25, math.Min(150, length*.15)) {
						continue
					}
					ring, area := validateTrackPlaceRing(points[start : end+1])
					if ring != nil && area > bestArea {
						best, bestArea = ring, area
					}
				}
			}
		}
	}
	return best
}

func trackPlaceRingGridCell(p models.TrackPoint) [2]int {
	return [2]int{int(math.Floor(p.Longitude * 111000 / 150)), int(math.Floor(p.Latitude * 111000 / 150))}
}

func validateTrackPlaceRing(points []models.TrackPoint) (*trackPlaceRing, float64) {
	if len(points) < 4 {
		return nil, 0
	}
	r := &trackPlaceRing{points: points, minLat: 90, minLng: 180, maxLat: -90, maxLng: -180}
	for _, p := range points {
		r.minLat = math.Min(r.minLat, p.Latitude)
		r.maxLat = math.Max(r.maxLat, p.Latitude)
		r.minLng = math.Min(r.minLng, p.Longitude)
		r.maxLng = math.Max(r.maxLng, p.Longitude)
	}
	if r.maxLng-r.minLng > 1 || r.maxLat-r.minLat > 1 {
		return nil, 0
	}
	xScale := 111000 * math.Cos((r.minLat+r.maxLat)/2*math.Pi/180)
	if (r.maxLng-r.minLng)*xScale*(r.maxLat-r.minLat)*111000 > 200e6 {
		return nil, 0
	}
	var twiceArea float64
	for i, a := range points {
		b := points[(i+1)%len(points)]
		twiceArea += (a.Longitude-r.minLng)*(b.Latitude-r.minLat) - (b.Longitude-r.minLng)*(a.Latitude-r.minLat)
	}
	area := math.Abs(twiceArea) * xScale * 111000 / 2
	if area < 400 {
		return nil, 0
	}
	return r, area
}

func (r *trackPlaceRing) contains(lat, lng float64) bool {
	if lat <= r.minLat || lat >= r.maxLat || lng <= r.minLng || lng >= r.maxLng {
		return false
	}
	inside := false
	xScale := 111000 * math.Cos(lat*math.Pi/180)
	for i, a := range r.points {
		b := r.points[(i+1)%len(r.points)]
		ax, ay := (a.Longitude-lng)*xScale, (a.Latitude-lat)*111000
		bx, by := (b.Longitude-lng)*xScale, (b.Latitude-lat)*111000
		dx, dy := bx-ax, by-ay
		t := 0.0
		if d := dx*dx + dy*dy; d > 0 {
			t = math.Max(0, math.Min(1, -(ax*dx+ay*dy)/d))
		}
		if math.Hypot(ax+t*dx, ay+t*dy) < 3 {
			return false
		}
		if (a.Latitude > lat) != (b.Latitude > lat) && lng < (b.Longitude-a.Longitude)*(lat-a.Latitude)/(b.Latitude-a.Latitude)+a.Longitude {
			inside = !inside
		}
	}
	return inside
}
