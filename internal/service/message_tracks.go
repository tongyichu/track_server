package service

import (
	"context"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
)

func (s *MessageService) messageTrackViews(ctx context.Context, viewer int64, ids []string) (map[string]*models.MessageTrack, error) {
	views := make(map[string]*models.MessageTrack, len(ids))
	if len(ids) == 0 {
		return views, nil
	}
	tracks, err := s.tracks.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	owners := make(map[int64]bool)
	var ownerIDs []int64
	for _, id := range ids {
		v := &models.MessageTrack{ID: id, Title: "轨迹已不可用"}
		views[id] = v
		t := tracks[id]
		if t == nil || t.Status == models.TrackStatusDeleted || (t.Status != models.TrackStatusNormal && t.UserID != viewer) {
			continue
		}
		v.Available = true
		v.Title = t.Title
		v.PreviewURL = trackPreviewURLs(t.ID, t.PreviewImages).LandscapeDayURL
		v.Distance, v.Duration = t.Distance, t.Duration
		v.CityCode = config.CanonicalCityCode(t.CityCode)
		v.CityName = config.CityNameByCode(v.CityCode)
		if t.UserID > 0 && !owners[t.UserID] {
			owners[t.UserID] = true
			ownerIDs = append(ownerIDs, t.UserID)
		}
	}
	if len(ownerIDs) == 0 {
		return views, nil
	}
	people, err := s.users.FindByIDs(ctx, ownerIDs)
	if err != nil {
		return nil, err
	}
	authors := make(map[int64]*models.MessageUser, len(ownerIDs))
	for _, id := range ownerIDs {
		view := s.messageUserView(ctx, id, people[id])
		authors[id] = &view
	}
	for id, v := range views {
		if v.Available {
			v.Author = authors[tracks[id].UserID]
		}
	}
	return views, nil
}
