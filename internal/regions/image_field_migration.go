package regions

import "encoding/json"

// Photos are retained; only the obsolete standalone cover field is removed.
func hasLegacyRegionImageFields(payload []byte) bool {
	var nodes []map[string]json.RawMessage
	if json.Unmarshal(payload, &nodes) != nil {
		return false
	}
	for _, node := range nodes {
		if _, ok := node["cover_url"]; ok {
			return true
		}
	}
	return false
}
