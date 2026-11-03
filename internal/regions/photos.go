package regions

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// normalizeRegionPhotos preserves old single-cover records and keeps the
// compatibility cover URL equal to the first photo in display order.
func normalizeRegionPhotos(nodes []Node) bool {
	changed := false
	for i := range nodes {
		node := &nodes[i]
		if node.Photos == nil && strings.TrimSpace(node.CoverURL) != "" {
			url := strings.TrimSpace(node.CoverURL)
			sum := sha256.Sum256([]byte(url))
			node.Photos = []Photo{{ID: "legacy-" + hex.EncodeToString(sum[:8]), URL: url, Provider: "legacy"}}
			changed = true
		}
		cover := ""
		if len(node.Photos) > 0 {
			cover = node.Photos[0].URL
		}
		if node.CoverURL != cover {
			node.CoverURL = cover
			changed = true
		}
	}
	return changed
}
