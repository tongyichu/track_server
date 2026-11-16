package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

// Reuse catalog associations while retaining explicit per-track manual decisions.
func (s *ScenicSpotCatalogService) syncCompanionLink(ctx context.Context, target, source string, clearMissing bool) (bool, error) {
	if s == nil || s.spots == nil {
		return false, nil
	}
	current, err := s.spots.GetTrackScenicSpotLink(ctx, target)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return false, err
	}
	if current != nil && current.Origin != "auto" && current.Origin != "backfill" && current.Origin != "companion" {
		return true, nil
	}
	original, err := s.spots.GetTrackScenicSpotLink(ctx, source)
	if errors.Is(err, repository.ErrNotFound) {
		if clearMissing && current != nil {
			if err = s.spots.DeleteTrackScenicSpotLink(ctx, target, current.SpotID); errors.Is(err, repository.ErrConflict) {
				return false, nil
			}
			return false, err
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if current != nil && current.SpotID == original.SpotID && current.Origin == "companion" && current.UpdatedBy == source {
		return false, nil
	}
	link := &models.TrackScenicSpotLink{TrackID: target, SpotID: original.SpotID, Origin: "companion", Provider: original.Provider, Confidence: original.Confidence, UpdatedAt: time.Now(), UpdatedBy: source}
	expected := ""
	if current != nil {
		expected = current.SpotID
	}
	err = s.spots.SetTrackScenicSpotLink(ctx, link, expected, current == nil)
	if errors.Is(err, repository.ErrConflict) {
		return false, nil
	}
	return false, err
}

func (s *TrackPlaceService) reuseCompanionPlace(ctx context.Context, index *models.TrackGeoIndex, track, source *models.Track, out *TrackPlaceRunResult) error {
	if source == nil {
		out.Deferred++
		return nil
	}
	sourceRecord, err := s.store.GetPlaceRecord(ctx, "match:"+source.ID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return err
	}
	var original models.TrackPlaceMatch
	ready := trackPlaceRecordFresh(sourceRecord) && json.Unmarshal([]byte(sourceRecord.Data), &original) == nil &&
		original.TrackID == source.ID && (original.Status == "matched" || original.Status == "candidate" || original.Status == "unmatched")
	if ready {
		sourceIndex, e := s.maps.FindTrackGeoIndex(ctx, source.ID)
		if e != nil && !errors.Is(e, repository.ErrNotFound) {
			return e
		}
		ready = sourceIndex != nil && original.SourceVersion == trackPlaceSourceVersion(sourceIndex)
	}
	if s.scenicCatalog != nil {
		protected, e := s.scenicCatalog.syncCompanionLink(ctx, track.ID, source.ID, ready)
		if e != nil {
			return e
		}
		if protected {
			out.Skipped++
			return nil
		}
	}
	if !ready {
		out.Deferred++
		return nil
	}
	match := original
	match.TrackID = track.ID
	match.SourceVersion = trackPlaceSourceVersion(index)
	match.PrimaryMethod = "companion_owner"
	match.SharedSourceTrackID = source.ID
	match.SharedSourceVersion = fmt.Sprintf("%x", sha256.Sum256([]byte(sourceRecord.Data)))
	match.Relations = []models.TrackPlaceRelation{}
	match.Attempts = 0
	match.UpdatedAt = time.Now()
	match.ErrorCode = ""
	var previous models.TrackPlaceMatch
	_, err = s.loadFresh(ctx, "match:"+track.ID, &previous)
	if err != nil {
		return err
	}
	if previous.SourceVersion == match.SourceVersion && previous.SharedSourceTrackID == source.ID &&
		previous.SharedSourceVersion == match.SharedSourceVersion {
		out.Reused++
		return s.refreshGroup(ctx, index, &previous)
	}
	owner := s.owner()
	now := time.Now()
	leased, err := s.store.LeasePlaceRecord(ctx, "match:"+track.ID, owner, now, now.Add(2*time.Minute))
	if err != nil {
		return err
	}
	if !leased {
		out.Deferred++
		out.BusyDeferred++
		return nil
	}
	latest, err := s.tracks.FindByID(ctx, track.ID)
	if err != nil {
		return err
	}
	if !isTrackMapIndexEligible(latest) || latest.RawTrackURL != track.RawTrackURL {
		return nil
	}
	if err = s.save(ctx, "match:"+track.ID, &match, sourceRecord.ExpiresAt, owner); err != nil {
		return err
	}
	out.Reused++
	return s.refreshGroup(ctx, index, &match)
}

func (s *TrackPlaceService) syncCompanionPlacesFromOwner(ctx context.Context, source *models.Track, out *TrackPlaceRunResult) error {
	targets, err := companionOwnerTargets(ctx, s.companions, s.tracks, source)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}
	ids := []string{}
	for _, t := range targets {
		ids = append(ids, t.ID)
	}
	indexes := map[string]*models.TrackGeoIndex{}
	if s.reader != nil {
		indexes, err = s.reader.FindPlaceGeoIndexes(ctx, ids)
		if err != nil {
			return err
		}
	}
	for _, t := range targets {
		if index := indexes[t.ID]; index != nil && isTrackMapIndexEligible(t) {
			if err = s.reuseCompanionPlace(ctx, index, t, source, out); err != nil {
				return err
			}
		} else if s.scenicCatalog != nil {
			if _, err = s.scenicCatalog.syncCompanionLink(ctx, t.ID, source.ID, false); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *TrackPlaceService) reconcileCompanionScenic(ctx context.Context, track *models.Track) error {
	shared, source, err := companionContentSource(ctx, s.companions, s.tracks, track)
	if err != nil {
		return err
	}
	if shared {
		if source != nil && s.scenicCatalog != nil {
			_, err = s.scenicCatalog.syncCompanionLink(ctx, track.ID, source.ID, false)
		}
		return err
	}
	return s.syncCompanionPlacesFromOwner(ctx, track, &TrackPlaceRunResult{})
}
