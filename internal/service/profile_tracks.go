package service

import (
	"context"
	"errors"

	"github.com/tongyichu/track_server/internal/models"
)

type ProfileTrackPage struct {
	Items      []*models.TrackSummary `json:"items"`
	TotalCount int64                  `json:"total_count"`
	NextCursor string                 `json:"next_cursor,omitempty"`
	HasMore    bool                   `json:"has_more"`
}

func (s *TrackService) requireProfileUser(ctx context.Context, viewerID, targetID int64) error {
	if viewerID <= 0 || targetID <= 0 {
		return invalidArg("user_id must be positive")
	}
	if s.users == nil {
		return errors.New("user repository not configured")
	}
	_, err := s.users.FindByID(ctx, targetID)
	return err
}

func (s *TrackService) ListProfileTracks(ctx context.Context, viewerID, targetID int64, input ListMyTracksInput) (*ProfileTrackPage, error) {
	if err := s.requireProfileUser(ctx, viewerID, targetID); err != nil {
		return nil, err
	}
	cursor, err := decodeTrackListCursor(input.Cursor)
	if err != nil {
		return nil, err
	}
	limit := normalizeTrackPageLimit(input.Limit)
	tracks, err := s.tracks.ListPublicByUserID(ctx, targetID, cursor, limit+1)
	if err != nil {
		return nil, err
	}
	count, err := s.tracks.CountPublicByUserID(ctx, targetID)
	if err != nil {
		return nil, err
	}
	page := &ProfileTrackPage{TotalCount: count, HasMore: len(tracks) > limit}
	if page.HasMore {
		tracks = tracks[:limit]
		last := tracks[len(tracks)-1]
		page.NextCursor, err = encodeTrackListCursor(last.StartTime, last.ID)
		if err != nil {
			return nil, err
		}
	}
	page.Items, err = s.BuildTrackSummaries(ctx, viewerID, tracks)
	return page, err
}

func (s *TrackService) ListProfileCollections(ctx context.Context, viewerID, targetID int64, input ListMyTracksInput) (*ProfileTrackPage, error) {
	if err := s.requireProfileUser(ctx, viewerID, targetID); err != nil {
		return nil, err
	}
	if s.collects == nil {
		return nil, errors.New("collect repository not configured")
	}
	cursor, err := decodeTrackCollectCursor(input.Cursor)
	if err != nil {
		return nil, err
	}
	limit := normalizeTrackPageLimit(input.Limit)
	tracks := make([]*models.Track, 0, limit+1)
	var lastIncluded *models.TrackCollectCursor
	// Visibility can change after collection. Scan past stale/private entries before deciding has_more.
	for len(tracks) <= limit {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		records, err := s.collects.ListByUserID(ctx, targetID, cursor, 200)
		if err != nil {
			return nil, err
		}
		if len(records) == 0 {
			break
		}
		ids := make([]string, 0, len(records))
		for _, record := range records {
			ids = append(ids, record.TrackID)
		}
		byID, err := s.tracks.FindByIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			track := byID[record.TrackID]
			if track == nil || track.Status != models.TrackStatusNormal || track.IsRunning {
				continue
			}
			tracks = append(tracks, track)
			if len(tracks) <= limit {
				lastIncluded = &models.TrackCollectCursor{CreatedAt: record.CreatedAt, TrackID: record.TrackID}
			}
			if len(tracks) > limit {
				break
			}
		}
		last := records[len(records)-1]
		cursor = &models.TrackCollectCursor{CreatedAt: last.CreatedAt, TrackID: last.TrackID}
		if len(records) < 200 {
			break
		}
	}
	count, err := s.countVisibleCollectedTotalCount(ctx, targetID)
	if err != nil {
		return nil, err
	}
	page := &ProfileTrackPage{TotalCount: count, HasMore: len(tracks) > limit}
	if page.HasMore {
		tracks = tracks[:limit]
		page.NextCursor, err = encodeTrackCollectCursor(lastIncluded.CreatedAt, lastIncluded.TrackID)
		if err != nil {
			return nil, err
		}
	}
	// Collected belongs to the viewer, even when the list itself belongs to someone else.
	page.Items, err = s.BuildTrackSummaries(ctx, viewerID, tracks)
	return page, err
}
