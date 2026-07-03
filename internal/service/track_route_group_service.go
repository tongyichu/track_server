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
	"github.com/tongyichu/track_server/internal/maparea"
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
	repo        repository.TrackMapRepository
	areas       *maparea.Catalog
	tracks      repository.TrackRepository
	submissions *TrackSubmissionService
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

type AdminRouteGroupArea struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Type            string `json:"type"`
	CityCode        string `json:"city_code,omitempty"`
	CityName        string `json:"city_name,omitempty"`
	IntroductionURL string `json:"introduction_url,omitempty"`
}

type AdminRouteGroupSummary struct {
	*models.TrackRouteGroup
	Area *AdminRouteGroupArea `json:"area,omitempty"`
}

type AdminRouteGroupDetail struct {
	Group   *models.TrackRouteGroup      `json:"group"`
	Area    *AdminRouteGroupArea         `json:"area,omitempty"`
	Members []*AdminRouteGroupMemberView `json:"members"`
}

type routeGroupCluster struct {
	group   *models.TrackRouteGroup
	members []*models.TrackRouteGroupMember
	indexes []*models.TrackGeoIndex
}

func NewTrackRouteGroupService(repo repository.TrackMapRepository) *TrackRouteGroupService {
	return &TrackRouteGroupService{repo: repo, areas: maparea.DefaultCatalog()}
}

func (s *TrackRouteGroupService) SetTrackRepository(tracks repository.TrackRepository) {
	if s != nil {
		s.tracks = tracks
	}
}

func (s *TrackRouteGroupService) SetTrackSubmissionService(submissions *TrackSubmissionService) {
	if s != nil {
		s.submissions = submissions
	}
}

func (s *TrackRouteGroupService) ListRouteGroups(ctx context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackRouteGroup, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("track route group service is not configured")
	}
	return s.repo.ListRouteGroups(ctx, filter)
}

func (s *TrackRouteGroupService) ListRouteGroupSummaries(ctx context.Context, filter models.TrackMapQueryFilter) ([]*AdminRouteGroupSummary, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("track route group service is not configured")
	}
	groups, err := s.repo.ListRouteGroupSummaries(ctx, filter)
	if err != nil {
		return nil, err
	}
	items := make([]*AdminRouteGroupSummary, 0, len(groups))
	for _, group := range groups {
		if group == nil {
			continue
		}
		items = append(items, &AdminRouteGroupSummary{
			TrackRouteGroup: group,
			Area:            s.adminRouteGroupArea(group.AreaID),
		})
	}
	return items, nil
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
	return &AdminRouteGroupDetail{Group: group, Area: s.adminRouteGroupArea(group.AreaID), Members: views}, nil
}

func (s *TrackRouteGroupService) adminRouteGroupArea(areaID string) *AdminRouteGroupArea {
	areaID = strings.TrimSpace(areaID)
	if s == nil || s.areas == nil || areaID == "" {
		return nil
	}
	area := s.areas.Find(areaID)
	if area == nil {
		return nil
	}
	result := &AdminRouteGroupArea{
		ID:       area.ID,
		Name:     area.Name(),
		Type:     area.Type,
		CityCode: area.CityCode,
		CityName: config.CityNameByCode(area.CityCode),
	}
	if area.HasIntroduction() {
		result.IntroductionURL = mapAreaIntroductionURL(area.ID, area.ContentVersion)
	}
	return result
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

func (s *TrackRouteGroupService) GetRouteIntroduction(ctx context.Context, groupID string) (*models.TrackRouteIntroduction, error) {
	if _, err := s.repo.FindRouteGroup(ctx, strings.TrimSpace(groupID)); err != nil {
		return nil, err
	}
	return s.repo.FindRouteIntroductionByGroupID(ctx, strings.TrimSpace(groupID))
}

func (s *TrackRouteGroupService) SaveRouteIntroduction(ctx context.Context, groupID string, input *models.TrackRouteIntroduction) (*models.TrackRouteIntroduction, error) {
	group, err := s.repo.FindRouteGroup(ctx, strings.TrimSpace(groupID))
	if err != nil {
		return nil, err
	}
	if input == nil {
		return nil, invalidArg("route introduction is required")
	}
	input.Chinese.Name = strings.TrimSpace(input.Chinese.Name)
	input.Chinese.Summary = strings.TrimSpace(input.Chinese.Summary)
	if input.Chinese.Name == "" || input.Chinese.Summary == "" {
		return nil, invalidArg("zh.name and zh.summary are required")
	}
	if input.EstimatedDurationMin < 0 || input.EstimatedDurationMax < 0 || (input.EstimatedDurationMax > 0 && input.EstimatedDurationMax < input.EstimatedDurationMin) {
		return nil, invalidArg("invalid estimated duration")
	}
	existing, findErr := s.repo.FindRouteIntroductionByGroupID(ctx, group.GroupID)
	if findErr != nil && !errors.Is(findErr, repository.ErrNotFound) {
		return nil, findErr
	}
	if existing != nil {
		input.ID, input.AnchorTrackID, input.CreatedAt, input.PublishedAt = existing.ID, existing.AnchorTrackID, existing.CreatedAt, existing.PublishedAt
	} else {
		input.AnchorTrackID = group.RepresentativeTrackID
	}
	input.CurrentGroupID = group.GroupID
	input.Status = models.TrackRouteIntroductionStatusDraft
	input.PublishedAt = nil
	input.ContentVersion = 1
	if existing != nil {
		input.ContentVersion = existing.ContentVersion + 1
	}
	if err := s.repo.UpsertRouteIntroduction(ctx, input); err != nil {
		return nil, err
	}
	return input, nil
}

func (s *TrackRouteGroupService) SetRouteIntroductionPublished(ctx context.Context, groupID string, published bool) (*models.TrackRouteIntroduction, error) {
	introduction, err := s.GetRouteIntroduction(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if published {
		if strings.TrimSpace(introduction.Chinese.Name) == "" || strings.TrimSpace(introduction.Chinese.Summary) == "" {
			return nil, invalidArg("route introduction content is incomplete")
		}
		now := time.Now()
		introduction.Status, introduction.PublishedAt = models.TrackRouteIntroductionStatusPublished, &now
	} else {
		introduction.Status, introduction.PublishedAt = models.TrackRouteIntroductionStatusDraft, nil
	}
	introduction.ContentVersion++
	if err := s.repo.UpsertRouteIntroduction(ctx, introduction); err != nil {
		return nil, err
	}
	return introduction, nil
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
	if introduction, err := s.repo.FindRouteIntroductionByGroupID(ctx, groupID); err == nil {
		introduction.AnchorTrackID = trackID
		if err := s.repo.UpsertRouteIntroduction(ctx, introduction); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, repository.ErrNotFound) {
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
	targetIntroduction, targetIntroErr := s.repo.FindRouteIntroductionByGroupID(ctx, targetGroupID)
	if targetIntroErr != nil && !errors.Is(targetIntroErr, repository.ErrNotFound) {
		return nil, targetIntroErr
	}
	sourceIntroduction, sourceIntroErr := s.repo.FindRouteIntroductionByGroupID(ctx, sourceGroupID)
	if sourceIntroErr != nil && !errors.Is(sourceIntroErr, repository.ErrNotFound) {
		return nil, sourceIntroErr
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
	if sourceIntroduction != nil {
		if targetIntroduction == nil {
			sourceIntroduction.CurrentGroupID = targetGroupID
		} else {
			sourceIntroduction.CurrentGroupID = ""
			sourceIntroduction.Status = models.TrackRouteIntroductionStatusArchived
		}
		sourceIntroduction.ContentVersion++
		if err := s.repo.UpsertRouteIntroduction(ctx, sourceIntroduction); err != nil {
			return nil, err
		}
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
	s.assignRouteGroupArea(group)
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
	sort.SliceStable(indexes, func(i, j int) bool {
		if indexes[i].TrackType != indexes[j].TrackType {
			return indexes[i].TrackType < indexes[j].TrackType
		}
		if indexes[i].CenterLat != indexes[j].CenterLat {
			return indexes[i].CenterLat < indexes[j].CenterLat
		}
		return indexes[i].TrackID < indexes[j].TrackID
	})
	matrix := buildRouteSimilarityMatrix(indexes)
	positions := make([]int, len(indexes))
	for i := range positions {
		positions[i] = i
	}
	medoidPosition := chooseRouteMedoid(positions, matrix)
	rebuildMedoidRouteGroup(group, indexes, indexes[medoidPosition])
	s.assignRouteGroupArea(group)
	indexPositions := make(map[string]int, len(indexes))
	for position, index := range indexes {
		indexPositions[index.TrackID] = position
	}
	for _, member := range members {
		if member == nil {
			continue
		}
		position, exists := indexPositions[member.TrackID]
		if !exists {
			continue
		}
		evaluation := matrix.get(medoidPosition, position)
		member.Role = models.TrackRouteGroupMemberRoleMember
		if position == medoidPosition {
			member.Role = models.TrackRouteGroupMemberRoleRepresentative
		}
		member.SimilarityScore, member.MatchDirection = evaluation.score, evaluation.direction
		if err := s.repo.UpsertRouteGroupMember(ctx, member); err != nil {
			return nil, err
		}
	}
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
	oldGroups, err := s.repo.ListAllRouteGroups(ctx)
	if err != nil {
		return result, err
	}
	oldMembers, err := s.repo.ListAllRouteGroupMembers(ctx)
	if err != nil {
		return result, err
	}
	oldGroupIDs := make([]string, 0, len(oldGroups))
	for _, group := range oldGroups {
		if group != nil {
			oldGroupIDs = append(oldGroupIDs, group.GroupID)
		}
	}
	introductions, err := s.repo.ListPublishedRouteIntroductions(ctx, oldGroupIDs)
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
	eligible := make([]*models.TrackGeoIndex, 0, len(indexes))
	for _, index := range indexes {
		result.Scanned++
		if !isRouteGroupIndexEligible(index) {
			result.Skipped++
			continue
		}
		eligible = append(eligible, index)
	}
	matrix := buildRouteSimilarityMatrix(eligible)
	positions := constrainedRouteClusters(eligible, matrix)
	clusters := make([]*routeGroupCluster, 0, len(positions))
	for _, clusterPositions := range positions {
		clusters = append(clusters, buildRouteGroupFromCluster(eligible, clusterPositions, matrix))
	}
	result.Created = len(clusters)
	result.Merged = len(eligible) - len(clusters)
	groups, members := flattenRouteGroupClusters(clusters)
	for _, group := range groups {
		s.assignRouteGroupArea(group)
	}
	semanticNames := s.routeClusterSemanticNames(ctx, clusters)
	applyRouteGroupHistory(clusters, oldGroups, oldMembers, introductions, semanticNames, s.areas)
	if err := s.applySubmissionRepresentatives(ctx, clusters, oldMembers); err != nil {
		return result, err
	}
	groups, members = flattenRouteGroupClusters(clusters)
	if err := s.repo.ReplaceRouteGroups(ctx, groups, members); err != nil {
		return result, err
	}
	return result, nil
}

func (s *TrackRouteGroupService) applySubmissionRepresentatives(ctx context.Context, clusters []*routeGroupCluster, oldMembers []*models.TrackRouteGroupMember) error {
	if s == nil || s.submissions == nil {
		return nil
	}
	manualRepresentatives := make(map[string]string)
	for _, member := range oldMembers {
		if member != nil && member.Role == models.TrackRouteGroupMemberRoleRepresentative && member.Source == models.TrackRouteGroupSourceManual {
			manualRepresentatives[member.GroupID] = member.TrackID
		}
	}
	for _, cluster := range clusters {
		if cluster == nil || cluster.group == nil {
			continue
		}
		trackIDs := make([]string, 0, len(cluster.members))
		memberByID := make(map[string]*models.TrackRouteGroupMember, len(cluster.members))
		for _, member := range cluster.members {
			if member != nil {
				trackIDs = append(trackIDs, member.TrackID)
				memberByID[member.TrackID] = member
			}
		}
		selectedID := ""
		selectedSource := models.TrackRouteGroupSourceSubmission
		if manualID := manualRepresentatives[cluster.group.GroupID]; memberByID[manualID] != nil {
			selectedID, selectedSource = manualID, models.TrackRouteGroupSourceManual
		} else {
			approved, err := s.submissions.ApprovedTrackIDs(ctx, trackIDs)
			if err != nil {
				return err
			}
			var bestScore float64 = -1
			for trackID := range approved {
				member := memberByID[trackID]
				if member != nil && (member.SimilarityScore > bestScore || member.SimilarityScore == bestScore && (selectedID == "" || trackID < selectedID)) {
					selectedID, bestScore = trackID, member.SimilarityScore
				}
			}
		}
		if selectedID == "" {
			continue
		}
		cluster.group.RepresentativeTrackID = selectedID
		if selectedSource == models.TrackRouteGroupSourceManual {
			cluster.group.Source = routeGroupSourceAfterManual(cluster.group.Source)
		}
		for _, member := range cluster.members {
			if member == nil {
				continue
			}
			member.Role = models.TrackRouteGroupMemberRoleMember
			if member.TrackID == selectedID {
				member.Role, member.Source = models.TrackRouteGroupMemberRoleRepresentative, selectedSource
			}
		}
	}
	return nil
}

func (s *TrackRouteGroupService) assignRouteGroupArea(group *models.TrackRouteGroup) {
	if group == nil {
		return
	}
	group.AreaID = ""
	if s == nil || s.areas == nil {
		return
	}
	area := s.areas.Resolve(group.CenterLat, group.CenterLng)
	if area != nil {
		group.AreaID = area.ID
	}
}

func isRouteGroupIndexEligible(index *models.TrackGeoIndex) bool {
	return index != nil &&
		index.TrackID != "" &&
		index.TrackType != "" &&
		index.PointCount >= minRouteGroupPointCount &&
		index.Distance >= minRouteGroupDistanceMeters
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
