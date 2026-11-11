package service

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
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
	durationMillisByDate := make(map[string]int64)
	result := &ActivityCalendar{StartDate: from.Format("2006-01-02"), EndDate: today.Format("2006-01-02"), TrackType: selected, MetricsVersion: 2, Days: make([]ActivityDay, 0)}
	for _, record := range records {
		code := normalizeTrackTypeCode(record.TrackType)
		if code == "" {
			code = "other"
		}
		if selected != "" && selected != code {
			continue
		}
		slices := []models.RecordingInterval{{StartMillis: record.StartTime.UnixMilli(),
			EndMillis: record.StartTime.UnixMilli() + max(record.Duration, 0)*1000,
			Kind:      "recording", Distance: record.Distance, ElevationGain: record.ElevationGain}}
		if record.StepCount != nil {
			slices[0].Steps = *record.StepCount
		}
		if record.RecordingTimeline != nil {
			slices = record.RecordingTimeline.CalendarSlices(loc)
		}
		countedDates := make(map[string]bool)
		for _, slice := range slices {
			moment := time.UnixMilli(slice.StartMillis)
			if moment.Before(from) || !moment.Before(until) {
				continue
			}
			date := moment.In(loc).Format("2006-01-02")
			day := days[date]
			if day == nil {
				day = &ActivityDay{Date: date}
				days[date] = day
			}
			if !countedDates[date] {
				day.Count++
				if record.StepCount != nil && *record.StepCount >= 0 && code != "driving" && code != "riding" {
					day.StepTrackCount++
				}
				countedDates[date] = true
			}
			if slice.Distance > 0 && !math.IsNaN(slice.Distance) && !math.IsInf(slice.Distance, 0) {
				day.DistanceMeters += slice.Distance
			}
			durationMillisByDate[date] += max(slice.EndMillis-slice.StartMillis, 0)
			day.ElevationGainMeters += max(slice.ElevationGain, 0)
			if record.StepCount != nil && *record.StepCount >= 0 && code != "driving" && code != "riding" {
				if day.StepCount == nil {
					day.StepCount = new(int64)
				}
				*day.StepCount += max(slice.Steps, 0)
			}
		}
		if len(countedDates) > 0 {
			result.TotalCount++
		}
	}
	for date := from; !date.After(today); date = date.AddDate(0, 0, 1) {
		if day := days[date.Format("2006-01-02")]; day != nil {
			day.DurationSeconds = durationMillisByDate[day.Date] / 1000
			result.Days = append(result.Days, *day)
		}
	}
	return result, nil
}
