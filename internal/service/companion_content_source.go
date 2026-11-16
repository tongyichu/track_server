package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

// Only a verified completion manifest links a track to a room's final owner.
func companionContentSource(ctx context.Context, rooms repository.CompanionRepository, tracks repository.TrackRepository, track *models.Track) (bool, *models.Track, error) {
	if rooms == nil || track == nil || track.SessionID == "" {
		return false, nil, nil
	}
	origins, ok := rooms.(repository.CompanionTrackOriginRepository)
	if !ok {
		return false, nil, nil
	}
	verified, err := origins.FindCompanionTrackIDs(ctx, []repository.CompanionTrackReference{{TrackID: track.ID, SessionID: track.SessionID, UserID: track.UserID}})
	if err != nil || !verified[track.ID] {
		return false, nil, err
	}
	room, err := rooms.FindSessionByID(ctx, track.SessionID)
	if err != nil {
		return true, nil, err
	}
	if room.OwnerUserID == track.UserID {
		return false, nil, nil
	}
	if room.Status != models.CompanionSessionStatusEnded {
		return true, nil, nil
	}
	storage, ok := rooms.(repository.CompanionTrackRepository)
	if !ok {
		return true, nil, errors.New("companion track storage unavailable")
	}
	manifests, err := storage.ListTrackManifests(ctx, track.SessionID)
	if err != nil {
		return true, nil, err
	}
	for _, row := range manifests {
		if row.UserID != room.OwnerUserID {
			continue
		}
		var manifest companionTrackManifest
		if err = json.Unmarshal([]byte(row.Data), &manifest); err != nil {
			return true, nil, err
		}
		if manifest.TrackID == "" {
			return true, nil, nil
		}
		source, err := tracks.FindByID(ctx, manifest.TrackID)
		if errors.Is(err, repository.ErrNotFound) {
			return true, nil, nil
		}
		if err != nil {
			return true, nil, err
		}
		if source.UserID != room.OwnerUserID || source.SessionID != room.SessionID || source.IsRunning ||
			source.Status == models.TrackStatusDeleted || !source.DeletedAt.IsZero() || strings.TrimSpace(source.RawTrackURL) == "" {
			return true, nil, nil
		}
		return true, source, nil
	}
	return true, nil, nil
}

func (s *TrackService) syncCompanionDescription(ctx context.Context, track *models.Track) (bool, bool, error) {
	shared, source, err := companionContentSource(ctx, s.companions, s.tracks, track)
	if err != nil || !shared {
		return shared, false, err
	}
	if source == nil || strings.TrimSpace(source.Description) == "" || (source.DescriptionSource != "auto" && source.DescriptionSource != "ai") {
		return true, false, nil
	}
	if track.DescriptionSource != "" && track.DescriptionSource != "auto" && track.DescriptionSource != "ai" && track.DescriptionSource != "companion" {
		return true, true, nil
	}
	if track.Description == source.Description {
		return true, true, nil
	}
	writer, ok := s.tracks.(repository.CompanionTrackMetadataWriter)
	if !ok {
		return true, false, errors.New("track narrative writer unavailable")
	}
	written, err := writer.ApplyCompanionDescription(ctx, track, source.Description)
	return true, written, err
}

func companionOwnerTargets(ctx context.Context, rooms repository.CompanionRepository, tracks repository.TrackRepository, source *models.Track) ([]*models.Track, error) {
	if rooms == nil || source == nil || source.SessionID == "" || source.IsRunning || !source.DeletedAt.IsZero() || source.Status == models.TrackStatusDeleted || strings.TrimSpace(source.RawTrackURL) == "" {
		return nil, nil
	}
	origins, ok := rooms.(repository.CompanionTrackOriginRepository)
	if !ok {
		return nil, nil
	}
	verified, err := origins.FindCompanionTrackIDs(ctx, []repository.CompanionTrackReference{{TrackID: source.ID, SessionID: source.SessionID, UserID: source.UserID}})
	if err != nil || !verified[source.ID] {
		return nil, err
	}
	room, err := rooms.FindSessionByID(ctx, source.SessionID)
	if err != nil {
		return nil, err
	}
	if room.Status != models.CompanionSessionStatusEnded || room.OwnerUserID != source.UserID {
		return nil, nil
	}
	storage, ok := rooms.(repository.CompanionTrackRepository)
	if !ok {
		return nil, nil
	}
	rows, err := storage.ListTrackManifests(ctx, source.SessionID)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	owners := map[string]int64{}
	for _, row := range rows {
		var m companionTrackManifest
		if err = json.Unmarshal([]byte(row.Data), &m); err != nil {
			return nil, err
		}
		if m.TrackID != "" && m.TrackID != source.ID {
			ids = append(ids, m.TrackID)
			owners[m.TrackID] = row.UserID
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	found, err := tracks.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := []*models.Track{}
	for _, id := range ids {
		t := found[id]
		if t != nil && t.UserID == owners[id] && t.SessionID == source.SessionID && !t.IsRunning && t.DeletedAt.IsZero() && (t.Status == models.TrackStatusNormal || t.Status == models.TrackStatusPrivate) {
			result = append(result, t)
		}
	}
	return result, nil
}

func (s *TrackService) syncCompanionDescriptionsFromOwner(ctx context.Context, source *models.Track) error {
	if source == nil || strings.TrimSpace(source.Description) == "" || (source.DescriptionSource != "auto" && source.DescriptionSource != "ai") {
		return nil
	}
	targets, err := companionOwnerTargets(ctx, s.companions, s.tracks, source)
	if err != nil {
		return err
	}
	writer, ok := s.tracks.(repository.CompanionTrackMetadataWriter)
	if !ok {
		return nil
	}
	for _, t := range targets {
		if t.Description == source.Description || (t.DescriptionSource != "" && t.DescriptionSource != "auto" && t.DescriptionSource != "ai" && t.DescriptionSource != "companion") {
			continue
		}
		if _, err = writer.ApplyCompanionDescription(ctx, t, source.Description); err != nil {
			return err
		}
	}
	return nil
}

func (s *TrackService) reconcileCompanionContent(ctx context.Context, id string) error {
	t, err := s.tracks.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if _, _, err = s.syncCompanionDescription(ctx, t); err != nil {
		return err
	}
	if err = s.syncCompanionDescriptionsFromOwner(ctx, t); err != nil {
		return err
	}
	if s.places != nil {
		return s.places.reconcileCompanionScenic(ctx, t)
	}
	return nil
}
