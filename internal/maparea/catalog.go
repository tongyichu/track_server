package maparea

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

const (
	TypeDistrict   = "district"
	TypeScenicSpot = "scenic_spot"
)

// Bounds is a GCJ-02 bounding box used to match an area-cluster center.
type Bounds struct {
	MinLatitude  float64 `json:"min_latitude"`
	MinLongitude float64 `json:"min_longitude"`
	MaxLatitude  float64 `json:"max_latitude"`
	MaxLongitude float64 `json:"max_longitude"`
}

func (b Bounds) contains(latitude, longitude float64) bool {
	return latitude >= b.MinLatitude && latitude <= b.MaxLatitude &&
		longitude >= b.MinLongitude && longitude <= b.MaxLongitude
}

func (b Bounds) size() float64 {
	return (b.MaxLatitude - b.MinLatitude) * (b.MaxLongitude - b.MinLongitude)
}

// LocalizedContent is the copy rendered by the embedded introduction page.
type LocalizedContent struct {
	Name       string   `json:"name"`
	Summary    string   `json:"summary"`
	Highlights []string `json:"highlights"`
	Tips       []string `json:"tips"`
}

// Definition is one curated administrative district or scenic spot.
type Definition struct {
	ID              string           `json:"id"`
	Type            string           `json:"type"`
	CityCode        string           `json:"city_code"`
	CityNameEnglish string           `json:"city_name_en"`
	Priority        int              `json:"priority"`
	Bounds          Bounds           `json:"bounds"`
	Geometry        *Geometry        `json:"geometry,omitempty"`
	Chinese         LocalizedContent `json:"zh"`
	English         LocalizedContent `json:"en"`
	Source          string           `json:"source"`
	ContentVersion  string           `json:"content_version"`
}

// DistrictDefinition is a compact generated district record. Its introduction
// copy is expanded from a stable template while parsing the catalog.
type DistrictDefinition struct {
	ID              string    `json:"id"`
	CityCode        string    `json:"city_code"`
	CityNameEnglish string    `json:"city_name_en"`
	Name            string    `json:"name"`
	NameEnglish     string    `json:"name_en"`
	Priority        int       `json:"priority"`
	Bounds          Bounds    `json:"bounds"`
	Geometry        *Geometry `json:"geometry,omitempty"`
	Source          string    `json:"source"`
	ContentVersion  string    `json:"content_version"`
}

func (d Definition) Name() string {
	return strings.TrimSpace(d.Chinese.Name)
}

func (d Definition) HasIntroduction() bool {
	return strings.TrimSpace(d.Chinese.Summary) != "" || len(d.Chinese.Highlights) > 0 || len(d.Chinese.Tips) > 0
}

// Catalog is an immutable in-process map-area directory.
type Catalog struct {
	byID  map[string]Definition
	areas []Definition
}

type catalogFile struct {
	CoordinateSystem string       `json:"coordinate_system"`
	Areas            []Definition `json:"areas"`
}

type districtCatalogFile struct {
	CoordinateSystem string               `json:"coordinate_system"`
	DataVersion      string               `json:"data_version"`
	Source           string               `json:"source"`
	Districts        []DistrictDefinition `json:"districts"`
}

//go:embed catalog.json
var embeddedCatalogJSON []byte

//go:embed districts.json
var embeddedDistrictsJSON []byte

var (
	defaultCatalogOnce sync.Once
	defaultCatalog     *Catalog
	areaIDPattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
)

// DefaultCatalog returns the versioned map-area catalog bundled with the binary.
func DefaultCatalog() *Catalog {
	defaultCatalogOnce.Do(func() {
		catalog, err := ParseCatalogFiles(embeddedCatalogJSON, embeddedDistrictsJSON)
		if err != nil {
			panic(fmt.Sprintf("invalid embedded map-area catalog: %v", err))
		}
		defaultCatalog = catalog
	})
	return defaultCatalog
}

// ParseCatalog validates and builds a map-area catalog.
func ParseCatalog(raw []byte) (*Catalog, error) {
	return ParseCatalogFiles(raw, nil)
}

// ParseCatalogFiles merges manually curated areas with generated core districts.
func ParseCatalogFiles(raw, districtsRaw []byte) (*Catalog, error) {
	var file catalogFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("decode catalog: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(file.CoordinateSystem), "GCJ02") {
		return nil, fmt.Errorf("coordinate_system must be GCJ02")
	}
	districts := make([]DistrictDefinition, 0)
	if len(districtsRaw) > 0 {
		var districtFile districtCatalogFile
		if err := json.Unmarshal(districtsRaw, &districtFile); err != nil {
			return nil, fmt.Errorf("decode districts: %w", err)
		}
		if !strings.EqualFold(strings.TrimSpace(districtFile.CoordinateSystem), "GCJ02") {
			return nil, fmt.Errorf("district coordinate_system must be GCJ02")
		}
		districts = districtFile.Districts
	}
	catalog := &Catalog{byID: make(map[string]Definition, len(file.Areas)+len(districts))}
	for _, district := range districts {
		area, err := normalizeDefinition(district.definition())
		if err != nil {
			return nil, err
		}
		if _, exists := catalog.byID[area.ID]; exists {
			return nil, fmt.Errorf("duplicate generated district id %q", area.ID)
		}
		catalog.byID[area.ID] = area
	}
	manualIDs := make(map[string]struct{}, len(file.Areas))
	for _, rawArea := range file.Areas {
		area, err := normalizeDefinition(rawArea)
		if err != nil {
			return nil, err
		}
		if _, exists := manualIDs[area.ID]; exists {
			return nil, fmt.Errorf("duplicate manual area id %q", area.ID)
		}
		manualIDs[area.ID] = struct{}{}
		// Manually curated entries intentionally override generated districts
		// with the same stable ID (for example richer copy or tighter bounds).
		// If the override only changes copy or metadata, retain the generated
		// geometry so it does not silently fall back to bounding-box matching.
		if generated, exists := catalog.byID[area.ID]; exists && area.Geometry == nil {
			area.Geometry = generated.Geometry
		}
		catalog.byID[area.ID] = area
	}
	for _, area := range catalog.byID {
		catalog.areas = append(catalog.areas, area)
	}
	sort.SliceStable(catalog.areas, func(i, j int) bool {
		if catalog.areas[i].Priority != catalog.areas[j].Priority {
			return catalog.areas[i].Priority > catalog.areas[j].Priority
		}
		return catalog.areas[i].Bounds.size() < catalog.areas[j].Bounds.size()
	})
	return catalog, nil
}

func normalizeDefinition(area Definition) (Definition, error) {
	area.ID = strings.TrimSpace(area.ID)
	area.Type = strings.TrimSpace(area.Type)
	area.CityCode = strings.TrimSpace(area.CityCode)
	if area.Geometry != nil {
		if err := area.Geometry.prepare(); err != nil {
			return Definition{}, fmt.Errorf("area %q has invalid geometry: %w", area.ID, err)
		}
	}
	if err := validateDefinition(area); err != nil {
		return Definition{}, err
	}
	return area, nil
}

func (d DistrictDefinition) definition() Definition {
	nameEnglish := strings.TrimSpace(d.NameEnglish)
	if nameEnglish == "" {
		nameEnglish = strings.TrimSpace(d.Name)
	}
	return Definition{
		ID:              d.ID,
		Type:            TypeDistrict,
		CityCode:        d.CityCode,
		CityNameEnglish: d.CityNameEnglish,
		Priority:        d.Priority,
		Bounds:          d.Bounds,
		Geometry:        d.Geometry,
		Chinese: LocalizedContent{
			Name:       d.Name,
			Summary:    fmt.Sprintf("%s收录城市街区、公园绿道与周边户外路线，可结合运动类型、距离和路线详情选择合适线路。", d.Name),
			Highlights: []string{"探索城市步行与跑步路线", "发现公园绿道及周边户外路线", "按运动类型和距离筛选路线"},
			Tips:       []string{"出发前确认天气和路线状态", "遵守公园、绿道和保护区域的现场规定", "根据个人体力选择路线并合理安排返程"},
		},
		English: LocalizedContent{
			Name:       nameEnglish,
			Summary:    "Explore urban streets, parks, greenways, and nearby outdoor routes, then choose a route by activity, distance, and route details.",
			Highlights: []string{"Urban walking and running routes", "Parks, greenways, and nearby outdoor routes", "Filters for activity type and distance"},
			Tips:       []string{"Check weather and route conditions before departure", "Follow local notices in parks, greenways, and protected areas", "Choose a route for your fitness level and plan your return"},
		},
		Source:         d.Source,
		ContentVersion: d.ContentVersion,
	}
}

func validateDefinition(area Definition) error {
	if area.ID == "" {
		return fmt.Errorf("area id is required")
	}
	if !areaIDPattern.MatchString(area.ID) {
		return fmt.Errorf("area %q has invalid id", area.ID)
	}
	if area.Type != TypeDistrict && area.Type != TypeScenicSpot {
		return fmt.Errorf("area %q has unsupported type %q", area.ID, area.Type)
	}
	if area.Name() == "" {
		return fmt.Errorf("area %q has no Chinese name", area.ID)
	}
	if area.Bounds.MinLatitude < -90 || area.Bounds.MaxLatitude > 90 ||
		area.Bounds.MinLongitude < -180 || area.Bounds.MaxLongitude > 180 ||
		area.Bounds.MinLatitude >= area.Bounds.MaxLatitude ||
		area.Bounds.MinLongitude >= area.Bounds.MaxLongitude {
		return fmt.Errorf("area %q has invalid bounds", area.ID)
	}
	return nil
}

// Resolve returns the highest-priority, most-specific area containing a point.
func (c *Catalog) Resolve(latitude, longitude float64) *Definition {
	if c == nil {
		return nil
	}
	for i := range c.areas {
		if !c.areas[i].Bounds.contains(latitude, longitude) {
			continue
		}
		if c.areas[i].Geometry != nil && !c.areas[i].Geometry.contains(latitude, longitude) {
			continue
		}
		area := c.areas[i]
		return &area
	}
	return nil
}

// Find returns an area by its stable catalog ID.
func (c *Catalog) Find(id string) *Definition {
	if c == nil {
		return nil
	}
	area, ok := c.byID[strings.TrimSpace(id)]
	if !ok {
		return nil
	}
	return &area
}
