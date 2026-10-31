package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

func zodiacFromMonthDay(value string) string {
	switch {
	case value < "01-20" || value >= "12-22":
		return "capricorn"
	case value < "02-19":
		return "aquarius"
	case value < "03-21":
		return "pisces"
	case value < "04-20":
		return "aries"
	case value < "05-21":
		return "taurus"
	case value < "06-21":
		return "gemini"
	case value < "07-23":
		return "cancer"
	case value < "08-23":
		return "leo"
	case value < "09-23":
		return "virgo"
	case value < "10-23":
		return "libra"
	case value < "11-22":
		return "scorpio"
	default:
		return "sagittarius"
	}
}

func buildAdminUserDistributionStats(total int64, genderCounts, birthdayCounts, cityCounts, trackOutputCounts map[string]int64, now time.Time) *models.AdminUserDistributionStats {
	gender := map[string]int64{"male": 0, "female": 0, "unknown": 0}
	for key, count := range genderCounts {
		if key == "male" || key == "female" {
			gender[key] += count
		} else {
			gender["unknown"] += count
		}
	}

	zodiacCounts := map[string]int64{
		"capricorn": 0, "aquarius": 0, "pisces": 0, "aries": 0,
		"taurus": 0, "gemini": 0, "cancer": 0, "leo": 0,
		"virgo": 0, "libra": 0, "scorpio": 0, "sagittarius": 0,
		"unknown": 0,
	}
	ageCounts := make(map[string]int64)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	var usersWithBirthday int64
	for value, count := range birthdayCounts {
		birthday, err := time.Parse(time.DateOnly, value)
		if err != nil || birthday.Year() < 1900 || birthday.After(today) {
			continue
		}
		monthDay := birthday.Format("01-02")
		zodiacCounts[zodiacFromMonthDay(monthDay)] += count
		usersWithBirthday += count
		age := today.Year() - birthday.Year()
		if today.Format("01-02") < monthDay {
			age--
		}
		key := "55_plus"
		switch {
		case age < 18:
			key = "under_18"
		case age < 25:
			key = "18_24"
		case age < 35:
			key = "25_34"
		case age < 45:
			key = "35_44"
		case age < 55:
			key = "45_54"
		}
		ageCounts[key] += count
	}
	if total > usersWithBirthday {
		zodiacCounts["unknown"] = total - usersWithBirthday
		ageCounts["unknown"] = total - usersWithBirthday
	}
	var usersWithTracks int64
	for _, key := range []string{"one", "2_5", "6_20", "21_plus"} {
		usersWithTracks += trackOutputCounts[key]
	}
	trackOutputCounts["zero"] = max(int64(0), total-usersWithTracks)

	toItems := func(counts map[string]int64, order []string) []models.AdminDashboardDistributionItem {
		items := make([]models.AdminDashboardDistributionItem, 0, len(order))
		for _, key := range order {
			items = append(items, models.AdminDashboardDistributionItem{Key: key, Count: counts[key]})
		}
		return items
	}
	cityItems := distributionItems(cityCounts, 0)
	return &models.AdminUserDistributionStats{
		TotalUsers:  total,
		Gender:      toItems(gender, []string{"male", "female", "unknown"}),
		Zodiac:      toItems(zodiacCounts, []string{"aries", "taurus", "gemini", "cancer", "leo", "virgo", "libra", "scorpio", "sagittarius", "capricorn", "aquarius", "pisces", "unknown"}),
		UserCities:  cityItems,
		Age:         toItems(ageCounts, []string{"under_18", "18_24", "25_34", "35_44", "45_54", "55_plus", "unknown"}),
		TrackOutput: toItems(trackOutputCounts, []string{"zero", "one", "2_5", "6_20", "21_plus"}),
	}
}

func adminTrackOutputBucket(count int64) string {
	switch {
	case count == 0:
		return "zero"
	case count == 1:
		return "one"
	case count <= 5:
		return "2_5"
	case count <= 20:
		return "6_20"
	default:
		return "21_plus"
	}
}

func (r *memoryAdminDashboardRepository) GetAdminUserDistributionStatistics(_ context.Context) (*models.AdminUserDistributionStats, error) {
	users, okUsers := r.deps.users.(*InMemoryUserRepository)
	if !okUsers {
		return nil, errAdminStatisticsRepositoriesUnavailable
	}
	tracks, okTracks := r.deps.tracks.(*InMemoryTrackRepository)
	if !okTracks {
		return nil, errAdminStatisticsRepositoriesUnavailable
	}
	userTrackCounts := make(map[int64]int64)
	tracks.mu.RLock()
	for _, track := range tracks.tracks {
		if track != nil && track.Status != models.TrackStatusDeleted && track.DeletedAt.IsZero() && !track.IsRunning && track.RawTrackURL != "" {
			userTrackCounts[track.UserID]++
		}
	}
	tracks.mu.RUnlock()

	genderCounts := make(map[string]int64)
	birthdayCounts := make(map[string]int64)
	cityCounts := make(map[string]int64)
	trackOutputCounts := make(map[string]int64)
	users.mu.RLock()
	total := int64(len(users.users))
	for _, user := range users.users {
		if user == nil {
			continue
		}
		genderCounts[user.Gender]++
		birthdayCounts[user.Birthday]++
		if count := userTrackCounts[user.ID]; count > 0 {
			trackOutputCounts[adminTrackOutputBucket(count)]++
		}
		city := strings.TrimSpace(user.CityCode)
		if city == "" {
			city = "unknown"
		}
		cityCounts[city]++
	}
	users.mu.RUnlock()
	return buildAdminUserDistributionStats(total, genderCounts, birthdayCounts, cityCounts, trackOutputCounts, time.Now()), nil
}

func (r *memoryAdminDashboardRepository) GetAdminUserLastLoginTimes(_ context.Context, userIDs []int64) (map[int64]time.Time, error) {
	result := make(map[int64]time.Time, len(userIDs))
	logs, ok := r.deps.loginLogs.(*InMemoryLoginLogRepository)
	if !ok {
		return result, nil
	}
	logs.mu.RLock()
	defer logs.mu.RUnlock()
	for _, userID := range userIDs {
		items := logs.byUser[userID]
		if len(items) == 0 {
			continue
		}
		latest := items[0].CreatedAt
		for _, item := range items[1:] {
			if item.CreatedAt.After(latest) {
				latest = item.CreatedAt
			}
		}
		result[userID] = latest
	}
	return result, nil
}

var errAdminStatisticsRepositoriesUnavailable = errors.New("admin statistics repositories unavailable")
