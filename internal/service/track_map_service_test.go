package service

import (
	"testing"

	"github.com/tongyichu/track_server/internal/models"
)

func TestChooseTrackMapViewLevel_UsesZoomThresholds(t *testing.T) {
	filter := models.TrackMapQueryFilter{}
	cases := []struct {
		name string
		zoom float64
		want string
	}{
		{name: "city upper bound", zoom: 9, want: trackMapViewCity},
		{name: "area lower bound", zoom: 10, want: trackMapViewArea},
		{name: "area upper bound", zoom: 11, want: trackMapViewArea},
		{name: "route above area", zoom: 12, want: trackMapViewRoute},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := chooseTrackMapViewLevel(tt.zoom, filter); got != tt.want {
				t.Fatalf("chooseTrackMapViewLevel(%v)=%s, want %s", tt.zoom, got, tt.want)
			}
		})
	}
}
