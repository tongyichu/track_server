package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"net/url"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

func validateTrackPreviewImages(images *models.TrackPreviewImages) error {
	if images == nil {
		return nil
	}
	for _, source := range []string{images.SquareDayURL, images.SquareNightURL, images.LandscapeDayURL, images.LandscapeNightURL} {
		u, err := url.Parse(source)
		if err != nil || len(source) > 2048 || u == nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
			return invalidArg("preview_images must contain four valid HTTP image URLs")
		}
	}
	return nil
}

func previewVersion(source string) string {
	hash := sha256.Sum256([]byte(source))
	return fmt.Sprintf("%x", hash[:8])
}

func TrackPreviewCacheKey(trackID, variant, source string) string {
	return trackID + "_preview_" + variant + "_" + previewVersion(source)
}

func (s *TrackService) removeTrackPreviewCache(track *models.Track) {
	remove := func(cache *AssetCacheService, key string) {
		if cache == nil {
			return
		}
		if err := cache.RemoveCached(key); err != nil {
			log.Printf("[TrackPreview] remove cache failed track=%s key=%s err=%v", track.ID, key, err)
		}
	}
	// Older admin lists cached the landscape preview without a source version.
	remove(s.screenshotCache, track.ID+"_landscape_day")
	for _, variant := range previewVariants {
		source := track.PreviewImages.Variant(variant)
		if source == "" {
			continue
		}
		key := TrackPreviewCacheKey(track.ID, variant, source)
		remove(s.previewCache, key)
		if variant == "landscape_day" {
			remove(s.screenshotCache, key)
		}
	}
}

// Lists only construct versioned URLs; the image request performs the cache fill.
func trackPreviewURLs(trackID string, images models.TrackPreviewImages) models.TrackPreviewImages {
	resource := func(variant, source string) string {
		if source == "" {
			return ""
		}
		return fmt.Sprintf("/api/v1/track/%s/previews/%s?v=%s", url.PathEscape(trackID), variant, previewVersion(source))
	}
	return models.TrackPreviewImages{
		SquareDayURL:      resource("square_day", images.SquareDayURL),
		SquareNightURL:    resource("square_night", images.SquareNightURL),
		LandscapeDayURL:   resource("landscape_day", images.LandscapeDayURL),
		LandscapeNightURL: resource("landscape_night", images.LandscapeNightURL),
	}
}

func (s *TrackService) SetPreviewCache(cache *AssetCacheService) {
	s.previewCache = cache
}

func (s *TrackService) TrackPreviewFile(ctx context.Context, viewerID int64, trackID, variant, version string) (string, error) {
	track, err := s.tracks.FindByID(ctx, trackID)
	if err != nil {
		return "", err
	}
	if track.Status == models.TrackStatusDeleted || (track.UserID != viewerID && !likeEligible(track)) {
		return "", repository.ErrNotFound
	}
	source := track.PreviewImages.Variant(variant)
	if source == "" || strings.TrimSpace(version) != previewVersion(source) {
		return "", repository.ErrNotFound
	}
	if s.previewCache == nil {
		return "", fmt.Errorf("preview cache is not configured")
	}
	key := TrackPreviewCacheKey(track.ID, variant, source)
	return s.previewCache.EnsureCachedFile(ctx, track.UserID, key, source)
}
