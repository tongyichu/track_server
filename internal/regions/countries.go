package regions

import (
	"context"
	"errors"
	"strings"
)

// DeleteCountry archives the complete country subtree while retaining stable
// region identities for historical users and tracks. The catalog CAS protects
// the administrator's deletion preview from concurrent changes.
func (s *Service) DeleteCountry(ctx context.Context, id string, expectedVersion int64) (*Catalog, error) {
	catalog, err := s.Read(ctx)
	if err != nil {
		return nil, err
	}
	if catalog.Version != expectedVersion {
		return nil, ErrVersionConflict
	}
	var country *Node
	children := make(map[string][]int, len(catalog.Nodes))
	for i := range catalog.Nodes {
		node := &catalog.Nodes[i]
		children[node.ParentID] = append(children[node.ParentID], i)
		if node.ID == id {
			country = node
		}
	}
	if country == nil {
		return nil, ErrNodeConflict
	}
	if country.Level != "country" || country.ParentID != "" {
		return nil, errors.New("仅支持删除整个国家")
	}
	queue := []string{id}
	for next := 0; next < len(queue); next++ {
		for _, index := range children[queue[next]] {
			queue = append(queue, catalog.Nodes[index].ID)
		}
	}
	removed := make(map[string]bool, len(queue))
	for _, nodeID := range queue {
		removed[nodeID] = true
	}
	for i := range catalog.Nodes {
		node := &catalog.Nodes[i]
		if !removed[node.ID] {
			continue
		}
		if node.ID == "country:CN" || strings.HasPrefix(node.ID, "cn:") || strings.EqualFold(node.CountryCode, "CN") || node.RegionID == "CN" {
			return nil, errors.New("中国及其下级地区不能删除")
		}
		node.Active = false
	}
	return s.Save(ctx, expectedVersion, catalog.Nodes)
}
