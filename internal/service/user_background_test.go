package service

import (
	"context"
	"errors"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
	"os"
	"testing"
)

type backgroundTestDownloader struct{ fail bool }

func (d backgroundTestDownloader) DownloadObject(_ int64, source, dest string) error {
	if d.fail {
		return errors.New("download unavailable")
	}
	return os.WriteFile(dest, []byte(source), 0600)
}

func TestBackgroundCacheVersionsPreserveRawStorage(t *testing.T) {
	ctx := context.Background()
	users := repository.NewInMemoryUserRepository()
	_, _ = users.CreateIfNotExists(ctx, &models.User{ID: 1})
	service := NewUserService(users)
	cache, err := NewAssetCacheService(t.TempDir(), "/api/v1/static/profile_backgrounds", []string{".jpg"}, ".jpg")
	if err != nil {
		t.Fatal(err)
	}
	cache.SetDownloader(backgroundTestDownloader{})
	service.SetBackgroundCache(cache)
	first := "https://example.com/first.jpg"
	second := "https://example.com/second.jpg"
	one, err := service.UpdateProfile(ctx, 1, UserProfilePatch{BackgroundURL: &first})
	if err != nil {
		t.Fatal(err)
	}
	two, err := service.UpdateProfile(ctx, 1, UserProfilePatch{BackgroundURL: &second})
	if err != nil {
		t.Fatal(err)
	}
	if one.BackgroundURL == two.BackgroundURL {
		t.Fatal("new cover reused an old cache URL")
	}
	detail, err := service.GetUserProfile(ctx, 1)
	if err != nil || detail.BackgroundURL != two.BackgroundURL {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	stored, _ := users.FindByID(ctx, 1)
	if stored.BackgroundURL != second {
		t.Fatalf("storage was decorated: %q", stored.BackgroundURL)
	}
	cache.SetDownloader(backgroundTestDownloader{fail: true})
	failed := "https://example.com/failed.jpg"
	_, err = service.UpdateProfile(ctx, 1, UserProfilePatch{BackgroundURL: &failed})
	if !errors.Is(err, ErrBackgroundUnavailable) {
		t.Fatalf("expected cache error, got %v", err)
	}
	stored, _ = users.FindByID(ctx, 1)
	if stored.BackgroundURL != second {
		t.Fatal("failed image replaced the previous background")
	}
}
