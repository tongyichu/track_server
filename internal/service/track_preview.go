package service

import (
	"context"
	"crypto/sha256"
	"fmt"
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
	key := track.ID + "_preview_" + variant + "_" + previewVersion(source)
	return s.previewCache.EnsureCachedFile(ctx, track.UserID, key, source)
}
