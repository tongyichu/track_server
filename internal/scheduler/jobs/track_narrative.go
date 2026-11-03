package jobs

import (
	"context"
	"log"

	"github.com/tongyichu/track_server/internal/service"
)

type TrackNarrative struct {
	service *service.TrackNarrativeService
	spec    string
}

func NewTrackNarrative(s *service.TrackNarrativeService, spec string) *TrackNarrative {
	if spec == "" {
		spec = "@every 1m"
	}
	return &TrackNarrative{s, spec}
}
func (j *TrackNarrative) Name() string { return "track_narrative" }
func (j *TrackNarrative) Spec() string { return j.spec }
func (j *TrackNarrative) Run(ctx context.Context) error {
	result, err := j.service.RunOnce(ctx)
	if result != nil {
		log.Printf("[scheduler] track_narrative: scanned=%d generated=%d skipped=%d deferred=%d failed=%d", result.Scanned, result.Generated, result.Skipped, result.Deferred, result.Failed)
	}
	return err
}
