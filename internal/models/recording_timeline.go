package models

import "time"

type RecordingTimeline struct {
	Version       int                 `json:"version" bson:"version"`
	IncludePauses bool                `json:"include_pauses" bson:"include_pauses"`
	Intervals     []RecordingInterval `json:"intervals" bson:"intervals"`
}

type RecordingInterval struct {
	SegmentID     int     `json:"segment_id" bson:"segment_id"`
	StartMillis   int64   `json:"start_ms" bson:"start_ms"`
	EndMillis     int64   `json:"end_ms" bson:"end_ms"`
	Kind          string  `json:"kind" bson:"kind"`
	Distance      float64 `json:"distance_m" bson:"distance_m"`
	ElevationGain int64   `json:"elevation_gain_m" bson:"elevation_gain_m"`
	Steps         int64   `json:"steps" bson:"steps"`
}

func (t *RecordingTimeline) RecordedMillis() int64 {
	var total int64
	if t != nil {
		for _, interval := range t.Intervals {
			if interval.Kind == "recording" {
				total += max(interval.EndMillis-interval.StartMillis, 0)
			}
		}
	}
	return total
}

func (t *RecordingTimeline) SelectedSeconds() uint32 {
	if t == nil || len(t.Intervals) == 0 {
		return 0
	}
	if t.IncludePauses {
		return uint32(max(t.Intervals[len(t.Intervals)-1].EndMillis-t.Intervals[0].StartMillis, 0) / 1000)
	}
	return uint32(t.RecordedMillis() / 1000)
}

func (t *RecordingTimeline) Clone() *RecordingTimeline {
	if t == nil {
		return nil
	}
	copy := *t
	copy.Intervals = append([]RecordingInterval(nil), t.Intervals...)
	return &copy
}

// CalendarSlices splits only recorded intervals. Paused nights never create activity days.
func (t *RecordingTimeline) CalendarSlices(loc *time.Location) []RecordingInterval {
	if t == nil {
		return nil
	}
	var result []RecordingInterval
	for _, interval := range t.Intervals {
		if interval.Kind != "recording" || interval.EndMillis <= interval.StartMillis {
			continue
		}
		span := interval.EndMillis - interval.StartMillis
		var assignedElevation, assignedSteps int64
		var assignedDistance float64
		for start := interval.StartMillis; start < interval.EndMillis; {
			date := time.UnixMilli(start).In(loc)
			midnight := time.Date(date.Year(), date.Month(), date.Day()+1, 0, 0, 0, 0, loc).UnixMilli()
			end := min(interval.EndMillis, midnight)
			slice := interval
			slice.StartMillis, slice.EndMillis = start, end
			ratio := float64(end-start) / float64(span)
			slice.Distance = interval.Distance * ratio
			slice.ElevationGain = interval.ElevationGain * (end - start) / span
			slice.Steps = interval.Steps * (end - start) / span
			if end == interval.EndMillis {
				slice.Distance = interval.Distance - assignedDistance
				slice.ElevationGain = interval.ElevationGain - assignedElevation
				slice.Steps = interval.Steps - assignedSteps
			}
			assignedDistance += slice.Distance
			assignedElevation += slice.ElevationGain
			assignedSteps += slice.Steps
			result = append(result, slice)
			start = end
		}
	}
	return result
}
