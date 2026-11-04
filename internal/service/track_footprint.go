package service

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/regions"
	"github.com/tongyichu/track_server/internal/repository"
)

func (s *TrackService) SetFootprintRegionService(v *regions.Service) { s.footprintGeo = v }

func (s *TrackService) footprintTracks(ctx context.Context, userID int64, trackType string) ([]models.FootprintTrack, error) {
	if userID <= 0 {
		return nil, invalidArg("userID is required")
	}
	selected := normalizeTrackTypeCode(strings.TrimSpace(trackType))
	if len(selected) > 32 || selected == "all" {
		return nil, invalidArg("invalid track_type")
	}
	repo, ok := s.tracks.(repository.ActivityFootprintRepository)
	if !ok {
		return nil, errors.New("activity footprint unavailable")
	}
	tracks, err := repo.ReadFootprintTracks(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]models.FootprintTrack, 0, len(tracks))
	for _, t := range tracks {
		code := normalizeTrackTypeCode(t.TrackType)
		if code == "" {
			code = "other"
		}
		if selected == "" || code == selected {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *TrackService) ActivityFootprint(ctx context.Context, userID int64, trackType string) (*models.ActivityFootprint, error) {
	tracks, err := s.footprintTracks(ctx, userID, trackType)
	if err != nil {
		return nil, err
	}
	out, known, err := s.footprintRegionsForMap(ctx)
	if err != nil {
		return nil, err
	}
	cities := make(map[string]*models.FootprintRegion)
	provinces := make(map[string]*models.FootprintRegion)
	for i := range out.Cities {
		cities[out.Cities[i].ID] = &out.Cities[i]
	}
	for i := range out.Provinces {
		provinces[out.Provinces[i].ID] = &out.Provinces[i]
	}
	cityByTrack := make(map[string]string)
	for _, t := range tracks {
		code := known[strings.TrimSpace(t.CityCode)]
		city := cities[code]
		if city == nil {
			if code != "" {
				out.OutsideChinaTrackCount++
			} else {
				out.UnlocatedTrackCount++
			}
			continue
		}
		city.TrackCount++
		if t.Distance > 0 && !math.IsNaN(t.Distance) && !math.IsInf(t.Distance, 0) {
			city.DistanceMeters += t.Distance
		}
		cityByTrack[t.ID] = city.ID
	}
	citySpots, err := s.footprintScenicSpots(ctx, cityByTrack)
	if err != nil {
		return nil, err
	}
	allSpots := map[string]bool{}
	provinceSpots := make(map[string]map[string]bool)
	for _, c := range cities {
		c.ScenicSpotCount = len(citySpots[c.ID])
		p := provinces[c.ProvinceID]
		if p == nil {
			continue
		}
		p.TrackCount += c.TrackCount
		p.DistanceMeters += c.DistanceMeters
		if c.TrackCount > 0 {
			p.CityCount++
		}
		if provinceSpots[p.ID] == nil {
			provinceSpots[p.ID] = map[string]bool{}
		}
		for spot := range citySpots[c.ID] {
			provinceSpots[p.ID][spot] = true
			allSpots[spot] = true
		}
	}
	for id, p := range provinces {
		p.ScenicSpotCount = len(provinceSpots[id])
	}
	out.ScenicSpotCount = len(allSpots)
	return out, nil
}

func (s *TrackService) footprintRegionsForMap(ctx context.Context) (*models.ActivityFootprint, map[string]string, error) {
	if s.footprintGeo == nil {
		return nil, nil, errors.New("region catalog unavailable")
	}
	catalog, err := s.footprintGeo.Read(ctx)
	if err != nil {
		return nil, nil, err
	}
	byID := make(map[string]regions.Node, len(catalog.Nodes))
	for _, n := range catalog.Nodes {
		byID[n.ID] = n
	}
	known := map[string]string{}
	out := &models.ActivityFootprint{Provinces: []models.FootprintRegion{}, Cities: []models.FootprintRegion{}}
	for _, n := range catalog.Nodes {
		if !n.Active || n.RegionID == "" {
			continue
		}
		root := n
		province := n
		valid := true
		seen := map[string]bool{n.ID: true}
		for root.ParentID != "" {
			parent, ok := byID[root.ParentID]
			if !ok || !parent.Active || seen[parent.ID] {
				valid = false
				break
			}
			seen[parent.ID] = true
			if parent.Level == "province" {
				province = parent
			}
			root = parent
		}
		if !valid {
			continue
		}
		item := models.FootprintRegion{ID: n.RegionID, Name: n.Name, NameEN: n.NameEN, Aliases: []string{n.RegionID, n.ID}, Latitude: n.Latitude, Longitude: n.Longitude}
		if n.CodeSystem == "cn_adcode" && n.Code != "" {
			item.Aliases = append(item.Aliases, n.Code)
			item.MapCode = n.Code
		}
		for _, e := range n.ExternalIDs {
			if e.Provider == "amap" && e.Type == "adcode" {
				item.Aliases = append(item.Aliases, e.Value)
				if item.MapCode == "" {
					item.MapCode = e.Value
				}
			}
		}
		if n.Selectable {
			for _, alias := range item.Aliases {
				known[alias] = n.RegionID
			}
		}
		if n.CountryCode != "CN" && root.CountryCode != "CN" {
			continue
		}
		if n.Level == "province" {
			out.Provinces = append(out.Provinces, item)
		}
		if n.Selectable && province.Level == "province" {
			item.ProvinceID = province.RegionID
			out.Cities = append(out.Cities, item)
		}
	}
	sort.Slice(out.Provinces, func(i, j int) bool { return out.Provinces[i].ID < out.Provinces[j].ID })
	sort.Slice(out.Cities, func(i, j int) bool { return out.Cities[i].ID < out.Cities[j].ID })
	return out, known, nil
}

func (s *TrackService) footprintScenicSpots(ctx context.Context, cityByTrack map[string]string) (map[string]map[string]bool, error) {
	out := map[string]map[string]bool{}
	if s.scenicCatalog == nil || s.scenicCatalog.spots == nil {
		return out, nil
	}
	ids := make([]string, 0, len(cityByTrack))
	for id := range cityByTrack {
		ids = append(ids, id)
	}
	for start := 0; start < len(ids); start += 200 {
		links, err := s.scenicCatalog.spots.GetTrackScenicSpotLinks(ctx, ids[start:min(start+200, len(ids))])
		if err != nil {
			return nil, err
		}
		spotIDs := []string{}
		for _, link := range links {
			if link != nil {
				spotIDs = append(spotIDs, link.SpotID)
			}
		}
		spots, err := s.scenicCatalog.spots.GetScenicSpots(ctx, spotIDs)
		if err != nil {
			return nil, err
		}
		for id, link := range links {
			if link == nil || link.TrackID != id {
				continue
			}
			spot := spots[link.SpotID]
			if spot == nil || !scenicSpotClientVisible(spot) {
				continue
			}
			city := cityByTrack[id]
			if city == "" {
				continue
			}
			if out[city] == nil {
				out[city] = map[string]bool{}
			}
			out[city][spot.ID] = true
		}
	}
	return out, nil
}
