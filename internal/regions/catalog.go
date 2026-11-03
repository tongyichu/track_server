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

type Node struct {
	ID               string   `json:"id"`
	ParentID         string   `json:"parent_id"`
	Name             string   `json:"name"`
	NameEN           string   `json:"name_en,omitempty"`
	Level            string   `json:"level"`
	Code             string   `json:"code,omitempty"`
	CodeSystem       string   `json:"code_system,omitempty"`
	CountryCode      string   `json:"country_code,omitempty"`
	Continent        string   `json:"continent,omitempty"`
	PostalCodes      []string `json:"postal_codes,omitempty"`
	Slogan           string   `json:"slogan,omitempty"`
	Description      string   `json:"description,omitempty"`
	CoverURL         string   `json:"cover_url,omitempty"`
	Latitude         *float64 `json:"latitude,omitempty"`
	Longitude        *float64 `json:"longitude,omitempty"`
	CoordinateStatus string   `json:"coordinate_status,omitempty"`
	Selectable       bool     `json:"selectable"`
	Active           bool     `json:"active"`
	SortOrder        int      `json:"sort_order"`
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
	version, payload, err := s.store.Load(ctx)
	if err != nil {
		return nil, err
	}
	if version == 0 {
		var seed []Node
		if err := json.Unmarshal(initialNodes, &seed); err != nil {
			return nil, err
		}
		if err := validate(seed, nil); err != nil {
			return nil, fmt.Errorf("invalid region seed: %w", err)
		}
		ok, err := s.store.Save(ctx, 0, initialNodes)
		if err != nil {
			return nil, err
		}
		if ok {
			version, payload = 1, initialNodes
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
	if err := validate(nodes, nil); err != nil {
		return nil, fmt.Errorf("invalid persisted region catalog: %w", err)
	}
	config.SetCityCatalog(projectCityOptions(nodes), projectCityNames(nodes))
	return &Catalog{Version: version, Nodes: nodes}, nil
}

func (s *Service) Save(ctx context.Context, expectedVersion int64, nodes []Node) (*Catalog, error) {
	current, err := s.Read(ctx)
	if err != nil {
		return nil, err
	}
	if current.Version != expectedVersion {
		return nil, ErrVersionConflict
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
	config.SetCityCatalog(projectCityOptions(nodes), projectCityNames(nodes))
	return &Catalog{Version: expectedVersion + 1, Nodes: nodes}, nil
}

func projectCityOptions(nodes []Node) []config.CityOption {
	options := make([]config.CityOption, 0, len(nodes))
	for _, n := range nodes {
		if !n.Active || (!n.Selectable && !(n.CodeSystem == "cn_adcode" && n.Level == "province")) {
			continue
		}
		code := n.ID
		parentCode := n.ParentID
		if n.CodeSystem == "cn_adcode" {
			code = n.Code
			parentCode = strings.TrimPrefix(n.ParentID, "cn:")
		}
		if n.Level == "province" {
			parentCode = ""
		}
		options = append(options, config.CityOption{
			Code: code, Name: n.Name, Level: n.Level, ProvinceCode: parentCode,
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
	names := make(map[string]string, len(nodes))
	for _, n := range nodes {
		if n.CodeSystem == "cn_adcode" {
			names[n.Code] = n.Name
		} else {
			names[n.ID] = n.Name
		}
	}
	return names
}

func validate(nodes, previous []Node) error {
	if len(nodes) == 0 || len(nodes) > 20000 {
		return errors.New("region catalog must contain 1 to 20000 nodes")
	}
	byID := make(map[string]Node, len(nodes))
	codes := make(map[string]string, len(nodes))
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
		if n.Selectable && n.CodeSystem != "cn_adcode" && (len(n.ID) > 6 || len(n.ParentID) > 6) {
			return fmt.Errorf("selectable overseas region and parent ids must fit the 6-character location fields: %s", n.ID)
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
		if _, ok := byID[old.ID]; !ok {
			return fmt.Errorf("saved region must be archived instead of deleted: %s", old.ID)
		}
		if !strings.HasPrefix(old.ID, "cn:") {
			continue
		}
		next, ok := byID[old.ID]
		if !ok || next.Code != old.Code || next.CodeSystem != old.CodeSystem || next.Level != old.Level || next.ParentID != old.ParentID {
			return fmt.Errorf("existing China region identity cannot change: %s", old.ID)
		}
	}
	return nil
}
