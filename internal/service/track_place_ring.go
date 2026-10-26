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
	var length float64
	r := &trackPlaceRing{points: points, minLat: 90, minLng: 180, maxLat: -90, maxLng: -180}
	for i, p := range points {
		if !validMapLatLng(p.Latitude, p.Longitude) {
			return nil
		}
		r.minLat = math.Min(r.minLat, p.Latitude)
		r.maxLat = math.Max(r.maxLat, p.Latitude)
		r.minLng = math.Min(r.minLng, p.Longitude)
		r.maxLng = math.Max(r.maxLng, p.Longitude)
		if i > 0 {
			length += haversineMeters(points[i-1].Latitude, points[i-1].Longitude, p.Latitude, p.Longitude)
		}
	}
	first, last := points[0], points[len(points)-1]
	if length < 100 || haversineMeters(first.Latitude, first.Longitude, last.Latitude, last.Longitude) > math.Max(25, math.Min(150, length*.02)) {
		return nil
	}
	// Avoid interpreting a dateline crossing or a huge city tour as one scenic loop.
	if r.maxLng-r.minLng > 1 || r.maxLat-r.minLat > 1 {
		return nil
	}
	xScale := 111000 * math.Cos((r.minLat+r.maxLat)/2*math.Pi/180)
	boxArea := (r.maxLng - r.minLng) * xScale * (r.maxLat - r.minLat) * 111000
	if boxArea > 200e6 {
		return nil
	}
	var twiceArea float64
	for i, a := range points {
		b := points[(i+1)%len(points)]
		twiceArea += (a.Longitude-r.minLng)*(b.Latitude-r.minLat) - (b.Longitude-r.minLng)*(a.Latitude-r.minLat)
	}
	if math.Abs(twiceArea)*xScale*111000/2 < 400 {
		return nil
	}
	return r
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
