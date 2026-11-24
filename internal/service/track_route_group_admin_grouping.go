package service

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

// AdminRouteGroupGrouping is the display path of a route group in the admin list.
type AdminRouteGroupGrouping struct {
	CityCode     string `json:"city_code,omitempty"`
	CityName     string `json:"city_name,omitempty"`
	DistrictID   string `json:"district_id,omitempty"`
	DistrictName string `json:"district_name,omitempty"`
	ScenicID     string `json:"scenic_id,omitempty"`
	ScenicName   string `json:"scenic_name,omitempty"`
	ScenicSpotID string `json:"scenic_spot_id,omitempty"`
}

type AdminRouteGroupDirectoryItem struct {
	GroupID     string                  `json:"group_id"`
	Name        string                  `json:"name"`
	TrackType   string                  `json:"track_type"`
	MemberCount int64                   `json:"member_count"`
	Grouping    AdminRouteGroupGrouping `json:"grouping"`
	MapPoint    *models.TrackMapPoint   `json:"map_point,omitempty"`
}

func (s *TrackRouteGroupService) ListRouteGroupDirectory(ctx context.Context, filter models.TrackMapQueryFilter) ([]AdminRouteGroupDirectoryItem, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("track route group service is not configured")
	}
	groups, err := s.repo.ListRouteGroupDirectory(ctx, filter)
	if err != nil {
		return nil, err
	}
	const batchSize = 200
	items := make([]AdminRouteGroupDirectoryItem, 0, len(groups))
	for start := 0; start < len(groups); start += batchSize {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		batch, err := s.buildAdminRouteGroupSummaries(ctx, groups[start:min(start+batchSize, len(groups))], false)
		if err != nil {
			return nil, err
		}
		for _, row := range batch {
			point := row.mapPoint
			center := models.TrackMapPoint{Latitude: row.CenterLat, Longitude: row.CenterLng}
			if point == nil && adminRouteGroupHasStart(center) {
				point = &center
			}
			items = append(items, AdminRouteGroupDirectoryItem{
				GroupID: row.GroupID, Name: row.Name, TrackType: row.TrackType,
				MemberCount: row.MemberCount, Grouping: row.Grouping, MapPoint: point,
			})
		}
	}
	return items, nil
}

type AdminScenicMembersScope struct {
	ScenicID   string
	CityCode   string
	DistrictID string
}

func (s *TrackRouteGroupService) ListAdminScenicMembers(ctx context.Context, filter models.TrackMapQueryFilter, scope AdminScenicMembersScope) ([]*models.TrackRouteGroupMember, int64, int, error) {
	if scope.ScenicID == "" || len(scope.ScenicID) > 128 {
		return nil, 0, 0, invalidArg("invalid scenic_id")
	}
	directory, err := s.ListRouteGroupDirectory(ctx, filter)
	if err != nil {
		return nil, 0, 0, err
	}
	var groupIDs []string
	for _, row := range directory {
		g := row.Grouping
		if g.ScenicID == scope.ScenicID && g.CityCode == scope.CityCode && g.DistrictID == scope.DistrictID {
			groupIDs = append(groupIDs, row.GroupID)
		}
	}
	if len(groupIDs) == 0 {
		return nil, 0, 0, repository.ErrNotFound
	}
	return s.visibleAdminRouteGroupMembers(ctx, groupIDs, max(1, filter.Limit), filter.Offset)
}

func (s *TrackRouteGroupService) buildAdminRouteGroupSummaries(ctx context.Context, groups []*models.TrackRouteGroup, includeArea bool) ([]*AdminRouteGroupSummary, error) {
	items := make([]*AdminRouteGroupSummary, 0, len(groups))
	groupIDs := make([]string, 0, len(groups))
	var placeIDs []string
	for _, group := range groups {
		if group == nil {
			continue
		}
		groupIDs = append(groupIDs, group.GroupID)
		item := &AdminRouteGroupSummary{TrackRouteGroup: group}
		if includeArea {
			item.Area = s.adminCatalogArea(group.AreaID)
			if item.Area == nil && s.places.Enabled() && strings.HasPrefix(strings.TrimSpace(group.AreaID), "place-") {
				placeIDs = append(placeIDs, strings.TrimSpace(group.AreaID))
			}
		}
		items = append(items, item)
	}
	departures, err := s.adminRouteGroupDepartures(ctx, groupIDs)
	if err != nil {
		return nil, err
	}
	for _, d := range departures {
		if d.scenic != nil && !d.scenic.catalog {
			placeIDs = append(placeIDs, d.scenic.id)
		}
	}
	slices.Sort(placeIDs)
	var places map[string]*models.TrackPlace
	if s.places.Enabled() && len(placeIDs) > 0 {
		places, err = s.places.GetPlaces(ctx, slices.Compact(placeIDs))
		if err != nil {
			return nil, err
		}
	}
	type scenicLocation struct {
		grouping AdminRouteGroupGrouping
		point    *models.TrackMapPoint
	}
	scenicGroups := make(map[string]scenicLocation)
	for _, item := range items {
		if includeArea && item.Area == nil {
			item.Area = adminRouteGroupPlaceArea(places[strings.TrimSpace(item.AreaID)])
		}
		d := departures[item.GroupID]
		if adminRouteGroupHasStart(d.Start) {
			item.mapPoint = &d.Start
		}
		if d.scenic == nil {
			cityCode := d.CityCode
			if cityCode == "" && len(item.CityCodes) == 1 {
				cityCode = item.CityCodes[0]
			}
			item.Grouping = s.adminRouteGroupLocation(cityCode, item.mapPoint)
			continue
		}
		spot := d.scenic
		location, ok := scenicGroups[spot.id]
		if !ok {
			cityCode, point := spot.cityCode, spot.point
			if place := places[spot.id]; !spot.catalog && place != nil {
				cityCode = place.CityCode
				point = scenicMapPoint(place.Latitude, place.Longitude, place.CoordinateSystem)
			}
			grouping := s.adminRouteGroupLocation(cityCode, point)
			grouping.ScenicID, grouping.ScenicName = spot.id, spot.name
			if spot.catalog {
				grouping.ScenicSpotID = spot.id
			}
			location = scenicLocation{grouping: grouping, point: point}
			scenicGroups[spot.id] = location
		}
		item.Grouping = location.grouping
		if location.point != nil {
			item.mapPoint = location.point
		}
	}
	return items, nil
}

func (s *TrackRouteGroupService) adminRouteGroupDepartures(ctx context.Context, groupIDs []string) (map[string]mapDeparture, error) {
	if s.trackMap == nil || len(groupIDs) == 0 {
		return nil, nil
	}
	var rows []models.TrackMapDeparture
	var trackIDs []string
	err := s.repo.VisitRouteDepartures(ctx, models.TrackMapQueryFilter{DepartureGroupIDs: groupIDs}, func(row models.TrackMapDeparture) {
		rows = append(rows, row)
		trackIDs = append(trackIDs, row.TrackID)
	})
	if err != nil {
		return nil, err
	}
	matches, err := s.trackMap.trackScenicMatches(ctx, trackIDs)
	if err != nil {
		return nil, err
	}
	departures := make(map[string]mapDeparture, len(groupIDs))
	for _, row := range rows {
		d := mapDeparture{TrackMapDeparture: row, scenic: matches[row.TrackID]}
		if d.scenic == nil && !adminRouteGroupHasStart(d.Start) {
			continue
		}
		if previous, exists := departures[row.GroupID]; !exists || preferScenicDeparture(d, previous) {
			departures[row.GroupID] = d
		}
	}
	return departures, nil
}

func adminRouteGroupHasStart(p models.TrackMapPoint) bool {
	return (p.Latitude != 0 || p.Longitude != 0) && validMapLatLng(p.Latitude, p.Longitude)
}

func (s *TrackRouteGroupService) adminRouteGroupLocation(cityCode string, point *models.TrackMapPoint) AdminRouteGroupGrouping {
	if point == nil {
		code := config.CanonicalCityCode(cityCode)
		return AdminRouteGroupGrouping{CityCode: code, CityName: config.CityNameByCode(code)}
	}
	d := s.trackMap.locateDeparture(models.TrackMapDeparture{CityCode: cityCode, Start: *point})
	return AdminRouteGroupGrouping{CityCode: d.cityCode, CityName: d.cityName, DistrictID: d.districtID, DistrictName: d.districtName}
}
