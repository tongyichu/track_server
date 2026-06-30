package maparea

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPolygonContainsOuterRingAndExcludesHole(t *testing.T) {
	geometry := mustPrepareGeometry(t, "Polygon", `[
		[[0,0],[10,0],[10,10],[0,10],[0,0]],
		[[3,3],[7,3],[7,7],[3,7],[3,3]]
	]`)

	for _, test := range []struct {
		name                string
		latitude, longitude float64
		want                bool
	}{
		{name: "inside outer ring", latitude: 1, longitude: 1, want: true},
		{name: "inside hole", latitude: 5, longitude: 5, want: false},
		{name: "outside", latitude: 5, longitude: 11, want: false},
		{name: "outer boundary", latitude: 0, longitude: 5, want: true},
		{name: "hole boundary", latitude: 3, longitude: 5, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := geometry.contains(test.latitude, test.longitude); got != test.want {
				t.Fatalf("contains(%v, %v)=%v, want %v", test.latitude, test.longitude, got, test.want)
			}
		})
	}
}

func TestMultiPolygonContainsAnyPolygon(t *testing.T) {
	geometry := mustPrepareGeometry(t, "MultiPolygon", `[
		[[[0,0],[2,0],[2,2],[0,2],[0,0]]],
		[[[8,8],[10,8],[10,10],[8,10],[8,8]]]
	]`)

	if !geometry.contains(1, 1) || !geometry.contains(9, 9) {
		t.Fatal("expected points in both polygons to match")
	}
	if geometry.contains(5, 5) {
		t.Fatal("point between polygons unexpectedly matched")
	}
}

func TestCatalogResolveRejectsBoundingBoxFalsePositive(t *testing.T) {
	raw := []byte(`{
		"coordinate_system":"GCJ02",
		"areas":[{
			"id":"district-triangle",
			"type":"district",
			"city_code":"test",
			"priority":100,
			"bounds":{"min_latitude":0,"min_longitude":0,"max_latitude":10,"max_longitude":10},
			"geometry":{"type":"Polygon","coordinates":[[[0,0],[10,0],[0,10],[0,0]]]},
			"zh":{"name":"三角区域"},
			"en":{"name":"Triangle"},
			"source":"test",
			"content_version":"1"
		}]
	}`)
	catalog, err := ParseCatalog(raw)
	if err != nil {
		t.Fatalf("parse catalog: %v", err)
	}
	if area := catalog.Resolve(1, 1); area == nil {
		t.Fatal("point inside polygon did not match")
	}
	if area := catalog.Resolve(9, 9); area != nil {
		t.Fatalf("bounding-box false positive matched %+v", area)
	}
}

func TestGeometryValidation(t *testing.T) {
	for _, test := range []struct {
		name        string
		geometry    Geometry
		wantMessage string
	}{
		{
			name:        "unsupported type",
			geometry:    Geometry{Type: "LineString", Coordinates: json.RawMessage(`[[0,0],[1,1]]`)},
			wantMessage: "unsupported geometry type",
		},
		{
			name:        "unclosed ring",
			geometry:    Geometry{Type: "Polygon", Coordinates: json.RawMessage(`[[[0,0],[1,0],[1,1],[0,1]]]`)},
			wantMessage: "linear ring is not closed",
		},
		{
			name:        "invalid position",
			geometry:    Geometry{Type: "Polygon", Coordinates: json.RawMessage(`[[[0,0],[181,0],[1,1],[0,0]]]`)},
			wantMessage: "outside valid longitude/latitude range",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.geometry.prepare()
			if err == nil || !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf("prepare error=%v, want message %q", err, test.wantMessage)
			}
		})
	}
}

func mustPrepareGeometry(t *testing.T, geometryType, coordinates string) *Geometry {
	t.Helper()
	geometry := &Geometry{Type: geometryType, Coordinates: json.RawMessage(coordinates)}
	if err := geometry.prepare(); err != nil {
		t.Fatalf("prepare geometry: %v", err)
	}
	return geometry
}
