package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/maparea"
	"github.com/tongyichu/track_server/internal/models"
)

func (s *TrackRouteGroupService) routeClusterSemanticNames(ctx context.Context, clusters []*routeGroupCluster) map[int]string {
	result := make(map[int]string)
	if s == nil || s.tracks == nil {
		return result
	}
	for clusterIndex, cluster := range clusters {
		if cluster == nil {
			continue
		}
		manual, common := make(map[string]int), make(map[string]int)
		validTitles := 0
		for _, index := range cluster.indexes {
			if index == nil {
				continue
			}
			track, err := s.tracks.FindByID(ctx, index.TrackID)
			if err != nil || track == nil {
				continue
			}
			title := normalizeRouteTitle(track.Title)
			if title == "" {
				continue
			}
			validTitles++
			common[title]++
			if track.SourceTag == "manual_seed" {
				manual[title]++
			}
		}
		if title, count := mostFrequentRouteTitle(manual); count > 0 {
			result[clusterIndex] = title
			continue
		}
		if title, count := mostFrequentRouteTitle(common); count >= 2 && count*2 >= validTitles {
			result[clusterIndex] = title
		}
	}
	return result
}

func normalizeRouteTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" || title == "新的轨迹" {
		return ""
	}
	return title
}

func mostFrequentRouteTitle(counts map[string]int) (string, int) {
	best, bestCount := "", 0
	for title, count := range counts {
		if count > bestCount || (count == bestCount && (best == "" || title < best)) {
			best, bestCount = title, count
		}
	}
	return best, bestCount
}

type routeGroupHistoryMatch struct {
	clusterIndex int
	groupID      string
	overlap      int
	jaccard      float64
}

func applyRouteGroupHistory(clusters []*routeGroupCluster, oldGroups []*models.TrackRouteGroup, oldMembers []*models.TrackRouteGroupMember, introductions map[string]*models.TrackRouteIntroduction, semanticNames map[int]string, areas *maparea.Catalog) {
	oldByID := make(map[string]*models.TrackRouteGroup, len(oldGroups))
	oldSizes := make(map[string]int, len(oldGroups))
	trackGroup := make(map[string]string, len(oldMembers))
	for _, group := range oldGroups {
		if group != nil {
			oldByID[group.GroupID] = group
		}
	}
	for _, member := range oldMembers {
		if member == nil || oldByID[member.GroupID] == nil {
			continue
		}
		trackGroup[member.TrackID] = member.GroupID
		oldSizes[member.GroupID]++
	}
	matches := make([]routeGroupHistoryMatch, 0)
	for clusterIndex, cluster := range clusters {
		counts := make(map[string]int)
		for _, index := range cluster.indexes {
			if index != nil && trackGroup[index.TrackID] != "" {
				counts[trackGroup[index.TrackID]]++
			}
		}
		for groupID, overlap := range counts {
			union := len(cluster.indexes) + oldSizes[groupID] - overlap
			jaccard := 0.0
			if union > 0 {
				jaccard = float64(overlap) / float64(union)
			}
			matches = append(matches, routeGroupHistoryMatch{clusterIndex: clusterIndex, groupID: groupID, overlap: overlap, jaccard: jaccard})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].overlap != matches[j].overlap {
			return matches[i].overlap > matches[j].overlap
		}
		if matches[i].jaccard != matches[j].jaccard {
			return matches[i].jaccard > matches[j].jaccard
		}
		if matches[i].groupID != matches[j].groupID {
			return matches[i].groupID < matches[j].groupID
		}
		return matches[i].clusterIndex < matches[j].clusterIndex
	})
	assignedClusters := make(map[int]bool)
	assignedGroups := make(map[string]bool)
	protectedNames := make(map[int]bool)
	for _, match := range matches {
		if assignedClusters[match.clusterIndex] || assignedGroups[match.groupID] {
			continue
		}
		cluster, old := clusters[match.clusterIndex], oldByID[match.groupID]
		if cluster == nil || cluster.group == nil || old == nil {
			continue
		}
		cluster.group.GroupID = old.GroupID
		cluster.group.CreatedAt = old.CreatedAt
		if old.Source == models.TrackRouteGroupSourceManual || old.Source == models.TrackRouteGroupSourceMixed {
			cluster.group.Name = old.Name
			cluster.group.Source = routeGroupSourceAfterManual(old.Source)
			protectedNames[match.clusterIndex] = true
		}
		if introduction := introductions[old.GroupID]; introduction != nil && strings.TrimSpace(introduction.Chinese.Name) != "" {
			cluster.group.Name = routeNameWithTrackType(introduction.Chinese.Name, cluster.group.TrackType)
			cluster.group.Source = routeGroupSourceAfterManual(cluster.group.Source)
			protectedNames[match.clusterIndex] = true
		}
		assignedClusters[match.clusterIndex], assignedGroups[match.groupID] = true, true
	}
	for clusterIndex, cluster := range clusters {
		if cluster == nil || cluster.group == nil {
			continue
		}
		if !protectedNames[clusterIndex] && strings.TrimSpace(semanticNames[clusterIndex]) != "" {
			cluster.group.Name = routeNameWithTrackType(semanticNames[clusterIndex], cluster.group.TrackType)
		} else if strings.TrimSpace(cluster.group.Name) == "" || cluster.group.Source == models.TrackRouteGroupSourceAuto {
			cluster.group.Name = defaultRouteGroupNameForArea(cluster.group, areas)
		}
		for _, member := range cluster.members {
			if member != nil {
				member.GroupID = cluster.group.GroupID
			}
		}
		cluster.group.UpdatedAt = time.Now()
	}
}

func defaultRouteGroupNameForArea(group *models.TrackRouteGroup, areas *maparea.Catalog) string {
	if group != nil && areas != nil && strings.TrimSpace(group.AreaID) != "" {
		if area := areas.Find(group.AreaID); area != nil {
			name := strings.TrimSpace(area.Name())
			for _, suffix := range []string{"风景名胜区", "风景区", "景区"} {
				name = strings.TrimSuffix(name, suffix)
			}
			if name != "" {
				return routeNameWithTrackType(name, group.TrackType)
			}
		}
	}
	if group == nil {
		return "徒步路线"
	}
	cityCode := ""
	if len(group.CityCodes) > 0 {
		cityCode = group.CityCodes[0]
	}
	return defaultRouteGroupName(&models.TrackGeoIndex{CityCode: cityCode, TrackType: group.TrackType})
}

func routeNameWithTrackType(name, trackType string) string {
	name = strings.TrimSpace(name)
	trackTypeName := trackTypeDisplayName(trackType)
	if name == "" {
		return trackTypeName + "路线"
	}
	if strings.Contains(name, trackTypeName) {
		if strings.HasSuffix(name, "路线") || strings.HasSuffix(name, "环线") || strings.HasSuffix(name, "古道") || strings.HasSuffix(name, "步道") {
			return name
		}
		return name + "路线"
	}
	return name + trackTypeName + "路线"
}
