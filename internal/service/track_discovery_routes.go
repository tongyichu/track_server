package service

import (
	"context"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

// DiscoverRoutes filters the authoritative label sources before forming a page.
// Internal scan cursors are independent of the last returned matching track.
func (s *TrackService) DiscoverRoutes(ctx context.Context, userID int64, city, label, trackType, rawCursor string, limit int, shapes ...string) (*models.TrackSummaryPage, error) {
	city, label, trackType = strings.TrimSpace(city), strings.TrimSpace(label), strings.TrimSpace(trackType)
	if city == "" {
		return nil, invalidArg("city_code is required")
	}
	if label != "featured" && label != "scenic_spot" && label != "shape" {
		return nil, invalidArg("invalid label_type")
	}
	shape := ""
	if len(shapes) > 0 {
		shape = strings.TrimSpace(shapes[0])
	}
	if shape != "" && label != "shape" {
		return nil, invalidArg("shape_label requires label_type=shape")
	}
	if err := validateTrackShapeLabel(&shape); err != nil {
		return nil, err
	}
	cursor, err := decodeTrackListCursor(rawCursor)
	if err != nil {
		return nil, err
	}
	limit = normalizeTrackPageLimit(limit)
	selected := make([]*models.Track, 0, limit+1)
	const batchSize = 100
	for len(selected) <= limit {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var batch []*models.Track
		switch label {
		case "featured":
			var submissions repository.TrackSubmissionRepository
			if s.submissions != nil {
				submissions = s.submissions.repo
			}
			batch, err = repository.ListFeaturedDiscoveryTracks(ctx, s.tracks, submissions, city, trackType, cursor, batchSize)
		case "shape":
			batch, err = s.tracks.ListShaped(ctx, city, cursor, batchSize)
		default:
			batch, err = s.tracks.Search(ctx, "", city, trackType, cursor, batchSize)
		}
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		ids := make([]string, 0, len(batch))
		for _, track := range batch {
			ids = append(ids, track.ID)
		}
		var spots map[string]*models.TrackPrimaryScenicSpot
		if label == "scenic_spot" {
			spots, err = s.discoveryLabelSpots(ctx, ids)
			if err != nil {
				return nil, err
			}
		}
		for _, track := range batch {
			if track.Status != models.TrackStatusNormal || track.IsRunning || strings.TrimSpace(track.RawTrackURL) == "" || trackType != "" && track.TrackType != trackType {
				continue
			}
			if shape != "" && strings.TrimSpace(track.ShapeLabel) != shape {
				continue
			}
			for _, candidate := range buildTrackLabels(track.ShapeLabel, track.TrackType, spots[track.ID], label == "featured") {
				if candidate.Type == label && candidate.Value {
					selected = append(selected, track)
					break
				}
			}
			if len(selected) > limit {
				break
			}
		}
		last := batch[len(batch)-1]
		cursor = &models.TrackListCursor{StartTime: last.StartTime, ID: last.ID}
		if len(batch) < batchSize {
			break
		}
	}
	hasMore := len(selected) > limit
	if hasMore {
		selected = selected[:limit]
	}
	summaries, err := s.BuildTrackSummaries(ctx, userID, selected)
	if err != nil {
		return nil, err
	}
	page, err := buildTrackSummaryPage(summaries, hasMore)
	if err == nil && label == "shape" {
		page.ShapeFilterVersion = 1
	}
	return page, err
}

// A failed label read must not turn a populated city into a successful empty page.
func (s *TrackService) discoveryLabelSpots(ctx context.Context, ids []string) (map[string]*models.TrackPrimaryScenicSpot, error) {
	spots, err := s.places.PrimaryScenicSpots(ctx, ids)
	if err != nil {
		return nil, err
	}
	if s.scenicCatalog != nil {
		catalog, err := s.scenicCatalog.PrimarySpots(ctx, ids)
		if err != nil {
			return nil, err
		}
		for id, spot := range catalog {
			if previous := spots[id]; previous != nil && spot != nil && strings.TrimSpace(spot.Name) == "" {
				spot.Name = previous.Name
			}
			spots[id] = spot
		}
	}
	return spots, nil
}
