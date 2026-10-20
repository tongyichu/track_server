package service

import (
	"context"
	"testing"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

func TestTrackCommentSummarySurvivesAssetCacheMiss(t *testing.T) {
	ctx := context.Background()
	tracks := repository.NewInMemoryTrackRepository()
	source := &models.Track{ID: "NO.00000001", UserID: 2, Status: models.TrackStatusNormal, RawTrackURL: "https://example.test/raw.kmz"}
	if err := tracks.Create(ctx, source); err != nil {
		t.Fatal(err)
	}
	comments := repository.NewMemoryCommentRepository()
	_, err := comments.Apply(ctx, repository.CommentMutation{Action: "create", TrackID: source.ID, Comment: &models.TrackComment{ID: "comment", TrackID: source.ID, UserID: 1, ClientID: "client-001", Content: "路线", CreatedAt: time.Now()}})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewTrackService(tracks, nil)
	svc.SetTrackCommentRepository(comments)
	cache, err := NewAssetCacheService(t.TempDir(), "/raw", []string{".kmz"}, ".kmz")
	if err != nil {
		t.Fatal(err)
	}
	svc.SetRawTrackCache(cache)
	response, err := svc.GetTrackDetail(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if response.RawTrackURL != "" || !response.CanComment || response.CommentCount != 1 {
		t.Fatalf("asset miss erased comment summary: %+v", response)
	}
}
