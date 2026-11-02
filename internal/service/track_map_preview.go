package service

import (
	"context"
	"errors"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

// PreviewCoordinateSystems interprets the same raw points in each supported CRS without updating the track or index.
func (s *TrackMapIndexService) PreviewCoordinateSystems(ctx context.Context, trackID string) (map[string][]models.TrackPoint, error) {
	if s == nil || s.rawTrackCache == nil {
		return nil, errors.New("raw track cache is not configured")
	}
	track, err := s.tracks.FindByID(ctx, trackID)
	if err != nil {
		return nil, err
	}
	if track.RawTrackURL == "" {
		return nil, errors.New("raw track file is missing")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	path, err := s.rawTrackCache.EnsureCachedFile(ctx, track.UserID, track.ID, track.RawTrackURL)
	if err != nil {
		return nil, err
	}
	points, err := parseTrackPointsFile(path)
	if err != nil {
		return nil, err
	}
	if len(points) < 2 {
		return nil, errors.New("not enough valid track points")
	}
	points = simplifyTrackPoints(points, maxSimplifiedTrackMapPoints)
	variants := make(map[string][]models.TrackPoint, 3)
	for _, coordinate := range []string{"WGS84", "GCJ02", "BD09"} {
		converted := make([]models.TrackPoint, len(points))
		for i, point := range points {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			converted[i], err = mapPointToGCJ02(point, coordinate)
			if err != nil {
				return nil, err
			}
		}
		variants[coordinate] = converted
	}
	return variants, nil
}
