package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

const (
	minRouteGroupPointCount        = 20
	minRouteGroupDistanceMeters    = 300
	defaultRouteGroupCenterRadiusM = 3000.0
	ridingRouteGroupCenterRadiusM  = 8000.0
	drivingRouteGroupCenterRadiusM = 15000.0
)

// TrackRouteGroupService builds persistent route groups from track_geo_indexes.
type TrackRouteGroupService struct {
	repo repository.TrackMapRepository
}

type TrackRouteGroupRunResult struct {
	Scanned int `json:"scanned"`
	Created int `json:"created"`
	Merged  int `json:"merged"`
	Skipped int `json:"skipped"`
}

type AdminRouteGroupMemberView struct {
	Member   *models.TrackRouteGroupMember `json:"member"`
	Track    *models.Track                 `json:"track,omitempty"`
	GeoIndex *models.TrackGeoIndex         `json:"geo_index,omitempty"`
}

type AdminRouteGroupDetail struct {
	Group   *models.TrackRouteGroup      `json:"group"`
	Members []*AdminRouteGroupMemberView `json:"members"`
}

type routeGroupCluster struct {
	group   *models.TrackRouteGroup
	members []*models.TrackRouteGroupMember
	indexes []*models.TrackGeoIndex
}

func NewTrackRouteGroupService(repo repository.TrackMapRepository) *TrackRouteGroupService {
	return &TrackRouteGroupService{repo: repo}
}

func (s *TrackRouteGroupService) ListRouteGroups(ctx context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackRouteGroup, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("track route group service is not configured")
	}
	return s.repo.ListRouteGroups(ctx, filter)
}

func (s *TrackRouteGroupService) ListRouteGroupSummaries(ctx context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackRouteGroup, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("track route group service is not configured")
	}
	return s.repo.ListRouteGroupSummaries(ctx, filter)
}

func (s *TrackRouteGroupService) GetRouteGroupDetail(ctx context.Context, groupID string, limit int) (*AdminRouteGroupDetail, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("track route group service is not configured")
	}
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, invalidArg("group_id is required")
	}
	if limit <= 0 {
		limit = 100
	}
	group, err := s.repo.FindRouteGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	members, err := s.repo.ListRouteGroupMembers(ctx, groupID, limit)
	if err != nil {
		return nil, err
	}
	views := make([]*AdminRouteGroupMemberView, 0, len(members))
	for _, member := range members {
		if member == nil {
			continue
		}
		view := &AdminRouteGroupMemberView{Member: member}
		if index, err := s.repo.FindTrackGeoIndex(ctx, member.TrackID); err == nil {
			view.GeoIndex = index
		}
		views = append(views, view)
	}
	return &AdminRouteGroupDetail{Group: group, Members: views}, nil
}

func (s *TrackRouteGroupService) RenameRouteGroup(ctx context.Context, groupID, name string) (*models.TrackRouteGroup, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("track route group service is not configured")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, invalidArg("name is required")
	}
	if len([]rune(name)) > 128 {
		return nil, invalidArg("name is too long")
	}
	group, err := s.repo.FindRouteGroup(ctx, strings.TrimSpace(groupID))
	if err != nil {
		return nil, err
	}
	group.Name = name
	group.Source = routeGroupSourceAfterManual(group.Source)
	group.UpdatedAt = time.Now()
	if err := s.repo.UpsertRouteGroup(ctx, group); err != nil {
		return nil, err
	}
	return group, nil
}

func (s *TrackRouteGroupService) SetRepresentativeTrack(ctx context.Context, groupID, trackID string) (*models.TrackRouteGroup, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("track route group service is not configured")
	}
	groupID = strings.TrimSpace(groupID)
	trackID = strings.TrimSpace(trackID)
	if groupID == "" || trackID == "" {
		return nil, invalidArg("group_id and track_id are required")
	}
	group, err := s.repo.FindRouteGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	members, err := s.repo.ListRouteGroupMembers(ctx, groupID, 1000)
	if err != nil {
		return nil, err
	}
	var selected *models.TrackRouteGroupMember
	for _, member := range members {
		if member != nil && member.TrackID == trackID {
			selected = member
			break
		}
	}
	if selected == nil {
		return nil, repository.ErrNotFound
	}
	index, err := s.repo.FindTrackGeoIndex(ctx, trackID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for _, member := range members {
		if member == nil {
			continue
		}
		if member.TrackID == trackID {
			member.Role = models.TrackRouteGroupMemberRoleRepresentative
		} else if member.Role == models.TrackRouteGroupMemberRoleRepresentative {
			member.Role = models.TrackRouteGroupMemberRoleMember
		} else {
			continue
		}
		member.Source = models.TrackRouteGroupSourceManual
		member.UpdatedAt = now
		if err := s.repo.UpsertRouteGroupMember(ctx, member); err != nil {
			return nil, err
		}
	}
	group.RepresentativeTrackID = trackID
	group.CoordinateSystem = mapCoordinateSystem(index.CoordinateSystem)
	group.Source = routeGroupSourceAfterManual(group.Source)
	group.UpdatedAt = now
	if err := s.repo.UpsertRouteGroup(ctx, group); err != nil {
		return nil, err
	}
	return group, nil
}

func (s *TrackRouteGroupService) RemoveRouteGroupMember(ctx context.Context, groupID, trackID string) (*models.TrackRouteGroup, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("track route group service is not configured")
	}
	groupID = strings.TrimSpace(groupID)
	trackID = strings.TrimSpace(trackID)
	if groupID == "" || trackID == "" {
		return nil, invalidArg("group_id and track_id are required")
	}
	group, err := s.repo.FindRouteGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	members, err := s.repo.ListRouteGroupMembers(ctx, groupID, 1000)
	if err != nil {
		return nil, err
	}
	if len(members) <= 1 {
		return nil, invalidArg("cannot remove the last member of a route group")
	}
	var removed *models.TrackRouteGroupMember
	for _, member := range members {
		if member != nil && member.TrackID == trackID {
			removed = member
			break
		}
	}
	if removed == nil {
		return nil, repository.ErrNotFound
	}
	if err := s.repo.DeleteRouteGroupMember(ctx, groupID, trackID); err != nil {
		return nil, err
	}
	if removed.Role == models.TrackRouteGroupMemberRoleRepresentative || group.RepresentativeTrackID == trackID {
		for _, member := range members {
			if member != nil && member.TrackID != trackID {
				return s.SetRepresentativeTrack(ctx, groupID, member.TrackID)
			}
		}
	}
	return s.rebuildRouteGroupBounds(ctx, groupID)
}

func (s *TrackRouteGroupService) MergeRouteGroups(ctx context.Context, targetGroupID, sourceGroupID string) (*models.TrackRouteGroup, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("track route group service is not configured")
	}
	targetGroupID = strings.TrimSpace(targetGroupID)
	sourceGroupID = strings.TrimSpace(sourceGroupID)
	if targetGroupID == "" || sourceGroupID == "" {
		return nil, invalidArg("target_group_id and source_group_id are required")
	}
	if targetGroupID == sourceGroupID {
		return nil, invalidArg("cannot merge the same route group")
	}
	target, err := s.repo.FindRouteGroup(ctx, targetGroupID)
	if err != nil {
		return nil, err
	}
	source, err := s.repo.FindRouteGroup(ctx, sourceGroupID)
	if err != nil {
		return nil, err
	}
	if target.TrackType != source.TrackType {
		return nil, invalidArg("cannot merge route groups with different track_type")
	}
	sourceMembers, err := s.repo.ListRouteGroupMembers(ctx, sourceGroupID, 1000)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for _, member := range sourceMembers {
		if member == nil {
			continue
		}
		if err := s.repo.DeleteRouteGroupMember(ctx, sourceGroupID, member.TrackID); err != nil && !errors.Is(err, repository.ErrNotFound) {
			return nil, err
		}
		member.GroupID = targetGroupID
		member.Role = models.TrackRouteGroupMemberRoleMember
		member.Source = models.TrackRouteGroupSourceManual
		member.UpdatedAt = now
		if err := s.repo.UpsertRouteGroupMember(ctx, member); err != nil {
			return nil, err
		}
	}
	if err := s.repo.ArchiveRouteGroup(ctx, sourceGroupID, now); err != nil {
		return nil, err
	}
	return s.rebuildRouteGroupBounds(ctx, targetGroupID)
}

func (s *TrackRouteGroupService) rebuildRouteGroupBounds(ctx context.Context, groupID string) (*models.TrackRouteGroup, error) {
	group, err := s.repo.FindRouteGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	members, err := s.repo.ListRouteGroupMembers(ctx, groupID, 1000)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return nil, invalidArg("route group has no members")
	}
	var (
		sumLat, sumLng float64
		minLat, minLng float64
		maxLat, maxLng float64
		maxDistance    float64
		radiusM        float64
		cityCodes      []string
		indexes        []*models.TrackGeoIndex
		validCount     int64
		initialized    bool
	)
	for _, member := range members {
		if member == nil {
			continue
		}
		index, err := s.repo.FindTrackGeoIndex(ctx, member.TrackID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				continue
			}
			return nil, err
		}
		validCount++
		indexes = append(indexes, index)
		cityCodes = append(cityCodes, index.CityCode)
		sumLat += index.CenterLat
		sumLng += index.CenterLng
		if index.Distance > maxDistance {
			maxDistance = index.Distance
		}
		if !initialized {
			minLat, minLng, maxLat, maxLng = index.MinLat, index.MinLng, index.MaxLat, index.MaxLng
			initialized = true
			continue
		}
		minLat = math.Min(minLat, index.MinLat)
		minLng = math.Min(minLng, index.MinLng)
		maxLat = math.Max(maxLat, index.MaxLat)
		maxLng = math.Max(maxLng, index.MaxLng)
	}
	if !initialized {
		return nil, invalidArg("route group has no valid geo indexes")
	}
	group.CityCodes = compactCityCodes(cityCodes)
	group.CenterLat = sumLat / float64(validCount)
	group.CenterLng = sumLng / float64(validCount)
	for _, index := range indexes {
		radiusM = math.Max(radiusM, routeGroupCoverageRadiusM(group.CenterLat, group.CenterLng, index))
	}
	group.RadiusM = radiusM
	group.MinLat = minLat
	group.MinLng = minLng
	group.MaxLat = maxLat
	group.MaxLng = maxLng
	group.Distance = maxDistance
	group.MemberCount = validCount
	group.Source = routeGroupSourceAfterManual(group.Source)
	group.UpdatedAt = time.Now()
	if err := s.repo.UpsertRouteGroup(ctx, group); err != nil {
		return nil, err
	}
	return group, nil
}

func (s *TrackRouteGroupService) RunOnce(ctx context.Context) (*TrackRouteGroupRunResult, error) {
	result := &TrackRouteGroupRunResult{}
	if s == nil || s.repo == nil {
		return result, fmt.Errorf("track route group service is not configured")
	}
	indexes, err := s.repo.ListAllTrackGeoIndexes(ctx)
	if err != nil {
		return result, err
	}
	sort.SliceStable(indexes, func(i, j int) bool {
		if indexes[i].TrackType == indexes[j].TrackType {
			if indexes[i].CenterLat == indexes[j].CenterLat {
				if indexes[i].CenterLng == indexes[j].CenterLng {
					return indexes[i].TrackID < indexes[j].TrackID
				}
				return indexes[i].CenterLng < indexes[j].CenterLng
			}
			return indexes[i].CenterLat < indexes[j].CenterLat
		}
		return indexes[i].TrackType < indexes[j].TrackType
	})
	clusters := make([]*routeGroupCluster, 0)
	for _, index := range indexes {
		result.Scanned++
		if !isRouteGroupIndexEligible(index) {
			result.Skipped++
			continue
		}
		cluster := bestSpatialCluster(clusters, index)
		if cluster == nil {
			cluster = newRouteGroupCluster(index)
			clusters = append(clusters, cluster)
			result.Created++
			continue
		}
		addIndexToRouteGroupCluster(cluster, index)
		result.Merged++
	}
	groups, members := flattenRouteGroupClusters(clusters)
	if err := s.repo.ReplaceRouteGroups(ctx, groups, members); err != nil {
		return result, err
	}
	return result, nil
}

func bestSpatialCluster(clusters []*routeGroupCluster, index *models.TrackGeoIndex) *routeGroupCluster {
	var best *routeGroupCluster
	bestDistance := math.MaxFloat64
	threshold := routeGroupCenterThresholdM(index.TrackType)
	for _, cluster := range clusters {
		if cluster == nil || cluster.group == nil || cluster.group.TrackType != index.TrackType {
			continue
		}
		distance := haversineMeters(cluster.group.CenterLat, cluster.group.CenterLng, index.CenterLat, index.CenterLng)
		if distance <= threshold && distance < bestDistance {
			best = cluster
			bestDistance = distance
		}
	}
	return best
}

func isRouteGroupIndexEligible(index *models.TrackGeoIndex) bool {
	return index != nil &&
		index.TrackID != "" &&
		index.TrackType != "" &&
		index.PointCount >= minRouteGroupPointCount &&
		index.Distance >= minRouteGroupDistanceMeters
}

func newRouteGroupCluster(index *models.TrackGeoIndex) *routeGroupCluster {
	now := time.Now()
	group := &models.TrackRouteGroup{
		GroupID:               routeGroupIDFromTrackID(index.TrackID),
		Name:                  defaultRouteGroupName(index),
		TrackType:             index.TrackType,
		Status:                models.TrackRouteGroupStatusActive,
		CityCodes:             compactCityCodes([]string{index.CityCode}),
		CoordinateSystem:      mapCoordinateSystem(index.CoordinateSystem),
		CenterLat:             index.CenterLat,
		CenterLng:             index.CenterLng,
		RadiusM:               routeGroupCoverageRadiusM(index.CenterLat, index.CenterLng, index),
		MinLat:                index.MinLat,
		MinLng:                index.MinLng,
		MaxLat:                index.MaxLat,
		MaxLng:                index.MaxLng,
		Distance:              index.Distance,
		RepresentativeTrackID: index.TrackID,
		MemberCount:           1,
		Source:                models.TrackRouteGroupSourceAuto,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	member := &models.TrackRouteGroupMember{
		GroupID:         group.GroupID,
		TrackID:         index.TrackID,
		SimilarityScore: 1,
		MatchDirection:  models.TrackRouteGroupMemberDirectionForward,
		Role:            models.TrackRouteGroupMemberRoleRepresentative,
		Source:          models.TrackRouteGroupSourceAuto,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	return &routeGroupCluster{group: group, members: []*models.TrackRouteGroupMember{member}, indexes: []*models.TrackGeoIndex{index}}
}

func addIndexToRouteGroupCluster(cluster *routeGroupCluster, index *models.TrackGeoIndex) {
	if cluster == nil || cluster.group == nil || index == nil {
		return
	}
	group := cluster.group
	now := time.Now()
	score := routeGroupCenterSimilarity(group, index)
	cluster.members = append(cluster.members, &models.TrackRouteGroupMember{
		GroupID:         group.GroupID,
		TrackID:         index.TrackID,
		SimilarityScore: score,
		MatchDirection:  models.TrackRouteGroupMemberDirectionForward,
		Role:            models.TrackRouteGroupMemberRoleMember,
		Source:          models.TrackRouteGroupSourceAuto,
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	cluster.indexes = append(cluster.indexes, index)
	rebuildRouteGroupFromIndexes(group, cluster.indexes)
}

func flattenRouteGroupClusters(clusters []*routeGroupCluster) ([]*models.TrackRouteGroup, []*models.TrackRouteGroupMember) {
	groups := make([]*models.TrackRouteGroup, 0, len(clusters))
	members := make([]*models.TrackRouteGroupMember, 0)
	for _, cluster := range clusters {
		if cluster == nil || cluster.group == nil {
			continue
		}
		groups = append(groups, cluster.group)
		members = append(members, cluster.members...)
	}
	return groups, members
}

func rebuildRouteGroupFromIndexes(group *models.TrackRouteGroup, indexes []*models.TrackGeoIndex) {
	if group == nil || len(indexes) == 0 {
		return
	}
	var sumLat, sumLng, maxDistance float64
	var cityCodes []string
	minLat, minLng := indexes[0].MinLat, indexes[0].MinLng
	maxLat, maxLng := indexes[0].MaxLat, indexes[0].MaxLng
	for _, index := range indexes {
		if index == nil {
			continue
		}
		sumLat += index.CenterLat
		sumLng += index.CenterLng
		cityCodes = append(cityCodes, index.CityCode)
		minLat = math.Min(minLat, index.MinLat)
		minLng = math.Min(minLng, index.MinLng)
		maxLat = math.Max(maxLat, index.MaxLat)
		maxLng = math.Max(maxLng, index.MaxLng)
		maxDistance = math.Max(maxDistance, index.Distance)
	}
	group.CityCodes = compactCityCodes(cityCodes)
	group.CenterLat = sumLat / float64(len(indexes))
	group.CenterLng = sumLng / float64(len(indexes))
	group.MinLat = minLat
	group.MinLng = minLng
	group.MaxLat = maxLat
	group.MaxLng = maxLng
	group.Distance = maxDistance
	group.MemberCount = int64(len(indexes))
	group.RadiusM = 0
	for _, index := range indexes {
		group.RadiusM = math.Max(group.RadiusM, routeGroupCoverageRadiusM(group.CenterLat, group.CenterLng, index))
	}
	group.UpdatedAt = time.Now()
}

func routeGroupCenterSimilarity(group *models.TrackRouteGroup, index *models.TrackGeoIndex) float64 {
	if group == nil || index == nil {
		return 0
	}
	threshold := routeGroupCenterThresholdM(index.TrackType)
	if threshold <= 0 {
		return 0
	}
	distance := haversineMeters(group.CenterLat, group.CenterLng, index.CenterLat, index.CenterLng)
	return 1 - math.Min(distance/threshold, 1)
}

func routeGroupCenterThresholdM(trackType string) float64 {
	switch strings.TrimSpace(trackType) {
	case "riding":
		return ridingRouteGroupCenterRadiusM
	case "driving":
		return drivingRouteGroupCenterRadiusM
	default:
		return defaultRouteGroupCenterRadiusM
	}
}

func routeGroupCoverageRadiusM(centerLat, centerLng float64, index *models.TrackGeoIndex) float64 {
	if index == nil {
		return 0
	}
	centerDistance := haversineMeters(centerLat, centerLng, index.CenterLat, index.CenterLng)
	return centerDistance + trackGeoIndexExtentRadiusM(index)
}

func trackGeoIndexExtentRadiusM(index *models.TrackGeoIndex) float64 {
	if index == nil {
		return 0
	}
	corners := [][2]float64{
		{index.MinLat, index.MinLng},
		{index.MinLat, index.MaxLng},
		{index.MaxLat, index.MinLng},
		{index.MaxLat, index.MaxLng},
	}
	var radius float64
	for _, corner := range corners {
		radius = math.Max(radius, haversineMeters(index.CenterLat, index.CenterLng, corner[0], corner[1]))
	}
	return radius
}

func routeGroupIDFromTrackID(trackID string) string {
	trackID = strings.TrimSpace(trackID)
	if trackID == "" {
		return fmt.Sprintf("RG.%d", time.Now().UnixNano())
	}
	return "RG." + strings.TrimPrefix(trackID, "NO.")
}

func defaultRouteGroupName(index *models.TrackGeoIndex) string {
	cityName := config.CityNameByCode(index.CityCode)
	trackTypeName := trackTypeDisplayName(index.TrackType)
	if cityName == "" {
		return trackTypeName + "路线"
	}
	return cityName + trackTypeName + "路线"
}

func trackTypeDisplayName(trackType string) string {
	trackType = strings.TrimSpace(trackType)
	for _, option := range config.DefaultTrackTypeConfigs {
		if option.Type == trackType || option.Name == trackType {
			return option.Name
		}
	}
	if trackType == "" {
		return "徒步"
	}
	return trackType
}

func compactCityCodes(codes []string) []string {
	seen := make(map[string]struct{}, len(codes))
	out := make([]string, 0, len(codes))
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}
	return out
}

func routeGroupSourceAfterManual(source models.TrackRouteGroupSource) models.TrackRouteGroupSource {
	if source == models.TrackRouteGroupSourceAuto || source == "" {
		return models.TrackRouteGroupSourceMixed
	}
	return source
}
