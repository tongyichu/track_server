package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

const (
	defaultRouteGroupBatchSize      = 200
	defaultRouteGroupCandidateLimit = 30
	minRouteGroupPointCount         = 20
	minRouteGroupDistanceMeters     = 300
	maxRouteGroupDistanceRatio      = 0.35
	maxRouteGroupEndpointDistanceM  = 1200
	minRouteGroupSimilarityScore    = 0.72
	routeGroupPolylineSampleSize    = 64
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

func NewTrackRouteGroupService(repo repository.TrackMapRepository) *TrackRouteGroupService {
	return &TrackRouteGroupService{repo: repo}
}

func (s *TrackRouteGroupService) ListRouteGroups(ctx context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackRouteGroup, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("track route group service is not configured")
	}
	return s.repo.ListRouteGroups(ctx, filter)
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
	group.RepresentativePolyline = append([]models.TrackPoint(nil), index.SimplifiedPolyline...)
	polylineJSON, _ := json.Marshal(group.RepresentativePolyline)
	group.RepresentativePolylineJSON = string(polylineJSON)
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
		cityCodes      []string
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
	indexes, err := s.repo.ListGeoIndexesWithoutRouteGroup(ctx, defaultRouteGroupBatchSize)
	if err != nil {
		return result, err
	}
	for _, index := range indexes {
		result.Scanned++
		if !isRouteGroupIndexEligible(index) {
			result.Skipped++
			continue
		}
		group, score, direction, err := s.bestGroupCandidate(ctx, index)
		if err != nil {
			return result, err
		}
		if group == nil {
			newGroup, err := buildRouteGroupFromIndex(index)
			if err != nil {
				result.Skipped++
				continue
			}
			if err := s.repo.UpsertRouteGroup(ctx, newGroup); err != nil {
				return result, err
			}
			if err := s.repo.UpsertRouteGroupMember(ctx, &models.TrackRouteGroupMember{
				GroupID:         newGroup.GroupID,
				TrackID:         index.TrackID,
				SimilarityScore: 1,
				MatchDirection:  models.TrackRouteGroupMemberDirectionForward,
				Role:            models.TrackRouteGroupMemberRoleRepresentative,
				Source:          models.TrackRouteGroupSourceAuto,
			}); err != nil {
				return result, err
			}
			result.Created++
			continue
		}
		updated := mergeIndexIntoRouteGroup(group, index)
		if err := s.repo.UpsertRouteGroup(ctx, updated); err != nil {
			return result, err
		}
		if err := s.repo.UpsertRouteGroupMember(ctx, &models.TrackRouteGroupMember{
			GroupID:         updated.GroupID,
			TrackID:         index.TrackID,
			SimilarityScore: score,
			MatchDirection:  direction,
			Role:            models.TrackRouteGroupMemberRoleMember,
			Source:          models.TrackRouteGroupSourceAuto,
		}); err != nil {
			return result, err
		}
		result.Merged++
	}
	return result, nil
}

func (s *TrackRouteGroupService) bestGroupCandidate(ctx context.Context, index *models.TrackGeoIndex) (*models.TrackRouteGroup, float64, models.TrackRouteGroupMemberDirection, error) {
	candidates, err := s.repo.ListRouteGroupCandidates(ctx, index, defaultRouteGroupCandidateLimit)
	if err != nil {
		return nil, 0, "", err
	}
	var best *models.TrackRouteGroup
	bestScore := 0.0
	bestDirection := models.TrackRouteGroupMemberDirectionForward
	for _, candidate := range candidates {
		if candidate == nil || candidate.Group == nil || candidate.Index == nil {
			continue
		}
		score, direction := routeGroupSimilarity(index, candidate.Index)
		if score > bestScore {
			best = candidate.Group
			bestScore = score
			bestDirection = direction
		}
	}
	if best == nil || bestScore < minRouteGroupSimilarityScore {
		return nil, 0, "", nil
	}
	return best, bestScore, bestDirection, nil
}

func isRouteGroupIndexEligible(index *models.TrackGeoIndex) bool {
	return index != nil &&
		index.TrackID != "" &&
		index.TrackType != "" &&
		index.PointCount >= minRouteGroupPointCount &&
		index.Distance >= minRouteGroupDistanceMeters &&
		len(index.SimplifiedPolyline) >= 2
}

func buildRouteGroupFromIndex(index *models.TrackGeoIndex) (*models.TrackRouteGroup, error) {
	if index == nil {
		return nil, repository.ErrNotFound
	}
	polylineJSON, _ := json.Marshal(index.SimplifiedPolyline)
	now := time.Now()
	return &models.TrackRouteGroup{
		GroupID:                    routeGroupIDFromTrackID(index.TrackID),
		Name:                       defaultRouteGroupName(index),
		TrackType:                  index.TrackType,
		Status:                     models.TrackRouteGroupStatusActive,
		CityCodes:                  compactCityCodes([]string{index.CityCode}),
		CoordinateSystem:           mapCoordinateSystem(index.CoordinateSystem),
		CenterLat:                  index.CenterLat,
		CenterLng:                  index.CenterLng,
		MinLat:                     index.MinLat,
		MinLng:                     index.MinLng,
		MaxLat:                     index.MaxLat,
		MaxLng:                     index.MaxLng,
		Distance:                   index.Distance,
		RepresentativeTrackID:      index.TrackID,
		RepresentativePolyline:     append([]models.TrackPoint(nil), index.SimplifiedPolyline...),
		RepresentativePolylineJSON: string(polylineJSON),
		MemberCount:                1,
		Source:                     models.TrackRouteGroupSourceAuto,
		CreatedAt:                  now,
		UpdatedAt:                  now,
	}, nil
}

func mergeIndexIntoRouteGroup(group *models.TrackRouteGroup, index *models.TrackGeoIndex) *models.TrackRouteGroup {
	updated := *group
	updated.CityCodes = compactCityCodes(append(updated.CityCodes, index.CityCode))
	updated.MinLat = math.Min(updated.MinLat, index.MinLat)
	updated.MinLng = math.Min(updated.MinLng, index.MinLng)
	updated.MaxLat = math.Max(updated.MaxLat, index.MaxLat)
	updated.MaxLng = math.Max(updated.MaxLng, index.MaxLng)
	updated.CenterLat = (updated.CenterLat*float64(maxInt64(updated.MemberCount, 1)) + index.CenterLat) / float64(maxInt64(updated.MemberCount, 1)+1)
	updated.CenterLng = (updated.CenterLng*float64(maxInt64(updated.MemberCount, 1)) + index.CenterLng) / float64(maxInt64(updated.MemberCount, 1)+1)
	if index.Distance > updated.Distance {
		updated.Distance = index.Distance
	}
	updated.MemberCount++
	if updated.Source == models.TrackRouteGroupSourceManual {
		updated.Source = models.TrackRouteGroupSourceMixed
	}
	updated.UpdatedAt = time.Now()
	return &updated
}

func routeGroupSimilarity(a, b *models.TrackGeoIndex) (float64, models.TrackRouteGroupMemberDirection) {
	if a == nil || b == nil || a.TrackType != b.TrackType {
		return 0, models.TrackRouteGroupMemberDirectionForward
	}
	if a.Distance <= 0 || b.Distance <= 0 {
		return 0, models.TrackRouteGroupMemberDirectionForward
	}
	ratio := math.Abs(a.Distance-b.Distance) / math.Max(a.Distance, b.Distance)
	if ratio > maxRouteGroupDistanceRatio {
		return 0, models.TrackRouteGroupMemberDirectionForward
	}
	forwardEndpoint := (haversineMeters(a.StartLat, a.StartLng, b.StartLat, b.StartLng) + haversineMeters(a.EndLat, a.EndLng, b.EndLat, b.EndLng)) / 2
	reverseEndpoint := (haversineMeters(a.StartLat, a.StartLng, b.EndLat, b.EndLng) + haversineMeters(a.EndLat, a.EndLng, b.StartLat, b.StartLng)) / 2
	direction := models.TrackRouteGroupMemberDirectionForward
	endpointDistance := forwardEndpoint
	if reverseEndpoint < forwardEndpoint {
		endpointDistance = reverseEndpoint
		direction = models.TrackRouteGroupMemberDirectionReverse
	}
	if endpointDistance > maxRouteGroupEndpointDistanceM {
		return 0, direction
	}
	shapeDistance := averagePolylineDistance(a.SimplifiedPolyline, b.SimplifiedPolyline, direction == models.TrackRouteGroupMemberDirectionReverse)
	shapeScore := 1 - math.Min(shapeDistance/1500, 1)
	endpointScore := 1 - math.Min(endpointDistance/maxRouteGroupEndpointDistanceM, 1)
	distanceScore := 1 - ratio
	return 0.55*shapeScore + 0.30*endpointScore + 0.15*distanceScore, direction
}

func averagePolylineDistance(a, b []models.TrackPoint, reverseB bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return math.MaxFloat64
	}
	sa := sampleTrackPoints(a, routeGroupPolylineSampleSize)
	sb := sampleTrackPoints(b, routeGroupPolylineSampleSize)
	if reverseB {
		for i, j := 0, len(sb)-1; i < j; i, j = i+1, j-1 {
			sb[i], sb[j] = sb[j], sb[i]
		}
	}
	n := len(sa)
	if len(sb) < n {
		n = len(sb)
	}
	var total float64
	for i := 0; i < n; i++ {
		total += haversineMeters(sa[i].Latitude, sa[i].Longitude, sb[i].Latitude, sb[i].Longitude)
	}
	return total / float64(n)
}

func sampleTrackPoints(points []models.TrackPoint, size int) []models.TrackPoint {
	if size <= 0 || len(points) <= size {
		return append([]models.TrackPoint(nil), points...)
	}
	out := make([]models.TrackPoint, 0, size)
	step := float64(len(points)-1) / float64(size-1)
	for i := 0; i < size; i++ {
		idx := int(math.Round(float64(i) * step))
		if idx >= len(points) {
			idx = len(points) - 1
		}
		out = append(out, points[idx])
	}
	return out
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

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
