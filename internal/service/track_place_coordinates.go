package service

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/tongyichu/track_server/internal/maparea"
	"github.com/tongyichu/track_server/internal/models"
)

// Map indexes and viewport APIs use GCJ02 in mainland China; elsewhere the
// transform is identity. The original uploaded track and its CRS are unchanged.
func mapPointToGCJ02(p models.TrackPoint, source string) (models.TrackPoint, error) {
	switch strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(source), "-", "")) {
	case "", "GCJ02":
		return p, nil // legacy indexes were already treated as GCJ02.
	case "WGS84":
		if maparea.IsMainland(p.Latitude, p.Longitude) {
			a, b := mapCoordinateDelta(p.Latitude, p.Longitude)
			p.Latitude += a
			p.Longitude += b
		}
		return p, nil
	case "BD09":
		x, y := p.Longitude-0.0065, p.Latitude-0.006
		z := math.Sqrt(x*x+y*y) - 0.00002*math.Sin(y*math.Pi*3000/180)
		theta := math.Atan2(y, x) - 0.000003*math.Cos(x*math.Pi*3000/180)
		p.Longitude = z * math.Cos(theta)
		p.Latitude = z * math.Sin(theta)
		return p, nil
	default:
		return p, fmt.Errorf("unsupported track coordinate system")
	}
}
func mapGCJ02ToWGS84(p models.TrackPoint) models.TrackPoint {
	if !maparea.IsMainland(p.Latitude, p.Longitude) {
		return p
	}
	target := p
	for i := 0; i < 3; i++ {
		a, b := mapCoordinateDelta(p.Latitude, p.Longitude)
		p.Latitude = target.Latitude - a
		p.Longitude = target.Longitude - b
	}
	return p
}
func mapCoordinateDelta(lat, lng float64) (float64, float64) {
	x, y := lng-105, lat-35
	a := -100 + 2*x + 3*y + 0.2*y*y + 0.1*x*y + 0.2*math.Sqrt(math.Abs(x))
	a += (20*math.Sin(6*x*math.Pi) + 20*math.Sin(2*x*math.Pi)) * 2 / 3
	a += (20*math.Sin(y*math.Pi) + 40*math.Sin(y/3*math.Pi)) * 2 / 3
	a += (160*math.Sin(y/12*math.Pi) + 320*math.Sin(y*math.Pi/30)) * 2 / 3
	b := 300 + x + 2*y + 0.1*x*x + 0.1*x*y + 0.1*math.Sqrt(math.Abs(x))
	b += (20*math.Sin(6*x*math.Pi) + 20*math.Sin(2*x*math.Pi)) * 2 / 3
	b += (20*math.Sin(x*math.Pi) + 40*math.Sin(x/3*math.Pi)) * 2 / 3
	b += (150*math.Sin(x/12*math.Pi) + 300*math.Sin(x/30*math.Pi)) * 2 / 3
	rad := lat / 180 * math.Pi
	magic := 1 - 0.00669342162296594323*math.Sin(rad)*math.Sin(rad)
	root := math.Sqrt(magic)
	return a * 180 / ((6378245 * (1 - 0.00669342162296594323)) / (magic * root) * math.Pi), b * 180 / (6378245 / root * math.Cos(rad) * math.Pi)
}

func trackPlaceSourceVersion(index *models.TrackGeoIndex) string {
	data, _ := json.Marshal(struct {
		Version                            int
		CRS, Type                          string
		Points                             []models.TrackPoint
		StartLat, StartLng, EndLat, EndLng float64
	}{trackPlaceRuleVersion, index.CoordinateSystem, index.TrackType, index.SimplifiedPolyline, index.StartLat, index.StartLng, index.EndLat, index.EndLng})
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// Equal-distance samples avoid stationary recordings and irregular GPS sampling
// density determining the result. They are evidence, not a measured polygon.
func trackPlaceSamples(index *models.TrackGeoIndex, count int) ([]models.TrackPoint, error) {
	points := index.SimplifiedPolyline
	if len(points) < 2 {
		points = []models.TrackPoint{{Latitude: index.StartLat, Longitude: index.StartLng}, {Latitude: index.EndLat, Longitude: index.EndLng}}
	}
	canonical := make([]models.TrackPoint, len(points))
	cumulative := make([]float64, len(points))
	for i, p := range points {
		if !validLatLng(p.Latitude, p.Longitude) || math.IsNaN(p.Latitude) || math.IsNaN(p.Longitude) {
			return nil, fmt.Errorf("invalid track coordinates")
		}
		v, err := mapPointToGCJ02(p, index.CoordinateSystem)
		if err != nil {
			return nil, err
		}
		canonical[i] = v
		if i > 0 {
			cumulative[i] = cumulative[i-1] + haversineMeters(canonical[i-1].Latitude, canonical[i-1].Longitude, v.Latitude, v.Longitude)
		}
	}
	total := cumulative[len(cumulative)-1]
	if total < 100 {
		return nil, nil
	}
	out := make([]models.TrackPoint, 0, count)
	segment := 1
	for i := 0; i < count; i++ {
		distance := total * (float64(i) + 0.5) / float64(count)
		for segment < len(cumulative)-1 && cumulative[segment] < distance {
			segment++
		}
		length := cumulative[segment] - cumulative[segment-1]
		if length <= 0 {
			continue
		}
		t := (distance - cumulative[segment-1]) / length
		a, b := canonical[segment-1], canonical[segment]
		out = append(out, models.TrackPoint{Latitude: a.Latitude + (b.Latitude-a.Latitude)*t, Longitude: a.Longitude + (b.Longitude-a.Longitude)*t})
	}
	return out, nil
}
