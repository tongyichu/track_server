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

func buildAdminUserDistributionStats(total int64, genderCounts, birthdayCounts, cityCounts map[string]int64) *models.AdminUserDistributionStats {
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
	for monthDay, count := range birthdayCounts {
		if _, err := time.Parse("01-02", monthDay); err != nil {
			zodiacCounts["unknown"] += count
			continue
		}
		zodiacCounts[zodiacFromMonthDay(monthDay)] += count
	}
	var usersWithBirthday int64
	for _, count := range birthdayCounts {
		usersWithBirthday += count
	}
	if total > usersWithBirthday {
		zodiacCounts["unknown"] += total - usersWithBirthday
	}

	toItems := func(counts map[string]int64, order []string) []models.AdminDashboardDistributionItem {
		items := make([]models.AdminDashboardDistributionItem, 0, len(order))
		for _, key := range order {
			items = append(items, models.AdminDashboardDistributionItem{Key: key, Count: counts[key]})
		}
		return items
	}
	cityItems := distributionItems(cityCounts, 10)
	return &models.AdminUserDistributionStats{
		TotalUsers:  total,
		Gender:      toItems(gender, []string{"male", "female", "unknown"}),
		Zodiac:      toItems(zodiacCounts, []string{"aries", "taurus", "gemini", "cancer", "leo", "virgo", "libra", "scorpio", "sagittarius", "capricorn", "aquarius", "pisces", "unknown"}),
		TrackCities: cityItems,
	}
}

func (r *memoryAdminDashboardRepository) GetAdminUserDistributionStatistics(_ context.Context) (*models.AdminUserDistributionStats, error) {
	users, okUsers := r.deps.users.(*InMemoryUserRepository)
	tracks, okTracks := r.deps.tracks.(*InMemoryTrackRepository)
	if !okUsers || !okTracks {
		return nil, errAdminStatisticsRepositoriesUnavailable
	}

	genderCounts := make(map[string]int64)
	birthdayCounts := make(map[string]int64)
	users.mu.RLock()
	total := int64(len(users.users))
	for _, user := range users.users {
		if user == nil {
			continue
		}
		genderCounts[user.Gender]++
		if birthday, err := time.Parse("2006-01-02", user.Birthday); err == nil && !birthday.After(time.Now()) && birthday.Year() >= 1900 {
			birthdayCounts[birthday.Format("01-02")]++
		}
	}
	users.mu.RUnlock()

	cityUsers := make(map[string]map[int64]struct{})
	tracks.mu.RLock()
	for _, track := range tracks.tracks {
		if track == nil || !track.DeletedAt.IsZero() || track.Status == models.TrackStatusDeleted || track.IsRunning || strings.TrimSpace(track.RawTrackURL) == "" {
			continue
		}
		city := strings.TrimSpace(track.CityCode)
		if city == "" {
			city = "unknown"
		}
		if cityUsers[city] == nil {
			cityUsers[city] = make(map[int64]struct{})
		}
		cityUsers[city][track.UserID] = struct{}{}
	}
	tracks.mu.RUnlock()
	cityCounts := make(map[string]int64, len(cityUsers))
	for city, userIDs := range cityUsers {
		cityCounts[city] = int64(len(userIDs))
	}
	return buildAdminUserDistributionStats(total, genderCounts, birthdayCounts, cityCounts), nil
}

var errAdminStatisticsRepositoriesUnavailable = errors.New("admin statistics repositories unavailable")
