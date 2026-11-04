package service

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/repository"
)

type ActivityDay struct {
	Date                string  `json:"date"`
	Count               int     `json:"count"`
	DistanceMeters      float64 `json:"distance_meters"`
	DurationSeconds     int64   `json:"duration_seconds"`
	ElevationGainMeters int64   `json:"elevation_gain_meters"`
	StepCount           *int64  `json:"step_count"`
	StepTrackCount      int     `json:"step_track_count"`
}

type ActivityCalendar struct {
	StartDate      string        `json:"start_date"`
	EndDate        string        `json:"end_date"`
	TotalCount     int           `json:"total_count"`
	TrackType      string        `json:"track_type"`
	MetricsVersion int           `json:"metrics_version"`
	Days           []ActivityDay `json:"days"`
}

func (s *TrackService) ActivityCalendar(ctx context.Context, userID int64, timezone string, trackTypes ...string) (*ActivityCalendar, error) {
	if userID <= 0 {
		return nil, invalidArg("userID is required")
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, invalidArg("invalid timezone")
	}
	selected := ""
	if len(trackTypes) > 0 {
		selected = normalizeTrackTypeCode(strings.TrimSpace(trackTypes[0]))
	}
	if len(selected) > 32 {
		return nil, invalidArg("invalid track_type")
	}
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	from, until := today.AddDate(0, 0, -374), today.AddDate(0, 0, 1)
	repo, ok := s.tracks.(repository.ActivityCalendarRepository)
	if !ok {
		return nil, errors.New("activity calendar statistics unavailable")
	}
	records, err := repo.ListActivityRecords(ctx, userID, from, until)
	if err != nil {
		return nil, err
	}
	days := make(map[string]*ActivityDay)
	result := &ActivityCalendar{StartDate: from.Format("2006-01-02"), EndDate: today.Format("2006-01-02"), TrackType: selected, MetricsVersion: 1, Days: make([]ActivityDay, 0)}
	for _, record := range records {
		code := normalizeTrackTypeCode(record.TrackType)
		if code == "" {
			code = "other"
		}
		if selected != "" && selected != code {
			continue
		}
		date := record.StartTime.In(loc).Format("2006-01-02")
		day := days[date]
		if day == nil {
			day = &ActivityDay{Date: date}
			days[date] = day
		}
		day.Count++
		result.TotalCount++
		if record.Distance > 0 && !math.IsNaN(record.Distance) && !math.IsInf(record.Distance, 0) {
			day.DistanceMeters += record.Distance
		}
		day.DurationSeconds += max(record.Duration, 0)
		day.ElevationGainMeters += max(record.ElevationGain, 0)
		if record.StepCount != nil && *record.StepCount >= 0 && code != "driving" && code != "riding" {
			if day.StepCount == nil {
				day.StepCount = new(int64)
			}
			*day.StepCount += *record.StepCount
			day.StepTrackCount++
		}
	}
	for date := from; !date.After(today); date = date.AddDate(0, 0, 1) {
		if day := days[date.Format("2006-01-02")]; day != nil {
			result.Days = append(result.Days, *day)
		}
	}
	return result, nil
}
