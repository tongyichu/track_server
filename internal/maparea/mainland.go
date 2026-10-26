package maparea

import (
	_ "embed"
	"encoding/json"
	"sync"
)

// Provider selection uses a province mask rather than a rectangular approximation
// which would incorrectly send India, Vietnam and other countries to AMap.
//
//go:embed mainland.json
var mainlandJSON []byte
var mainlandOnce sync.Once
var mainlandPolygons []struct {
	ring   geoPolygon
	bounds Bounds
}

func IsMainland(latitude, longitude float64) bool {
	if latitude < 3 || latitude > 54 || longitude < 73 || longitude > 136 {
		return false
	}
	mainlandOnce.Do(func() {
		var file struct {
			Geometry Geometry `json:"geometry"`
		}
		if err := json.Unmarshal(mainlandJSON, &file); err != nil {
			panic(err)
		}
		if err := file.Geometry.prepare(); err != nil {
			panic(err)
		}
		for _, p := range file.Geometry.polygons {
			b := Bounds{MinLatitude: 90, MinLongitude: 180, MaxLatitude: -90, MaxLongitude: -180}
			for _, point := range p[0] {
				if point.latitude < b.MinLatitude {
					b.MinLatitude = point.latitude
				}
				if point.latitude > b.MaxLatitude {
					b.MaxLatitude = point.latitude
				}
				if point.longitude < b.MinLongitude {
					b.MinLongitude = point.longitude
				}
				if point.longitude > b.MaxLongitude {
					b.MaxLongitude = point.longitude
				}
			}
			mainlandPolygons = append(mainlandPolygons, struct {
				ring   geoPolygon
				bounds Bounds
			}{p, b})
		}
	})
	for _, p := range mainlandPolygons {
		if p.bounds.contains(latitude, longitude) && polygonContains(p.ring, geoPoint{longitude: longitude, latitude: latitude}) {
			return true
		}
	}
	return false
}
