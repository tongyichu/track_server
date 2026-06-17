package service

import (
	"context"
	"testing"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

func TestTrackRouteGroupService_MergesNearbyRouteCenters(t *testing.T) {
	trackRepo := repository.NewInMemoryTrackRepository()
	mapRepo := repository.NewInMemoryTrackMapRepository(trackRepo)
	now := time.Now()
	a := testRouteGroupIndex("NO.00000001", "hiking", []models.TrackPoint{
		{Latitude: 22.30, Longitude: 114.10},
		{Latitude: 22.31, Longitude: 114.11},
		{Latitude: 22.32, Longitude: 114.12},
	}, now)
	b := testRouteGroupIndex("NO.00000002", "hiking", []models.TrackPoint{
		{Latitude: 22.3201, Longitude: 114.1201},
		{Latitude: 22.3101, Longitude: 114.1101},
		{Latitude: 22.3001, Longitude: 114.1001},
	}, now)
	if err := mapRepo.UpsertTrackGeoIndex(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if err := mapRepo.UpsertTrackGeoIndex(context.Background(), b); err != nil {
		t.Fatal(err)
	}

	result, err := NewTrackRouteGroupService(mapRepo).RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 1 || result.Merged != 1 {
		t.Fatalf("expected one created and one merged group, got %+v", result)
	}
	group, err := mapRepo.FindRouteGroupByTrackID(context.Background(), b.TrackID)
	if err != nil {
		t.Fatal(err)
	}
	if group.MemberCount != 2 {
		t.Fatalf("expected member_count=2, got %d", group.MemberCount)
	}
	if group.RadiusM <= 0 {
		t.Fatalf("expected positive radius_m, got %f", group.RadiusM)
	}
	members, err := mapRepo.ListRouteGroupMembers(context.Background(), group.GroupID, 10)
	if err != nil {
		t.Fatal(err)
	}
	var mergedFound bool
	for _, member := range members {
		if member.TrackID == b.TrackID && member.MatchDirection == models.TrackRouteGroupMemberDirectionForward {
			mergedFound = true
		}
	}
	if !mergedFound {
		t.Fatalf("expected merged member for %s", b.TrackID)
	}
}

func TestTrackRouteGroupService_KeepsTrackTypeSeparate(t *testing.T) {
	trackRepo := repository.NewInMemoryTrackRepository()
	mapRepo := repository.NewInMemoryTrackMapRepository(trackRepo)
	now := time.Now()
	points := []models.TrackPoint{
		{Latitude: 22.30, Longitude: 114.10},
		{Latitude: 22.31, Longitude: 114.11},
		{Latitude: 22.32, Longitude: 114.12},
	}
	if err := mapRepo.UpsertTrackGeoIndex(context.Background(), testRouteGroupIndex("NO.00000003", "hiking", points, now)); err != nil {
		t.Fatal(err)
	}
	if err := mapRepo.UpsertTrackGeoIndex(context.Background(), testRouteGroupIndex("NO.00000004", "running", points, now)); err != nil {
		t.Fatal(err)
	}

	result, err := NewTrackRouteGroupService(mapRepo).RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 2 || result.Merged != 0 {
		t.Fatalf("expected two separate groups, got %+v", result)
	}
}

func TestTrackRouteGroupService_SkipsShortRoute(t *testing.T) {
	trackRepo := repository.NewInMemoryTrackRepository()
	mapRepo := repository.NewInMemoryTrackMapRepository(trackRepo)
	index := testRouteGroupIndex("NO.00000005", "hiking", []models.TrackPoint{
		{Latitude: 22.30, Longitude: 114.10},
		{Latitude: 22.3001, Longitude: 114.1001},
	}, time.Now())
	index.Distance = 20
	index.PointCount = 2
	if err := mapRepo.UpsertTrackGeoIndex(context.Background(), index); err != nil {
		t.Fatal(err)
	}
	result, err := NewTrackRouteGroupService(mapRepo).RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 0 || result.Skipped != 1 {
		t.Fatalf("expected short route skipped, got %+v", result)
	}
}

func testRouteGroupIndex(trackID, trackType string, base []models.TrackPoint, now time.Time) *models.TrackGeoIndex {
	points := make([]models.TrackPoint, 0, 30)
	start := base[0]
	end := base[len(base)-1]
	for i := 0; i < 30; i++ {
		ratio := float64(i) / 29
		points = append(points, models.TrackPoint{
			Index:     i,
			Latitude:  start.Latitude + (end.Latitude-start.Latitude)*ratio,
			Longitude: start.Longitude + (end.Longitude-start.Longitude)*ratio,
		})
	}
	minLat, maxLat := points[0].Latitude, points[0].Latitude
	minLng, maxLng := points[0].Longitude, points[0].Longitude
	var sumLat, sumLng float64
	for _, p := range points {
		if p.Latitude < minLat {
			minLat = p.Latitude
		}
		if p.Latitude > maxLat {
			maxLat = p.Latitude
		}
		if p.Longitude < minLng {
			minLng = p.Longitude
		}
		if p.Longitude > maxLng {
			maxLng = p.Longitude
		}
		sumLat += p.Latitude
		sumLng += p.Longitude
	}
	return &models.TrackGeoIndex{
		TrackID:            trackID,
		UserID:             1,
		CityCode:           "810000",
		TrackType:          trackType,
		CoordinateSystem:   "GCJ02",
		StartLat:           points[0].Latitude,
		StartLng:           points[0].Longitude,
		EndLat:             points[len(points)-1].Latitude,
		EndLng:             points[len(points)-1].Longitude,
		CenterLat:          sumLat / float64(len(points)),
		CenterLng:          sumLng / float64(len(points)),
		MinLat:             minLat,
		MinLng:             minLng,
		MaxLat:             maxLat,
		MaxLng:             maxLng,
		Distance:           3200,
		PointCount:         len(points),
		SimplifiedPolyline: points,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}
