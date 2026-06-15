package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

const (
	defaultTrackMapRadiusM = 10000
	maxTrackMapRadiusM     = 50000
	defaultTrackMapLimit   = 100
	maxTrackMapLimit       = 200
	trackMapViewRoute      = "route"
	trackMapViewArea       = "area"
	trackMapViewCity       = "city"
)

// TrackMapService serves client-facing map mode APIs.
type TrackMapService struct {
	maps     repository.TrackMapRepository
	tracks   repository.TrackRepository
	trackSvc *TrackService
}

type TrackMapViewInput struct {
	BBox      string
	Zoom      float64
	Latitude  *float64
	Longitude *float64
	RadiusM   int
	CityCode  string
	TrackType string
	Limit     int
}

type TrackMapGroupTracksInput struct {
	Limit int
}

func NewTrackMapService(maps repository.TrackMapRepository, tracks repository.TrackRepository, trackSvc *TrackService) *TrackMapService {
	return &TrackMapService{maps: maps, tracks: tracks, trackSvc: trackSvc}
}

func (s *TrackMapService) View(ctx context.Context, input TrackMapViewInput) (*models.TrackMapViewResponse, error) {
	filter, viewLevel, err := s.buildFilter(input)
	if err != nil {
		return nil, err
	}
	switch viewLevel {
	case trackMapViewCity:
		items, err := s.maps.CountRouteGroupsByCity(ctx, filter)
		if err != nil {
			return nil, err
		}
		s.decorateClusters(items)
		return &models.TrackMapViewResponse{ViewLevel: trackMapViewCity, CoordinateSystem: "GCJ02", Items: items}, nil
	case trackMapViewArea:
		items, err := s.maps.CountRouteGroupsByArea(ctx, filter)
		if err != nil {
			return nil, err
		}
		return &models.TrackMapViewResponse{ViewLevel: trackMapViewArea, CoordinateSystem: "GCJ02", Items: items}, nil
	default:
		items, err := s.ListGroups(ctx, input)
		if err != nil {
			return nil, err
		}
		return &models.TrackMapViewResponse{ViewLevel: trackMapViewRoute, CoordinateSystem: "GCJ02", Items: items.Items}, nil
	}
}

func (s *TrackMapService) ListGroups(ctx context.Context, input TrackMapViewInput) (*models.TrackMapRouteGroupList, error) {
	filter, _, err := s.buildFilter(input)
	if err != nil {
		return nil, err
	}
	groups, err := s.maps.ListRouteGroups(ctx, filter)
	if err != nil {
		return nil, err
	}
	items := make([]*models.TrackMapRouteGroupItem, 0, len(groups))
	for _, group := range groups {
		item, err := s.routeGroupItem(ctx, group)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				continue
			}
			return nil, err
		}
		items = append(items, item)
	}
	return &models.TrackMapRouteGroupList{Items: items}, nil
}

func (s *TrackMapService) GetGroupDetail(ctx context.Context, groupID string) (*models.TrackMapRouteGroupItem, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, invalidArg("group_id is required")
	}
	group, err := s.maps.FindRouteGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	return s.routeGroupItem(ctx, group)
}

func (s *TrackMapService) ListGroupTracks(ctx context.Context, userID int64, groupID string, input TrackMapGroupTracksInput) (*models.TrackSummaryPage, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, invalidArg("group_id is required")
	}
	if _, err := s.maps.FindRouteGroup(ctx, groupID); err != nil {
		return nil, err
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > maxTrackMapLimit {
		limit = maxTrackMapLimit
	}
	members, err := s.maps.ListRouteGroupMembers(ctx, groupID, limit)
	if err != nil {
		return nil, err
	}
	tracks := make([]*models.Track, 0, len(members))
	for _, member := range members {
		if member == nil || member.TrackID == "" {
			continue
		}
		track, err := s.tracks.FindByID(ctx, member.TrackID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				continue
			}
			return nil, err
		}
		if track.Status != models.TrackStatusNormal || track.IsRunning {
			continue
		}
		tracks = append(tracks, track)
	}
	summaries := toSummaries(tracks)
	if s.trackSvc != nil {
		cacheCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if s.trackSvc.screenshotCache != nil {
			for i, track := range tracks {
				summaries[i].TrackScreenshotURL = s.trackSvc.screenshotCache.EnsureCached(cacheCtx, track.UserID, track.ID, track.TrackScreenshotURL)
				if track.TrackNoMapBgScreenshotURL != "" {
					summaries[i].TrackNoMapBgScreenshotURL = s.trackSvc.screenshotCache.EnsureCached(cacheCtx, track.UserID, track.ID+"_no_map_bg", track.TrackNoMapBgScreenshotURL)
				}
			}
		}
		if s.trackSvc.rawTrackCache != nil {
			for i, track := range tracks {
				summaries[i].RawTrackURL = s.trackSvc.rawTrackCache.EnsureCached(cacheCtx, track.UserID, track.ID, track.RawTrackURL)
			}
		}
		cancel()
		if len(summaries) > 0 {
			if err := s.trackSvc.fillTrackSummaryExtras(ctx, userID, summaries); err != nil {
				return nil, err
			}
		}
	}
	return &models.TrackSummaryPage{Items: summaries, HasMore: false}, nil
}

func (s *TrackMapService) routeGroupItem(ctx context.Context, group *models.TrackRouteGroup) (*models.TrackMapRouteGroupItem, error) {
	if group == nil {
		return nil, repository.ErrNotFound
	}
	track, err := s.tracks.FindByID(ctx, group.RepresentativeTrackID)
	if err != nil {
		return nil, err
	}
	cityCode := ""
	if len(group.CityCodes) > 0 {
		cityCode = group.CityCodes[0]
	}
	item := &models.TrackMapRouteGroupItem{
		Type:             "route_group",
		GroupID:          group.GroupID,
		Name:             routeGroupDisplayName(group),
		CityCode:         cityCode,
		CityName:         config.CityNameByCode(cityCode),
		TrackType:        group.TrackType,
		CoordinateSystem: mapCoordinateSystem(group.CoordinateSystem),
		Center:           models.TrackMapPoint{Latitude: group.CenterLat, Longitude: group.CenterLng},
		BBox: models.TrackMapBBox{
			MinLatitude:  group.MinLat,
			MinLongitude: group.MinLng,
			MaxLatitude:  group.MaxLat,
			MaxLongitude: group.MaxLng,
		},
		RepresentativePolyline: trackMapPolyline(group.RepresentativePolyline),
		CoverTrack:             &models.TrackMapCoverTrack{TrackID: track.ID},
		RawTrackID:             track.ID,
		Track:                  track,
	}
	if s.trackSvc != nil && s.trackSvc.screenshotCache != nil && track.TrackScreenshotURL != "" {
		cacheCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		item.CoverTrack.TrackScreenshotURL = s.trackSvc.screenshotCache.EnsureCached(cacheCtx, track.UserID, track.ID, track.TrackScreenshotURL)
		cancel()
	}
	return item, nil
}

func routeGroupDisplayName(group *models.TrackRouteGroup) string {
	name := strings.TrimSpace(group.Name)
	if name != "" {
		return name
	}
	cityCode := ""
	if len(group.CityCodes) > 0 {
		cityCode = group.CityCodes[0]
	}
	cityName := config.CityNameByCode(cityCode)
	trackTypeName := trackTypeDisplayName(group.TrackType)
	if cityName == "" {
		return trackTypeName + "路线"
	}
	return cityName + trackTypeName + "路线"
}

func (s *TrackMapService) legacyRouteGroupItem(ctx context.Context, index *models.TrackGeoIndex) (*models.TrackMapRouteGroupItem, error) {
	if index == nil {
		return nil, repository.ErrNotFound
	}
	track, err := s.tracks.FindByID(ctx, index.TrackID)
	if err != nil {
		return nil, err
	}
	item := &models.TrackMapRouteGroupItem{
		Type:             "route_group",
		GroupID:          index.TrackID,
		Name:             trackMapRouteName(track, index),
		CityCode:         index.CityCode,
		CityName:         config.CityNameByCode(index.CityCode),
		TrackType:        index.TrackType,
		CoordinateSystem: mapCoordinateSystem(index.CoordinateSystem),
		Center:           models.TrackMapPoint{Latitude: index.CenterLat, Longitude: index.CenterLng},
		BBox: models.TrackMapBBox{
			MinLatitude:  index.MinLat,
			MinLongitude: index.MinLng,
			MaxLatitude:  index.MaxLat,
			MaxLongitude: index.MaxLng,
		},
		RepresentativePolyline: trackMapPolyline(index.SimplifiedPolyline),
		CoverTrack:             &models.TrackMapCoverTrack{TrackID: track.ID},
		RawTrackID:             track.ID,
		SourceGeoIndex:         index,
		Track:                  track,
	}
	if s.trackSvc != nil && s.trackSvc.screenshotCache != nil && track.TrackScreenshotURL != "" {
		cacheCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		item.CoverTrack.TrackScreenshotURL = s.trackSvc.screenshotCache.EnsureCached(cacheCtx, track.UserID, track.ID, track.TrackScreenshotURL)
		cancel()
	}
	return item, nil
}

func (s *TrackMapService) buildFilter(input TrackMapViewInput) (models.TrackMapQueryFilter, string, error) {
	if s == nil || s.maps == nil {
		return models.TrackMapQueryFilter{}, "", errors.New("track map service is not configured")
	}
	limit := input.Limit
	if limit <= 0 {
		limit = defaultTrackMapLimit
	}
	if limit > maxTrackMapLimit {
		limit = maxTrackMapLimit
	}
	trackType := normalizeTrackTypeCode(input.TrackType)
	if trackType == "" {
		trackType = "hiking"
	}
	filter := models.TrackMapQueryFilter{
		TrackType: trackType,
		CityCode:  strings.TrimSpace(input.CityCode),
		Limit:     limit,
	}
	if input.BBox != "" {
		bbox, err := parseTrackMapBBox(input.BBox)
		if err != nil {
			return filter, "", err
		}
		filter.BBox = bbox
	}
	if input.Latitude != nil || input.Longitude != nil {
		if input.Latitude == nil || input.Longitude == nil {
			return filter, "", invalidArg("latitude and longitude must be provided together")
		}
		if !validMapLatLng(*input.Latitude, *input.Longitude) {
			return filter, "", invalidArg("invalid latitude or longitude")
		}
		radius := input.RadiusM
		if radius <= 0 {
			radius = defaultTrackMapRadiusM
		}
		if radius > maxTrackMapRadiusM {
			radius = maxTrackMapRadiusM
		}
		filter.Center = &models.TrackMapPoint{Latitude: *input.Latitude, Longitude: *input.Longitude}
		filter.RadiusM = radius
	}
	if filter.BBox == nil && filter.Center == nil && filter.CityCode == "" {
		return filter, "", invalidArg("bbox, city_code, or latitude/longitude is required")
	}
	return filter, chooseTrackMapViewLevel(input.Zoom, filter), nil
}

func chooseTrackMapViewLevel(zoom float64, filter models.TrackMapQueryFilter) string {
	if zoom > 0 {
		if zoom <= 7 {
			return trackMapViewCity
		}
		if zoom <= 11 {
			return trackMapViewArea
		}
		return trackMapViewRoute
	}
	if filter.BBox != nil {
		latSpan := filter.BBox.MaxLatitude - filter.BBox.MinLatitude
		lngSpan := filter.BBox.MaxLongitude - filter.BBox.MinLongitude
		if latSpan > 5 || lngSpan > 5 {
			return trackMapViewCity
		}
		if latSpan > 0.8 || lngSpan > 0.8 {
			return trackMapViewArea
		}
	}
	return trackMapViewRoute
}

func parseTrackMapBBox(raw string) (*models.TrackMapBBox, error) {
	parts := strings.Split(raw, ",")
	if len(parts) != 4 {
		return nil, invalidArg("invalid bbox")
	}
	values := make([]float64, 4)
	for i, part := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return nil, invalidArg("invalid bbox")
		}
		values[i] = v
	}
	bbox := &models.TrackMapBBox{
		MinLongitude: values[0],
		MinLatitude:  values[1],
		MaxLongitude: values[2],
		MaxLatitude:  values[3],
	}
	if bbox.MinLatitude > bbox.MaxLatitude || bbox.MinLongitude > bbox.MaxLongitude {
		return nil, invalidArg("invalid bbox")
	}
	if !validMapLatLng(bbox.MinLatitude, bbox.MinLongitude) || !validMapLatLng(bbox.MaxLatitude, bbox.MaxLongitude) {
		return nil, invalidArg("invalid bbox")
	}
	return bbox, nil
}

func validMapLatLng(lat, lng float64) bool {
	return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180
}

func trackMapRouteName(track *models.Track, index *models.TrackGeoIndex) string {
	title := ""
	if track != nil {
		title = strings.TrimSpace(track.Title)
	}
	if title != "" && title != "新的轨迹" {
		return title
	}
	trackType := index.TrackType
	if trackType == "" {
		trackType = "hiking"
	}
	cityName := config.CityNameByCode(index.CityCode)
	if cityName != "" {
		return fmt.Sprintf("%s%s路线", cityName, trackType)
	}
	return fmt.Sprintf("%s路线", trackType)
}

func trackMapPolyline(points []models.TrackPoint) []models.TrackMapPoint {
	out := make([]models.TrackMapPoint, 0, len(points))
	for _, p := range points {
		out = append(out, models.TrackMapPoint{Latitude: p.Latitude, Longitude: p.Longitude})
	}
	return out
}

func mapCoordinateSystem(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "GCJ02"
	}
	return raw
}

func (s *TrackMapService) decorateClusters(items []*models.TrackMapClusterItem) {
	for _, item := range items {
		if item == nil {
			continue
		}
		item.CityName = config.CityNameByCode(item.CityCode)
	}
}
