package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

func TestTrackLikeConcurrentIdempotencyAndStableCursor(t *testing.T) {
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
	rows, err := likes.ListReceived(ctx, 9, nil, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("duplicate relationships: %v %v", rows, err)
	}
	created := rows[0].CreatedAt
	if err := likes.SetLike(ctx, 1, "route-a", true); err != nil {
		t.Fatal(err)
	}
	rows, _ = likes.ListReceived(ctx, 9, nil, 10)
	if !rows[0].CreatedAt.Equal(created) {
		t.Fatal("duplicate like changed original time")
	}
	for _, pair := range []struct {
		user  int64
		track string
	}{{2, "route-a"}, {1, "route-b"}, {2, "route-b"}} {
		if err := likes.SetLike(ctx, pair.user, pair.track, true); err != nil {
			t.Fatal(err)
		}
	}
	// Equal timestamps must still page across both track and user tie breakers.
	likes.mu.Lock()
	for _, users := range likes.likes {
		for _, like := range users {
			like.CreatedAt = time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
		}
	}
	likes.mu.Unlock()
	var cursor *models.TrackLikeCursor
	var seen []string
	for {
		page, err := likes.ListReceived(ctx, 9, cursor, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		row := page[0]
		seen = append(seen, row.TrackID+string(rune('0'+row.UserID)))
		cursor = &models.TrackLikeCursor{CreatedAt: row.CreatedAt, TrackID: row.TrackID, UserID: row.UserID}
		if len(seen) > 4 {
			t.Fatal("cursor repeated records")
		}
	}
	expected := []string{"route-b2", "route-b1", "route-a2", "route-a1"}
	if len(seen) != len(expected) {
		t.Fatalf("missing records: %v", seen)
	}
	for i := range expected {
		if seen[i] != expected[i] {
			t.Fatalf("wrong stable order: %v", seen)
		}
	}
}

func TestLikeGroupCursorTiesAndRecentCancellation(t *testing.T) {
	ctx := context.Background()
	tracks := NewInMemoryTrackRepository()
	likes := NewInMemoryTrackLikeRepository(tracks)
	timestamp := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	for _, id := range []string{"route-a", "route-b"} {
		if err := tracks.Create(ctx, &models.Track{ID: id, UserID: 9, Status: models.TrackStatusNormal, RawTrackURL: "raw"}); err != nil {
			t.Fatal(err)
		}
		for _, user := range []int64{1, 2, 3, 4, 9} {
			if err := likes.SetLike(ctx, user, id, true); err != nil {
				t.Fatal(err)
			}
			likes.likes[id][user].CreatedAt = timestamp
		}
	}
	first, err := likes.ListReceivedGroups(ctx, 9, nil, 1)
	if err != nil || len(first) != 1 || first[0].TrackID != "route-b" || first[0].LikeCount != 4 {
		t.Fatalf("first group: %v %v", first, err)
	}
	if len(first[0].RecentLikes) != 3 || first[0].RecentLikes[0].UserID != 4 {
		t.Fatal("recent users must be limited and tie ordered")
	}
	c := &models.ReceivedLikeTrackCursor{LastLikedAt: timestamp, TrackID: "route-b"}
	second, err := likes.ListReceivedGroups(ctx, 9, c, 1)
	if err != nil || len(second) != 1 || second[0].TrackID != "route-a" {
		t.Fatalf("group tie lost next row: %v %v", second, err)
	}
	people, err := likes.ListReceivedTrackLikers(ctx, 9, "route-b", &models.TrackLikeCursor{CreatedAt: timestamp, TrackID: "route-b", UserID: 3}, 2)
	if err != nil || len(people) != 2 || people[0].UserID != 2 || people[1].UserID != 1 {
		t.Fatalf("bad liker tie cursor: %v %v", people, err)
	}
	if err := likes.SetLike(ctx, 4, "route-b", false); err != nil {
		t.Fatal(err)
	}
	after, err := likes.ListReceivedGroups(ctx, 9, nil, 1)
	if err != nil || after[0].LikeCount != 3 || after[0].RecentLikes[0].UserID != 3 {
		t.Fatal("recent avatars not updated after cancellation")
	}
}
