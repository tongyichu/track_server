package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type ScenicSpotCatalogService struct {
	spots    repository.ScenicSpotRepository
	tracks   repository.TrackRepository
	places   repository.TrackPlaceRepository
	provider *TrackPlaceService
}

func invalidScenicSpot(message string) error { return &InvalidArgumentError{Msg: message} }

func NewScenicSpotCatalogService(spots repository.ScenicSpotRepository, tracks repository.TrackRepository, places repository.TrackPlaceRepository) *ScenicSpotCatalogService {
	return &ScenicSpotCatalogService{spots: spots, tracks: tracks, places: places}
}
func (s *ScenicSpotCatalogService) SetProviderService(provider *TrackPlaceService) {
	if s != nil {
		s.provider = provider
	}
}
func (s *ScenicSpotCatalogService) Enabled() bool { return s != nil && s.spots != nil }
func newScenicID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(b[:])
}
func scenicJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
func (s *ScenicSpotCatalogService) audit(ctx context.Context, spotID, action, operator, details string, before, after any) {
	if s == nil || s.spots == nil {
		return
	}
	now := time.Now()
	_ = s.spots.AppendScenicSpotAudit(ctx, &models.ScenicSpotAudit{ID: fmt.Sprintf("AUD-%020d-%s", now.UnixNano(), newScenicID("")), SpotID: spotID, Action: action, Operator: operator, Details: details, Before: scenicJSON(before), After: scenicJSON(after), CreatedAt: now})
}
func validateScenicSpot(v *models.ScenicSpot) error {
	v.Name = strings.TrimSpace(v.Name)
	v.Description = strings.TrimSpace(v.Description)
	v.Address = strings.TrimSpace(v.Address)
	v.Category = strings.TrimSpace(v.Category)
	v.CityCode = strings.TrimSpace(v.CityCode)
	v.CityName = strings.TrimSpace(v.CityName)
	if utf8.RuneCountInString(v.Name) > 160 || utf8.RuneCountInString(v.Description) > 8000 || utf8.RuneCountInString(v.Address) > 500 || utf8.RuneCountInString(v.Category) > 100 || len(v.Aliases) > 40 || len(v.Photos) > 30 || len(v.Sources) > 8 {
		return invalidScenicSpot("scenic spot fields exceed limits")
	}
	seen := map[string]bool{}
	for i := range v.Aliases {
		v.Aliases[i] = strings.TrimSpace(v.Aliases[i])
		if v.Aliases[i] == "" || utf8.RuneCountInString(v.Aliases[i]) > 160 {
			return invalidScenicSpot("invalid scenic spot alias")
		}
	}
	for i := range v.Photos {
		p := &v.Photos[i]
		if strings.TrimSpace(p.ID) == "" || (!strings.HasPrefix(p.URL, "/api/v1/static/scenic_spots/") && !strings.HasPrefix(p.URL, "https://")) {
			return invalidScenicSpot("scenic spot photos must use an owned static URL or HTTPS URL")
		}
		if utf8.RuneCountInString(p.Caption) > 255 || utf8.RuneCountInString(p.AltText) > 255 || utf8.RuneCountInString(p.Attribution) > 255 {
			return invalidScenicSpot("scenic spot photo metadata exceeds limits")
		}
	}
	for i := range v.Sources {
		source := &v.Sources[i]
		source.Provider = strings.ToLower(strings.TrimSpace(source.Provider))
		source.ProviderID = strings.TrimSpace(source.ProviderID)
		if source.Provider != "amap" && source.Provider != "google" {
			return invalidScenicSpot("provider must be amap or google")
		}
		if source.ProviderID == "" || len(source.ProviderID) > 512 {
			return invalidScenicSpot("invalid provider_id")
		}
		key := source.Provider + ":" + source.ProviderID
		if seen[key] {
			return repository.ErrAlreadyExists
		}
		seen[key] = true
		if source.Status == "" {
			source.Status = "active"
		}
		if source.Status != "active" && source.Status != "unverified" && source.Status != "disabled" {
			return invalidScenicSpot("invalid provider source status")
		}
	}
	if v.Latitude < -90 || v.Latitude > 90 || v.Longitude < -180 || v.Longitude > 180 {
		if v.Latitude != 0 || v.Longitude != 0 {
			return invalidScenicSpot("invalid scenic spot coordinates")
		}
	}
	return nil
}
func (s *ScenicSpotCatalogService) List(ctx context.Context, f repository.ScenicSpotListFilter) ([]*models.ScenicSpot, int64, error) {
	if !s.Enabled() {
		return nil, 0, errors.New("scenic spot catalog unavailable")
	}
	return s.spots.ListScenicSpots(ctx, f)
}

type ScenicSpotProviderContentBackfillResult struct {
	Scanned     int    `json:"scanned"`
	Synced      int    `json:"synced"`
	Coordinates int    `json:"coordinates"`
	NoSource    int    `json:"no_source"`
	Skipped     int    `json:"skipped"`
	Failed      int    `json:"failed"`
	HasMore     bool   `json:"has_more"`
	NextCursor  string `json:"next_cursor,omitempty"`
}

// BackfillProviderContent fetches missing provider text and photos once and
// stores them alongside the scenic spot. Explicit admin edits remain intact.
func (s *ScenicSpotCatalogService) BackfillProviderContent(ctx context.Context, operator, cursor string, limit int) (*ScenicSpotProviderContentBackfillResult, error) {
	if !s.Enabled() {
		return nil, errors.New("scenic spot catalog unavailable")
	}
	if limit < 1 || limit > 10 {
		limit = 5
	}
	spots, _, err := s.spots.ListScenicSpots(ctx, repository.ScenicSpotListFilter{Cursor: cursor, Limit: limit})
	if err != nil {
		return nil, err
	}
	out := &ScenicSpotProviderContentBackfillResult{Scanned: len(spots), HasMore: len(spots) == limit}
	if len(spots) > 0 {
		out.NextCursor = spots[len(spots)-1].ID
	}
	for _, spot := range spots {
		if spot == nil || spot.Status == models.ScenicSpotMerged || spot.ProviderDataStatus == "no_source" {
			out.Skipped++
			continue
		}
		contentSynced := spot.ProviderDataStatus == "synced"
		coordinatesChecked := spot.ProviderCoordinatesStatus == "synced" || spot.ProviderCoordinatesStatus == "unavailable"
		if contentSynced && (coordinatesChecked || (spot.Latitude != 0 && spot.Longitude != 0)) {
			out.Skipped++
			continue
		}
		source := preferredScenicSpotSource(spot.Sources)
		if source == nil {
			if contentSynced {
				spot.ProviderCoordinatesStatus = "unavailable"
				if err := s.persistProviderContentSync(ctx, spot, operator, "provider coordinates skipped: no active source"); err != nil {
					out.Failed++
					continue
				}
				out.NoSource++
				continue
			}
			spot.ProviderDataStatus = "no_source"
			if err := s.persistProviderContentSync(ctx, spot, operator, "provider content skipped: no active source"); err != nil {
				out.Failed++
				continue
			}
			out.NoSource++
			continue
		}
		if contentSynced {
			lat, lng, coordinateSystem, coordinateErr := s.provider.ScenicSpotProviderCoordinates(ctx, spot.ID, source.Provider, source.ProviderID)
			if coordinateErr != nil {
				spot.ProviderCoordinatesStatus = "failed"
				if err := s.persistProviderContentSync(ctx, spot, operator, "provider coordinates fetch failed"); err != nil {
					out.Failed++
					continue
				}
				out.Failed++
				continue
			}
			if spot.Latitude == 0 || spot.Longitude == 0 {
				spot.Latitude, spot.Longitude, spot.CoordinateSystem = lat, lng, coordinateSystem
			}
			spot.ProviderCoordinatesStatus = "synced"
			if err := s.persistProviderContentSync(ctx, spot, operator, source.Provider+" provider coordinates synced"); err != nil {
				out.Failed++
				continue
			}
			out.Coordinates++
			continue
		}
		preview, fetchErr := s.PreviewProviderSource(ctx, spot.ID, source.Provider, source.ProviderID)
		if fetchErr != nil {
			if !contentSynced {
				spot.ProviderDataStatus = "failed"
			}
			spot.ProviderCoordinatesStatus = "failed"
			if err := s.persistProviderContentSync(ctx, spot, operator, "provider content fetch failed"); err != nil {
				out.Failed++
				continue
			}
			out.Failed++
			continue
		}
		if !spot.ManualEdited && (strings.TrimSpace(spot.Name) != "" || strings.TrimSpace(spot.Category) != "" || strings.TrimSpace(spot.Address) != "" || strings.TrimSpace(spot.Description) != "" || len(spot.Photos) > 0) {
			spot.ManualEdited = true
		}
		if strings.TrimSpace(spot.Name) == "" {
			spot.Name = strings.TrimSpace(preview.Name)
		}
		if strings.TrimSpace(spot.Category) == "" {
			spot.Category = strings.TrimSpace(preview.Category)
		}
		if strings.TrimSpace(spot.Address) == "" {
			spot.Address = strings.TrimSpace(preview.Address)
		}
		if strings.TrimSpace(spot.Description) == "" {
			spot.Description = strings.TrimSpace(preview.Description)
		}
		if (spot.Latitude == 0 || spot.Longitude == 0) && preview.Latitude != 0 && preview.Longitude != 0 {
			spot.Latitude, spot.Longitude = preview.Latitude, preview.Longitude
			spot.CoordinateSystem = preview.CoordinateSystem
		}
		if preview.Latitude != 0 && preview.Longitude != 0 {
			spot.ProviderCoordinatesStatus = "synced"
		} else {
			spot.ProviderCoordinatesStatus = "unavailable"
		}
		knownPhotos := make(map[string]struct{}, len(spot.Photos))
		for _, photo := range spot.Photos {
			knownPhotos[photo.URL] = struct{}{}
		}
		for _, photo := range preview.Photos {
			if len(spot.Photos) >= 30 || strings.TrimSpace(photo.URL) == "" {
				break
			}
			if _, exists := knownPhotos[photo.URL]; exists {
				continue
			}
			spot.Photos = append(spot.Photos, models.ScenicSpotPhoto{
				ID:             newScenicID("SPP-"),
				URL:            photo.URL,
				Caption:        strings.TrimSpace(photo.Title),
				AltText:        strings.TrimSpace(photo.Title),
				Attribution:    strings.TrimSpace(photo.Author),
				AttributionURL: strings.TrimSpace(photo.AuthorURL),
				SourceURL:      strings.TrimSpace(photo.SourceURL),
				Provider:       source.Provider,
			})
			knownPhotos[photo.URL] = struct{}{}
		}
		now := time.Now()
		spot.ProviderDataStatus = "synced"
		spot.ProviderDataFetchedAt = &now
		if err := s.persistProviderContentSync(ctx, spot, operator, source.Provider+" provider content synced"); err != nil {
			out.Failed++
			continue
		}
		out.Synced++
	}
	return out, nil
}

func preferredScenicSpotSource(sources []models.ScenicSpotSource) *models.ScenicSpotSource {
	for _, provider := range []string{"amap", "google"} {
		for i := range sources {
			if sources[i].Provider == provider && sources[i].ProviderID != "" && sources[i].Status == "active" {
				return &sources[i]
			}
		}
	}
	for i := range sources {
		if sources[i].ProviderID != "" && sources[i].Status == "unverified" {
			return &sources[i]
		}
	}
	return nil
}

func (s *ScenicSpotCatalogService) persistProviderContentSync(ctx context.Context, spot *models.ScenicSpot, operator, details string) error {
	expected := spot.Version
	spot.Version++
	spot.UpdatedAt = time.Now()
	spot.UpdatedBy = operator
	if err := s.spots.UpdateScenicSpot(ctx, spot, expected); err != nil {
		return err
	}
	s.audit(ctx, spot.ID, "provider_content_sync", operator, details, nil, spot)
	return nil
}

func (s *ScenicSpotCatalogService) Get(ctx context.Context, id string) (*models.ScenicSpot, error) {
	return s.spots.GetScenicSpot(ctx, id)
}
func (s *ScenicSpotCatalogService) StaticMap(ctx context.Context, id string) ([]byte, string, error) {
	spot, err := s.Get(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if s.provider == nil || spot.Latitude == 0 || spot.Longitude == 0 {
		return nil, "", repository.ErrNotFound
	}
	return s.provider.ScenicSpotStaticMap(ctx, spot.ID, spot.Latitude, spot.Longitude, spot.CoordinateSystem)
}
func (s *ScenicSpotCatalogService) MapJSKey() string {
	if s == nil || s.provider == nil {
		return ""
	}
	return s.provider.ScenicSpotMapJSKey()
}
func (s *ScenicSpotCatalogService) ProxyAMapJSAPI(ctx context.Context, requestPath string, query url.Values) ([]byte, string, error) {
	if s == nil || s.provider == nil {
		return nil, "", errors.New("provider details unavailable")
	}
	return s.provider.ProxyAMapJSAPI(ctx, requestPath, query)
}
func (s *ScenicSpotCatalogService) Create(ctx context.Context, operator string, v *models.ScenicSpot) (*models.ScenicSpot, error) {
	if err := validateScenicSpot(v); err != nil {
		return nil, err
	}
	v.ID = newScenicID("SCP-")
	v.Status = models.ScenicSpotDraft
	v.ManualEdited = true
	v.Version = 1
	v.CreatedAt = time.Now()
	v.UpdatedAt = v.CreatedAt
	v.CreatedBy = operator
	v.UpdatedBy = operator
	for _, source := range v.Sources {
		if old, err := s.spots.FindScenicSpotBySource(ctx, source.Provider, source.ProviderID); err == nil && old.ID != "" {
			return nil, repository.ErrAlreadyExists
		}
	}
	if err := s.spots.CreateScenicSpot(ctx, v); err != nil {
		return nil, err
	}
	s.audit(ctx, v.ID, "create", operator, "draft created", nil, v)
	return v, nil
}
func (s *ScenicSpotCatalogService) Update(ctx context.Context, operator, id string, expected int64, v *models.ScenicSpot) (*models.ScenicSpot, error) {
	old, err := s.spots.GetScenicSpot(ctx, id)
	if err != nil {
		return nil, err
	}
	if old.Version != expected {
		return nil, repository.ErrConflict
	}
	if old.Status == models.ScenicSpotMerged {
		return nil, repository.ErrConflict
	}
	v.ID = id
	v.Status = old.Status
	v.ManualEdited = true
	v.MergedInto = old.MergedInto
	v.CreatedAt = old.CreatedAt
	v.CreatedBy = old.CreatedBy
	v.PublishedAt = old.PublishedAt
	v.Version = expected + 1
	v.UpdatedAt = time.Now()
	v.UpdatedBy = operator
	if err := validateScenicSpot(v); err != nil {
		return nil, err
	}
	for _, source := range v.Sources {
		found, e := s.spots.FindScenicSpotBySource(ctx, source.Provider, source.ProviderID)
		if e == nil && found.ID != id {
			return nil, repository.ErrAlreadyExists
		}
	}
	if err := s.spots.UpdateScenicSpot(ctx, v, expected); err != nil {
		return nil, err
	}
	s.audit(ctx, id, "update", operator, "content or provider sources updated", old, v)
	return v, nil
}
func (s *ScenicSpotCatalogService) SetStatus(ctx context.Context, operator, id, status string, expected int64) (*models.ScenicSpot, error) {
	if status != models.ScenicSpotPublished && status != models.ScenicSpotArchived && status != models.ScenicSpotDraft {
		return nil, invalidScenicSpot("invalid scenic spot status")
	}
	old, err := s.spots.GetScenicSpot(ctx, id)
	if err != nil {
		return nil, err
	}
	if old.Status == models.ScenicSpotMerged {
		return nil, repository.ErrConflict
	}
	if old.Version != expected {
		return nil, repository.ErrConflict
	}
	if status == models.ScenicSpotPublished && old.Name == "" {
		return nil, invalidScenicSpot("name is required before publishing")
	}
	old.Status = status
	old.Version++
	old.UpdatedAt = time.Now()
	old.UpdatedBy = operator
	if status == models.ScenicSpotPublished {
		now := old.UpdatedAt
		old.PublishedAt = &now
	}
	if err := s.spots.UpdateScenicSpot(ctx, old, expected); err != nil {
		return nil, err
	}
	s.audit(ctx, id, "status", operator, status, nil, old)
	return old, nil
}
func (s *ScenicSpotCatalogService) AssignTrack(ctx context.Context, operator, spotID, trackID, origin string) error {
	spot, err := s.spots.GetScenicSpot(ctx, spotID)
	if err != nil {
		return err
	}
	if spot.Status == models.ScenicSpotMerged || spot.Status == models.ScenicSpotArchived {
		return repository.ErrConflict
	}
	track, err := s.tracks.FindByID(ctx, trackID)
	if err != nil {
		return err
	}
	if track.Status == models.TrackStatusDeleted {
		return repository.ErrNotFound
	}
	old, oldErr := s.spots.GetTrackScenicSpotLink(ctx, trackID)
	link := &models.TrackScenicSpotLink{TrackID: trackID, SpotID: spotID, Origin: origin, UpdatedAt: time.Now(), UpdatedBy: operator}
	if oldErr == nil {
		if old.SpotID == spotID {
			return nil
		}
		if err := s.spots.SetTrackScenicSpotLink(ctx, link, old.SpotID, false); err != nil {
			return err
		}
	} else if errors.Is(oldErr, repository.ErrNotFound) {
		if err := s.spots.SetTrackScenicSpotLink(ctx, link, "", true); err != nil {
			return err
		}
	} else {
		return oldErr
	}
	s.audit(ctx, spotID, "track_assign", operator, trackID, old, link)
	return nil
}
func (s *ScenicSpotCatalogService) UnassignTrack(ctx context.Context, operator, trackID string) error {
	old, err := s.spots.GetTrackScenicSpotLink(ctx, trackID)
	if err != nil {
		return err
	}
	if err := s.spots.DeleteTrackScenicSpotLink(ctx, trackID, old.SpotID); err != nil {
		return err
	}
	s.audit(ctx, old.SpotID, "track_unassign", operator, trackID, old, nil)
	return nil
}
func (s *ScenicSpotCatalogService) TrackLinks(ctx context.Context, spotID, cursor string, limit int) ([]*models.TrackScenicSpotLink, error) {
	return s.spots.ListTrackScenicSpotLinks(ctx, spotID, cursor, limit)
}
func (s *ScenicSpotCatalogService) Audit(ctx context.Context, spotID, cursor string, limit int) ([]*models.ScenicSpotAudit, error) {
	return s.spots.ListScenicSpotAudit(ctx, spotID, cursor, limit)
}

func (s *ScenicSpotCatalogService) VerifySource(ctx context.Context, operator, spotID, provider, providerID string) (*models.ScenicSpot, error) {
	if s.provider == nil {
		return nil, errors.New("provider verification unavailable")
	}
	spot, err := s.spots.GetScenicSpot(ctx, spotID)
	if err != nil {
		return nil, err
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	providerID = strings.TrimSpace(providerID)
	if spot.Status == models.ScenicSpotMerged {
		return nil, repository.ErrConflict
	}
	if existing, e := s.spots.FindScenicSpotBySource(ctx, provider, providerID); e == nil && existing.ID != spotID {
		return nil, repository.ErrAlreadyExists
	}
	if err := s.provider.VerifyScenicSpotSource(ctx, spotID, provider, providerID); err != nil {
		return nil, err
	}
	now := time.Now()
	source := models.ScenicSpotSource{Provider: provider, ProviderID: providerID, Status: "active", LastVerifiedAt: &now}
	if provider == "google" {
		refresh := now.AddDate(1, 0, 0)
		source.RefreshAfter = &refresh
	}
	found := false
	for i, v := range spot.Sources {
		if v.Provider == provider && v.ProviderID == providerID {
			spot.Sources[i] = source
			found = true
			break
		}
	}
	if !found {
		if len(spot.Sources) >= 8 {
			return nil, errors.New("maximum provider sources reached")
		}
		spot.Sources = append(spot.Sources, source)
	}
	spot.ProviderDataStatus = ""
	spot.ProviderDataFetchedAt = nil
	expected := spot.Version
	spot.Version++
	spot.UpdatedAt = now
	spot.UpdatedBy = operator
	if err := s.spots.UpdateScenicSpot(ctx, spot, expected); err != nil {
		return nil, err
	}
	s.audit(ctx, spotID, "provider_verified", operator, provider+" source verified", nil, source)
	return spot, nil
}

// PreviewProviderSource loads the current provider details. The admin preview
// endpoint returns it directly; the backfill flow stores it in the catalog.
func (s *ScenicSpotCatalogService) PreviewProviderSource(ctx context.Context, spotID, provider, providerID string) (*models.TrackPrimaryScenicSpot, error) {
	if s == nil || s.provider == nil {
		return nil, errors.New("provider details unavailable")
	}
	if _, err := s.spots.GetScenicSpot(ctx, spotID); err != nil {
		return nil, err
	}
	return s.provider.ScenicSpotProviderPreview(ctx, spotID, provider, providerID)
}

func (s *ScenicSpotCatalogService) Merge(ctx context.Context, operator, targetID, sourceID string) (*models.ScenicSpot, error) {
	if targetID == sourceID {
		return nil, errors.New("cannot merge a scenic spot into itself")
	}
	target, err := s.spots.GetScenicSpot(ctx, targetID)
	if err != nil {
		return nil, err
	}
	source, err := s.spots.GetScenicSpot(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	if target.Status == models.ScenicSpotMerged || source.Status == models.ScenicSpotMerged {
		return nil, repository.ErrConflict
	}
	if target.Status == models.ScenicSpotArchived {
		return nil, repository.ErrConflict
	}
	sourcesChanged := false
	for _, candidate := range source.Sources {
		found := false
		for _, current := range target.Sources {
			if current.Provider == candidate.Provider && current.ProviderID == candidate.ProviderID {
				found = true
				break
			}
		}
		if !found {
			target.Sources = append(target.Sources, candidate)
			sourcesChanged = true
		}
	}
	if len(target.Sources) > 8 {
		return nil, errors.New("merge would exceed maximum provider sources")
	}
	if sourcesChanged {
		expectedTarget := target.Version
		target.Version++
		target.UpdatedAt = time.Now()
		target.UpdatedBy = operator
		if err := s.spots.UpdateScenicSpot(ctx, target, expectedTarget); err != nil {
			return nil, err
		}
		s.audit(ctx, targetID, "merge_sources", operator, "provider identities copied from "+sourceID, nil, target.Sources)
	}
	cursor := ""
	for {
		links, e := s.spots.ListTrackScenicSpotLinks(ctx, sourceID, cursor, 100)
		if e != nil {
			return nil, e
		}
		if len(links) == 0 {
			break
		}
		for _, link := range links {
			if e := s.AssignTrack(ctx, operator, targetID, link.TrackID, "merge"); e != nil {
				return nil, e
			}
		}
		cursor = links[len(links)-1].TrackID
		if len(links) < 100 {
			break
		}
	}
	sourceBefore := *source
	expected := source.Version
	source.Status = models.ScenicSpotMerged
	source.MergedInto = targetID
	source.Version++
	source.UpdatedAt = time.Now()
	source.UpdatedBy = operator
	if err := s.spots.UpdateScenicSpot(ctx, source, expected); err != nil {
		return nil, err
	}
	s.audit(ctx, sourceID, "merge", operator, "merged into "+targetID, sourceBefore, source)
	return target, nil
}
func (s *ScenicSpotCatalogService) Split(ctx context.Context, operator, sourceID string, newSpot *models.ScenicSpot, trackIDs []string) (*models.ScenicSpot, error) {
	if len(trackIDs) == 0 || len(trackIDs) > 500 {
		return nil, invalidScenicSpot("split requires 1 to 500 tracks")
	}
	if _, err := s.spots.GetScenicSpot(ctx, sourceID); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(trackIDs))
	for _, trackID := range trackIDs {
		if _, exists := seen[trackID]; exists {
			return nil, invalidScenicSpot("split contains duplicate track IDs")
		}
		seen[trackID] = struct{}{}
		link, err := s.spots.GetTrackScenicSpotLink(ctx, trackID)
		if err != nil || link.SpotID != sourceID {
			return nil, invalidScenicSpot("every selected track must belong to the source scenic spot")
		}
	}
	created, err := s.Create(ctx, operator, newSpot)
	if err != nil {
		return nil, err
	}
	moved := 0
	for _, trackID := range trackIDs {
		if e := s.AssignTrack(ctx, operator, created.ID, trackID, "manual"); e != nil {
			return nil, e
		}
		moved++
	}
	s.audit(ctx, sourceID, "split", operator, fmt.Sprintf("split %d tracks into %s", moved, created.ID), nil, created)
	return created, nil
}

type ScenicSpotBackfillResult struct {
	Scanned              int    `json:"scanned"`
	Linked               int    `json:"linked"`
	Skipped              int    `json:"skipped"`
	MissingSource        int    `json:"missing_source"`
	ProviderQueries      int    `json:"provider_queries"`
	ProviderMatched      int    `json:"provider_matched"`
	ProviderUnmatched    int    `json:"provider_unmatched"`
	ProviderSkipped      int    `json:"provider_skipped"`
	ProviderIneligible   int    `json:"provider_ineligible"`
	ProviderDeferred     int    `json:"provider_deferred"`
	QuotaDeferred        int    `json:"quota_deferred"`
	BusyDeferred         int    `json:"busy_deferred"`
	ProviderFailed       int    `json:"provider_failed"`
	IndexRefreshFailed   int    `json:"index_refresh_failed"`
	InvalidSamples       int    `json:"invalid_samples"`
	ProviderUnconfigured int    `json:"provider_unconfigured"`
	MissingIndex         int    `json:"missing_index"`
	NextStartTime        string `json:"next_cursor_start_time,omitempty"`
	NextTrackID          string `json:"next_cursor_id,omitempty"`
	HasMore              bool   `json:"has_more"`
}

// BackfillLegacy materializes the existing primary-place decisions as internal
// draft spots and track links. It never publishes imported provider-derived names.
func (s *ScenicSpotCatalogService) BackfillLegacy(ctx context.Context, cursor *models.TrackListCursor, limit int) (*ScenicSpotBackfillResult, error) {
	if s.tracks == nil || s.places == nil {
		return nil, errors.New("legacy place data unavailable")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	tracks, err := s.tracks.ListAll(ctx, cursor, limit+1)
	if err != nil {
		return nil, err
	}
	out := &ScenicSpotBackfillResult{}
	hasMore := len(tracks) > limit
	if hasMore {
		tracks = tracks[:limit]
	}
	existingLinks := make(map[string]bool, len(tracks))
	for _, track := range tracks {
		if _, e := s.spots.GetTrackScenicSpotLink(ctx, track.ID); e == nil {
			existingLinks[track.ID] = true
		}
	}
	// Resolve only tracks that have no usable legacy provider ID. The provider
	// service applies its normal daily budget and a ten-request per-run cap.
	if s.provider != nil {
		var resolveIDs []string
		for _, track := range tracks {
			if existingLinks[track.ID] {
				continue
			}
			if !s.hasLegacyProviderID(ctx, track.ID) {
				resolveIDs = append(resolveIDs, track.ID)
			}
		}
		resolved, e := s.provider.ResolveTracksForBackfill(ctx, resolveIDs)
		if e != nil {
			return nil, e
		}
		out.ProviderQueries = resolved.ProviderQueries
		out.ProviderMatched = resolved.Matched
		out.ProviderUnmatched = resolved.Unmatched
		out.ProviderSkipped = resolved.Skipped
		out.ProviderIneligible = resolved.Ineligible
		out.ProviderDeferred = resolved.Deferred
		out.QuotaDeferred = resolved.QuotaDeferred
		out.BusyDeferred = resolved.BusyDeferred
		out.ProviderFailed = resolved.Failed
		out.IndexRefreshFailed = resolved.IndexRefreshFailed
		out.InvalidSamples = resolved.InvalidSamples
		out.ProviderUnconfigured = resolved.Unconfigured
		out.MissingIndex = resolved.MissingIndex
	}
	for _, track := range tracks {
		out.Scanned++
		out.NextStartTime = track.StartTime.Format(time.RFC3339Nano)
		out.NextTrackID = track.ID
		if _, e := s.spots.GetTrackScenicSpotLink(ctx, track.ID); e == nil {
			if existingLinks[track.ID] {
				out.Skipped++
			} else {
				out.Linked++
			}
			continue
		}
		records, e := s.places.GetPlaceRecords(ctx, []string{"match:" + track.ID})
		if e != nil {
			return nil, e
		}
		record := records["match:"+track.ID]
		if record == nil {
			out.MissingSource++
			continue
		}
		var match models.TrackPlaceMatch
		if json.Unmarshal([]byte(record.Data), &match) != nil || match.Status != "matched" || match.PrimaryPlaceID == "" {
			out.MissingSource++
			continue
		}
		placeRecords, e := s.places.GetPlaceRecords(ctx, []string{"place:" + match.PrimaryPlaceID})
		if e != nil {
			return nil, e
		}
		placeRecord := placeRecords["place:"+match.PrimaryPlaceID]
		if placeRecord == nil {
			out.MissingSource++
			continue
		}
		var place models.TrackPlace
		if json.Unmarshal([]byte(placeRecord.Data), &place) != nil || place.ProviderID == "" {
			out.MissingSource++
			continue
		}
		spot, e := s.spots.FindScenicSpotBySource(ctx, place.Provider, place.ProviderID)
		if errors.Is(e, repository.ErrNotFound) {
			source := models.ScenicSpotSource{Provider: place.Provider, ProviderID: place.ProviderID, Status: "active"}
			if place.Provider == "google" {
				expired := time.Now().Add(-time.Second)
				source.RefreshAfter = &expired
			}
			// Backfill only the provider identifier. Provider-derived names,
			// coordinates, descriptions and photos are not copied into the catalog.
			spot = &models.ScenicSpot{ID: newScenicID("SCP-"), Sources: []models.ScenicSpotSource{source}, Status: models.ScenicSpotDraft, Version: 1, CreatedAt: time.Now(), UpdatedAt: time.Now(), CreatedBy: "backfill", UpdatedBy: "backfill"}
			if e = s.spots.CreateScenicSpot(ctx, spot); e != nil {
				if !errors.Is(e, repository.ErrAlreadyExists) {
					return nil, e
				}
				spot, e = s.spots.FindScenicSpotBySource(ctx, place.Provider, place.ProviderID)
			}
		}
		if e != nil {
			return nil, e
		}
		link := &models.TrackScenicSpotLink{TrackID: track.ID, SpotID: spot.ID, Origin: "backfill", Provider: place.Provider, Confidence: 1, UpdatedAt: time.Now(), UpdatedBy: "backfill"}
		if e = s.spots.SetTrackScenicSpotLink(ctx, link, "", true); e != nil {
			if errors.Is(e, repository.ErrConflict) {
				out.Skipped++
				continue
			}
			return nil, e
		}
		s.audit(ctx, spot.ID, "backfill_link", "backfill", "legacy primary match linked to internal spot", nil, link)
		out.Linked++
	}
	out.HasMore = hasMore
	if hasMore && len(tracks) > 0 {
		out.NextStartTime = tracks[len(tracks)-1].StartTime.Format(time.RFC3339Nano)
		out.NextTrackID = tracks[len(tracks)-1].ID
	}
	return out, nil
}

func (s *ScenicSpotCatalogService) hasLegacyProviderID(ctx context.Context, trackID string) bool {
	records, err := s.places.GetPlaceRecords(ctx, []string{"match:" + trackID})
	if err != nil {
		return false
	}
	record := records["match:"+trackID]
	if record == nil {
		return false
	}
	var match models.TrackPlaceMatch
	if json.Unmarshal([]byte(record.Data), &match) != nil || match.Status != "matched" || match.PrimaryPlaceID == "" {
		return false
	}
	placeRecords, err := s.places.GetPlaceRecords(ctx, []string{"place:" + match.PrimaryPlaceID})
	if err != nil {
		return false
	}
	placeRecord := placeRecords["place:"+match.PrimaryPlaceID]
	if placeRecord == nil {
		return false
	}
	var place models.TrackPlace
	return json.Unmarshal([]byte(placeRecord.Data), &place) == nil && place.Provider != "" && place.ProviderID != ""
}

// SyncLegacyTrack links newly completed provider matches to the internal catalog.
// It is called by the background place scanner, never by client list requests.
func (s *ScenicSpotCatalogService) SyncLegacyTrack(ctx context.Context, trackID string) error {
	if s == nil || s.spots == nil || s.places == nil {
		return nil
	}
	if _, err := s.spots.GetTrackScenicSpotLink(ctx, trackID); err == nil {
		return nil
	} else if !errors.Is(err, repository.ErrNotFound) {
		return err
	}
	records, err := s.places.GetPlaceRecords(ctx, []string{"match:" + trackID})
	if err != nil {
		return err
	}
	var match models.TrackPlaceMatch
	record := records["match:"+trackID]
	if record == nil || json.Unmarshal([]byte(record.Data), &match) != nil || match.Status != "matched" || match.PrimaryPlaceID == "" {
		return nil
	}
	placeRecords, err := s.places.GetPlaceRecords(ctx, []string{"place:" + match.PrimaryPlaceID})
	if err != nil {
		return err
	}
	var place models.TrackPlace
	placeRecord := placeRecords["place:"+match.PrimaryPlaceID]
	if placeRecord == nil || json.Unmarshal([]byte(placeRecord.Data), &place) != nil || place.ProviderID == "" || (place.Provider != "amap" && place.Provider != "google") {
		return nil
	}
	spot, err := s.spots.FindScenicSpotBySource(ctx, place.Provider, place.ProviderID)
	if errors.Is(err, repository.ErrNotFound) {
		source := models.ScenicSpotSource{Provider: place.Provider, ProviderID: place.ProviderID, Status: "active"}
		if place.Provider == "google" {
			expired := time.Now().Add(-time.Second)
			source.RefreshAfter = &expired
		}
		now := time.Now()
		spot = &models.ScenicSpot{ID: newScenicID("SCP-"), Sources: []models.ScenicSpotSource{source}, Status: models.ScenicSpotDraft, Version: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "system", UpdatedBy: "system"}
		if err = s.spots.CreateScenicSpot(ctx, spot); err != nil {
			if !errors.Is(err, repository.ErrAlreadyExists) {
				return err
			}
			spot, err = s.spots.FindScenicSpotBySource(ctx, place.Provider, place.ProviderID)
		}
	}
	if err != nil {
		return err
	}
	link := &models.TrackScenicSpotLink{TrackID: trackID, SpotID: spot.ID, Origin: "auto", Provider: place.Provider, Confidence: 1, UpdatedAt: time.Now(), UpdatedBy: "system"}
	if err = s.spots.SetTrackScenicSpotLink(ctx, link, "", true); err != nil && !errors.Is(err, repository.ErrConflict) {
		return err
	}
	s.audit(ctx, spot.ID, "auto_link", "system", "background primary match linked to internal spot", nil, link)
	return nil
}

func (s *ScenicSpotCatalogService) PublicSpotForTrack(ctx context.Context, trackID string) (*models.ScenicSpot, error) {
	link, err := s.spots.GetTrackScenicSpotLink(ctx, trackID)
	if err != nil {
		return nil, err
	}
	spot, err := s.spots.GetScenicSpot(ctx, link.SpotID)
	if err != nil {
		return nil, err
	}
	for i := 0; i < 4 && spot.Status == models.ScenicSpotMerged && spot.MergedInto != ""; i++ {
		spot, err = s.spots.GetScenicSpot(ctx, spot.MergedInto)
		if err != nil {
			return nil, err
		}
	}
	if spot.Status != models.ScenicSpotPublished {
		return nil, repository.ErrNotFound
	}
	return spot, nil
}

func (s *ScenicSpotCatalogService) RefreshExpiredSources(ctx context.Context, trackID string) error {
	if s.provider == nil {
		return nil
	}
	link, err := s.spots.GetTrackScenicSpotLink(ctx, trackID)
	if err != nil {
		return nil
	}
	spot, err := s.spots.GetScenicSpot(ctx, link.SpotID)
	if err != nil || spot.Status != models.ScenicSpotPublished {
		return nil
	}
	for _, source := range spot.Sources {
		if source.Provider != "google" || source.Status != "active" {
			continue
		}
		if source.RefreshAfter != nil && source.RefreshAfter.After(time.Now()) {
			continue
		}
		if _, err := s.VerifySource(ctx, "system", spot.ID, source.Provider, source.ProviderID); err != nil {
			return err
		}
	}
	return nil
}

func (s *ScenicSpotCatalogService) PrimarySpots(ctx context.Context, trackIDs []string) (map[string]*models.TrackPrimaryScenicSpot, error) {
	out := map[string]*models.TrackPrimaryScenicSpot{}
	if !s.Enabled() || len(trackIDs) == 0 {
		return out, nil
	}
	links, err := s.spots.GetTrackScenicSpotLinks(ctx, trackIDs)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(links))
	for _, link := range links {
		ids = append(ids, link.SpotID)
	}
	spots, err := s.spots.GetScenicSpots(ctx, ids)
	if err != nil {
		return nil, err
	}
	for trackID, link := range links {
		spot := spots[link.SpotID]
		if spot == nil || spot.Status != models.ScenicSpotPublished {
			continue
		}
		item := &models.TrackPrimaryScenicSpot{ID: spot.ID, Name: spot.Name, Category: spot.Category, Description: spot.Description, Address: spot.Address, Sources: spot.Sources}
		for _, photo := range spot.Photos {
			item.Photos = append(item.Photos, models.TrackScenicSpotPhoto{URL: photo.URL, Title: photo.Caption, Author: photo.Attribution, AuthorURL: photo.AttributionURL, SourceURL: photo.SourceURL})
		}
		for _, source := range spot.Sources {
			if source.Provider == link.Provider && source.Status == "active" {
				item.Provider = source.Provider
				item.ProviderID = source.ProviderID
				break
			}
			if item.Provider == "" && source.Status == "active" {
				item.Provider = source.Provider
				item.ProviderID = source.ProviderID
			}
		}
		out[trackID] = item
	}
	return out, nil
}
