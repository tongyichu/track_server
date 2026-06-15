package service

import (
	"encoding/json"
	"testing"

	"github.com/tongyichu/track_server/internal/models"
)

func TestParseTrackPointsJSONSupportsPointArray(t *testing.T) {
	raw, err := json.Marshal([]models.TrackPoint{
		{Latitude: 30.1, Longitude: 120.1},
		{Latitude: 30.2, Longitude: 120.2},
		{Latitude: 91, Longitude: 120.3},
	})
	if err != nil {
		t.Fatalf("marshal points: %v", err)
	}

	points, err := parseTrackPointsJSON(raw)
	if err != nil {
		t.Fatalf("parse points failed: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("expected invalid coordinate filtered, got %d points", len(points))
	}
	if points[0].Latitude != 30.1 || points[1].Longitude != 120.2 {
		t.Fatalf("unexpected parsed points: %+v", points)
	}
}

func TestParseTrackPointsJSONSupportsGeoJSONLineString(t *testing.T) {
	raw := []byte(`{
		"type": "Feature",
		"geometry": {
			"type": "LineString",
			"coordinates": [[114.1,22.3],[114.2,22.4]]
		}
	}`)

	points, err := parseTrackPointsJSON(raw)
	if err != nil {
		t.Fatalf("parse geojson failed: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(points))
	}
	if points[0].Longitude != 114.1 || points[0].Latitude != 22.3 {
		t.Fatalf("unexpected first point: %+v", points[0])
	}
}

func TestBuildTrackGeoIndexDefaultsTrackTypeAndSimplifies(t *testing.T) {
	points := make([]models.TrackPoint, 0, 600)
	for i := 0; i < 600; i++ {
		points = append(points, models.TrackPoint{
			Index:     i,
			Latitude:  22 + float64(i)*0.0001,
			Longitude: 114 + float64(i)*0.0001,
		})
	}
	track := &models.Track{ID: "NO.00000001", UserID: 1001, CityCode: "810000", Distance: 1200}

	index, err := buildTrackGeoIndex(track, points)
	if err != nil {
		t.Fatalf("build index failed: %v", err)
	}
	if index.TrackType != "徒步" {
		t.Fatalf("expected default track type 徒步, got %q", index.TrackType)
	}
	if index.PointCount != 600 {
		t.Fatalf("expected point count 600, got %d", index.PointCount)
	}
	if len(index.SimplifiedPolyline) != maxSimplifiedTrackMapPoints {
		t.Fatalf("expected simplified polyline capped at %d, got %d", maxSimplifiedTrackMapPoints, len(index.SimplifiedPolyline))
	}
	if index.MinLat != 22 || index.MaxLng <= index.MinLng {
		t.Fatalf("unexpected bounds: %+v", index)
	}
	if index.SimplifiedPolylineJSON == "" {
		t.Fatalf("expected simplified polyline json")
	}
}
