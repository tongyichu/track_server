package service

import (
	"context"
	"testing"
	"time"

	"github.com/tongyichu/track_server/internal/maparea"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
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

func TestTrackMapServiceDecorateAreaClusters(t *testing.T) {
	svc := &TrackMapService{areas: maparea.DefaultCatalog()}
	items := []*models.TrackMapClusterItem{
		{
			Type:      "area_cluster",
			ClusterID: "scenic-west-lake",
			AreaID:    "scenic-west-lake",
			Center:    models.TrackMapPoint{Latitude: 45, Longitude: 90},
		},
	}

	svc.decorateAreaClusters(items)

	item := items[0]
	if item.Name != "西湖景区" || item.AreaType != maparea.TypeScenicSpot {
		t.Fatalf("decorated item=%+v", item)
	}
	if item.CityCode != "330100" || item.CityName != "杭州市" {
		t.Fatalf("unexpected city fields: %+v", item)
	}
	if item.Area == nil || item.Area.ID != "scenic-west-lake" || item.Area.IntroductionURL == "" {
		t.Fatalf("unexpected area reference: %+v", item.Area)
	}
}

func TestTrackMapServiceDecorateAreaClustersLeavesUnknownUnchanged(t *testing.T) {
	svc := &TrackMapService{areas: maparea.DefaultCatalog()}
	item := &models.TrackMapClusterItem{
		Type:      "area_cluster",
		ClusterID: "cell_30.2_120.1",
		Center:    models.TrackMapPoint{Latitude: 30.25, Longitude: 120.14},
	}

	svc.decorateAreaClusters([]*models.TrackMapClusterItem{item})

	if item.Name != "" || item.AreaType != "" || item.Area != nil {
		t.Fatalf("unknown area should keep legacy payload: %+v", item)
	}
}

func TestTrackMapServiceViewAreaIncludesSemanticArea(t *testing.T) {
	repo := repository.NewInMemoryTrackMapRepository(nil)
	now := time.Now()
	err := repo.UpsertRouteGroup(context.Background(), &models.TrackRouteGroup{
		GroupID:     "RG.00000001",
		AreaID:      "scenic-west-lake",
		TrackType:   "hiking",
		Status:      models.TrackRouteGroupStatusActive,
		CenterLat:   30.25,
		CenterLng:   120.14,
		MinLat:      30.24,
		MinLng:      120.13,
		MaxLat:      30.26,
		MaxLng:      120.15,
		MemberCount: 1,
		Source:      models.TrackRouteGroupSourceAuto,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("upsert route group: %v", err)
	}
	if err := repo.UpsertRouteGroup(context.Background(), &models.TrackRouteGroup{
		GroupID:     "RG.00000002",
		AreaID:      "scenic-west-lake",
		TrackType:   "hiking",
		Status:      models.TrackRouteGroupStatusActive,
		CenterLat:   30.25,
		CenterLng:   120.095,
		MinLat:      30.24,
		MinLng:      120.09,
		MaxLat:      30.26,
		MaxLng:      120.10,
		MemberCount: 1,
		Source:      models.TrackRouteGroupSourceAuto,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("upsert second route group: %v", err)
	}
	svc := NewTrackMapService(repo, nil, nil)

	resp, err := svc.View(context.Background(), TrackMapViewInput{
		BBox:      "120.0,30.1,120.3,30.4",
		Zoom:      10,
		TrackType: "hiking",
	})
	if err != nil {
		t.Fatalf("view area: %v", err)
	}
	items, ok := resp.Items.([]*models.TrackMapClusterItem)
	if !ok || len(items) != 1 {
		t.Fatalf("items=%T %+v", resp.Items, resp.Items)
	}
	if items[0].Name != "西湖景区" || items[0].Area == nil || items[0].Area.IntroductionURL == "" {
		t.Fatalf("area item=%+v", items[0])
	}
	if items[0].ClusterID != "area_scenic-west-lake" || items[0].RouteCount != 2 {
		t.Fatalf("area_id groups should aggregate together: %+v", items[0])
	}
}

func TestTrackMapServiceViewAreaDoesNotResolveMissingAreaIDOnline(t *testing.T) {
	repo := repository.NewInMemoryTrackMapRepository(nil)
	now := time.Now()
	if err := repo.UpsertRouteGroup(context.Background(), &models.TrackRouteGroup{
		GroupID:     "RG.00000003",
		TrackType:   "hiking",
		Status:      models.TrackRouteGroupStatusActive,
		CenterLat:   30.25,
		CenterLng:   120.14,
		MinLat:      30.24,
		MinLng:      120.13,
		MaxLat:      30.26,
		MaxLng:      120.15,
		MemberCount: 1,
		Source:      models.TrackRouteGroupSourceAuto,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("upsert route group: %v", err)
	}

	resp, err := NewTrackMapService(repo, nil, nil).View(context.Background(), TrackMapViewInput{
		BBox:      "120.0,30.1,120.3,30.4",
		Zoom:      10,
		TrackType: "hiking",
	})
	if err != nil {
		t.Fatalf("view area: %v", err)
	}
	items := resp.Items.([]*models.TrackMapClusterItem)
	if len(items) != 1 || items[0].Area != nil || items[0].Name != "" {
		t.Fatalf("missing offline area_id should not be resolved online: %+v", items)
	}
}
