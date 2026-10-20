package repository

import (
	"context"
	"sync"
	"testing"

	"github.com/tongyichu/track_server/internal/models"
)

func TestTrackLikeConcurrentIdempotencyAndCounts(t *testing.T) {
	ctx := context.Background()
	tracks := NewInMemoryTrackRepository()
	for _, id := range []string{"route-a", "route-b"} {
		if err := tracks.Create(ctx, &models.Track{ID: id, UserID: 9, Status: models.TrackStatusNormal, RawTrackURL: "https://example.test/raw"}); err != nil {
			t.Fatal(err)
		}
	}
	likes := NewInMemoryTrackLikeRepository(tracks)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := likes.SetLike(ctx, 1, "route-a", true); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	stats, err := likes.Stats(ctx, 1, []string{"route-a"})
	if err != nil || stats["route-a"].LikeCount != 1 || !stats["route-a"].Liked {
		t.Fatalf("duplicate relationships: %v %v", stats, err)
	}
	created := likes.likes["route-a"][1].CreatedAt
	if err := likes.SetLike(ctx, 1, "route-a", true); err != nil {
		t.Fatal(err)
	}
	if !likes.likes["route-a"][1].CreatedAt.Equal(created) {
		t.Fatal("duplicate like changed original time")
	}
	for _, pair := range []struct {
		user  int64
		track string
	}{{2, "route-a"}, {1, "route-b"}, {2, "route-b"}, {9, "route-a"}} {
		if err := likes.SetLike(ctx, pair.user, pair.track, true); err != nil {
			t.Fatal(err)
		}
	}
	count, err := likes.CountReceived(ctx, 9)
	if err != nil || count != 4 {
		t.Fatalf("received count includes self-like or loses records: %d %v", count, err)
	}
	for i := 0; i < 2; i++ {
		if err := likes.SetLike(ctx, 1, "route-a", false); err != nil {
			t.Fatal(err)
		}
	}
	count, err = likes.CountReceived(ctx, 9)
	if err != nil || count != 3 {
		t.Fatalf("duplicate cancellation changed count: %d %v", count, err)
	}
}

func TestReceivedCountTrackVisibility(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*models.Track)
		want   int64
	}{
		{"public", func(*models.Track) {}, 1},
		{"private", func(track *models.Track) { track.Status = models.TrackStatusPrivate }, 0},
		{"deleted", func(track *models.Track) { track.Status = models.TrackStatusDeleted }, 0},
		{"running", func(track *models.Track) { track.IsRunning = true }, 0},
		{"not_uploaded", func(track *models.Track) { track.RawTrackURL = "  " }, 0},
		{"another_owner", func(track *models.Track) { track.UserID = 8 }, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			tracks := NewInMemoryTrackRepository()
			track := &models.Track{ID: "route-a", UserID: 9, Status: models.TrackStatusNormal, RawTrackURL: "raw"}
			test.change(track)
			if err := tracks.Create(ctx, track); err != nil {
				t.Fatal(err)
			}
			likes := NewInMemoryTrackLikeRepository(tracks)
			if err := likes.SetLike(ctx, 1, track.ID, true); err != nil {
				t.Fatal(err)
			}
			if err := likes.SetLike(ctx, 1, "missing", true); err != nil {
				t.Fatal(err)
			}
			count, err := likes.CountReceived(ctx, 9)
			if err != nil || count != test.want {
				t.Fatalf("count=%d want=%d err=%v", count, test.want, err)
			}
		})
	}
}
