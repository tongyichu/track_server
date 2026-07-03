package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

func newSubmissionTestService(t *testing.T) (*TrackSubmissionService, *repository.InMemoryTrackSubmissionRepository) {
	t.Helper()
	tracks := repository.NewInMemoryTrackRepository()
	now := time.Now()
	if err := tracks.Create(context.Background(), &models.Track{ID: "NO.00000001", UserID: 1001, TrackType: "hiking", Title: "2026-07-02", RawTrackURL: "https://oss.example.com/user/1001/raw.json", TrackScreenshotURL: "https://oss.example.com/user/1001/shot.jpg", Status: models.TrackStatusNormal, StartTime: now, EndTime: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewInMemoryTrackSubmissionRepository()
	return NewTrackSubmissionService(repo, tracks), repo
}

func validSubmissionInput() TrackSubmissionInput {
	return TrackSubmissionInput{Title: "西湖群山十里徒步环线", Description: "从云栖竹径出发，经过五云山和九溪返回，沿途林荫路段较多。", Difficulty: "standard", RiskLevel: "low", SuitableMonths: []int{11, 3, 3}, SurfaceTypes: []string{"dirt", "stairs"}, TransportModes: []string{"taxi"}, TransportDescription: "建议乘坐网约车到云栖竹径入口后开始徒步"}
}

func TestTrackSubmissionLifecycleWithoutImages(t *testing.T) {
	svc, repo := newSubmissionTestService(t)
	ctx := context.Background()
	sub, err := svc.Submit(ctx, 1001, "NO.00000001", validSubmissionInput(), false)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if sub.Status != models.TrackSubmissionStatusPending || sub.Revision != 1 || len(sub.Images) != 0 {
		t.Fatalf("unexpected submission: %+v", sub)
	}
	approved, err := svc.Review(ctx, sub.SubmissionID, 1, "approved", "reviewer", "")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.Status != models.TrackSubmissionStatusApproved || approved.ApprovedAt == nil {
		t.Fatalf("unexpected approved submission: %+v", approved)
	}
	if err := svc.Withdraw(ctx, 1001, sub.TrackID); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	stored, err := repo.FindByTrackID(ctx, sub.TrackID)
	if err != nil || stored.Status != models.TrackSubmissionStatusWithdrawn {
		t.Fatalf("unexpected withdrawn submission: %+v err=%v", stored, err)
	}
	events, err := repo.ListEvents(ctx, sub.SubmissionID)
	if err != nil || len(events) != 3 {
		t.Fatalf("expected three lifecycle events, got %d err=%v", len(events), err)
	}
}

func TestTrackSubmissionRejectAndResubmit(t *testing.T) {
	svc, _ := newSubmissionTestService(t)
	ctx := context.Background()
	sub, err := svc.Submit(ctx, 1001, "NO.00000001", validSubmissionInput(), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Review(ctx, sub.SubmissionID, 1, "rejected", "reviewer", "标题需要更明确"); err != nil {
		t.Fatal(err)
	}
	input := validSubmissionInput()
	input.Title = "西湖云栖至九溪徒步路线"
	resubmitted, err := svc.Submit(ctx, 1001, sub.TrackID, input, false)
	if err != nil {
		t.Fatal(err)
	}
	if resubmitted.Revision != 2 || resubmitted.Status != models.TrackSubmissionStatusPending {
		t.Fatalf("unexpected resubmission: %+v", resubmitted)
	}
	if _, err := svc.Review(ctx, sub.SubmissionID, 1, "approved", "reviewer", ""); !errors.Is(err, repository.ErrAlreadyExists) {
		t.Fatalf("expected revision conflict, got %v", err)
	}
}

func TestTrackSubmissionValidation(t *testing.T) {
	svc, _ := newSubmissionTestService(t)
	input := validSubmissionInput()
	input.Images = make([]TrackSubmissionImageInput, 10)
	if _, err := svc.Submit(context.Background(), 1001, "NO.00000001", input, false); err == nil {
		t.Fatal("expected image count validation error")
	}
	input = validSubmissionInput()
	input.SuitableMonths = nil
	if _, err := svc.Submit(context.Background(), 1001, "NO.00000001", input, false); err == nil {
		t.Fatal("expected suitable month validation error")
	}
}

func TestApprovedSubmissionCanBecomeRouteGroupRepresentative(t *testing.T) {
	ctx := context.Background()
	tracks := repository.NewInMemoryTrackRepository()
	now := time.Now()
	for _, trackID := range []string{"NO.00000001", "NO.00000002"} {
		if err := tracks.Create(ctx, &models.Track{ID: trackID, UserID: 1001, TrackType: "hiking", Title: "原始标题", RawTrackURL: "https://oss.example.com/raw.json", TrackScreenshotURL: "https://oss.example.com/shot.jpg", Status: models.TrackStatusNormal, StartTime: now, EndTime: now.Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	subRepo := repository.NewInMemoryTrackSubmissionRepository()
	subSvc := NewTrackSubmissionService(subRepo, tracks)
	sub, err := subSvc.Submit(ctx, 1001, "NO.00000002", validSubmissionInput(), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := subSvc.Review(ctx, sub.SubmissionID, sub.Revision, "approved", "reviewer", ""); err != nil {
		t.Fatal(err)
	}
	groupSvc := NewTrackRouteGroupService(repository.NewInMemoryTrackMapRepository(tracks))
	groupSvc.SetTrackSubmissionService(subSvc)
	cluster := &routeGroupCluster{group: &models.TrackRouteGroup{GroupID: "RG.1", RepresentativeTrackID: "NO.00000001"}, members: []*models.TrackRouteGroupMember{{GroupID: "RG.1", TrackID: "NO.00000001", Role: models.TrackRouteGroupMemberRoleRepresentative, SimilarityScore: 1, Source: models.TrackRouteGroupSourceAuto}, {GroupID: "RG.1", TrackID: "NO.00000002", Role: models.TrackRouteGroupMemberRoleMember, SimilarityScore: .9, Source: models.TrackRouteGroupSourceAuto}}}
	if err := groupSvc.applySubmissionRepresentatives(ctx, []*routeGroupCluster{cluster}, nil); err != nil {
		t.Fatal(err)
	}
	if cluster.group.RepresentativeTrackID != "NO.00000002" || cluster.members[1].Source != models.TrackRouteGroupSourceSubmission {
		t.Fatalf("approved submission was not selected: group=%+v member=%+v", cluster.group, cluster.members[1])
	}
}
