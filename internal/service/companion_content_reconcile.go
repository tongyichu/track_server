package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

// Repair two rooms per background pass without calling providers or AI.
func (s *TrackPlaceService) reconcileCompanionContentPage(ctx context.Context) error {
	if s.companions == nil {
		return nil
	}
	storage, ok := s.companions.(repository.CompanionTrackRepository)
	if !ok {
		return nil
	}
	writer, ok := s.tracks.(repository.CompanionTrackMetadataWriter)
	if !ok {
		return nil
	}
	const key = "companion:content_cursor:v1"
	var cursor *models.CompanionSessionListCursor
	record, err := s.store.GetPlaceRecord(ctx, key)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return err
	}
	if record != nil {
		if err = json.Unmarshal([]byte(record.Data), &cursor); err != nil {
			return err
		}
	}
	rooms, err := s.companions.ListAllSessions(ctx, cursor, 2)
	if err != nil {
		return err
	}
	if len(rooms) == 0 {
		return s.save(ctx, key, nil, time.Time{}, "")
	}
	for _, room := range rooms {
		if room.Status == models.CompanionSessionStatusEnded {
			rows, e := storage.ListTrackManifests(ctx, room.SessionID)
			if e != nil {
				return e
			}
			ids := []string{}
			owners := map[string]int64{}
			for _, row := range rows {
				var m companionTrackManifest
				if e = json.Unmarshal([]byte(row.Data), &m); e != nil {
					return e
				}
				if m.TrackID != "" {
					ids = append(ids, m.TrackID)
					owners[m.TrackID] = row.UserID
				}
			}
			tracks, e := s.tracks.FindByIDs(ctx, ids)
			if e != nil {
				return e
			}
			for _, id := range ids {
				t := tracks[id]
				if t == nil || t.UserID != owners[id] || t.SessionID != room.SessionID || t.IsRunning || !t.DeletedAt.IsZero() || (t.Status != models.TrackStatusNormal && t.Status != models.TrackStatusPrivate) {
					continue
				}
				if t.CityCode != "" && t.LocateAddr != "" {
					continue
				}
				var input CreateTrackInput
				if e = fillCompanionTrackLocation(ctx, storage, room.SessionID, t.UserID, &input); e != nil {
					return e
				}
				city, addr := "", ""
				if input.CityCode != nil {
					city = *input.CityCode
				}
				if input.LocateAddr != nil {
					addr = *input.LocateAddr
				}
				changed, e := writer.FillMissingCompanionLocation(ctx, t, city, addr)
				if e != nil {
					return e
				}
				if changed && s.indexer != nil {
					current, e := s.tracks.FindByID(ctx, id)
					if e != nil {
						return e
					}
					if e = s.indexer.EnqueueTrackIndexIfEligible(ctx, current); e != nil {
						return e
					}
				}
			}
			sourceID := ""
			for id, user := range owners {
				if user == room.OwnerUserID {
					sourceID = id
					break
				}
			}
			if sourceID != "" {
				source, e := s.tracks.FindByID(ctx, sourceID)
				if e != nil && !errors.Is(e, repository.ErrNotFound) {
					return e
				}
				if e == nil && source != nil && source.UserID == room.OwnerUserID && source.SessionID == room.SessionID && !source.IsRunning && source.DeletedAt.IsZero() && strings.TrimSpace(source.RawTrackURL) != "" && (source.Status == models.TrackStatusNormal || source.Status == models.TrackStatusPrivate) {
					if e = s.syncCompanionPlacesFromOwner(ctx, source, &TrackPlaceRunResult{}); e != nil {
						return e
					}
					if source.Description != "" && (source.DescriptionSource == "auto" || source.DescriptionSource == "ai") {
						// Reload after location repairs so the conditional description writes use fresh versions.
						current, e := s.tracks.FindByIDs(ctx, ids)
						if e != nil {
							return e
						}
						for id, t := range current {
							if t == nil || id == sourceID || t.UserID != owners[id] || t.SessionID != room.SessionID || t.Description == source.Description {
								continue
							}
							if _, e = writer.ApplyCompanionDescription(ctx, t, source.Description); e != nil {
								return e
							}
						}
					}
				}
			}
		}
		next := &models.CompanionSessionListCursor{StartedAt: room.StartedAt, SessionID: room.SessionID}
		if err = s.save(ctx, key, next, time.Time{}, ""); err != nil {
			return err
		}
	}
	return nil
}
