package service

import (
	"math"
	"time"
)

func (in CreateTrackInput) validateRecordingTimeline() error {
	timeline := in.RecordingTimeline
	if timeline == nil {
		return nil
	}
	if timeline.Version != 1 || len(timeline.Intervals) == 0 || len(timeline.Intervals) > 8192 {
		return invalidArg("invalid recording_timeline version or interval count")
	}
	if in.StartTime == nil || in.EndTime == nil || in.IsRunning == nil || *in.IsRunning {
		return invalidArg("recording_timeline requires a completed track with start_time and end_time")
	}
	first, last := timeline.Intervals[0], timeline.Intervals[len(timeline.Intervals)-1]
	if math.Abs(float64(first.StartMillis-in.StartTime.UnixMilli())) >= 1000 ||
		math.Abs(float64(last.EndMillis-in.EndTime.UnixMilli())) >= 1000 ||
		last.EndMillis-first.StartMillis > int64((10*366*24*time.Hour)/time.Millisecond) {
		return invalidArg("recording_timeline must match track start_time and end_time")
	}
	var distance float64
	var steps int64
	for i, interval := range timeline.Intervals {
		if interval.SegmentID < 0 || interval.StartMillis <= 0 || interval.EndMillis < interval.StartMillis ||
			!finiteNonNegative(interval.Distance) || interval.ElevationGain < 0 || interval.Steps < 0 ||
			interval.ElevationGain > math.MaxInt32 || interval.Steps > math.MaxInt32 {
			return invalidArg("invalid recording_timeline interval")
		}
		if i > 0 {
			previous := timeline.Intervals[i-1]
			if interval.StartMillis != previous.EndMillis || interval.SegmentID < previous.SegmentID ||
				(interval.SegmentID == previous.SegmentID && (interval.Kind != "recording" || previous.Kind != "recording")) {
				return invalidArg("recording_timeline intervals must be ordered and contiguous")
			}
		}
		switch interval.Kind {
		case "recording":
			distance += interval.Distance
			steps += interval.Steps
		case "paused", "interrupted":
			if interval.Distance != 0 || interval.ElevationGain != 0 || interval.Steps != 0 {
				return invalidArg("paused intervals cannot contain activity metrics")
			}
		default:
			return invalidArg("invalid recording_timeline interval kind")
		}
	}
	if in.Distance != nil && math.Abs(*in.Distance-distance) > math.Max(1, distance*0.001) {
		return invalidArg("recording_timeline distance does not match track distance")
	}
	if in.StepCount != nil && steps != *in.StepCount {
		return invalidArg("recording_timeline steps do not match step_count")
	}
	return nil
}

func finiteNonNegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}
