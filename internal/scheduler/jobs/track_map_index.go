package jobs

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/tongyichu/track_server/internal/service"
)

const defaultTrackMapIndexSpec = "@every 1m"

// TrackMapIndex builds pending map indexes and compensates missing jobs.
type TrackMapIndex struct {
	indexSvc *service.TrackMapIndexService
	spec     string
}

func NewTrackMapIndex(indexSvc *service.TrackMapIndexService, spec string) *TrackMapIndex {
	if strings.TrimSpace(spec) == "" {
		spec = defaultTrackMapIndexSpec
	}
	return &TrackMapIndex{indexSvc: indexSvc, spec: spec}
}

func (j *TrackMapIndex) Name() string { return "track_map_index" }

func (j *TrackMapIndex) Spec() string { return j.spec }

func (j *TrackMapIndex) Run(ctx context.Context) error {
	if j.indexSvc == nil {
		return fmt.Errorf("track_map_index: service is nil")
	}
	result, err := j.indexSvc.RunOnce(ctx)
	if result != nil {
		log.Printf("[scheduler] track_map_index: enqueued_missing=%d claimed=%d succeeded=%d failed=%d", result.EnqueuedMissing, result.Claimed, result.Succeeded, result.Failed)
	}
	return err
}
