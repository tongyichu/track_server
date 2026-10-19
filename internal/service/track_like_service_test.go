package service

import (
	"context"
	"testing"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

func TestTrackDetailLikeCountSurvivesAssetCacheMiss(t *testing.T) {
	ctx := context.Background()
	tracks := repository.NewInMemoryTrackRepository()
	source := &models.Track{ID: "NO.00000001", UserID: 2, Status: models.TrackStatusNormal, RawTrackURL: "https://example.test/raw"}
	if err := tracks.Create(ctx, source); err != nil {
		t.Fatal(err)
	}
	likes := repository.NewInMemoryTrackLikeRepository(tracks)
	if err := likes.SetLike(ctx, 1, source.ID, true); err != nil {
		t.Fatal(err)
	}
	service := NewTrackService(tracks, nil)
	service.SetTrackLikeRepository(likes)
	response := *source
	response.RawTrackURL = ""
	if err := service.FillTrackLikeState(ctx, 1, &response); err != nil {
		t.Fatal(err)
	}
	if !response.Liked || response.LikeCount != 1 {
		t.Fatalf("asset failure erased likes: %+v", response)
	}
	source.RawTrackURL = ""
	if err := tracks.Update(ctx, source); err != nil {
		t.Fatal(err)
	}
	response = *source
	if err := service.FillTrackLikeState(ctx, 1, &response); err != nil {
		t.Fatal(err)
	}
	if response.LikeCount != 0 {
		t.Fatal("unuploaded route exposed likes")
	}
}
