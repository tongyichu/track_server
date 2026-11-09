package service

import (
	"context"
	"errors"
	"log"
	"time"
)

var errNarrativeTrackBusy = errors.New("这条轨迹已有 AI 任务正在执行，请在 AI 使用中查看或中断后重试")

// Manual previews must not hold the scan cursor lock. A scoped lease only prevents
// concurrent paid work for the same track, including automatic generation and learning.
func (s *TrackNarrativeService) leaseNarrativeTrack(ctx context.Context, id string) (func(), error) {
	owner, err := submissionID("NAR-TRACK-")
	if err != nil {
		return nil, err
	}
	key := "narrative:work:" + id
	now := time.Now().UTC()
	acquired, err := s.store.LeasePlaceRecord(ctx, key, owner, now, now.Add(5*time.Minute))
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, errNarrativeTrackBusy
	}
	return func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.saveState(cleanup, key, struct{}{}, owner); err != nil {
			log.Printf("[TrackNarrative] track lease release unavailable: track_id=%s", id)
		}
	}, nil
}
