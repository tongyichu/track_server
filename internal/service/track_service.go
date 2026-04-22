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

// ErrForbidden indicates current user has no permission to operate the resource.
var ErrForbidden = errors.New("forbidden")

// InvalidArgumentError represents a client-side request error.
type InvalidArgumentError struct{ Msg string }

func (e *InvalidArgumentError) Error() string { return e.Msg }

func invalidArg(msg string) error { return &InvalidArgumentError{Msg: msg} }

// CreateTrackInput describes optional fields accepted by track creation.
type CreateTrackInput struct {
	Title               *string    `json:"title"`
	StartTime           *time.Time `json:"start_time"`
	EndTime             *time.Time `json:"end_time"`
	Distance            *float64   `json:"distance"`
	Duration            *uint32    `json:"duration"`
	ElevationGain       *int       `json:"elevation_gain"`
	RawTrackURL         *string    `json:"raw_track_url"`
	TrackScreenshotURL  *string    `json:"track_screenshot_url"`
	LegacyScreenshotURL *string    `json:"screenshot_url,omitempty"`
	IsRunning           *bool      `json:"is_running"`
	AvgSpeedKmh         *float64   `json:"avg_speed_kmh"`
}

func (in *CreateTrackInput) normalize() {
	if in.TrackScreenshotURL == nil && in.LegacyScreenshotURL != nil {
		in.TrackScreenshotURL = in.LegacyScreenshotURL
	}
}

func (in CreateTrackInput) validate() error {
	if in.Distance != nil && *in.Distance < 0 {
		return invalidArg("distance must be >= 0")
	}
	if in.ElevationGain != nil && *in.ElevationGain < 0 {
		return invalidArg("elevation_gain must be >= 0")
	}
	if in.AvgSpeedKmh != nil && *in.AvgSpeedKmh < 0 {
		return invalidArg("avg_speed_kmh must be >= 0")
	}
	if in.StartTime != nil && in.EndTime != nil && in.EndTime.Before(*in.StartTime) {
		return invalidArg("end_time must be >= start_time")
	}
	return nil
}

// TrackInfoPatch describes which track summary fields should be updated.
// Nil means the field is not provided and should remain unchanged.
type TrackInfoPatch struct {
	Distance           *float64 `json:"distance"`
	Duration           *uint32  `json:"duration"`
	ElevationGain      *int     `json:"elevation_gain"`
	RawTrackURL        *string  `json:"raw_track_url"`
	TrackScreenshotURL *string  `json:"screenshot_url"`
	IsRunning          *bool    `json:"is_running"`
	AvgSpeedKmh        *float64 `json:"avg_speed_kmh"`
}

func (p TrackInfoPatch) empty() bool {
	return p.Distance == nil &&
		p.Duration == nil &&
		p.ElevationGain == nil &&
		p.RawTrackURL == nil &&
		p.TrackScreenshotURL == nil &&
		p.IsRunning == nil &&
		p.AvgSpeedKmh == nil
}

// NewTrackService constructs a new TrackService instance.
func NewTrackService(tracks repository.TrackRepository, collects repository.CollectRepository) *TrackService {
	return &TrackService{tracks: tracks, collects: collects}
}

// CreateTrack creates a new track for a user.
func (s *TrackService) CreateTrack(ctx context.Context, userID int64, input CreateTrackInput) (*models.Track, error) {
	if userID <= 0 {
		return nil, errors.New("userID is required")
	}
	input.normalize()
	if err := input.validate(); err != nil {
		return nil, err
	}
	// 轨迹 ID 在创建轨迹之前先由仓储层统一分配。
	// 这样 service 只负责组装业务对象，不关心底层是 MySQL 自增序列、Mongo 兼容实现，
	// 还是内存仓储中的测试序列，避免把 ID 生成细节散落在业务层。
	trackID, err := s.tracks.NextTrackID(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	startTime := now
	if input.StartTime != nil {
		startTime = *input.StartTime
	}
	isRunning := true
	if input.IsRunning != nil {
		isRunning = *input.IsRunning
	}
	track := &models.Track{
		ID:        trackID,
		UserID:    userID,
		Title:     "新的轨迹",
		StartTime: startTime,
		IsRunning: isRunning,
		Status:    models.TrackStatusNormal,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if input.Title != nil {
		track.Title = *input.Title
	}
	if input.EndTime != nil {
		track.EndTime = *input.EndTime
	}
	if input.Distance != nil {
		track.Distance = *input.Distance
	}
	if input.Duration != nil {
		track.Duration = *input.Duration
	}
	if input.ElevationGain != nil {
		track.ElevationGain = *input.ElevationGain
	}
	if input.RawTrackURL != nil {
		track.RawTrackURL = *input.RawTrackURL
	}
	if input.TrackScreenshotURL != nil {
		track.TrackScreenshotURL = *input.TrackScreenshotURL
	}
	if input.AvgSpeedKmh != nil {
		track.AvgSpeedKmh = *input.AvgSpeedKmh
	}
	// 到这里 track.ID 已经是最终可落库的业务 ID，格式固定为 `NO.` + 8 位 base36 编码。
	// Create 只负责持久化，不再承担“冲突重试生成 ID”的职责，唯一性保证前移到 NextTrackID。
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
	summaries := toSummaries(tracks)
	if userID <= 0 || s.collects == nil {
		return summaries, nil
	}
	for _, summary := range summaries {
		collected, err := s.collects.IsCollected(ctx, userID, summary.ID)
		if err != nil {
			return nil, err
		}
		summary.Collected = collected
	}
	return summaries, nil
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
	// For demo purposes we only normalize record status and end time.
	if track.Status != models.TrackStatusNormal {
		track.Status = models.TrackStatusNormal
	}
	track.IsRunning = false
	if track.EndTime.Before(track.StartTime) {
		track.EndTime = time.Now()
	}
	return s.tracks.Update(ctx, track)
}

// UpdateTrackInfo updates a track with partially provided fields.
func (s *TrackService) UpdateTrackInfo(ctx context.Context, userID int64, trackID string, patch TrackInfoPatch) (*models.Track, error) {
	if userID <= 0 {
		return nil, invalidArg("userID is required")
	}
	if trackID == "" {
		return nil, invalidArg("trackID is required")
	}
	if patch.empty() {
		return nil, invalidArg("no fields to update")
	}
	if patch.Distance != nil && *patch.Distance < 0 {
		return nil, invalidArg("distance must be >= 0")
	}
	if patch.ElevationGain != nil && *patch.ElevationGain < 0 {
		return nil, invalidArg("elevation_gain must be >= 0")
	}
	if patch.AvgSpeedKmh != nil && *patch.AvgSpeedKmh < 0 {
		return nil, invalidArg("avg_speed_kmh must be >= 0")
	}

	track, err := s.tracks.FindByID(ctx, trackID)
	if err != nil {
		return nil, err
	}
	if track.UserID != userID {
		return nil, ErrForbidden
	}

	if patch.Distance != nil {
		track.Distance = *patch.Distance
	}
	if patch.Duration != nil {
		track.Duration = *patch.Duration
	}
	if patch.ElevationGain != nil {
		track.ElevationGain = *patch.ElevationGain
	}
	if patch.RawTrackURL != nil {
		track.RawTrackURL = *patch.RawTrackURL
	}
	if patch.TrackScreenshotURL != nil {
		track.TrackScreenshotURL = *patch.TrackScreenshotURL
	}
	if patch.IsRunning != nil {
		track.IsRunning = *patch.IsRunning
	}
	if patch.AvgSpeedKmh != nil {
		track.AvgSpeedKmh = *patch.AvgSpeedKmh
	}

	if err := s.tracks.Update(ctx, track); err != nil {
		return nil, err
	}
	return track, nil
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
	t.Distance = distance
	t.ElevationGain = int(math.Round(ascent))
	t.Duration = uint32(duration)
	if t.StartTime.IsZero() {
		t.StartTime = t.Points[0].Timestamp
	}
	t.EndTime = t.Points[len(t.Points)-1].Timestamp
	if duration > 0 {
		// m/s to km/h
		t.AvgSpeedKmh = (distance / duration) * 3.6
	}
}

func toSummaries(tracks []*models.Track) []*models.TrackSummary {
	res := make([]*models.TrackSummary, 0, len(tracks))
	for _, t := range tracks {
		res = append(res, &models.TrackSummary{
			ID:            t.ID,
			UserID:        t.UserID,
			Title:         t.Title,
			Distance:      t.Distance,
			Duration:      t.Duration,
			ElevationGain: t.ElevationGain,
		})
	}
	return res
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
