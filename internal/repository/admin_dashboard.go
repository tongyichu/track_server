package repository

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

type adminDashboardDependencies struct {
	users       UserRepository
	tracks      TrackRepository
	collects    CollectRepository
	loginLogs   LoginLogRepository
	navigations NavigationRepository
	companions  CompanionRepository
	feedback    FeedbackRepository
	trackMap    TrackMapRepository
	submissions TrackSubmissionRepository
	likes       TrackLikeRepository
	comments    CommentRepository
}

// NewAdminDashboardStatisticsRepository selects a read-only aggregator for the
// active persistence backend. Mixed backends are not supported because server
// startup always selects one complete repository family.
func NewAdminDashboardStatisticsRepository(
	users UserRepository,
	tracks TrackRepository,
	collects CollectRepository,
	loginLogs LoginLogRepository,
	navigations NavigationRepository,
	companions CompanionRepository,
	feedback FeedbackRepository,
	trackMap TrackMapRepository,
	submissions TrackSubmissionRepository,
	likes TrackLikeRepository,
	comments CommentRepository,
) AdminDashboardStatisticsRepository {
	deps := adminDashboardDependencies{
		users: users, tracks: tracks, collects: collects, loginLogs: loginLogs,
		navigations: navigations, companions: companions, feedback: feedback,
		trackMap: trackMap, submissions: submissions, likes: likes, comments: comments,
	}
	switch repo := users.(type) {
	case *MySQLUserRepository:
		if repo != nil && repo.db != nil {
			return &mysqlAdminDashboardRepository{db: repo.db}
		}
	case *MongoUserRepository:
		if repo != nil && repo.collection != nil {
			return &mongoAdminDashboardRepository{db: repo.collection.Database()}
		}
	case *InMemoryUserRepository:
		return &memoryAdminDashboardRepository{deps: deps}
	}
	return nil
}

func dashboardBetween(value, start, end time.Time) bool {
	return !value.IsZero() && !value.Before(start) && value.Before(end)
}

func dashboardDay(value time.Time, location *time.Location) string {
	if location == nil {
		location = time.Local
	}
	return value.In(location).Format("2006-01-02")
}

func newDashboardStatistics(query models.AdminDashboardQuery) *models.AdminDashboardStatistics {
	statistics := &models.AdminDashboardStatistics{
		Feedback: models.AdminDashboardFeedbackStats{
			Platforms: []models.AdminDashboardDistributionItem{},
			Versions:  []models.AdminDashboardDistributionItem{},
		},
		Content: models.AdminDashboardContentStats{
			Statuses: []models.AdminDashboardDistributionItem{},
			Types:    []models.AdminDashboardDistributionItem{},
			Cities:   []models.AdminDashboardDistributionItem{},
		},
		Engagement: models.AdminDashboardEngagementStats{TopTracks: []models.AdminDashboardTopTrack{}},
		Trend:      []models.AdminDashboardDaily{},
	}
	location := query.Now.Location()
	for day := query.TrendStart; day.Before(query.TrendEnd); day = day.AddDate(0, 0, 1) {
		statistics.Trend = append(statistics.Trend, models.AdminDashboardDaily{Date: dashboardDay(day, location)})
	}
	return statistics
}

func dashboardDailyIndex(items []models.AdminDashboardDaily) map[string]int {
	index := make(map[string]int, len(items))
	for i := range items {
		index[items[i].Date] = i
	}
	return index
}

func distributionItems(counts map[string]int64, limit int) []models.AdminDashboardDistributionItem {
	items := make([]models.AdminDashboardDistributionItem, 0, len(counts))
	for key, count := range counts {
		if strings.TrimSpace(key) == "" {
			key = "unknown"
		}
		items = append(items, models.AdminDashboardDistributionItem{Key: key, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Key < items[j].Key
		}
		return items[i].Count > items[j].Count
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items
}

type memoryAdminDashboardRepository struct {
	deps adminDashboardDependencies
}

func (r *memoryAdminDashboardRepository) GetAdminCompanionStatistics(_ context.Context, query models.AdminDashboardQuery) (*models.AdminDashboardCompanionStats, error) {
	companions, ok := r.deps.companions.(*InMemoryCompanionRepository)
	if !ok {
		return nil, errAdminStatisticsRepositoriesUnavailable
	}
	result := &models.AdminDashboardCompanionStats{}
	var roomsWithMembers int64
	companions.mu.RLock()
	defer companions.mu.RUnlock()
	for _, item := range companions.sessions {
		if item.Status == models.CompanionSessionStatusActive {
			result.Active++
		}
		if dashboardBetween(item.StartedAt, query.Start, query.End) {
			result.Created.Current++
			if n := len(companions.members[item.SessionID]); n > 0 {
				result.AvgParticipants += float64(n)
				roomsWithMembers++
			}
		}
		if dashboardBetween(item.StartedAt, query.PreviousStart, query.PreviousEnd) {
			result.Created.Previous++
		}
		if item.Status == models.CompanionSessionStatusEnded && dashboardBetween(item.EndedAt, query.Start, query.End) {
			result.Ended++
			if item.EndSource == models.CompanionSessionEndSourceAutoClose {
				result.AutoEnded++
			}
		}
	}
	if roomsWithMembers > 0 {
		result.AvgParticipants /= float64(roomsWithMembers)
	}
	return result, nil
}

func (r *memoryAdminDashboardRepository) GetAdminDashboardStatistics(_ context.Context, query models.AdminDashboardQuery) (*models.AdminDashboardStatistics, error) {
	statistics := newDashboardStatistics(query)
	dailyIndex := dashboardDailyIndex(statistics.Trend)
	location := query.Now.Location()

	users, _ := r.deps.users.(*InMemoryUserRepository)
	tracks, _ := r.deps.tracks.(*InMemoryTrackRepository)
	logs, _ := r.deps.loginLogs.(*InMemoryLoginLogRepository)
	collects, _ := r.deps.collects.(*InMemoryCollectRepository)
	navigations, _ := r.deps.navigations.(*InMemoryNavigationRepository)
	companions, _ := r.deps.companions.(*InMemoryCompanionRepository)
	feedback, _ := r.deps.feedback.(*InMemoryFeedbackRepository)
	trackMap, _ := r.deps.trackMap.(*InMemoryTrackMapRepository)
	submissions, _ := r.deps.submissions.(*InMemoryTrackSubmissionRepository)
	likes, _ := r.deps.likes.(*InMemoryTrackLikeRepository)
	comments, _ := r.deps.comments.(*MemoryCommentRepository)

	cohort := make(map[int64]struct{})
	if users != nil {
		users.mu.RLock()
		for _, user := range users.users {
			if dashboardBetween(user.CreatedAt, query.Start, query.End) {
				statistics.Metrics.NewUsers.Current++
				cohort[user.ID] = struct{}{}
			}
			if dashboardBetween(user.CreatedAt, query.PreviousStart, query.PreviousEnd) {
				statistics.Metrics.NewUsers.Previous++
			}
			if index, ok := dailyIndex[dashboardDay(user.CreatedAt, location)]; ok {
				statistics.Trend[index].NewUsers++
			}
		}
		users.mu.RUnlock()
	}
	statistics.Funnel.Registered = int64(len(cohort))

	activeCurrent := make(map[int64]struct{})
	activePrevious := make(map[int64]struct{})
	dailyActive := make(map[string]map[int64]struct{})
	if logs != nil {
		logs.mu.RLock()
		for _, item := range logs.ordered {
			if dashboardBetween(item.CreatedAt, query.Start, query.End) {
				activeCurrent[item.UserID] = struct{}{}
			}
			if dashboardBetween(item.CreatedAt, query.PreviousStart, query.PreviousEnd) {
				activePrevious[item.UserID] = struct{}{}
			}
			day := dashboardDay(item.CreatedAt, location)
			if _, ok := dailyIndex[day]; ok {
				if dailyActive[day] == nil {
					dailyActive[day] = make(map[int64]struct{})
				}
				dailyActive[day][item.UserID] = struct{}{}
			}
		}
		logs.mu.RUnlock()
	}
	statistics.Metrics.ActiveUsers = models.AdminDashboardComparison{Current: int64(len(activeCurrent)), Previous: int64(len(activePrevious))}
	for day, userIDs := range dailyActive {
		statistics.Trend[dailyIndex[day]].ActiveUsers = int64(len(userIDs))
	}

	uploadCurrentUsers := make(map[int64]struct{})
	uploadPreviousUsers := make(map[int64]struct{})
	funnelUploaded := make(map[int64]struct{})
	trackTypeCounts := make(map[string]int64)
	trackCityCounts := make(map[string]int64)
	trackStatusCounts := make(map[string]int64)
	trackByID := make(map[string]*models.Track)
	var distanceTotal float64
	var durationTotal float64
	var completedCount int64
	if tracks != nil {
		tracks.mu.RLock()
		for id, track := range tracks.tracks {
			clone := *track
			trackByID[id] = &clone
			completed := !track.IsRunning && strings.TrimSpace(track.RawTrackURL) != ""
			if completed && dashboardBetween(track.CreatedAt, query.Start, query.End) {
				statistics.Metrics.UploadedTracks.Current++
				uploadCurrentUsers[track.UserID] = struct{}{}
				trackTypeCounts[track.TrackType]++
				trackCityCounts[track.CityCode]++
				trackStatusCounts[trackStatusKey(track.Status)]++
				distanceTotal += track.Distance
				durationTotal += float64(track.Duration)
				completedCount++
				if _, ok := cohort[track.UserID]; ok {
					funnelUploaded[track.UserID] = struct{}{}
				}
			}
			if completed && dashboardBetween(track.CreatedAt, query.PreviousStart, query.PreviousEnd) {
				statistics.Metrics.UploadedTracks.Previous++
				uploadPreviousUsers[track.UserID] = struct{}{}
			}
			if completed {
				if index, ok := dailyIndex[dashboardDay(track.CreatedAt, location)]; ok {
					statistics.Trend[index].UploadedTracks++
				}
				if track.Status != models.TrackStatusDeleted && track.Distance <= 0 {
					statistics.Content.ZeroDistance++
				}
			}
			if track.Status != models.TrackStatusDeleted && track.IsRunning && track.CreatedAt.Before(query.Now.Add(-24*time.Hour)) {
				statistics.Content.StaleRunning++
			}
		}
		tracks.mu.RUnlock()
	}
	statistics.Metrics.UploadUsers = models.AdminDashboardComparison{Current: int64(len(uploadCurrentUsers)), Previous: int64(len(uploadPreviousUsers))}
	statistics.Funnel.Uploaded = int64(len(funnelUploaded))
	statistics.Content.Statuses = distributionItems(trackStatusCounts, 0)
	statistics.Content.Types = distributionItems(trackTypeCounts, 8)
	statistics.Content.Cities = distributionItems(trackCityCounts, 10)
	if completedCount > 0 {
		statistics.Content.AvgDistanceM = distanceTotal / float64(completedCount)
		statistics.Content.AvgDurationS = durationTotal / float64(completedCount)
	}

	funnelSubmitted := make(map[int64]struct{})
	funnelApproved := make(map[int64]struct{})
	if submissions != nil {
		submissions.mu.RLock()
		for _, item := range submissions.byID {
			if dashboardBetween(item.SubmittedAt, query.Start, query.End) {
				statistics.Metrics.Submissions.Current++
				if _, ok := funnelUploaded[item.UserID]; ok {
					funnelSubmitted[item.UserID] = struct{}{}
				}
			}
			if dashboardBetween(item.SubmittedAt, query.PreviousStart, query.PreviousEnd) {
				statistics.Metrics.Submissions.Previous++
			}
			if item.Status == models.TrackSubmissionStatusPending {
				statistics.Review.Pending++
				age := query.Now.Sub(item.SubmittedAt)
				if age >= 24*time.Hour {
					statistics.Review.PendingOver24H++
				}
				if age >= 72*time.Hour {
					statistics.Review.PendingOver72H++
				}
				if statistics.Review.OldestPending == "" || item.SubmittedAt.Format(time.RFC3339) < statistics.Review.OldestPending {
					statistics.Review.OldestPending = item.SubmittedAt.Format(time.RFC3339)
				}
			}
			if item.ReviewedAt != nil && dashboardBetween(*item.ReviewedAt, query.Start, query.End) {
				switch item.Status {
				case models.TrackSubmissionStatusApproved:
					statistics.Review.Approved++
					if _, ok := funnelSubmitted[item.UserID]; ok {
						funnelApproved[item.UserID] = struct{}{}
					}
				case models.TrackSubmissionStatusRejected:
					statistics.Review.Rejected++
				}
				statistics.Review.AvgReviewHours += item.ReviewedAt.Sub(item.SubmittedAt).Hours()
			}
		}
		submissions.mu.RUnlock()
	}
	statistics.Metrics.PendingSubmissions = statistics.Review.Pending
	statistics.Funnel.Submitted = int64(len(funnelSubmitted))
	statistics.Funnel.Approved = int64(len(funnelApproved))
	reviewed := statistics.Review.Approved + statistics.Review.Rejected
	if reviewed > 0 {
		statistics.Review.ApprovalRate = float64(statistics.Review.Approved) / float64(reviewed)
		statistics.Review.AvgReviewHours /= float64(reviewed)
	}

	platformCounts := make(map[string]int64)
	versionCounts := make(map[string]int64)
	var resolvedHours float64
	if feedback != nil {
		feedback.mu.RLock()
		for _, item := range feedback.feedbacks {
			switch item.Status {
			case models.FeedbackStatusPending:
				statistics.Feedback.Pending++
			case models.FeedbackStatusProcessing:
				statistics.Feedback.Processing++
			case models.FeedbackStatusResolved:
				statistics.Feedback.Resolved++
			case models.FeedbackStatusIgnored:
				statistics.Feedback.Ignored++
			}
			if item.Status == models.FeedbackStatusPending || item.Status == models.FeedbackStatusProcessing {
				age := query.Now.Sub(item.CreatedAt)
				if age >= 24*time.Hour {
					statistics.Feedback.PendingOver24H++
				}
				if age >= 72*time.Hour {
					statistics.Feedback.PendingOver72H++
				}
			}
			if dashboardBetween(item.CreatedAt, query.Start, query.End) {
				statistics.Feedback.Created.Current++
				platformCounts[item.Platform]++
				versionCounts[item.AppVersion]++
			}
			if dashboardBetween(item.CreatedAt, query.PreviousStart, query.PreviousEnd) {
				statistics.Feedback.Created.Previous++
			}
			if item.Status == models.FeedbackStatusResolved && dashboardBetween(item.UpdatedAt, query.Start, query.End) {
				statistics.Feedback.ResolvedInPeriod++
				resolvedHours += item.UpdatedAt.Sub(item.CreatedAt).Hours()
			}
		}
		feedback.mu.RUnlock()
	}
	statistics.Metrics.PendingFeedback = statistics.Feedback.Pending + statistics.Feedback.Processing
	if statistics.Feedback.ResolvedInPeriod > 0 {
		statistics.Feedback.AvgResolutionHours = resolvedHours / float64(statistics.Feedback.ResolvedInPeriod)
	}
	statistics.Feedback.Platforms = distributionItems(platformCounts, 6)
	statistics.Feedback.Versions = distributionItems(versionCounts, 8)

	trackEngagement := make(map[string]*models.AdminDashboardTopTrack)
	ensureTrack := func(trackID string) *models.AdminDashboardTopTrack {
		item := trackEngagement[trackID]
		if item == nil {
			item = &models.AdminDashboardTopTrack{TrackID: trackID}
			if track := trackByID[trackID]; track != nil {
				item.Title = track.Title
			}
			trackEngagement[trackID] = item
		}
		return item
	}
	if collects != nil {
		collects.mu.RLock()
		for _, byTrack := range collects.collects {
			for trackID, item := range byTrack {
				ensureTrack(trackID).Collections++
				addEngagementCount(&statistics.Engagement, &statistics.Trend, dailyIndex, location, item.CreatedAt, query, "collection")
			}
		}
		collects.mu.RUnlock()
	}
	if navigations != nil {
		navigations.mu.RLock()
		for trackID, items := range navigations.byTrack {
			for _, item := range items {
				ensureTrack(trackID).Navigations++
				addEngagementCount(&statistics.Engagement, &statistics.Trend, dailyIndex, location, item.CreatedAt, query, "navigation")
			}
		}
		navigations.mu.RUnlock()
	}
	if likes != nil {
		likes.mu.RLock()
		for trackID, usersByID := range likes.likes {
			for _, item := range usersByID {
				ensureTrack(trackID).Likes++
				addEngagementCount(&statistics.Engagement, &statistics.Trend, dailyIndex, location, item.CreatedAt, query, "like")
			}
		}
		likes.mu.RUnlock()
	}
	if comments != nil {
		comments.mu.RLock()
		for _, item := range comments.comments {
			if item.Deleted {
				continue
			}
			ensureTrack(item.TrackID).Comments++
			addEngagementCount(&statistics.Engagement, &statistics.Trend, dailyIndex, location, item.CreatedAt, query, "comment")
		}
		comments.mu.RUnlock()
	}
	for _, item := range trackEngagement {
		item.Score = item.Collections + item.Navigations + item.Likes + item.Comments
		if track := trackByID[item.TrackID]; track != nil && track.Status == models.TrackStatusNormal && !track.IsRunning {
			statistics.Engagement.TopTracks = append(statistics.Engagement.TopTracks, *item)
		}
	}
	sort.Slice(statistics.Engagement.TopTracks, func(i, j int) bool {
		if statistics.Engagement.TopTracks[i].Score == statistics.Engagement.TopTracks[j].Score {
			return statistics.Engagement.TopTracks[i].TrackID < statistics.Engagement.TopTracks[j].TrackID
		}
		return statistics.Engagement.TopTracks[i].Score > statistics.Engagement.TopTracks[j].Score
	})
	if len(statistics.Engagement.TopTracks) > 10 {
		statistics.Engagement.TopTracks = statistics.Engagement.TopTracks[:10]
	}

	var roomsWithMembers int64
	if companions != nil {
		companions.mu.RLock()
		for _, item := range companions.sessions {
			if item.Status == models.CompanionSessionStatusActive {
				statistics.Companion.Active++
			}
			if dashboardBetween(item.StartedAt, query.Start, query.End) {
				statistics.Companion.Created.Current++
				if n := len(companions.members[item.SessionID]); n > 0 {
					statistics.Companion.AvgParticipants += float64(n)
					roomsWithMembers++
				}
			}
			if dashboardBetween(item.StartedAt, query.PreviousStart, query.PreviousEnd) {
				statistics.Companion.Created.Previous++
			}
			if item.Status == models.CompanionSessionStatusEnded && dashboardBetween(item.EndedAt, query.Start, query.End) {
				statistics.Companion.Ended++
				if item.EndSource == models.CompanionSessionEndSourceAutoClose {
					statistics.Companion.AutoEnded++
				}
			}
			if index, ok := dailyIndex[dashboardDay(item.StartedAt, location)]; ok {
				statistics.Trend[index].CompanionSessions++
			}
		}
		companions.mu.RUnlock()
	}
	if roomsWithMembers > 0 {
		statistics.Companion.AvgParticipants /= float64(roomsWithMembers)
	}
	statistics.Metrics.ActiveCompanions = statistics.Companion.Active

	if trackMap != nil {
		trackMap.mu.RLock()
		for _, job := range trackMap.jobs {
			switch job.Status {
			case models.TrackMapIndexJobFailed:
				statistics.Content.FailedMapJobs++
			case models.TrackMapIndexJobPending, models.TrackMapIndexJobProcessing:
				statistics.Content.PendingMapJobs++
			}
		}
		for _, group := range trackMap.routeGroups {
			if group.Status == models.TrackRouteGroupStatusActive {
				statistics.Content.ActiveRouteGroups++
			}
		}
		trackMap.mu.RUnlock()
	}
	return statistics, nil
}

func trackStatusKey(status models.TrackStatus) string {
	switch status {
	case models.TrackStatusNormal:
		return "normal"
	case models.TrackStatusPrivate:
		return "private"
	default:
		return "deleted"
	}
}

func addEngagementCount(stats *models.AdminDashboardEngagementStats, daily *[]models.AdminDashboardDaily, index map[string]int, location *time.Location, createdAt time.Time, query models.AdminDashboardQuery, kind string) {
	var current, previous *int64
	switch kind {
	case "collection":
		current, previous = &stats.Current.Collections, &stats.Previous.Collections
	case "navigation":
		current, previous = &stats.Current.Navigations, &stats.Previous.Navigations
	case "like":
		current, previous = &stats.Current.Likes, &stats.Previous.Likes
	case "comment":
		current, previous = &stats.Current.Comments, &stats.Previous.Comments
	default:
		return
	}
	if dashboardBetween(createdAt, query.Start, query.End) {
		*current++
	}
	if dashboardBetween(createdAt, query.PreviousStart, query.PreviousEnd) {
		*previous++
	}
	if dailyIndex, ok := index[dashboardDay(createdAt, location)]; ok {
		switch kind {
		case "collection":
			(*daily)[dailyIndex].Collections++
		case "navigation":
			(*daily)[dailyIndex].Navigations++
		case "like":
			(*daily)[dailyIndex].Likes++
		case "comment":
			(*daily)[dailyIndex].Comments++
		}
	}
}
