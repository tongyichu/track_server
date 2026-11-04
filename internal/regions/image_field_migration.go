package regions

import "encoding/json"

// hasLegacyRegionImageFields detects old JSON keys so Read can persist a
// catalog without them and advance the version seen by cached clients.
func hasLegacyRegionImageFields(payload []byte) bool {
	var nodes []map[string]json.RawMessage
	if json.Unmarshal(payload, &nodes) != nil {
		return false
	}
	for _, node := range nodes {
		if _, ok := node["cover_url"]; ok {
			return true
		}
		if _, ok := node["photos"]; ok {
			return true
		}
	}
	return false
}
