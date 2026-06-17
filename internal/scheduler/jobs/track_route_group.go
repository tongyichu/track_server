package jobs

import (
	"context"
	"log"

	"github.com/tongyichu/track_server/internal/service"
)

const defaultTrackRouteGroupSpec = "0 4 * * *"

// TrackRouteGroup aggregates track_geo_indexes into persistent route groups.
type TrackRouteGroup struct {
	groupSvc *service.TrackRouteGroupService
	spec     string
}

func NewTrackRouteGroup(groupSvc *service.TrackRouteGroupService, spec string) *TrackRouteGroup {
	if spec == "" {
		spec = defaultTrackRouteGroupSpec
	}
	return &TrackRouteGroup{groupSvc: groupSvc, spec: spec}
}

func (j *TrackRouteGroup) Name() string { return "track_route_group" }

func (j *TrackRouteGroup) Spec() string { return j.spec }

func (j *TrackRouteGroup) Run(ctx context.Context) error {
	log.Printf("[TrackRouteGroup] start run ...")
	result, err := j.groupSvc.RunOnce(ctx)
	if err != nil {
		return err
	}
	log.Printf("[TrackRouteGroup] scanned=%d created=%d merged=%d skipped=%d", result.Scanned, result.Created, result.Merged, result.Skipped)
	return nil
}
