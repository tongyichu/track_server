package service

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type TrackPlaceCandidateItem struct {
	TrackID   string                      `json:"track_id"`
	Title     string                      `json:"title"`
	CityCode  string                      `json:"city_code"`
	TrackType string                      `json:"track_type"`
	StartTime time.Time                   `json:"start_time"`
	Relations []models.TrackPlaceRelation `json:"relations"`
	Places    []*models.TrackPlace        `json:"places"`
	UpdatedAt time.Time                   `json:"updated_at"`
}

type TrackPlaceCandidatePage struct {
	Items      []*TrackPlaceCandidateItem `json:"items"`
	NextCursor string                     `json:"next_cursor"`
	HasMore    bool                       `json:"has_more"`
}

func (s *TrackPlaceService) ListPlaceCandidates(ctx context.Context, cursor string, limit int) (*TrackPlaceCandidatePage, error) {
	if !s.Enabled() {
		return nil, repository.ErrForbidden
	}
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	records, err := s.store.ListPlaceCandidateRecords(ctx, cursor, limit)
	if err != nil {
		return nil, err
	}
	matches := make([]*models.TrackPlaceMatch, 0, len(records))
	trackIDs := make([]string, 0, len(records))
	for _, record := range records {
		var match models.TrackPlaceMatch
		if json.Unmarshal([]byte(record.Data), &match) != nil || match.TrackID == "" || match.RuleVersion != trackPlaceRuleVersion || match.Provider != "amap" || match.Status != "candidate" {
			continue
		}
		matches = append(matches, &match)
		trackIDs = append(trackIDs, match.TrackID)
	}
	indexes, err := s.reader.FindPlaceGeoIndexes(ctx, trackIDs)
	if err != nil {
		return nil, err
	}
	valid := make([]*models.TrackPlaceMatch, 0, len(matches))
	trackIDs = trackIDs[:0]
	placeIDs := make([]string, 0)
	for _, match := range matches {
		index := indexes[match.TrackID]
		if index == nil || match.SourceVersion != trackPlaceSourceVersion(index) {
			continue
		}
		relations := enclosedPlaceRelations(match.Relations)
		if len(relations) == 0 {
			continue
		}
		match.Relations = relations
		valid = append(valid, match)
		trackIDs = append(trackIDs, match.TrackID)
		for _, relation := range relations {
			placeIDs = append(placeIDs, relation.PlaceID)
		}
	}
	tracks, err := s.tracks.FindByIDs(ctx, trackIDs)
	if err != nil {
		return nil, err
	}
	places, err := s.GetPlaces(ctx, placeIDs)
	if err != nil {
		return nil, err
	}
	items := make([]*TrackPlaceCandidateItem, 0, minInt(limit, len(valid)))
	moreCandidates := false
	for _, match := range valid {
		track := tracks[match.TrackID]
		if track == nil || !isTrackMapIndexEligible(track) {
			continue
		}
		item := &TrackPlaceCandidateItem{
			TrackID: match.TrackID, Title: track.Title, CityCode: track.CityCode,
			TrackType: track.TrackType, StartTime: track.StartTime,
			Relations: match.Relations, UpdatedAt: match.UpdatedAt,
		}
		for _, relation := range match.Relations {
			if place := places[relation.PlaceID]; place != nil {
				item.Places = append(item.Places, place)
			}
		}
		if len(item.Places) == 0 {
			continue
		}
		if len(items) == limit {
			moreCandidates = true
			break
		}
		items = append(items, item)
	}
	nextCursor := ""
	if len(records) > 0 {
		nextCursor = strings.TrimPrefix(records[len(records)-1].Key, "match:")
	}
	if moreCandidates && len(items) > 0 {
		nextCursor = items[len(items)-1].TrackID
	}
	return &TrackPlaceCandidatePage{Items: items, NextCursor: nextCursor, HasMore: len(records) == limit || moreCandidates}, nil
}

func (s *TrackPlaceService) ConfirmPlaceCandidate(ctx context.Context, trackID, placeID, operator string) error {
	trackID = strings.TrimSpace(trackID)
	placeID = strings.TrimSpace(placeID)
	if trackID == "" || placeID == "" {
		return invalidArg("track_id and place_id are required")
	}
	match, index, err := s.loadCurrentCandidate(ctx, trackID)
	if err != nil {
		return err
	}
	found := false
	for _, relation := range match.Relations {
		if relation.PlaceID == placeID && relation.Kind == "enclosed_poi" && relation.Enclosed {
			found = true
			break
		}
	}
	if !found {
		return invalidArg("place is not a candidate for this track")
	}
	places, err := s.GetPlaces(ctx, []string{placeID})
	if err != nil {
		return err
	}
	if places[placeID] == nil || strings.TrimSpace(places[placeID].Name) == "" {
		return repository.ErrNotFound
	}
	match.Status = "matched"
	match.PrimaryPlaceID = placeID
	match.PrimaryMethod = "enclosed_poi"
	match.UpdatedAt = time.Now()
	if err := s.save(ctx, "match:"+trackID, match, time.Time{}, ""); err != nil {
		return err
	}
	if s.scenicCatalog != nil {
		if err := s.scenicCatalog.SyncConfirmedTrack(ctx, operator, trackID); err != nil {
			log.Printf("confirmed scenic spot catalog link failed track_id=%s", trackID)
		}
	}
	return s.refreshGroup(ctx, index, match)
}

func (s *TrackPlaceService) DismissPlaceCandidate(ctx context.Context, trackID string) error {
	trackID = strings.TrimSpace(trackID)
	if trackID == "" {
		return invalidArg("track_id is required")
	}
	match, index, err := s.loadCurrentCandidate(ctx, trackID)
	if err != nil {
		return err
	}
	match.Status = "unmatched"
	match.PrimaryPlaceID = ""
	match.PrimaryMethod = ""
	match.UpdatedAt = time.Now()
	if err := s.save(ctx, "match:"+trackID, match, time.Time{}, ""); err != nil {
		return err
	}
	return s.refreshGroup(ctx, index, match)
}

func (s *TrackPlaceService) loadCurrentCandidate(ctx context.Context, trackID string) (*models.TrackPlaceMatch, *models.TrackGeoIndex, error) {
	if !s.Enabled() {
		return nil, nil, repository.ErrForbidden
	}
	record, err := s.store.GetPlaceRecord(ctx, "match:"+trackID)
	if err != nil {
		return nil, nil, err
	}
	var match models.TrackPlaceMatch
	if json.Unmarshal([]byte(record.Data), &match) != nil || match.TrackID != trackID || match.RuleVersion != trackPlaceRuleVersion || match.Provider != "amap" || match.Status != "candidate" {
		return nil, nil, repository.ErrNotFound
	}
	indexes, err := s.reader.FindPlaceGeoIndexes(ctx, []string{trackID})
	if err != nil {
		return nil, nil, err
	}
	index := indexes[trackID]
	if index == nil || match.SourceVersion != trackPlaceSourceVersion(index) {
		return nil, nil, repository.ErrConflict
	}
	return &match, index, nil
}

func enclosedPlaceRelations(relations []models.TrackPlaceRelation) []models.TrackPlaceRelation {
	items := make([]models.TrackPlaceRelation, 0, len(relations))
	for _, relation := range relations {
		if relation.Kind == "enclosed_poi" && relation.Enclosed && relation.PlaceID != "" {
			items = append(items, relation)
		}
	}
	return items
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
