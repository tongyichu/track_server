package regions

import (
	"fmt"
	"strconv"
	"strings"
)

// assignRegionIDs gives every usable catalog node a stable, country-scoped ID.
// Existing IDs are never recycled or renumbered; the catalog's stored order only
// determines the initial assignment when upgrading a legacy catalog.
func assignRegionIDs(nodes []Node) ([]Node, bool, error) {
	byID := make(map[string]Node, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
	}
	prefixes := make(map[string]string, len(nodes))
	var prefixFor func(string, map[string]bool) (string, error)
	prefixFor = func(id string, seen map[string]bool) (string, error) {
		if value, ok := prefixes[id]; ok {
			return value, nil
		}
		if seen[id] {
			return "", fmt.Errorf("region cycle at %s", id)
		}
		node, ok := byID[id]
		if !ok {
			return "", fmt.Errorf("missing region %s", id)
		}
		seen[id] = true
		var prefix string
		if node.ParentID == "" {
			prefix = strings.ToUpper(strings.TrimSpace(node.CountryCode))
			if prefix == "" && node.CodeSystem == "iso3166-1" {
				prefix = strings.ToUpper(strings.TrimSpace(node.Code))
			}
			if prefix == "" && strings.HasPrefix(node.ID, "country:") {
				prefix = strings.ToUpper(strings.TrimPrefix(node.ID, "country:"))
			}
			if len(prefix) != 2 || prefix[0] < 'A' || prefix[0] > 'Z' || prefix[1] < 'A' || prefix[1] > 'Z' {
				prefix = ""
			}
		} else {
			var err error
			prefix, err = prefixFor(node.ParentID, seen)
			if err != nil {
				return "", err
			}
		}
		delete(seen, id)
		prefixes[id] = prefix
		return prefix, nil
	}
	maxSequence := make(map[string]int)
	for _, node := range nodes {
		prefix, err := prefixFor(node.ID, map[string]bool{})
		if err != nil {
			return nil, false, err
		}
		if prefix == "" {
			if node.Active {
				return nil, false, fmt.Errorf("active region %s needs a two-letter country code", node.ID)
			}
			continue
		}
		if node.RegionID == "" {
			continue
		}
		if node.ParentID == "" {
			if node.RegionID != prefix {
				return nil, false, fmt.Errorf("country %s internal id must be %s", node.ID, prefix)
			}
			continue
		}
		if !strings.HasPrefix(node.RegionID, prefix) || len(node.RegionID) != 8 {
			return nil, false, fmt.Errorf("invalid internal region id %s", node.RegionID)
		}
		sequence, err := strconv.Atoi(node.RegionID[2:])
		if err != nil || sequence < 1 || sequence > 999999 {
			return nil, false, fmt.Errorf("invalid internal region id %s", node.RegionID)
		}
		if sequence > maxSequence[prefix] {
			maxSequence[prefix] = sequence
		}
	}
	changed := false
	for i := range nodes {
		if nodes[i].RegionID != "" {
			continue
		}
		prefix := prefixes[nodes[i].ID]
		if prefix == "" {
			continue // Legacy archived node with no country code.
		}
		if nodes[i].ParentID == "" {
			nodes[i].RegionID = prefix
		} else {
			maxSequence[prefix]++
			if maxSequence[prefix] > 999999 {
				return nil, false, fmt.Errorf("region ids exhausted for %s", prefix)
			}
			nodes[i].RegionID = fmt.Sprintf("%s%06d", prefix, maxSequence[prefix])
		}
		changed = true
	}
	return nodes, changed, nil
}
