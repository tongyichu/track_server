package service

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

func TestDateOnlyTrackTitle(t *testing.T) {
	for _, title := range []string{
		"2026年10月9日 11:08", "2026/10/9 11:08", "2026-10-09T03:08:00Z",
		"2026-10-09T11:08:00+08:00", "Oct 9, 2026, 11:08 AM", "9 Oct 2026, 11:08",
		"2026年10月9日 上午11:08", "2026年04月26日 • 09:20", "2026年04月26日 · 09:20", "10/09/2026 11:08 PM", "2026.10.09", " 2026-10-09 ",
	} {
		if !IsDateOnlyTrackTitle(title) {
			t.Errorf("missed default date title %q", title)
		}
	}
	for _, title := range []string{"2026年10月9日西湖跑步", "Oct 9, 2026 morning run", "2026-10-09 跑步", "123", "", "西湖环线", "2026年运动计划"} {
		if IsDateOnlyTrackTitle(title) {
			t.Errorf("descriptive title classified as default: %q", title)
		}
	}
}

func titleTestTrack() *models.Track {
	return &models.Track{ID: "NO.00000001", UserID: 7, Title: "2026年10月9日 11:08",
		TrackType: "running", LocateAddr: "杭州市·西湖区·西湖公园", Status: models.TrackStatusNormal,
		StartTime: time.Date(2026, 10, 8, 23, 0, 0, 0, time.UTC), Distance: 800}
}

func titleLoopPoints() []models.TrackPoint {
	return []models.TrackPoint{
		{Latitude: 30, Longitude: 120}, {Latitude: 30, Longitude: 120.002},
		{Latitude: 30.002, Longitude: 120.002}, {Latitude: 30.002, Longitude: 120},
		{Latitude: 30, Longitude: 120},
	}
}

func TestGeneratedTrackTitleRequiresAreaForLoop(t *testing.T) {
	track := titleTestTrack()
	if got := generatedTrackTitle(track, titleLoopPoints()); got != "西湖公园周边跑步环线" {
		t.Fatalf("loop title = %q", got)
	}
	outAndBack := []models.TrackPoint{
		{Latitude: 30, Longitude: 120}, {Latitude: 30, Longitude: 120.002},
		{Latitude: 30, Longitude: 120.004}, {Latitude: 30, Longitude: 120.002},
		{Latitude: 30, Longitude: 120},
	}
	if got := generatedTrackTitle(track, outAndBack); got != "西湖公园周边跑步" {
		t.Fatalf("out-and-back mislabeled as loop: %q", got)
	}
	if got := generatedTrackTitle(track, titleLoopPoints()[:4]); got != "西湖公园周边跑步" {
		t.Fatalf("open route mislabeled as loop: %q", got)
	}
}

func TestGeneratedTrackTitleFallbackAndLength(t *testing.T) {
	track := titleTestTrack()
	for _, address := range []string{"杭州市·西湖区", "unknown", "", strings.Repeat("湖", 40), "已定位", "28号", "甲6号", "No.15", "28WC+FCP"} {
		track.LocateAddr = address
		if got := generatedTrackTitle(track, nil); got != "800米跑步路线" {
			t.Errorf("fallback for %q = %q", address, got)
		}
	}
	track.LocateAddr = "西湖公园"
	track.TrackType = "骑车"
	if got := generatedTrackTitle(track, nil); got != "西湖公园周边骑行" {
		t.Fatalf("legacy workout name = %q", got)
	}
}

func TestTitleLocationUsesIntactRecordedPlace(t *testing.T) {
	for _, test := range []struct{ input, want string }{
		{"北京市·朝阳区·和平西苑(朝阳区人大常委会和平街街道工作委员会东", "和平西苑"},
		{"杭州市·西湖公园·2号", "西湖公园"},
		{"G6辅路中关村华侨创新产业园", "G6辅路中关村华侨创新产业园"},
		{"胜古南里朗丽兹酒店(北京中日友好医院店)", "胜古南里朗丽兹酒店(北京中日友好医院店)"},
		{"30.18706, 119.90183", ""},
		{"未知位置", ""},
		{"250-2", ""},
		{"2597-3", ""},
		{"清河路·124-2", "清河路"},
	} {
		if got := specificTitleLocation(test.input); got != test.want {
			t.Errorf("location %q = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestExpandedDateTitleCleanup(t *testing.T) {
	for _, test := range []struct{ title, address, want string }{
		{"2026年04月26日 • 09:20", "", "800米跑步路线"},
		{"2026年10月9日跑步记录", "", "800米跑步路线"},
		{"2024-09-07 11:31杭州萧山区", "眉山路", "眉山路周边跑步"},
		{"2025-11-15 11:43杭州富阳区", "30.18706, 119.90183", "杭州富阳区周边跑步"},
		{"走马岗 2025-11-22", "29.68467, 120.50944", "走马岗跑步路线"},
		{"2024-11-02 09:24 车耳营~凤凰岭环线", "车耳营路", "车耳营~凤凰岭环线"},
	} {
		t.Run(test.title, func(t *testing.T) {
			track := titleTestTrack()
			track.Title, track.LocateAddr = test.title, test.address
			if !IsDateOnlyTrackTitle(test.title) {
				change, err := (TrackTitleBackfillPlanner{}).Plan(context.Background(), track)
				if err != nil || change != nil {
					t.Fatalf("expanded cleanup requires opt-in: %+v %v", change, err)
				}
			}
			change, err := (TrackTitleBackfillPlanner{IncludeDatedTitles: true}).Plan(context.Background(), track)
			if err != nil || change == nil || change.NewTitle != test.want {
				t.Fatalf("cleanup = %+v %v, want %q", change, err, test.want)
			}
			if track.Title != test.title || track.LocateAddr != test.address {
				t.Fatal("preview mutated saved facts")
			}
		})
	}
	for _, title := range []string{"我的2026年10月9日西湖跑步", "西湖经典环线", "2026年运动计划"} {
		track := titleTestTrack()
		track.Title = title
		change, err := (TrackTitleBackfillPlanner{IncludeDatedTitles: true}).Plan(context.Background(), track)
		if err != nil || change != nil {
			t.Fatalf("non-stamped title selected: %+v %v", change, err)
		}
	}
}

func TestGeneratedTitleNeverFallsBackToDate(t *testing.T) {
	for _, test := range []struct {
		distance float64
		want     string
	}{
		{0, "跑步路线记录"}, {920.52, "921米跑步路线"}, {8388.87, "8.4公里跑步路线"},
		{math.NaN(), "跑步路线记录"}, {math.Inf(1), "跑步路线记录"},
	} {
		track := titleTestTrack()
		track.Distance, track.LocateAddr = test.distance, ""
		if got := generatedTrackTitle(track, nil); got != test.want || IsDatedTrackTitle(got) {
			t.Errorf("fallback for distance %v = %q, want %q", test.distance, got, test.want)
		}
	}
}

func TestTitleLoopIgnoresInvalidPointsAndDistances(t *testing.T) {
	points := append(titleLoopPoints(), models.TrackPoint{Latitude: math.NaN(), Longitude: 120})
	if !isTitleLoop(points, 800) {
		t.Fatal("invalid coordinates should be filtered")
	}
	for _, distance := range []float64{0, 499, math.NaN(), math.Inf(1)} {
		if isTitleLoop(points, distance) {
			t.Fatalf("invalid or too short distance accepted: %v", distance)
		}
	}
}

func TestTitlePlannerPreservesCustomAndUnavailableTracks(t *testing.T) {
	planner := TrackTitleBackfillPlanner{}
	for _, edit := range []func(*models.Track){
		func(t *models.Track) { t.Title = "我的西湖晨跑" },
		func(t *models.Track) { t.IsRunning = true },
		func(t *models.Track) { t.Status = models.TrackStatusDeleted },
		func(t *models.Track) { t.TrackType = "unknown_sport" },
	} {
		track := titleTestTrack()
		edit(track)
		change, err := planner.Plan(context.Background(), track)
		if err != nil || change != nil {
			t.Fatalf("protected track was selected: %+v %v", change, err)
		}
	}
}

func TestTitlePlannerPrefersSubmissionWithoutChangingIt(t *testing.T) {
	subs := repository.NewInMemoryTrackSubmissionRepository()
	sub := &models.TrackSubmission{SubmissionID: "sub-1", TrackID: "NO.00000001", UserID: 7,
		Title: "西湖经典晨跑环线", Status: models.TrackSubmissionStatusApproved}
	if err := subs.SavePending(context.Background(), sub, nil); err != nil {
		t.Fatal(err)
	}
	track := titleTestTrack()
	change, err := (TrackTitleBackfillPlanner{Submissions: subs}).Plan(context.Background(), track)
	if err != nil || change.NewTitle != sub.Title || change.Source != "submission" {
		t.Fatalf("submission was not reused: %+v %v", change, err)
	}
	if track.Title != change.OldTitle {
		t.Fatal("preview mutated original track")
	}
	saved, err := subs.FindByTrackID(context.Background(), sub.TrackID)
	if err != nil || saved.Title != sub.Title || saved.Status != sub.Status {
		t.Fatalf("submission changed during preview: %+v %v", saved, err)
	}
}

func TestTitlePlannerUsesCachedRawGeometryAndConservativeFallback(t *testing.T) {
	dir := t.TempDir()
	cache, err := NewAssetCacheService(dir, "/api/v1/static/raw_tracks", []string{".json"}, ".json")
	if err != nil {
		t.Fatal(err)
	}
	track := titleTestTrack()
	track.RawTrackURL = "https://example.test/raw.json"
	planner := TrackTitleBackfillPlanner{RawTracks: cache}
	change, err := planner.Plan(context.Background(), track)
	if err != nil || change.GeometryKnown || change.NewTitle != "西湖公园周边跑步" {
		t.Fatalf("missing geometry should not invent loop: %+v %v", change, err)
	}
	raw, err := json.Marshal(titleLoopPoints())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, track.ID+".json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	change, err = planner.Plan(context.Background(), track)
	if err != nil || !change.GeometryKnown || change.NewTitle != "西湖公园周边跑步环线" {
		t.Fatalf("cached route was not used: %+v %v", change, err)
	}
}

func TestTitlePlannerFallsBackWhenSubmissionTitleIsInvalid(t *testing.T) {
	for _, title := range []string{"2026年10月9日跑步记录", "2026年10月9日 11:08", "湖", strings.Repeat("湖", 41)} {
		t.Run(title, func(t *testing.T) {
			subs := repository.NewInMemoryTrackSubmissionRepository()
			track := titleTestTrack()
			sub := &models.TrackSubmission{SubmissionID: "sub-invalid", TrackID: track.ID, UserID: track.UserID,
				Title: title, Status: models.TrackSubmissionStatusApproved}
			if err := subs.SavePending(context.Background(), sub, nil); err != nil {
				t.Fatal(err)
			}
			change, err := (TrackTitleBackfillPlanner{Submissions: subs}).Plan(context.Background(), track)
			if err != nil || change == nil || change.NewTitle != "西湖公园周边跑步" || change.Source != "recorded_facts" {
				t.Fatalf("invalid submission should use recorded facts: %+v %v", change, err)
			}
			saved, err := subs.FindByTrackID(context.Background(), track.ID)
			if err != nil || saved.Title != title {
				t.Fatalf("submission was modified: %+v %v", saved, err)
			}
		})
	}
}
