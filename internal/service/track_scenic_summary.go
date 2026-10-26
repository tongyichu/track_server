package service

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
)

// Lists and details only read completed enrichment. They never call providers.
func (s *TrackPlaceService) PrimaryScenicSpots(ctx context.Context, ids []string) (map[string]*models.TrackPrimaryScenicSpot, error) {
	out := map[string]*models.TrackPrimaryScenicSpot{}
	if !s.Enabled() {
		return out, nil
	}
	for start := 0; start < len(ids); start += 200 {
		end := start + 200
		if end > len(ids) {
			end = len(ids)
		}
		keys := make([]string, 0, end-start)
		for _, id := range ids[start:end] {
			keys = append(keys, "match:"+id)
		}
		records, err := s.store.GetPlaceRecords(ctx, keys)
		if err != nil {
			return nil, err
		}
		matches := map[string]*models.TrackPlaceMatch{}
		matchedIDs := []string{}
		for _, id := range ids[start:end] {
			record := records["match:"+id]
			if !trackPlaceRecordFresh(record) {
				continue
			}
			var match models.TrackPlaceMatch
			if json.Unmarshal([]byte(record.Data), &match) != nil || match.TrackID != id || match.Status != "matched" || match.PrimaryPlaceID == "" || match.RuleVersion != trackPlaceRuleVersion {
				continue
			}
			matches[id] = &match
			matchedIDs = append(matchedIDs, id)
		}
		if len(matchedIDs) == 0 {
			continue
		}
		indexes, err := s.reader.FindPlaceGeoIndexes(ctx, matchedIDs)
		if err != nil {
			return nil, err
		}
		placeIDs := []string{}
		for id, match := range matches {
			index := indexes[id]
			if index == nil || match.SourceVersion != trackPlaceSourceVersion(index) {
				delete(matches, id)
				continue
			}
			placeIDs = append(placeIDs, match.PrimaryPlaceID)
		}
		places, err := s.GetPlaces(ctx, placeIDs)
		if err != nil {
			return nil, err
		}
		for id, match := range matches {
			if place := places[match.PrimaryPlaceID]; place != nil && strings.TrimSpace(place.Name) != "" {
				out[id] = &models.TrackPrimaryScenicSpot{ID: place.ID, Name: place.Name}
			}
		}
	}
	return out, nil
}

func (s *TrackService) SetPlaceService(places *TrackPlaceService) { s.places = places }

func (s *TrackService) primaryScenicSpots(ctx context.Context, ids []string) map[string]*models.TrackPrimaryScenicSpot {
	places, err := s.places.PrimaryScenicSpots(ctx, ids)
	if err != nil {
		log.Printf("track primary scenic enrichment unavailable")
		return nil
	}
	return places
}

func (s *TrackService) fillTrackPrimaryScenicSpot(ctx context.Context, track *models.Track) {
	track.PrimaryScenicSpot = s.primaryScenicSpots(ctx, []string{track.ID})[track.ID]
}
