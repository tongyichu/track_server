package main

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/tongyichu/track_server/internal/maparea"
)

func indexedLeaves(divisions []division) (map[int]division, []division, error) {
	byCode := make(map[int]division)
	children := make(map[int]int)
	for _, d := range divisions {
		if d.ADCode < 100000 || d.ADCode > 999999 || d.Name == "" {
			return nil, nil, fmt.Errorf("invalid administrative index entry: %+v", d)
		}
		if _, exists := byCode[d.ADCode]; exists {
			return nil, nil, fmt.Errorf("duplicate index adcode %d", d.ADCode)
		}
		byCode[d.ADCode] = d
		children[d.Parent]++
	}
	if byCode[100000].Level != "country" {
		return nil, nil, fmt.Errorf("index is missing the national root")
	}
	var leaves []division
	for _, d := range divisions {
		seen := map[int]bool{}
		for ancestor := d; ancestor.ADCode != 100000; ancestor = byCode[ancestor.Parent] {
			if ancestor.ADCode == 0 || seen[ancestor.ADCode] {
				return nil, nil, fmt.Errorf("broken ancestry for %d", d.ADCode)
			}
			seen[ancestor.ADCode] = true
		}
		if children[d.ADCode] == 0 {
			if d.Level != "district" && d.Level != "city" && d.Level != "province" {
				return nil, nil, fmt.Errorf("unsupported leaf level %q for %d", d.Level, d.ADCode)
			}
			leaves = append(leaves, d)
		}
	}
	sort.Slice(leaves, func(i, j int) bool { return leaves[i].ADCode < leaves[j].ADCode })
	return byCode, leaves, nil
}

func geometryBounds(g *maparea.Geometry) (maparea.Bounds, error) {
	if g == nil {
		return maparea.Bounds{}, fmt.Errorf("missing geometry")
	}
	var polygons [][][][]float64
	switch g.Type {
	case "Polygon":
		var polygon [][][]float64
		if err := json.Unmarshal(g.Coordinates, &polygon); err != nil {
			return maparea.Bounds{}, err
		}
		polygons = [][][][]float64{polygon}
	case "MultiPolygon":
		if err := json.Unmarshal(g.Coordinates, &polygons); err != nil {
			return maparea.Bounds{}, err
		}
	default:
		return maparea.Bounds{}, fmt.Errorf("unsupported geometry %q", g.Type)
	}
	bounds := maparea.Bounds{MinLatitude: 90, MinLongitude: 180, MaxLatitude: -90, MaxLongitude: -180}
	for _, polygon := range polygons {
		for _, ring := range polygon {
			for _, p := range ring {
				if len(p) < 2 || math.IsNaN(p[0]) || math.IsNaN(p[1]) || math.IsInf(p[0], 0) || math.IsInf(p[1], 0) {
					return bounds, fmt.Errorf("invalid coordinate")
				}
				bounds.MinLongitude = math.Min(bounds.MinLongitude, p[0])
				bounds.MaxLongitude = math.Max(bounds.MaxLongitude, p[0])
				bounds.MinLatitude = math.Min(bounds.MinLatitude, p[1])
				bounds.MaxLatitude = math.Max(bounds.MaxLatitude, p[1])
			}
		}
	}
	return bounds, nil // The production parser checks ranges and ring closure too.
}
