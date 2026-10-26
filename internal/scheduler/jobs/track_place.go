package jobs

import (
	"context"
	"log"

	"github.com/tongyichu/track_server/internal/service"
)

type TrackPlace struct {
	service *service.TrackPlaceService
	spec    string
}

func NewTrackPlace(s *service.TrackPlaceService, spec string) *TrackPlace {
	if spec == "" {
		spec = "@every 1m"
	}
	return &TrackPlace{s, spec}
}
func (j *TrackPlace) Name() string { return "track_place" }
func (j *TrackPlace) Spec() string { return j.spec }
func (j *TrackPlace) Run(ctx context.Context) error {
	result, err := j.service.RunOnce(ctx)
	if result != nil {
		log.Printf("[scheduler] track_place: scanned=%d matched=%d unmatched=%d deferred=%d failed=%d unconfigured=%d", result.Scanned, result.Matched, result.Unmatched, result.Deferred, result.Failed, result.Unconfigured)
	}
	return err
}
