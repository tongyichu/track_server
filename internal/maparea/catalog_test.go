package maparea

import (
	"encoding/json"
	"testing"

	"github.com/tongyichu/track_server/internal/config"
)

func TestDefaultCatalogResolvePrefersScenicSpot(t *testing.T) {
	catalog := DefaultCatalog()
	area := catalog.Resolve(30.25, 120.14)
	if area == nil {
		t.Fatal("expected an area")
	}
	if area.ID != "scenic-west-lake" || area.Type != TypeScenicSpot {
		t.Fatalf("resolved %+v, want scenic-west-lake", area)
	}
}

func TestDefaultCatalogResolveDistrict(t *testing.T) {
	catalog := DefaultCatalog()
	area := catalog.Resolve(30.30, 120.05)
	if area == nil {
		t.Fatal("expected an area")
	}
	if area.ID != "district-330106" || area.Type != TypeDistrict {
		t.Fatalf("resolved %+v, want district-330106", area)
	}
}

func TestDefaultCatalogResolveUnknown(t *testing.T) {
	if area := DefaultCatalog().Resolve(45.0, 90.0); area != nil {
		t.Fatalf("resolved unexpected area %+v", area)
	}
}

func TestDefaultCatalogIncludesMajorCityDistricts(t *testing.T) {
	catalog := DefaultCatalog()
	if len(catalog.byID) != 195 {
		t.Fatalf("catalog has %d areas, want 195", len(catalog.byID))
	}
	for _, id := range []string{
		"district-110101",
		"district-130102",
		"district-210102",
		"district-310101",
		"district-340102",
		"district-370102",
		"district-440304",
		"district-450103",
		"district-510104",
		"district-610113",
		"district-650102",
	} {
		area := catalog.Find(id)
		if area == nil || area.Type != TypeDistrict || !area.HasIntroduction() {
			t.Fatalf("missing generated district %s: %+v", id, area)
		}
	}
}

func TestEmbeddedDistrictCatalogMetadata(t *testing.T) {
	var file districtCatalogFile
	if err := json.Unmarshal(embeddedDistrictsJSON, &file); err != nil {
		t.Fatalf("decode embedded districts: %v", err)
	}
	if file.CoordinateSystem != "GCJ02" || file.DataVersion == "" || file.Source == "" {
		t.Fatalf("missing district metadata: %+v", file)
	}
	if len(file.Districts) != 194 {
		t.Fatalf("district count=%d, want 194", len(file.Districts))
	}
	cities := make(map[string]struct{})
	ids := make(map[string]struct{}, len(file.Districts))
	for _, district := range file.Districts {
		if _, exists := ids[district.ID]; exists {
			t.Fatalf("duplicate district id %s", district.ID)
		}
		ids[district.ID] = struct{}{}
		cities[district.CityCode] = struct{}{}
		if config.CityNameByCode(district.CityCode) == "" {
			t.Fatalf("unknown city_code %s for %s", district.CityCode, district.ID)
		}
		if district.Source == "" || district.ContentVersion == "" {
			t.Fatalf("missing source/version for %s", district.ID)
		}
		if district.Bounds.size() <= 0 {
			t.Fatalf("invalid bounds for %s: %+v", district.ID, district.Bounds)
		}
	}
	if len(cities) != 36 {
		t.Fatalf("city count=%d, want 36", len(cities))
	}
}

func TestManualAreaOverridesGeneratedDistrict(t *testing.T) {
	manual := []byte(`{
		"coordinate_system":"GCJ02",
		"areas":[{
			"id":"district-110101",
			"type":"district",
			"city_code":"110000",
			"priority":150,
			"bounds":{"min_latitude":39.8,"min_longitude":116.3,"max_latitude":40.0,"max_longitude":116.5},
			"zh":{"name":"人工东城区","summary":"人工介绍"},
			"en":{"name":"Curated Dongcheng"},
			"source":"manual",
			"content_version":"2"
		}]}`)
	catalog, err := ParseCatalogFiles(manual, embeddedDistrictsJSON)
	if err != nil {
		t.Fatalf("parse merged catalog: %v", err)
	}
	area := catalog.Find("district-110101")
	if area == nil || area.Name() != "人工东城区" || area.ContentVersion != "2" {
		t.Fatalf("manual override not applied: %+v", area)
	}
}
