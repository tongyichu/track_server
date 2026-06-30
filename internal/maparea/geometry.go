package maparea

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

const geometryEpsilon = 1e-10

// Geometry is a GCJ-02 GeoJSON Polygon or MultiPolygon. Coordinates are kept
// in their original JSON representation and prepared once when the catalog is
// loaded, so resolving an area does not decode JSON on the request path.
type Geometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`

	polygons []geoPolygon
}

type geoPoint struct {
	longitude float64
	latitude  float64
}

type geoRing []geoPoint
type geoPolygon []geoRing

func (g *Geometry) prepare() error {
	if g == nil {
		return nil
	}
	g.Type = strings.TrimSpace(g.Type)
	var rawPolygons [][][][]float64
	switch g.Type {
	case "Polygon":
		var polygon [][][]float64
		if err := json.Unmarshal(g.Coordinates, &polygon); err != nil {
			return fmt.Errorf("decode Polygon coordinates: %w", err)
		}
		rawPolygons = [][][][]float64{polygon}
	case "MultiPolygon":
		if err := json.Unmarshal(g.Coordinates, &rawPolygons); err != nil {
			return fmt.Errorf("decode MultiPolygon coordinates: %w", err)
		}
	default:
		return fmt.Errorf("unsupported geometry type %q", g.Type)
	}
	if len(rawPolygons) == 0 {
		return fmt.Errorf("geometry has no polygons")
	}

	polygons := make([]geoPolygon, 0, len(rawPolygons))
	for polygonIndex, rawPolygon := range rawPolygons {
		if len(rawPolygon) == 0 {
			return fmt.Errorf("polygon %d has no rings", polygonIndex)
		}
		polygon := make(geoPolygon, 0, len(rawPolygon))
		for ringIndex, rawRing := range rawPolygon {
			ring, err := prepareGeoRing(rawRing)
			if err != nil {
				return fmt.Errorf("polygon %d ring %d: %w", polygonIndex, ringIndex, err)
			}
			polygon = append(polygon, ring)
		}
		polygons = append(polygons, polygon)
	}
	g.polygons = polygons
	return nil
}

func prepareGeoRing(rawRing [][]float64) (geoRing, error) {
	if len(rawRing) < 4 {
		return nil, fmt.Errorf("must contain at least 4 positions")
	}
	ring := make(geoRing, 0, len(rawRing))
	for positionIndex, position := range rawRing {
		if len(position) < 2 {
			return nil, fmt.Errorf("position %d must contain longitude and latitude", positionIndex)
		}
		longitude, latitude := position[0], position[1]
		if math.IsNaN(longitude) || math.IsInf(longitude, 0) ||
			math.IsNaN(latitude) || math.IsInf(latitude, 0) ||
			longitude < -180 || longitude > 180 || latitude < -90 || latitude > 90 {
			return nil, fmt.Errorf("position %d is outside valid longitude/latitude range", positionIndex)
		}
		ring = append(ring, geoPoint{longitude: longitude, latitude: latitude})
	}
	if !sameGeoPoint(ring[0], ring[len(ring)-1]) {
		return nil, fmt.Errorf("linear ring is not closed")
	}
	return ring, nil
}

func sameGeoPoint(left, right geoPoint) bool {
	return math.Abs(left.longitude-right.longitude) <= geometryEpsilon &&
		math.Abs(left.latitude-right.latitude) <= geometryEpsilon
}

func (g *Geometry) contains(latitude, longitude float64) bool {
	if g == nil {
		return false
	}
	point := geoPoint{longitude: longitude, latitude: latitude}
	for _, polygon := range g.polygons {
		if polygonContains(polygon, point) {
			return true
		}
	}
	return false
}

func polygonContains(polygon geoPolygon, point geoPoint) bool {
	insideOuter, onOuterBoundary := ringContains(polygon[0], point)
	if onOuterBoundary {
		return true
	}
	if !insideOuter {
		return false
	}
	for _, hole := range polygon[1:] {
		insideHole, onHoleBoundary := ringContains(hole, point)
		if onHoleBoundary {
			return true
		}
		if insideHole {
			return false
		}
	}
	return true
}

func ringContains(ring geoRing, point geoPoint) (inside, boundary bool) {
	for i, j := 0, len(ring)-1; i < len(ring); j, i = i, i+1 {
		left, right := ring[j], ring[i]
		if pointOnSegment(point, left, right) {
			return false, true
		}
		if (left.latitude > point.latitude) == (right.latitude > point.latitude) {
			continue
		}
		intersectionLongitude := left.longitude +
			(point.latitude-left.latitude)*(right.longitude-left.longitude)/(right.latitude-left.latitude)
		if point.longitude < intersectionLongitude {
			inside = !inside
		}
	}
	return inside, false
}

func pointOnSegment(point, start, end geoPoint) bool {
	if point.longitude < math.Min(start.longitude, end.longitude)-geometryEpsilon ||
		point.longitude > math.Max(start.longitude, end.longitude)+geometryEpsilon ||
		point.latitude < math.Min(start.latitude, end.latitude)-geometryEpsilon ||
		point.latitude > math.Max(start.latitude, end.latitude)+geometryEpsilon {
		return false
	}
	cross := (point.longitude-start.longitude)*(end.latitude-start.latitude) -
		(point.latitude-start.latitude)*(end.longitude-start.longitude)
	scale := math.Abs(end.longitude-start.longitude) + math.Abs(end.latitude-start.latitude) + 1
	return math.Abs(cross) <= geometryEpsilon*scale
}
