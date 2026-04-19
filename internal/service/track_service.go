package service

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

// TrackService provides business logic around track lifecycle and statistics.
type TrackService struct {
	tracks   repository.TrackRepository
	collects repository.CollectRepository
}

// NewTrackService constructs a new TrackService instance.
func NewTrackService(tracks repository.TrackRepository, collects repository.CollectRepository) *TrackService {
	return &TrackService{tracks: tracks, collects: collects}
}

// CreateTrack creates a new running track for a user.
func (s *TrackService) CreateTrack(ctx context.Context, userID int64) (*models.Track, error) {
	if userID <= 0 {
		return nil, errors.New("userID is required")
	}
	now := time.Now()
	track := &models.Track{
		ID:        generateTrackID(),
		UserID:    userID,
		Name:      "新的轨迹",
		Status:    models.TrackStatusRunning,
		StartedAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.tracks.Create(ctx, track); err != nil {
		return nil, err
	}
	return track, nil
}

// GetTrackDetail returns detailed information of a track.
func (s *TrackService) GetTrackDetail(ctx context.Context, trackID string) (*models.Track, error) {
	if trackID == "" {
		return nil, errors.New("trackID is required")
	}
	track, err := s.tracks.FindByID(ctx, trackID)
	if err != nil {
		return nil, err
	}
	// Recompute summary metrics from points if available.
	updateTrackMetrics(track)
	return track, nil
}

// GetTrackMap returns map polyline for a track.
func (s *TrackService) GetTrackMap(ctx context.Context, trackID string) (*models.TrackMap, error) {
	track, err := s.tracks.FindByID(ctx, trackID)
	if err != nil {
		return nil, err
	}
	return &models.TrackMap{TrackID: track.ID, Points: track.Points}, nil
}

// GetRunningTrack returns currently running track of the user if exists.
func (s *TrackService) GetRunningTrack(ctx context.Context, userID int64) (*models.Track, error) {
	return s.tracks.FindRunningByUserID(ctx, userID)
}

// ListRecommend returns recommended tracks for the user.
func (s *TrackService) ListRecommend(ctx context.Context, userID int64, limit int) ([]*models.TrackSummary, error) {
	tracks, err := s.tracks.ListRecommend(ctx, userID, limit)
	if err != nil {
		return nil, err
	}
	return toSummaries(tracks), nil
}

// SearchTracks searches tracks globally by keyword.
func (s *TrackService) SearchTracks(ctx context.Context, keyword string, limit int) ([]*models.TrackSummary, error) {
	tracks, err := s.tracks.Search(ctx, keyword, limit)
	if err != nil {
		return nil, err
	}
	return toSummaries(tracks), nil
}

// MarkUploadedToCloud marks a finished track as uploaded to cloud.
func (s *TrackService) MarkUploadedToCloud(ctx context.Context, trackID string) error {
	track, err := s.tracks.FindByID(ctx, trackID)
	if err != nil {
		return err
	}
	// For demo purposes we just ensure it is finished; no extra field persisted.
	if track.Status != models.TrackStatusFinished {
		track.Status = models.TrackStatusFinished
	}
	return s.tracks.Update(ctx, track)
}

// IsCollected reports whether a track is collected by user.
func (s *TrackService) IsCollected(ctx context.Context, userID int64, trackID string) (bool, error) {
	return s.collects.IsCollected(ctx, userID, trackID)
}

// CollectTrack adds a collection for user-track pair.
func (s *TrackService) CollectTrack(ctx context.Context, userID int64, trackID string) error {
	// Check track existence.
	if _, err := s.tracks.FindByID(ctx, trackID); err != nil {
		return err
	}
	return s.collects.AddCollect(ctx, userID, trackID)
}

// UncollectTrack removes a collection for user-track pair.
func (s *TrackService) UncollectTrack(ctx context.Context, userID int64, trackID string) error {
	return s.collects.RemoveCollect(ctx, userID, trackID)
}

// updateTrackMetrics recomputes distance, duration, ascent and average speed based on points.
func updateTrackMetrics(t *models.Track) {
	if len(t.Points) < 2 {
		return
	}
	var distance float64
	var ascent float64
	for i := 1; i < len(t.Points); i++ {
		d := haversineDistance(t.Points[i-1].Latitude, t.Points[i-1].Longitude, t.Points[i].Latitude, t.Points[i].Longitude)
		distance += d
		deltaAlt := t.Points[i].Elevation - t.Points[i-1].Elevation
		if deltaAlt > 0 {
			ascent += deltaAlt
		}
	}
	duration := t.Points[len(t.Points)-1].Timestamp.Sub(t.Points[0].Timestamp).Seconds()
	if duration < 0 {
		duration = 0
	}
	t.DistanceMeters = distance
	t.AscentMeters = ascent
	t.DurationSec = int64(duration)
	if duration > 0 {
		// m/s to km/h
		t.AvgSpeedKmh = (distance / duration) * 3.6
	}
}

func toSummaries(tracks []*models.Track) []*models.TrackSummary {
	res := make([]*models.TrackSummary, 0, len(tracks))
	for _, t := range tracks {
		res = append(res, &models.TrackSummary{
			ID:             t.ID,
			UserID:         t.UserID,
			Name:           t.Name,
			DistanceMeters: t.DistanceMeters,
			DurationSec:    t.DurationSec,
			AscentMeters:   t.AscentMeters,
			AvgSpeedKmh:    t.AvgSpeedKmh,
		})
	}
	return res
}

// generateTrackID generates a simple unique track id.
func generateTrackID() string {
	// Use timestamp-based id to avoid external dependencies.
	return "trk_" + time.Now().Format("20060102150405.000000000")
}

// haversineDistance computes distance between two lat/lng points in meters.
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000.0
	toRad := func(d float64) float64 { return d * math.Pi / 180 }
	phi1 := toRad(lat1)
	phi2 := toRad(lat2)
	deltaPhi := toRad(lat2 - lat1)
	deltaLambda := toRad(lon2 - lon1)

	a := math.Sin(deltaPhi/2)*math.Sin(deltaPhi/2) +
		math.Cos(phi1)*math.Cos(phi2)*math.Sin(deltaLambda/2)*math.Sin(deltaLambda/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadius * c
}
