package regions

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/tongyichu/track_server/internal/config"
)

//go:embed seed.json
var initialNodes []byte

var ErrVersionConflict = errors.New("region catalog version conflict")
var ErrNodeConflict = errors.New("region node changed since it was loaded")

type Node struct {
	ID               string       `json:"id"`
	RegionID         string       `json:"region_id,omitempty"`
	ParentID         string       `json:"parent_id"`
	Name             string       `json:"name"`
	NameEN           string       `json:"name_en,omitempty"`
	Level            string       `json:"level"`
	Code             string       `json:"code,omitempty"`
	CodeSystem       string       `json:"code_system,omitempty"`
	ExternalIDs      []ExternalID `json:"external_ids,omitempty"`
	CountryCode      string       `json:"country_code,omitempty"`
	Continent        string       `json:"continent,omitempty"`
	PostalCodes      []string     `json:"postal_codes,omitempty"`
	Slogan           string       `json:"slogan,omitempty"`
	Description      string       `json:"description,omitempty"`
	Latitude         *float64     `json:"latitude,omitempty"`
	Longitude        *float64     `json:"longitude,omitempty"`
	CoordinateStatus string       `json:"coordinate_status,omitempty"`
	Selectable       bool         `json:"selectable"`
	Active           bool         `json:"active"`
	SortOrder        int          `json:"sort_order"`
}

type ExternalID struct {
	Provider string `json:"provider"`
	Type     string `json:"type"`
	Value    string `json:"value"`
}

type Catalog struct {
	Version int64  `json:"version"`
	Nodes   []Node `json:"nodes"`
}

type Store interface {
	Load(context.Context) (int64, []byte, error)
	Save(context.Context, int64, []byte) (bool, error)
}

type Service struct{ store Store }

func NewService(ctx context.Context, store Store) (*Service, error) {
	s := &Service{store: store}
	if _, err := s.Read(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) Read(ctx context.Context) (*Catalog, error) {
	for attempt := 0; attempt < 4; attempt++ {
		version, payload, err := s.store.Load(ctx)
		if err != nil {
			return nil, err
		}
		if version == 0 {
			var seed []Node
			if err := json.Unmarshal(initialNodes, &seed); err != nil {
				return nil, err
			}
			seed, _, err = assignRegionIDs(seed)
			if err != nil {
				return nil, err
			}
			if err := validate(seed, nil); err != nil {
				return nil, fmt.Errorf("invalid region seed: %w", err)
			}
			seedPayload, err := json.Marshal(seed)
			if err != nil {
				return nil, err
			}
			ok, err := s.store.Save(ctx, 0, seedPayload)
			if err != nil {
				return nil, err
			}
			if ok {
				version, payload = 1, seedPayload
			} else {
				version, payload, err = s.store.Load(ctx)
				if err != nil {
					return nil, err
				}
			}
		}
		var nodes []Node
		if err := json.Unmarshal(payload, &nodes); err != nil {
			return nil, err
		}
		nodes, changed, err := assignRegionIDs(nodes)
		if err != nil {
			return nil, err
		}
		if hasLegacyRegionImageFields(payload) {
			changed = true
		}
		if changed {
			updated, err := json.Marshal(nodes)
			if err != nil {
				return nil, err
			}
			ok, err := s.store.Save(ctx, version, updated)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			version++
		}
		if err := validate(nodes, nil); err != nil {
			return nil, fmt.Errorf("invalid persisted region catalog: %w", err)
		}
		config.SetCityCatalog(projectCityOptions(nodes), projectCityNames(nodes), projectCityAliases(nodes), projectExternalCityIDs(nodes))
		return &Catalog{Version: version, Nodes: nodes}, nil
	}
	return nil, ErrVersionConflict
}

func (s *Service) Save(ctx context.Context, expectedVersion int64, nodes []Node) (*Catalog, error) {
	current, err := s.Read(ctx)
	if err != nil {
		return nil, err
	}
	if current.Version != expectedVersion {
		return nil, ErrVersionConflict
	}
	nodes, _, err = assignRegionIDs(nodes)
	if err != nil {
		return nil, err
	}
	if err := validate(nodes, current.Nodes); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(nodes)
	if err != nil {
		return nil, err
	}
	ok, err := s.store.Save(ctx, expectedVersion, payload)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrVersionConflict
	}
	config.SetCityCatalog(projectCityOptions(nodes), projectCityNames(nodes), projectCityAliases(nodes), projectExternalCityIDs(nodes))
	return &Catalog{Version: expectedVersion + 1, Nodes: nodes}, nil
}

// SaveNode applies only one editor's node, keeping unrelated regions from the
// latest catalog. The baseline protects against concurrent edits to that node.
func (s *Service) SaveNode(ctx context.Context, base *Node, node Node) (*Catalog, error) {
	if node.ID == "" || base != nil && base.ID != node.ID {
		return nil, errors.New("invalid region node")
	}
	for attempt := 0; attempt < 4; attempt++ {
		current, err := s.Read(ctx)
		if err != nil {
			return nil, err
		}
		index := -1
		for i := range current.Nodes {
			if current.Nodes[i].ID == node.ID {
				index = i
				break
			}
		}
		if base == nil && index >= 0 || base != nil && index < 0 {
			return nil, ErrNodeConflict
		}
		if base != nil {
			stored, _ := json.Marshal(current.Nodes[index])
			baseline, _ := json.Marshal(base)
			if string(stored) != string(baseline) {
				return nil, ErrNodeConflict
			}
		}
		nodes := append([]Node(nil), current.Nodes...)
		if index >= 0 {
			nodes[index] = node
		} else {
			nodes = append(nodes, node)
		}
		saved, err := s.Save(ctx, current.Version, nodes)
		if errors.Is(err, ErrVersionConflict) {
			continue
		}
		return saved, err
	}
	return nil, ErrVersionConflict
}

func projectCityOptions(nodes []Node) []config.CityOption {
	byID := make(map[string]Node, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}
	options := make([]config.CityOption, 0, len(nodes))
	for _, n := range nodes {
		if !n.Active || (!n.Selectable && !(n.CountryCode == "CN" && n.Level == "province")) {
			continue
		}
		code := n.RegionID
		parentCode := byID[n.ParentID].RegionID
		if n.Level == "province" {
			parentCode = ""
		}
		options = append(options, config.CityOption{
			Code: code, Name: n.Name, Level: n.Level, ProvinceCode: parentCode, Selectable: n.Selectable,
		})
	}
	sort.Slice(options, func(i, j int) bool {
		if options[i].Name == options[j].Name {
			return options[i].Code < options[j].Code
		}
		return options[i].Name < options[j].Name
	})
	return options
}

func projectCityNames(nodes []Node) map[string]string {
	names := make(map[string]string, len(nodes)*3)
	for _, n := range nodes {
		if n.RegionID != "" {
			names[n.RegionID] = n.Name
		}
		names[n.ID] = n.Name
		if n.CodeSystem == "cn_adcode" && n.Code != "" {
			names[n.Code] = n.Name
		}
		for _, external := range n.ExternalIDs {
			if external.Provider == "amap" && external.Type == "adcode" {
				names[external.Value] = n.Name
			}
		}
	}
	return names
}

func projectCityAliases(nodes []Node) map[string]string {
	aliases := make(map[string]string, len(nodes)*2)
	for _, n := range nodes {
		if n.RegionID == "" {
			continue
		}
		aliases[n.RegionID] = n.RegionID
		aliases[n.ID] = n.RegionID
		if n.CodeSystem == "cn_adcode" && n.Code != "" {
			aliases[n.Code] = n.RegionID
		}
		for _, external := range n.ExternalIDs {
			if external.Provider == "amap" && external.Type == "adcode" {
				aliases[external.Value] = n.RegionID
			}
		}
	}
	return aliases
}

func projectExternalCityIDs(nodes []Node) []config.ExternalCityID {
	entries := make([]config.ExternalCityID, 0, len(nodes))
	for _, n := range nodes {
		if n.RegionID == "" {
			continue
		}
		if n.CodeSystem == "cn_adcode" && n.Code != "" {
			entries = append(entries, config.ExternalCityID{Provider: "amap", Type: "adcode", Value: n.Code, RegionID: n.RegionID})
		}
		for _, external := range n.ExternalIDs {
			entries = append(entries, config.ExternalCityID{Provider: external.Provider, Type: external.Type, Value: external.Value, RegionID: n.RegionID})
		}
	}
	return entries
}

func validate(nodes, previous []Node) error {
	if len(nodes) == 0 || len(nodes) > 20000 {
		return errors.New("region catalog must contain 1 to 20000 nodes")
	}
	byID := make(map[string]Node, len(nodes))
	regionIDs := make(map[string]string, len(nodes))
	codes := make(map[string]string, len(nodes))
	externalOwners := make(map[string]string, len(nodes))
	for _, n := range nodes {
		if n.ID == "" || len(n.ID) > 128 || strings.TrimSpace(n.Name) == "" || utf8.RuneCountInString(n.Name) > 64 {
			return errors.New("region id and name are required; name must fit 64 characters")
		}
		if n.ID == n.ParentID {
			return fmt.Errorf("region %s cannot be its own parent", n.ID)
		}
		if _, exists := byID[n.ID]; exists {
			return fmt.Errorf("duplicate region id %s", n.ID)
		}
		byID[n.ID] = n
		if n.Active && n.RegionID == "" {
			return fmt.Errorf("active region %s requires a country code and internal id", n.ID)
		}
		if n.RegionID != "" {
			if owner, exists := regionIDs[n.RegionID]; exists {
				return fmt.Errorf("duplicate internal region id %s (%s and %s)", n.RegionID, owner, n.ID)
			}
			regionIDs[n.RegionID] = n.ID
		}
		if n.Code != "" {
			if n.CodeSystem == "" {
				return fmt.Errorf("region %s requires code_system", n.ID)
			}
			key := n.CodeSystem + ":" + n.Code
			if owner, exists := codes[key]; exists {
				return fmt.Errorf("duplicate region code %s (%s and %s)", key, owner, n.ID)
			}
			codes[key] = n.ID
		}
		if n.CodeSystem == "cn_adcode" && n.Code != "" {
			key := "amap\x00adcode\x00" + n.Code
			if owner := externalOwners[key]; owner != "" && owner != n.ID {
				return fmt.Errorf("duplicate external region id %s (%s and %s)", n.Code, owner, n.ID)
			}
			externalOwners[key] = n.ID
		}
		if len(n.ExternalIDs) > 16 {
			return fmt.Errorf("too many external ids for %s", n.ID)
		}
		for _, external := range n.ExternalIDs {
			if external.Provider != strings.ToLower(strings.TrimSpace(external.Provider)) || external.Provider == "" || len(external.Provider) > 32 ||
				external.Type != strings.ToLower(strings.TrimSpace(external.Type)) || external.Type == "" || len(external.Type) > 32 ||
				external.Value != strings.TrimSpace(external.Value) || external.Value == "" || len(external.Value) > 2048 ||
				strings.ContainsAny(external.Provider+external.Type+external.Value, "\x00\r\n") {
				return fmt.Errorf("invalid external region id for %s", n.ID)
			}
			if external.Provider == "amap" && external.Type == "adcode" && (len(external.Value) != 6 || strings.Trim(external.Value, "0123456789") != "") {
				return fmt.Errorf("invalid AMap adcode for %s", n.ID)
			}
			key := external.Provider + "\x00" + external.Type + "\x00" + external.Value
			if owner := externalOwners[key]; owner != "" && owner != n.ID {
				return fmt.Errorf("duplicate external region id %s (%s and %s)", external.Value, owner, n.ID)
			}
			externalOwners[key] = n.ID
		}
		if n.Level == "country" && n.ParentID != "" {
			return fmt.Errorf("country %s must be a root", n.ID)
		}
		if n.Latitude != nil && (*n.Latitude < -90 || *n.Latitude > 90) ||
			n.Longitude != nil && (*n.Longitude < -180 || *n.Longitude > 180) {
			return fmt.Errorf("invalid coordinates for %s", n.ID)
		}
	}
	for _, n := range nodes {
		seen := map[string]bool{n.ID: true}
		parent := n.ParentID
		for parent != "" {
			p, ok := byID[parent]
			if !ok {
				return fmt.Errorf("missing parent %s for %s", parent, n.ID)
			}
			if n.Active && !p.Active {
				return fmt.Errorf("active region %s cannot have inactive ancestor %s", n.ID, parent)
			}
			if seen[parent] {
				return fmt.Errorf("region cycle at %s", n.ID)
			}
			seen[parent] = true
			parent = p.ParentID
		}
	}
	for _, old := range previous {
		next, ok := byID[old.ID]
		if !ok {
			return fmt.Errorf("saved region must be archived instead of deleted: %s", old.ID)
		}
		if next.RegionID != old.RegionID {
			return fmt.Errorf("internal region id cannot change: %s", old.ID)
		}
		if !strings.HasPrefix(old.ID, "cn:") {
			continue
		}
		if next.Code != old.Code || next.CodeSystem != old.CodeSystem || next.Level != old.Level || next.ParentID != old.ParentID {
			return fmt.Errorf("existing China region identity cannot change: %s", old.ID)
		}
	}
	return nil
}
