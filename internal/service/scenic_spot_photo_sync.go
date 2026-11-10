package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"reflect"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type ScenicPhotoSyncResult struct{ Scanned, Processed, Synced, Failed, Deferred int }
type scenicPhotoScan struct {
	Cursor string `json:"cursor"`
}
type scenicPhotoState struct {
	Version     string    `json:"version"`
	Attempts    int       `json:"attempts"`
	NextAttempt time.Time `json:"next_attempt"`
}

func (s *ScenicSpotCatalogService) PhotoSyncEnabled() bool {
	return s.Enabled() && s.places != nil && s.photoStore != nil && s.provider.Enabled()
}

func sameScenicPhotoList(left, right []models.ScenicSpotPhoto) bool {
	return len(left) == len(right) && (len(left) == 0 || reflect.DeepEqual(left, right))
}

func scenicPhotoVersion(spot *models.ScenicSpot) string {
	photos := spot.Photos
	if len(photos) == 0 {
		photos = nil
	}
	raw, _ := json.Marshal([]any{spot.Sources, photos, spot.CoverURL, spot.PhotosEdited})
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

func scenicPhotoSource(spot *models.ScenicSpot) *models.ScenicSpotSource {
	source := preferredScenicSpotSource(spot.Sources)
	if source == nil || source.Status != "active" || (source.Provider != "amap" && source.Provider != "google") {
		return nil
	}
	return source
}

func scenicPhotoNeedsSync(spot *models.ScenicSpot) bool {
	if spot == nil || !scenicSpotClientVisible(spot) {
		return false
	}
	for _, photo := range spot.Photos {
		if safeScenicPhotoURL(photo.URL) != "" {
			return true
		}
	}
	if len(spot.Photos) == 0 && !spot.PhotosEdited && (ownedScenicPhotoURL(spot.CoverURL) || safeScenicPhotoURL(spot.CoverURL) != "") {
		return true
	}
	return len(spot.Photos) == 0 && !spot.PhotosEdited && scenicPhotoSource(spot) != nil
}

func (s *ScenicSpotCatalogService) readPhotoState(ctx context.Context, key string, value any) error {
	record, err := s.places.GetPlaceRecord(ctx, key)
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(record.Data), value)
}

func (s *ScenicSpotCatalogService) savePhotoState(ctx context.Context, key, owner string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	saved, err := s.places.SavePlaceRecord(ctx, &models.TrackPlaceRecord{Key: key, Data: string(raw)}, owner)
	if err != nil {
		return err
	}
	if !saved {
		return repository.ErrConflict
	}
	return nil
}

// Persisted cursor and work leases bound paid requests across restarts and replicas.
// Neither discovering a spot nor reading a track invokes this worker.
func (s *ScenicSpotCatalogService) SyncPhotosOnce(ctx context.Context) (result *ScenicPhotoSyncResult, err error) {
	result = &ScenicPhotoSyncResult{}
	if !s.PhotoSyncEnabled() {
		return result, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 100*time.Second)
	defer cancel()
	owner, err := submissionID("SPH-")
	if err != nil {
		return result, err
	}
	const key = "scenic:photos:scan:v1"
	now := time.Now().UTC()
	acquired, err := s.places.LeasePlaceRecord(ctx, key, owner, now, now.Add(2*time.Minute))
	if err != nil || !acquired {
		return result, err
	}
	var scan scenicPhotoScan
	if err = s.readPhotoState(ctx, key, &scan); err != nil {
		return result, err
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if e := s.savePhotoState(cleanup, key, owner, scan); err == nil {
			err = e
		}
	}()
	spots, _, err := s.spots.ListScenicSpots(ctx, repository.ScenicSpotListFilter{Cursor: scan.Cursor, VisibleOnly: true, Limit: 100})
	if err != nil {
		return result, err
	}
	keys := make([]string, 0, len(spots))
	for _, spot := range spots {
		if scenicPhotoNeedsSync(spot) {
			keys = append(keys, "scenic:photos:work:"+spot.ID)
		}
	}
	states, err := s.places.GetPlaceRecords(ctx, keys)
	if err != nil {
		return result, err
	}
	for _, spot := range spots {
		if result.Processed >= 2 {
			return result, nil
		}
		if err = ctx.Err(); err != nil {
			return result, err
		}
		result.Scanned++
		if scenicPhotoNeedsSync(spot) {
			if record := states["scenic:photos:work:"+spot.ID]; record != nil {
				var state scenicPhotoState
				if json.Unmarshal([]byte(record.Data), &state) == nil && state.Version == scenicPhotoVersion(spot) && state.NextAttempt.After(now) {
					result.Deferred++
					scan.Cursor = spot.ID
					continue
				}
			}
			worked, changed, syncErr := s.syncSpotPhotos(ctx, spot.ID)
			if worked {
				result.Processed++
			} else {
				result.Deferred++
			}
			if syncErr != nil {
				log.Printf("scenic photo synchronization failed: spot=%s err=%v", spot.ID, syncErr)
				result.Failed++
			} else if changed {
				result.Synced++
			}
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
		}
		scan.Cursor = spot.ID
	}
	if len(spots) < 100 {
		scan.Cursor = ""
	}
	return result, nil
}

func (s *ScenicSpotCatalogService) syncSpotPhotos(ctx context.Context, id string) (worked, changed bool, err error) {
	owner, err := submissionID("SPH-WORK-")
	if err != nil {
		return false, false, err
	}
	key := "scenic:photos:work:" + id
	now := time.Now().UTC()
	acquired, err := s.places.LeasePlaceRecord(ctx, key, owner, now, now.Add(2*time.Minute))
	if err != nil || !acquired {
		return false, false, err
	}
	var state scenicPhotoState
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if e := s.savePhotoState(cleanup, key, owner, state); err == nil {
			err = e
		}
	}()
	if err = s.readPhotoState(ctx, key, &state); err != nil {
		return false, false, err
	}
	spot, err := s.spots.GetScenicSpot(ctx, id)
	if err != nil || !scenicPhotoNeedsSync(spot) {
		return false, false, err
	}
	version := scenicPhotoVersion(spot)
	if state.Version == version && state.NextAttempt.After(now) {
		return false, false, nil
	}
	if state.Version != version {
		state = scenicPhotoState{Version: version}
	}
	worked = true
	state.Attempts++
	// Any failure, including a failed CAS after paid IO, receives a persistent backoff.
	minutes := 5 << min(state.Attempts-1, 6)
	state.NextAttempt = now.Add(time.Duration(minutes) * time.Minute)
	if source := scenicPhotoSource(spot); source != nil && source.Provider == "google" && (source.RefreshAfter == nil || !source.RefreshAfter.After(now)) {
		verified, verifyErr := s.VerifySource(ctx, "system:photos", id, source.Provider, source.ProviderID)
		if verifyErr != nil {
			return true, false, verifyErr
		}
		spot = verified
		version = scenicPhotoVersion(spot)
		state.Version = version
		if !scenicPhotoNeedsSync(spot) {
			return true, false, nil
		}
	}
	hasOwned := false
	for _, photo := range spot.Photos {
		if ownedScenicPhotoURL(photo.URL) {
			hasOwned = true
			break
		}
	}
	refreshMedia := state.Attempts > 1 && !spot.PhotosEdited && !hasOwned && scenicPhotoSource(spot) != nil
	candidate := *spot
	candidate.Photos = append([]models.ScenicSpotPhoto(nil), spot.Photos...)
	if refreshMedia {
		candidate.Photos = nil
	}
	if len(candidate.Photos) == 0 && !refreshMedia && !spot.PhotosEdited && (ownedScenicPhotoURL(spot.CoverURL) || safeScenicPhotoURL(spot.CoverURL) != "") {
		candidate.Photos = append(candidate.Photos, models.ScenicSpotPhoto{ID: newScenicID("SPP-"), URL: spot.CoverURL})
	}
	if len(candidate.Photos) == 0 && !spot.PhotosEdited {
		source := scenicPhotoSource(spot)
		if source != nil {
			preview, fetchErr := s.PreviewProviderSource(ctx, id, source.Provider, source.ProviderID)
			if fetchErr != nil {
				return true, false, fetchErr
			}
			for _, photo := range preview.Photos {
				if safeScenicPhotoURL(photo.URL) == "" || len(candidate.Photos) >= 30 {
					continue
				}
				candidate.Photos = append(candidate.Photos, models.ScenicSpotPhoto{ID: newScenicID("SPP-"), URL: photo.URL, Caption: strings.TrimSpace(photo.Title), Attribution: photo.Author, AttributionURL: photo.AuthorURL, SourceURL: photo.SourceURL, Provider: source.Provider})
			}
		}
	}
	if refreshMedia && len(candidate.Photos) == 0 && len(spot.Photos) > 0 {
		return true, false, errors.New("scenic photo source refresh returned no images")
	}
	localized, _, failed := s.photoStore.localize(ctx, candidate.Photos)
	candidate.Photos = localized
	latest, err := s.spots.GetScenicSpot(ctx, id)
	if err != nil {
		return true, false, err
	}
	if scenicPhotoVersion(latest) != version || !scenicSpotClientVisible(latest) {
		return true, false, repository.ErrConflict
	}
	if !sameScenicPhotoList(candidate.Photos, spot.Photos) {
		latest.Photos = candidate.Photos
		if err = s.persistProviderContentSync(ctx, latest, "system:photos", "background photos synchronized to local storage"); err != nil {
			return true, false, err
		}
		changed = true
		state.Version = scenicPhotoVersion(latest)
	}
	if failed > 0 {
		return true, changed, errors.New("scenic photo download incomplete")
	}
	state.Attempts = 0
	// Empty results are negative caches, not permanent failures or minutely paid retries.
	state.NextAttempt = now.Add(24 * time.Hour)
	return true, changed, nil
}
