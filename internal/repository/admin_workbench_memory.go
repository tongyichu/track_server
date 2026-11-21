package repository

import (
	"context"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

func memoryWorkbenchTrack(track *models.Track) bool {
	return track.Status != models.TrackStatusDeleted && !track.IsRunning && track.DeletedAt.IsZero() && strings.TrimSpace(track.RawTrackURL) != ""
}

func (r *memoryAdminDashboardRepository) WorkbenchCities(_ context.Context, q models.AdminDashboardQuery) ([]models.WorkbenchCity, error) {
	result := newWorkbenchCities(q)
	if tracks, _ := r.deps.tracks.(*InMemoryTrackRepository); tracks != nil {
		tracks.mu.RLock()
		for _, t := range tracks.tracks {
			if t.Status == models.TrackStatusNormal && memoryWorkbenchTrack(t) {
				result.addTrack(t.ID, t.CityCode, t.CreatedAt)
			}
		}
		tracks.mu.RUnlock()
	}
	if collects, _ := r.deps.collects.(*InMemoryCollectRepository); collects != nil {
		collects.mu.RLock()
		for _, tracks := range collects.collects {
			for _, collect := range tracks {
				if dashboardBetween(collect.CreatedAt, q.Start, q.End) {
					result.addCollect(collect.TrackID, collect.UserID)
				}
			}
		}
		collects.mu.RUnlock()
	}
	if navs, _ := r.deps.navigations.(*InMemoryNavigationRepository); navs != nil {
		navs.mu.RLock()
		for trackID, entries := range navs.byTrack {
			for _, item := range entries {
				result.addNavigation(trackID, item.CreatedAt)
			}
		}
		navs.mu.RUnlock()
	}
	return result.rows(), nil
}

func (r *memoryAdminDashboardRepository) WorkbenchCohorts(_ context.Context, now time.Time) ([]models.WorkbenchCohort, error) {
	result := newWorkbenchCohorts(now)
	if users, _ := r.deps.users.(*InMemoryUserRepository); users != nil {
		users.mu.RLock()
		for _, user := range users.users {
			result.addUser(user.ID, user.CreatedAt)
		}
		users.mu.RUnlock()
	}
	if tracks, _ := r.deps.tracks.(*InMemoryTrackRepository); tracks != nil {
		tracks.mu.RLock()
		for _, track := range tracks.tracks {
			if memoryWorkbenchTrack(track) {
				result.addTrack(track.UserID, track.CreatedAt)
			}
		}
		tracks.mu.RUnlock()
	}
	if logs, _ := r.deps.loginLogs.(*InMemoryLoginLogRepository); logs != nil {
		logs.mu.RLock()
		for userID := range result.users {
			for _, attempt := range logs.byUser[userID] {
				result.addLogin(userID, attempt.CreatedAt)
			}
		}
		logs.mu.RUnlock()
	}
	return result.rows(), nil
}

func (r *memoryAdminDashboardRepository) WorkbenchVersions(_ context.Context, since time.Time, includeFeedback bool) (models.WorkbenchVersionCounts, error) {
	var result models.WorkbenchVersionCounts
	if stats, _ := r.deps.appClients.(*InMemoryAppClientStatsRepository); stats != nil {
		result.Devices = make(map[string]int64)
		stats.mu.RLock()
		for _, device := range stats.devices {
			if device.Platform == "android" && !device.LastSeenAt.Before(since) {
				result.Devices[workbenchVersionName(device.AppVersion)]++
			}
		}
		stats.mu.RUnlock()
	}
	if feedback, _ := r.deps.feedback.(*InMemoryFeedbackRepository); feedback != nil && includeFeedback {
		result.Feedback = make(map[string]int64)
		feedback.mu.RLock()
		for _, item := range feedback.feedbacks {
			if item.Platform == "android" && !item.CreatedAt.Before(since) {
				result.Feedback[workbenchVersionName(item.AppVersion)]++
			}
		}
		feedback.mu.RUnlock()
	}
	return result, nil
}

func (r *memoryAdminDashboardRepository) WorkbenchCompanions(_ context.Context, q models.AdminDashboardQuery) ([]models.WorkbenchCompanion, error) {
	result := newWorkbenchCompanions()
	repo, _ := r.deps.companions.(*InMemoryCompanionRepository)
	if repo == nil {
		return result.rows(), nil
	}
	repo.mu.RLock()
	for id, room := range repo.sessions {
		if room.Status == models.CompanionSessionStatusEnded && dashboardBetween(room.EndedAt, q.Start, q.End) {
			result.addRoom(id, room.TrackType)
		}
	}
	members := make(map[workbenchMember]string)
	manifests := make(map[workbenchMember]string)
	for id := range result.rooms {
		for user := range repo.members[id] {
			members[workbenchMember{id, user}] = ""
		}
	}
	for _, chunk := range repo.trackChunks {
		key := workbenchMember{chunk.SessionID, chunk.UserID}
		if _, relevant := members[key]; relevant && chunk.Index == 0 {
			manifests[key] = chunk.Data
		}
	}
	repo.mu.RUnlock()
	tracks := make(map[string]*models.Track)
	if repo, _ := r.deps.tracks.(*InMemoryTrackRepository); repo != nil {
		repo.mu.RLock()
		for _, payload := range manifests {
			if id, valid := workbenchManifestTrack(payload); valid {
				if track := repo.tracks[*id]; track != nil {
					snapshot := *track
					tracks[*id] = &snapshot
				}
			}
		}
		repo.mu.RUnlock()
	}
	for member := range members {
		payload, found := manifests[member]
		result.addMember(member.room, payload, found, member.user, tracks)
	}
	return result.rows(), nil
}

func (r *memoryAdminDashboardRepository) WorkbenchBacklogs(_ context.Context, now time.Time, scopes []string) ([]models.WorkbenchBacklog, error) {
	rows := make([]models.WorkbenchBacklog, 0, 4)
	if repo, _ := r.deps.submissions.(*InMemoryTrackSubmissionRepository); repo != nil && hasWorkbenchBacklog(scopes, backlogReview) {
		row := newWorkbenchBacklog("review", true)
		repo.mu.RLock()
		for _, s := range repo.byID {
			if s.Status == models.TrackSubmissionStatusPending {
				countWorkbenchBacklog(&row, s.SubmittedAt, s.SubmittedAt.Add(24*time.Hour), now)
			}
		}
		repo.mu.RUnlock()
		rows = append(rows, row)
	}
	if repo, _ := r.deps.feedback.(*InMemoryFeedbackRepository); repo != nil && hasWorkbenchBacklog(scopes, backlogFeedback) {
		row := newWorkbenchBacklog("feedback", true)
		repo.mu.RLock()
		for _, f := range repo.feedbacks {
			if f.Status == models.FeedbackStatusPending || f.Status == models.FeedbackStatusProcessing {
				countWorkbenchBacklog(&row, f.CreatedAt, f.CreatedAt.Add(24*time.Hour), now)
			}
		}
		repo.mu.RUnlock()
		rows = append(rows, row)
	}
	if repo, _ := r.deps.trackMap.(*InMemoryTrackMapRepository); repo != nil && hasWorkbenchBacklog(scopes, backlogIndex) {
		waiting, failed := newWorkbenchBacklog("index", true), newWorkbenchBacklog("index_failed", false)
		repo.mu.RLock()
		for _, job := range repo.jobs {
			switch job.Status {
			case models.TrackMapIndexJobPending, models.TrackMapIndexJobProcessing:
				countWorkbenchBacklog(&waiting, job.CreatedAt, job.NextRunAt.Add(24*time.Hour), now)
			case models.TrackMapIndexJobFailed:
				countWorkbenchBacklog(&failed, job.CreatedAt, time.Time{}, now)
			}
		}
		repo.mu.RUnlock()
		rows = append(rows, waiting, failed)
	}
	return rows, nil
}
