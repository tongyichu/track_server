package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

func newMapModeStatistics() *models.AdminMapModeStatistics {
	return &models.AdminMapModeStatistics{
		Points: []models.AdminFeatureMapPoint{},
		Distribution: []models.AdminDashboardDistributionItem{
			{Key: "zero", Label: "0 条"}, {Key: "one", Label: "1 条"},
			{Key: "2_5", Label: "2–5 条"}, {Key: "6_20", Label: "6–20 条"}, {Key: "21_plus", Label: "≥21 条"},
		},
	}
}

func addMapModeGroup(out *models.AdminMapModeStatistics, cities map[string]struct{}, group *models.TrackRouteGroup) {
	if group == nil || group.Status != models.TrackRouteGroupStatusActive {
		return
	}
	out.GroupCount++
	for _, code := range group.CityCodes {
		if code = strings.TrimSpace(code); code != "" {
			cities[code] = struct{}{}
		}
	}
	key := adminTrackOutputBucket(group.MemberCount)
	for i := range out.Distribution {
		if out.Distribution[i].Key == key {
			out.Distribution[i].Count++
			break
		}
	}
	if group.CenterLat >= -90 && group.CenterLat <= 90 && group.CenterLng >= -180 && group.CenterLng <= 180 && group.CenterLat != 0 && group.CenterLng != 0 {
		out.Points = append(out.Points, models.AdminFeatureMapPoint{ID: group.GroupID, Name: group.Name, Latitude: group.CenterLat, Longitude: group.CenterLng})
	} else {
		out.Unlocated++
	}
}

func newCompanionUsageStatistics(days int, now time.Time) (*models.AdminCompanionUsageStatistics, error) {
	if days != 1 && days != 7 && days != 30 {
		return nil, fmt.Errorf("days must be 1, 7 or 30")
	}
	now = now.In(time.FixedZone("Asia/Shanghai", 8*60*60))
	out := &models.AdminCompanionUsageStatistics{Days: days, End: now, Bucket: "day", Trend: []models.AdminCompanionUsagePoint{}, Participants: []models.AdminDashboardDistributionItem{
		{Key: "one", Label: "仅房主 / 1 人"}, {Key: "two", Label: "2 人"}, {Key: "three_five", Label: "3–5 人"}, {Key: "six_plus", Label: "≥6 人"}, {Key: "unknown", Label: "成员记录缺失"},
	}}
	out.Start = time.Date(now.Year(), now.Month(), now.Day()+1-days, 0, 0, 0, 0, now.Location())
	count, width, layout := days, 24*time.Hour, "01-02"
	if days == 1 {
		out.Bucket = "hour"
		out.Start = now.Add(-24 * time.Hour)
		count, width, layout = 24, time.Hour, "15:04"
	}
	for i := 0; i < count; i++ {
		at := out.Start.Add(time.Duration(i) * width)
		out.Trend = append(out.Trend, models.AdminCompanionUsagePoint{At: at, Label: at.Format(layout)})
	}
	return out, nil
}

func addCompanionUsage(out *models.AdminCompanionUsageStatistics, started, ended time.Time, status models.CompanionSessionStatus, participants int64) {
	width := 24 * time.Hour
	if out.Bucket == "hour" {
		width = time.Hour
	}
	if dashboardBetween(started, out.Start, out.End) {
		index := int(started.Sub(out.Start) / width)
		out.Trend[index].Created++
		key := "unknown"
		switch {
		case participants == 1:
			key = "one"
		case participants == 2:
			key = "two"
		case participants >= 3 && participants <= 5:
			key = "three_five"
		case participants >= 6:
			key = "six_plus"
		}
		for i := range out.Participants {
			if out.Participants[i].Key == key {
				out.Participants[i].Count++
				break
			}
		}
	}
	if status == models.CompanionSessionStatusEnded && dashboardBetween(ended, out.Start, out.End) {
		out.Trend[int(ended.Sub(out.Start)/width)].Ended++
	}
}

func (r *memoryAdminDashboardRepository) GetAdminMapModeStatistics(_ context.Context) (*models.AdminMapModeStatistics, error) {
	repo, ok := r.deps.trackMap.(*InMemoryTrackMapRepository)
	if !ok || repo == nil {
		return nil, errAdminStatisticsRepositoriesUnavailable
	}
	out, cities := newMapModeStatistics(), make(map[string]struct{})
	repo.mu.RLock()
	defer repo.mu.RUnlock()
	for _, group := range repo.routeGroups {
		addMapModeGroup(out, cities, group)
	}
	out.CityCount = int64(len(cities))
	return out, nil
}

func (r *memoryAdminDashboardRepository) GetAdminCompanionUsageStatistics(_ context.Context, days int, now time.Time) (*models.AdminCompanionUsageStatistics, error) {
	out, err := newCompanionUsageStatistics(days, now)
	if err != nil {
		return nil, err
	}
	repo, ok := r.deps.companions.(*InMemoryCompanionRepository)
	if !ok || repo == nil {
		return nil, errAdminStatisticsRepositoriesUnavailable
	}
	repo.mu.RLock()
	defer repo.mu.RUnlock()
	for id, session := range repo.sessions {
		if session != nil {
			addCompanionUsage(out, session.StartedAt, session.EndedAt, session.Status, int64(len(repo.members[id])))
		}
	}
	return out, nil
}
