package service

import (
	"context"
	"sort"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

func selectSampledPrimary(relations []models.TrackPlaceRelation) (string, bool) {
	best, count, tied := "", 1, false
	for _, relation := range relations {
		if relation.Kind != "sampled_inside" || relation.MatchedSamples < 2 {
			continue
		}
		if relation.MatchedSamples > count {
			best, count, tied = relation.PlaceID, relation.MatchedSamples, false
		} else if relation.MatchedSamples == count && best != relation.PlaceID {
			tied = true
		}
	}
	if tied {
		return "", true
	}
	return best, false
}

func (s *TrackPlaceService) decideTrackPlace(ctx context.Context, track *models.Track, index *models.TrackGeoIndex, match *models.TrackPlaceMatch, previous *models.TrackPlaceMatch) error {
	// Previously confirmed positives remain valid under the relaxed threshold.
	if previous != nil && previous.Provider == "amap" && previous.Status == "matched" && previous.PrimaryPlaceID != "" {
		match.PrimaryPlaceID = previous.PrimaryPlaceID
		match.PrimaryMethod = previous.PrimaryMethod
		if match.PrimaryMethod == "" {
			match.PrimaryMethod = "sampled_aoi"
		}
		return nil
	}
	primary, ambiguous := selectSampledPrimary(match.Relations)
	if primary != "" {
		match.PrimaryPlaceID, match.PrimaryMethod = primary, "sampled_aoi"
		return nil
	}
	if match.Provider == "google" {
		// Google nearby results have no polygon containment signal. Require the
		// same place to recur at least three times along the five sampled points.
		primary, ambiguous = selectRepeatedNearbyPrimary(match.Relations)
		if primary != "" && !ambiguous {
			match.PrimaryPlaceID, match.PrimaryMethod = primary, "repeated_nearby"
		}
		return nil
	}
	if ambiguous || match.Provider != "amap" {
		return nil
	}
	ring, err := s.loadTrackPlaceRing(ctx, track, index)
	if err != nil {
		return err
	}
	if ring == nil {
		return nil
	}
	places, err := s.enclosedPlaces(ctx, index, ring)
	if err != nil {
		return err
	}
	byID := map[string]*models.TrackPlace{}
	providerIDs := map[string]bool{}
	for _, place := range places {
		byID[place.ID] = place
		providerIDs[place.ProviderID] = true
	}
	ids := []string{}
	for id, place := range byID {
		if place.ParentID != "" && providerIDs[place.ParentID] {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		place := byID[id]
		place.ExpiresAt = time.Time{}
		if err := s.save(ctx, "place:"+id, place, time.Time{}, ""); err != nil {
			return err
		}
		found := false
		for i := range match.Relations {
			if match.Relations[i].PlaceID == id {
				match.Relations[i].Enclosed, found = true, true
				break
			}
		}
		if !found {
			match.Relations = append(match.Relations, models.TrackPlaceRelation{PlaceID: id, Kind: "enclosed_poi", Enclosed: true})
		}
	}
	if len(ids) == 1 {
		match.PrimaryPlaceID, match.PrimaryMethod = ids[0], "enclosed_poi"
	}
	return nil
}

func hasEnclosedPlaceCandidate(relations []models.TrackPlaceRelation) bool {
	for _, relation := range relations {
		if relation.Kind == "enclosed_poi" && relation.Enclosed && relation.PlaceID != "" {
			return true
		}
	}
	return false
}

func selectRepeatedNearbyPrimary(relations []models.TrackPlaceRelation) (string, bool) {
	best, count, tied := "", 2, false
	for _, relation := range relations {
		if relation.Kind != "nearby" || relation.MatchedSamples < 3 {
			continue
		}
		if relation.MatchedSamples > count {
			best, count, tied = relation.PlaceID, relation.MatchedSamples, false
		} else if relation.MatchedSamples == count && best != relation.PlaceID {
			tied = true
		}
	}
	if tied {
		return "", true
	}
	return best, false
}
