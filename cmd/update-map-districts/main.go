// Command update-map-districts builds the embedded administrative fallback
// catalog from every leaf in DataV.GeoAtlas's nationwide administrative index.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/maparea"
)

type division struct {
	ADCode int    `json:"adcode"`
	Name   string `json:"name"`
	Level  string `json:"level"`
	Parent int    `json:"parent"`
}

type districtFile struct {
	CoordinateSystem string                       `json:"coordinate_system"`
	DataVersion      string                       `json:"data_version"`
	Source           string                       `json:"source"`
	Districts        []maparea.DistrictDefinition `json:"districts"`
}

type coverageReport struct {
	DataVersion    string         `json:"data_version"`
	Scope          string         `json:"scope"`
	CatalogSHA256  string         `json:"catalog_sha256"`
	Index          sourceRecord   `json:"index"`
	ExpectedLeaves int            `json:"expected_leaves"`
	WrittenAreas   int            `json:"written_areas"`
	CityCodes      int            `json:"city_codes"`
	Levels         map[string]int `json:"admin_levels"`
	Provinces      map[string]int `json:"areas_per_province"`
	Fallbacks      []division     `json:"non_district_leaves"`
	Sources        []sourceRecord `json:"boundary_sources"`
}

func main() {
	output := flag.String("output", "internal/maparea/districts.json", "generated catalog path")
	cache := flag.String("cache", "build/maparea-source", "source snapshot cache (use a new directory to refresh)")
	version := flag.String("version", time.Now().UTC().Format("2006-01-02"), "snapshot version, not upstream boundary effective date")
	workers := flag.Int("workers", 6, "maximum concurrent downloads")
	offline := flag.Bool("offline", false, "use cached source files only")
	flag.Parse()
	if *workers < 1 || *workers > 16 || *version == "" {
		log.Fatal("workers must be 1..16 and version must not be empty")
	}
	client := &sourceClient{cache: *cache, offline: *offline, client: &http.Client{Timeout: 30 * time.Second}}
	if err := generate(client, *output, *version, *workers); err != nil {
		log.Fatal(err)
	}
}

func generate(client *sourceClient, output, version string, workers int) error {
	raw, indexSource, err := client.get("areas_v3/bound/all.json")
	if err != nil {
		return err
	}
	var divisions []division
	if err := json.Unmarshal(raw, &divisions); err != nil {
		return err
	}
	byCode, leaves, err := indexedLeaves(divisions)
	if err != nil {
		return err
	}
	byParent := make(map[int][]division)
	for _, leaf := range leaves {
		byParent[leaf.Parent] = append(byParent[leaf.Parent], leaf)
	}
	log.Printf("index: %d administrative leaves across %d parent collections", len(leaves), len(byParent))
	boundaries, err := downloadBoundaries(client, byParent, workers)
	if err != nil {
		return err
	}
	var previous districtFile
	if raw, err := os.ReadFile(output); err == nil {
		if err := json.Unmarshal(raw, &previous); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	old := make(map[string]maparea.DistrictDefinition)
	englishCities := make(map[string]string)
	for _, d := range previous.Districts {
		old[d.ID] = d
		if d.CityNameEnglish != "" {
			englishCities[d.CityCode] = d.CityNameEnglish
		}
	}
	file := districtFile{CoordinateSystem: "GCJ02", DataVersion: version, Source: "DataV.GeoAtlas areas_v3 + areas/bound; nationwide indexed administrative leaves"}
	report := coverageReport{DataVersion: version, Scope: "DataV index snapshot: district boundaries where supplied; childless cities and Taiwan province use their own boundaries. Not a guarantee of latest statutory divisions.", Index: indexSource, ExpectedLeaves: len(leaves), Levels: map[string]int{}, Provinces: map[string]int{}}
	sources := make(map[string]sourceRecord)
	cities := make(map[string]bool)
	for _, leaf := range leaves {
		b := boundaries[leaf.ADCode]
		bounds, err := geometryBounds(b.Geometry)
		if err != nil {
			return fmt.Errorf("%d %s: %w", leaf.ADCode, leaf.Name, err)
		}
		city := leaf
		if leaf.Level == "district" {
			city = byCode[leaf.Parent]
		}
		cityCode := strconv.Itoa(city.ADCode)
		if config.CityNameByCode(cityCode) == "" && city.ADCode != leaf.ADCode {
			return fmt.Errorf("unknown parent city %s for %d", cityCode, leaf.ADCode)
		}
		priority := map[string]int{"district": 100, "city": 50, "province": 10}[leaf.Level]
		d := maparea.DistrictDefinition{ID: fmt.Sprintf("district-%d", leaf.ADCode), CityCode: cityCode, CityNameEnglish: englishCities[cityCode], Name: leaf.Name, AdminLevel: leaf.Level, Priority: priority, Bounds: bounds, Geometry: b.Geometry, Source: b.source.URL}
		if p, ok := old[d.ID]; ok && p.Name == d.Name {
			d.NameEnglish = p.NameEnglish
		}
		// The introduction URL changes only when this area's generated content changes.
		content, err := json.Marshal(d)
		if err != nil {
			return err
		}
		hash := sha256.Sum256(content)
		d.ContentVersion = hex.EncodeToString(hash[:8])
		file.Districts = append(file.Districts, d)
		delete(old, d.ID)
		cities[cityCode] = true
		sources[b.source.URL] = b.source
		report.Levels[leaf.Level]++
		province := leaf
		for province.Level != "province" {
			province = byCode[province.Parent]
		}
		report.Provinces[strconv.Itoa(province.ADCode)+" "+province.Name]++
		if leaf.Level != "district" {
			report.Fallbacks = append(report.Fallbacks, leaf)
		}
	}
	if len(old) != 0 {
		return fmt.Errorf("refusing to remove %d existing area IDs; reconcile retired adcodes before updating", len(old))
	}
	if len(report.Provinces) != 34 {
		return fmt.Errorf("incomplete province coverage: %d, expected 34", len(report.Provinces))
	}
	data, err := json.Marshal(file)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	manual, err := os.ReadFile(filepath.Join(filepath.Dir(output), "catalog.json"))
	if err != nil {
		return err
	}
	// Validate every ring and the manual-override merge with the production parser
	// before replacing any embedded data. No partial national catalog is published.
	if _, err := maparea.ParseCatalogFiles(manual, data); err != nil {
		return fmt.Errorf("validate generated catalog: %w", err)
	}
	report.WrittenAreas = len(file.Districts)
	report.CityCodes = len(cities)
	hash := sha256.Sum256(data)
	report.CatalogSHA256 = hex.EncodeToString(hash[:])
	for _, source := range sources {
		report.Sources = append(report.Sources, source)
	}
	sort.Slice(report.Sources, func(i, j int) bool { return report.Sources[i].URL < report.Sources[j].URL })
	audit, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := writeAtomic(output, data); err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(filepath.Dir(output), "districts.manifest.json"), append(audit, '\n')); err != nil {
		return err
	}
	log.Printf("wrote %s: %d areas, %d city codes, %d provinces; levels=%v; all indexed leaves accounted for", output, report.WrittenAreas, report.CityCodes, len(report.Provinces), report.Levels)
	return nil
}

func downloadBoundaries(client *sourceClient, parents map[int][]division, workers int) (map[int]boundary, error) {
	jobs := make(chan int)
	result := make(map[int]boundary)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var failures []string
	completed := 0
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for parent := range jobs {
				found, parentErr := client.boundaries(parent, true)
				for _, leaf := range parents[parent] {
					b, ok := found[leaf.ADCode]
					if !ok {
						individual, err := client.boundaries(leaf.ADCode, false)
						b, ok = individual[leaf.ADCode]
						if !ok {
							mu.Lock()
							failures = append(failures, fmt.Sprintf("missing %d %s: parent=%v; individual=%v", leaf.ADCode, leaf.Name, parentErr, err))
							mu.Unlock()
							continue
						}
					}
					mu.Lock()
					result[leaf.ADCode] = b
					mu.Unlock()
				}
				mu.Lock()
				completed++
				if completed%25 == 0 || completed == len(parents) {
					log.Printf("boundary collections: %d/%d; areas=%d", completed, len(parents), len(result))
				}
				mu.Unlock()
			}
		}()
	}
	parentCodes := make([]int, 0, len(parents))
	for code := range parents {
		parentCodes = append(parentCodes, code)
	}
	sort.Ints(parentCodes)
	for _, parent := range parentCodes {
		jobs <- parent
	}
	close(jobs)
	wg.Wait()
	if len(failures) > 0 {
		sort.Strings(failures)
		return nil, fmt.Errorf("incomplete boundaries (%d): %v", len(failures), failures)
	}
	return result, nil
}
