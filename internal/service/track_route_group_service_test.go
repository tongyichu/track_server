package service

import (
	"context"
	"fmt"
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
	if group.AreaID != "" {
		t.Fatalf("unexpected area_id outside built-in catalog: %s", group.AreaID)
	}
	members, err := mapRepo.ListRouteGroupMembers(context.Background(), group.GroupID, 10)
	if err != nil {
		t.Fatal(err)
	}
	var mergedFound bool
	for _, member := range members {
		if member.TrackID == b.TrackID && member.MatchDirection == models.TrackRouteGroupMemberDirectionReverse {
			mergedFound = true
		}
	}
	if !mergedFound {
		t.Fatalf("expected merged member for %s", b.TrackID)
	}
}

func TestTrackRouteGroupService_AssignsAreaIDOffline(t *testing.T) {
	mapRepo := repository.NewInMemoryTrackMapRepository(nil)
	index := testRouteGroupIndex("NO.00000006", "hiking", []models.TrackPoint{
		{Latitude: 30.24, Longitude: 120.13},
		{Latitude: 30.25, Longitude: 120.14},
		{Latitude: 30.26, Longitude: 120.15},
	}, time.Now())
	if err := mapRepo.UpsertTrackGeoIndex(context.Background(), index); err != nil {
		t.Fatal(err)
	}
	if _, err := NewTrackRouteGroupService(mapRepo).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	group, err := mapRepo.FindRouteGroupByTrackID(context.Background(), index.TrackID)
	if err != nil {
		t.Fatal(err)
	}
	if group.AreaID != "scenic-west-lake" {
		t.Fatalf("area_id=%q, want scenic-west-lake", group.AreaID)
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

func TestTrackRouteGroupService_DoesNotMergeDifferentShapesWithNearbyCenters(t *testing.T) {
	mapRepo := repository.NewInMemoryTrackMapRepository(nil)
	now := time.Now()
	a := testRouteGroupIndex("NO.00000031", "hiking", []models.TrackPoint{{Latitude: 22.30, Longitude: 114.10}, {Latitude: 22.32, Longitude: 114.12}}, now)
	b := testRouteGroupIndex("NO.00000032", "hiking", []models.TrackPoint{{Latitude: 22.30, Longitude: 114.10}, {Latitude: 22.31, Longitude: 114.145}, {Latitude: 22.32, Longitude: 114.12}}, now)
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
	if result.Created != 2 || result.Merged != 0 {
		t.Fatalf("different shapes should stay separate: %+v", result)
	}
}

func TestTrackRouteGroupService_RejectsLargeDistanceRatioDifference(t *testing.T) {
	mapRepo := repository.NewInMemoryTrackMapRepository(nil)
	now := time.Now()
	a := testRouteGroupIndex("NO.00000033", "hiking", []models.TrackPoint{{Latitude: 22.30, Longitude: 114.10}, {Latitude: 22.32, Longitude: 114.12}}, now)
	b := testRouteGroupIndex("NO.00000034", "hiking", []models.TrackPoint{{Latitude: 22.30, Longitude: 114.10}, {Latitude: 22.32, Longitude: 114.12}}, now)
	b.Distance = a.Distance * 2
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
	if result.Created != 2 {
		t.Fatalf("distance ratio should split routes: %+v", result)
	}
}

func TestTrackRouteGroupService_SelectsMedoidRepresentative(t *testing.T) {
	mapRepo := repository.NewInMemoryTrackMapRepository(nil)
	now := time.Now()
	for i, offset := range []float64{0, 0.0002, 0.0004} {
		index := testRouteGroupIndex(fmt.Sprintf("NO.0000004%d", i), "hiking", []models.TrackPoint{{Latitude: 22.30 + offset, Longitude: 114.10}, {Latitude: 22.32 + offset, Longitude: 114.12}}, now)
		if err := mapRepo.UpsertTrackGeoIndex(context.Background(), index); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := NewTrackRouteGroupService(mapRepo).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	group, err := mapRepo.FindRouteGroupByTrackID(context.Background(), "NO.00000040")
	if err != nil {
		t.Fatal(err)
	}
	if group.RepresentativeTrackID != "NO.00000041" {
		t.Fatalf("representative=%s, want medoid NO.00000041", group.RepresentativeTrackID)
	}
}

func TestTrackRouteGroupService_MergesReversedLoop(t *testing.T) {
	mapRepo := repository.NewInMemoryTrackMapRepository(nil)
	now := time.Now()
	forward := []models.TrackPoint{{Latitude: 22.30, Longitude: 114.10}, {Latitude: 22.30, Longitude: 114.12}, {Latitude: 22.32, Longitude: 114.12}, {Latitude: 22.32, Longitude: 114.10}, {Latitude: 22.30, Longitude: 114.10}}
	reverse := []models.TrackPoint{{Latitude: 22.30, Longitude: 114.10}, {Latitude: 22.32, Longitude: 114.10}, {Latitude: 22.32, Longitude: 114.12}, {Latitude: 22.30, Longitude: 114.12}, {Latitude: 22.30, Longitude: 114.10}}
	for _, index := range []*models.TrackGeoIndex{testRouteGroupIndex("NO.00000045", "hiking", forward, now), testRouteGroupIndex("NO.00000046", "hiking", reverse, now)} {
		if err := mapRepo.UpsertTrackGeoIndex(context.Background(), index); err != nil {
			t.Fatal(err)
		}
	}
	result, err := NewTrackRouteGroupService(mapRepo).RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 1 {
		t.Fatalf("reversed loop should merge: %+v", result)
	}
	group, err := mapRepo.FindRouteGroupByTrackID(context.Background(), "NO.00000046")
	if err != nil {
		t.Fatal(err)
	}
	members, err := mapRepo.ListRouteGroupMembers(context.Background(), group.GroupID, 10)
	if err != nil {
		t.Fatal(err)
	}
	reverseFound := false
	for _, member := range members {
		if member.Role != models.TrackRouteGroupMemberRoleRepresentative && member.MatchDirection == models.TrackRouteGroupMemberDirectionReverse {
			reverseFound = true
		}
	}
	if !reverseFound {
		t.Fatalf("expected non-representative loop member to be reverse: %+v", members)
	}
}

func TestTrackRouteGroupService_PreservesHistoryAndManualName(t *testing.T) {
	ctx := context.Background()
	mapRepo := repository.NewInMemoryTrackMapRepository(nil)
	index := testRouteGroupIndex("NO.00000051", "hiking", []models.TrackPoint{{Latitude: 22.30, Longitude: 114.10}, {Latitude: 22.32, Longitude: 114.12}}, time.Now())
	if err := mapRepo.UpsertTrackGeoIndex(ctx, index); err != nil {
		t.Fatal(err)
	}
	svc := NewTrackRouteGroupService(mapRepo)
	if _, err := svc.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	before, err := mapRepo.FindRouteGroupByTrackID(ctx, index.TrackID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RenameRouteGroup(ctx, before.GroupID, "测试古道徒步路线"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := mapRepo.FindRouteGroupByTrackID(ctx, index.TrackID)
	if err != nil {
		t.Fatal(err)
	}
	if after.GroupID != before.GroupID || after.Name != "测试古道徒步路线" {
		t.Fatalf("history not preserved: before=%+v after=%+v", before, after)
	}
}

func TestTrackRouteGroupService_KeepsGroupIDWhenMedoidChanges(t *testing.T) {
	ctx := context.Background()
	mapRepo := repository.NewInMemoryTrackMapRepository(nil)
	now := time.Now()
	for i, offset := range []float64{0, 0.0002} {
		index := testRouteGroupIndex(fmt.Sprintf("NO.0000007%d", i), "hiking", []models.TrackPoint{{Latitude: 22.30 + offset, Longitude: 114.10}, {Latitude: 22.32 + offset, Longitude: 114.12}}, now)
		if err := mapRepo.UpsertTrackGeoIndex(ctx, index); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewTrackRouteGroupService(mapRepo)
	if _, err := svc.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	before, err := mapRepo.FindRouteGroupByTrackID(ctx, "NO.00000070")
	if err != nil {
		t.Fatal(err)
	}
	third := testRouteGroupIndex("NO.00000072", "hiking", []models.TrackPoint{{Latitude: 22.3004, Longitude: 114.10}, {Latitude: 22.3204, Longitude: 114.12}}, now)
	if err := mapRepo.UpsertTrackGeoIndex(ctx, third); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := mapRepo.FindRouteGroupByTrackID(ctx, third.TrackID)
	if err != nil {
		t.Fatal(err)
	}
	if after.GroupID != before.GroupID {
		t.Fatalf("group id changed: before=%s after=%s", before.GroupID, after.GroupID)
	}
	if after.RepresentativeTrackID != "NO.00000071" {
		t.Fatalf("representative=%s, want changed medoid", after.RepresentativeTrackID)
	}
}

func TestTrackRouteGroupService_UsesAreaAndManualSeedNames(t *testing.T) {
	ctx := context.Background()
	trackRepo := repository.NewInMemoryTrackRepository()
	mapRepo := repository.NewInMemoryTrackMapRepository(trackRepo)
	now := time.Now()
	areaIndex := testRouteGroupIndex("NO.00000061", "hiking", []models.TrackPoint{{Latitude: 30.24, Longitude: 120.13}, {Latitude: 30.26, Longitude: 120.15}}, now)
	seedIndex := testRouteGroupIndex("NO.00000062", "hiking", []models.TrackPoint{{Latitude: 29.20, Longitude: 119.10}, {Latitude: 29.22, Longitude: 119.12}}, now)
	if err := mapRepo.UpsertTrackGeoIndex(ctx, areaIndex); err != nil {
		t.Fatal(err)
	}
	if err := mapRepo.UpsertTrackGeoIndex(ctx, seedIndex); err != nil {
		t.Fatal(err)
	}
	if err := trackRepo.Create(ctx, &models.Track{ID: seedIndex.TrackID, UserID: 1, Status: models.TrackStatusNormal, SourceTag: "manual_seed", Title: "豆腐皮古道", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	svc := NewTrackRouteGroupService(mapRepo)
	svc.SetTrackRepository(trackRepo)
	if _, err := svc.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	areaGroup, err := mapRepo.FindRouteGroupByTrackID(ctx, areaIndex.TrackID)
	if err != nil {
		t.Fatal(err)
	}
	if areaGroup.Name != "西湖徒步路线" {
		t.Fatalf("area name=%q", areaGroup.Name)
	}
	seedGroup, err := mapRepo.FindRouteGroupByTrackID(ctx, seedIndex.TrackID)
	if err != nil {
		t.Fatal(err)
	}
	if seedGroup.Name != "豆腐皮古道徒步路线" {
		t.Fatalf("seed name=%q", seedGroup.Name)
	}
}

func TestTrackRouteIntroductionRebindsAfterRegroup(t *testing.T) {
	ctx := context.Background()
	mapRepo := repository.NewInMemoryTrackMapRepository(nil)
	now := time.Now()
	oldGroup := &models.TrackRouteGroup{GroupID: "RG.old", TrackType: "hiking", Status: models.TrackRouteGroupStatusActive, RepresentativeTrackID: "NO.00000021", CreatedAt: now, UpdatedAt: now}
	if err := mapRepo.UpsertRouteGroup(ctx, oldGroup); err != nil {
		t.Fatal(err)
	}
	if err := mapRepo.UpsertRouteGroupMember(ctx, &models.TrackRouteGroupMember{GroupID: oldGroup.GroupID, TrackID: oldGroup.RepresentativeTrackID, Role: models.TrackRouteGroupMemberRoleRepresentative}); err != nil {
		t.Fatal(err)
	}
	svc := NewTrackRouteGroupService(mapRepo)
	intro, err := svc.SaveRouteIntroduction(ctx, oldGroup.GroupID, &models.TrackRouteIntroduction{Chinese: models.TrackRouteIntroductionContent{Name: "测试路线", Summary: "测试摘要"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetRouteIntroductionPublished(ctx, oldGroup.GroupID, true); err != nil {
		t.Fatal(err)
	}
	newGroup := *oldGroup
	newGroup.GroupID = "RG.new"
	newMember := &models.TrackRouteGroupMember{GroupID: newGroup.GroupID, TrackID: intro.AnchorTrackID, Role: models.TrackRouteGroupMemberRoleRepresentative}
	if err := mapRepo.ReplaceRouteGroups(ctx, []*models.TrackRouteGroup{&newGroup}, []*models.TrackRouteGroupMember{newMember}); err != nil {
		t.Fatal(err)
	}
	rebound, err := mapRepo.FindRouteIntroductionByGroupID(ctx, newGroup.GroupID)
	if err != nil {
		t.Fatal(err)
	}
	if rebound.Status != models.TrackRouteIntroductionStatusPublished || rebound.AnchorTrackID != intro.AnchorTrackID {
		t.Fatalf("rebound=%+v", rebound)
	}
}

func testRouteGroupIndex(trackID, trackType string, base []models.TrackPoint, now time.Time) *models.TrackGeoIndex {
	points := make([]models.TrackPoint, 0, 30)
	for i := 0; i < 30; i++ {
		progress := float64(i) / 29 * float64(len(base)-1)
		segment := int(progress)
		if segment >= len(base)-1 {
			segment = len(base) - 2
			progress = float64(len(base) - 1)
		}
		ratio := progress - float64(segment)
		points = append(points, models.TrackPoint{
			Index:     i,
			Latitude:  base[segment].Latitude + (base[segment+1].Latitude-base[segment].Latitude)*ratio,
			Longitude: base[segment].Longitude + (base[segment+1].Longitude-base[segment].Longitude)*ratio,
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
