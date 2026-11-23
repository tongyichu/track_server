package service

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/tongyichu/track_server/internal/models"
)

func (s *TrackRouteGroupService) routeClusterRuleNames(ctx context.Context, clusters []*routeGroupCluster) (map[int]string, error) {
	var ids []string
	for _, cluster := range clusters {
		for _, index := range cluster.indexes {
			ids = append(ids, index.TrackID)
		}
	}
	tracks := make(map[string]*models.Track)
	submissions := make(map[string]*models.TrackSubmission)
	for start := 0; start < len(ids); start += 200 {
		batch := ids[start:min(start+200, len(ids))]
		if s.tracks != nil {
			found, err := s.tracks.FindByIDs(ctx, batch)
			if err != nil {
				return nil, err
			}
			for id, item := range found {
				if publicMapTrack(item) {
					tracks[id] = item
				}
			}
		}
		if s.submissions != nil {
			found, err := s.submissions.ListByTrackIDs(ctx, batch)
			if err != nil {
				return nil, err
			}
			for id, item := range found {
				if item != nil && item.Status == models.TrackSubmissionStatusApproved {
					submissions[id] = item
				}
			}
		}
	}
	var scenicMatches map[string]*departureScenicMatch
	if s.trackMap != nil {
		publicIDs := make([]string, 0, len(tracks))
		for id := range tracks {
			publicIDs = append(publicIDs, id)
		}
		var err error
		scenicMatches, err = s.trackMap.trackScenicMatches(ctx, publicIDs)
		if err != nil {
			return nil, err
		}
	}
	names := make(map[int]string, len(clusters))
	for position, cluster := range clusters {
		group := cluster.group
		var ref *models.TrackGeoIndex
		var groupIDs []string
		for _, index := range cluster.indexes {
			if index.TrackID == group.RepresentativeTrackID {
				ref = index
			}
			if tracks[index.TrackID] != nil {
				groupIDs = append(groupIDs, index.TrackID)
			}
		}
		if title := groupCuratedRouteTitle(cluster, tracks, submissions); title != "" {
			names[position] = title
			continue
		}
		scenic := routeGroupScenic(group, groupIDs, scenicMatches)
		facts := routeGroupLocationFacts(s.areas, group, ref, tracks[group.RepresentativeTrackID], scenic)
		names[position] = facts.ruleTitle(group.Distance)
	}
	return names, nil
}

func groupCuratedRouteTitle(cluster *routeGroupCluster, tracks map[string]*models.Track, submissions map[string]*models.TrackSubmission) string {
	bestName, bestID := "", ""
	bestRank, bestScore := 0, -1.0
	for _, member := range cluster.members {
		track := tracks[member.TrackID]
		if track == nil {
			continue
		}
		title, rank := "", 0
		if sub := submissions[track.ID]; sub != nil {
			title, rank = specificCuratedRouteTitle(sub.Title), 2
		}
		if title == "" && track.SourceTag == "manual_seed" {
			title, rank = specificCuratedRouteTitle(track.Title), 1
		}
		if title != "" && (rank > bestRank || rank == bestRank &&
			(member.SimilarityScore > bestScore || member.SimilarityScore == bestScore && track.ID < bestID)) {
			bestName, bestID, bestRank, bestScore = title, track.ID, rank, member.SimilarityScore
		}
	}
	return bestName
}

func specificCuratedRouteTitle(title string) string {
	title = strings.Join(strings.Fields(title), " ")
	for _, suffix := range []string{"徒步路线", "跑步路线", "爬山路线", "骑行路线", "自驾路线"} {
		title = strings.TrimSpace(strings.TrimSuffix(title, suffix))
	}
	if len(title) == 0 || utf8.RuneCountInString(title) > 36 || IsDatedTrackTitle(title) {
		return ""
	}
	lower := strings.ToLower(title)
	for _, fragment := range []string{"测试", "打卡", "随便", "我的", "记录", "新的轨迹", "test", "devmode", "http", "@", "#"} {
		if strings.Contains(lower, fragment) {
			return ""
		}
	}
	for _, suffix := range []string{"古道", "步道", "绿道", "栈道", "径", "环线", "往返", "穿越", "trail", "loop"} {
		if strings.HasSuffix(lower, suffix) && len(title) > len(suffix) {
			return title
		}
	}
	return ""
}
