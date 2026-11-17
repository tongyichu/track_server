package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

var companionCoverVersionPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (s *CompanionService) saveCompanionCover(ctx context.Context, session *models.CompanionSession, expected *string, cover models.CompanionCoverPreview) (*models.CompanionSession, error) {
	if s.restrictions != nil {
		if err := s.restrictions.EnsureAllowed(ctx, session.OwnerUserID, AccountRestrictionMessageUpload); err != nil {
			return nil, err
		}
	}
	if expected == nil || (*expected != "" && !companionCoverVersionPattern.MatchString(*expected)) || !companionCoverVersionPattern.MatchString(cover.SourceVersion) {
		return nil, invalidArg("cover_preview requires a source version and expected source version")
	}
	if err := validateTrackPreviewImages(&cover.TrackPreviewImages); err != nil {
		return nil, invalidArg("cover_preview requires all four valid square/landscape day/night HTTP image URLs")
	}
	store, ok := s.repo.(repository.CompanionCoverRepository)
	if !ok {
		return nil, errors.New("companion cover storage unavailable")
	}
	if err := store.SaveCompanionCover(ctx, session.SessionID, session.OwnerUserID, *expected, cover); err != nil {
		return nil, err
	}
	current, err := s.repo.FindSessionByID(ctx, session.SessionID)
	if err != nil {
		return nil, err
	}
	result := companionSessionForResponse(current)
	return result, nil
}

func companionCoverURLs(session *models.CompanionSession) models.CompanionCoverPreview {
	if session == nil {
		return models.CompanionCoverPreview{}
	}
	cover := session.CoverPreview
	if cover.SourceVersion == "" {
		return cover
	}
	proxy := func(variant, source string) string {
		if source == "" {
			return ""
		}
		return fmt.Sprintf("/api/v1/companion/session/%s/cover/%s?v=%s", url.PathEscape(session.SessionID), variant, cover.SourceVersion)
	}
	cover.SquareDayURL = proxy("square_day", cover.SquareDayURL)
	cover.SquareNightURL = proxy("square_night", cover.SquareNightURL)
	cover.LandscapeDayURL = proxy("landscape_day", cover.LandscapeDayURL)
	cover.LandscapeNightURL = proxy("landscape_night", cover.LandscapeNightURL)
	return cover
}

func (s *CompanionService) CompanionCoverFile(ctx context.Context, viewer int64, id, variant, version string) (string, error) {
	session, _, err := s.resultAccess(ctx, viewer, id)
	if err != nil {
		return "", err
	}
	if session.Status != models.CompanionSessionStatusEnded {
		return "", repository.ErrNotFound
	}
	cover := session.CoverPreview
	if version == "" || version != cover.SourceVersion {
		return "", repository.ErrNotFound
	}
	source := cover.Variant(variant)
	if source == "" {
		return "", repository.ErrNotFound
	}
	if s.screenshotCache == nil {
		return "", errors.New("companion cover cache unavailable")
	}
	key := companionScreenshotCacheKey(id) + "_preview_" + version + "_" + variant
	return s.screenshotCache.EnsureCachedFile(ctx, session.OwnerUserID, key, source)
}
