package admin

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type companionMemberTrack struct {
	State   string `json:"state"`
	TrackID string `json:"track_id,omitempty"`
	Title   string `json:"title,omitempty"`
}

// Only an exact completion-manifest, author and room match can link a member to a track.
func (h *Handler) loadCompanionMemberTracks(ctx context.Context, sessionID string, members []*models.CompanionSessionMember) (map[int64]*companionMemberTrack, error) {
	result := make(map[int64]*companionMemberTrack, len(members))
	for _, member := range members {
		if member != nil {
			result[member.UserID] = &companionMemberTrack{State: "unknown"}
		}
	}
	storage, ok := h.companionRepo.(repository.CompanionTrackRepository)
	if !ok || h.trackRepo == nil {
		return result, errors.New("companion track storage unavailable")
	}
	rows, err := storage.ListTrackManifests(ctx, sessionID)
	if err != nil {
		return result, err
	}
	for _, track := range result {
		track.State = "pending"
	}
	ids := make([]string, 0, len(rows))
	byUser := make(map[int64]string, len(rows))
	var warning error
	for _, row := range rows {
		track := result[row.UserID]
		if track == nil || row.SessionID != sessionID || row.Index != 0 {
			continue
		}
		var manifest struct {
			TrackID *string `json:"track_id"`
		}
		if json.Unmarshal([]byte(row.Data), &manifest) != nil || manifest.TrackID == nil {
			track.State = "unknown"
			warning = errors.New("invalid companion completion manifest")
			continue
		}
		if *manifest.TrackID == "" {
			track.State = "empty"
			continue
		}
		track.State = "unknown"
		byUser[row.UserID] = *manifest.TrackID
		ids = append(ids, *manifest.TrackID)
	}
	if len(ids) == 0 {
		return result, warning
	}
	tracks, err := h.trackRepo.FindByIDs(ctx, ids)
	if err != nil {
		return result, err
	}
	for userID, id := range byUser {
		item := result[userID]
		track := tracks[id]
		if track == nil {
			item.State = "unavailable"
			continue
		}
		if track.UserID != userID || track.SessionID != sessionID || track.ID != id {
			warning = errors.New("companion track ownership mismatch")
			continue
		}
		item.TrackID, item.Title, item.State = track.ID, track.Title, "ready"
		if track.Status == models.TrackStatusDeleted || !track.DeletedAt.IsZero() {
			item.State = "deleted"
		}
	}
	return result, warning
}
