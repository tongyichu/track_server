package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

const (
	defaultTrackMapIndexBatchSize = 10
	maxRawTrackIndexFileBytes     = 50 << 20 // 50 MiB
	maxSimplifiedTrackMapPoints   = 500
)

// TrackMapIndexService builds map indexes for completed public tracks.
type TrackMapIndexService struct {
	repo          repository.TrackMapRepository
	tracks        repository.TrackRepository
	rawTrackCache *AssetCacheService
	workerID      string
}

type TrackMapIndexRunResult struct {
	EnqueuedMissing int `json:"enqueued_missing"`
	Claimed         int `json:"claimed"`
	Succeeded       int `json:"succeeded"`
	Failed          int `json:"failed"`
}

func NewTrackMapIndexService(repo repository.TrackMapRepository, tracks repository.TrackRepository, rawTrackCache *AssetCacheService) *TrackMapIndexService {
	return &TrackMapIndexService{
		repo:          repo,
		tracks:        tracks,
		rawTrackCache: rawTrackCache,
		workerID:      fmt.Sprintf("track-map-index-%d", time.Now().UnixNano()),
	}
}

// EnqueueTrackIndexIfEligible records async index work for a completed public track.
// It performs no OSS download or geometry computation, so it is safe to call from
// request paths. Errors should be logged by callers and must not fail track save.
func (s *TrackMapIndexService) EnqueueTrackIndexIfEligible(ctx context.Context, track *models.Track) error {
	if s == nil || s.repo == nil || track == nil {
		return nil
	}
	if !isTrackMapIndexEligible(track) {
		return nil
	}
	return s.repo.EnqueueIndexJob(ctx, track.ID, time.Now())
}

func (s *TrackMapIndexService) RunOnce(ctx context.Context) (*TrackMapIndexRunResult, error) {
	result := &TrackMapIndexRunResult{}
	if s == nil || s.repo == nil {
		return result, errors.New("track map index service is not configured")
	}
	if s.tracks == nil {
		return result, errors.New("track repository is not configured")
	}
	now := time.Now()
	missing, err := s.repo.ListCompletedTracksMissingGeoIndex(ctx, 100)
	if err != nil {
		return result, err
	}
	for _, track := range missing {
		if err := s.EnqueueTrackIndexIfEligible(ctx, track); err != nil {
			return result, err
		}
		result.EnqueuedMissing++
	}

	jobs, err := s.repo.ClaimPendingIndexJobs(ctx, s.workerID, now, defaultTrackMapIndexBatchSize)
	if err != nil {
		return result, err
	}
	result.Claimed = len(jobs)
	for _, job := range jobs {
		if job == nil {
			continue
		}
		if err := s.processJob(ctx, job.TrackID); err != nil {
			result.Failed++
			nextRunAt := time.Now().Add(retryDelay(job.Attempts + 1))
			if markErr := s.repo.MarkIndexJobFailed(ctx, job.TrackID, err.Error(), nextRunAt, time.Now()); markErr != nil {
				return result, markErr
			}
			continue
		}
		if err := s.repo.MarkIndexJobSucceeded(ctx, job.TrackID, time.Now()); err != nil {
			return result, err
		}
		result.Succeeded++
	}
	return result, nil
}

func (s *TrackMapIndexService) processJob(ctx context.Context, trackID string) error {
	track, err := s.tracks.FindByID(ctx, trackID)
	if err != nil {
		return err
	}
	if !isTrackMapIndexEligible(track) {
		return fmt.Errorf("track is not eligible for map index")
	}
	if s.rawTrackCache == nil {
		return fmt.Errorf("raw track cache is not configured")
	}
	cacheCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	localPath, err := s.rawTrackCache.EnsureCachedFile(cacheCtx, track.UserID, track.ID, track.RawTrackURL)
	if err != nil {
		return fmt.Errorf("cache raw track through internal OSS endpoint: %w", err)
	}
	points, err := parseTrackPointsFile(localPath)
	if err != nil {
		return err
	}
	index, err := buildTrackGeoIndex(track, points)
	if err != nil {
		return err
	}
	return s.repo.UpsertTrackGeoIndex(ctx, index)
}

func isTrackMapIndexEligible(track *models.Track) bool {
	return track != nil &&
		track.ID != "" &&
		track.RawTrackURL != "" &&
		!track.IsRunning &&
		track.Status == models.TrackStatusNormal
}

func retryDelay(attempts int) time.Duration {
	if attempts <= 1 {
		return time.Minute
	}
	if attempts >= 6 {
		return time.Hour
	}
	return time.Duration(1<<uint(attempts-1)) * time.Minute
}

func parseTrackPointsFile(path string) ([]models.TrackPoint, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fi.Size() <= 0 {
		return nil, errors.New("raw track file is empty")
	}
	if fi.Size() > maxRawTrackIndexFileBytes {
		return nil, fmt.Errorf("raw track file too large: %d bytes", fi.Size())
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxRawTrackIndexFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxRawTrackIndexFileBytes {
		return nil, fmt.Errorf("raw track file exceeds %d bytes", maxRawTrackIndexFileBytes)
	}
	return parseTrackPointsJSON(data)
}

func parseTrackPointsJSON(data []byte) ([]models.TrackPoint, error) {
	var direct []models.TrackPoint
	if err := json.Unmarshal(data, &direct); err == nil && len(direct) > 0 {
		return normalizeTrackPoints(direct), nil
	}

	var wrapper struct {
		Points []models.TrackPoint `json:"points"`
		Track  []models.TrackPoint `json:"track"`
	}
	if err := json.Unmarshal(data, &wrapper); err == nil {
		if len(wrapper.Points) > 0 {
			return normalizeTrackPoints(wrapper.Points), nil
		}
		if len(wrapper.Track) > 0 {
			return normalizeTrackPoints(wrapper.Track), nil
		}
	}

	points, err := parseGeoJSONPoints(data)
	if err == nil && len(points) > 0 {
		return normalizeTrackPoints(points), nil
	}
	return nil, errors.New("unsupported raw track format for map index")
}

func parseGeoJSONPoints(data []byte) ([]models.TrackPoint, error) {
	var doc struct {
		Type     string `json:"type"`
		Geometry *struct {
			Type        string            `json:"type"`
			Coordinates []json.RawMessage `json:"coordinates"`
		} `json:"geometry"`
		Coordinates []json.RawMessage `json:"coordinates"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	geometryType := doc.Type
	coords := doc.Coordinates
	if strings.EqualFold(doc.Type, "Feature") && doc.Geometry != nil {
		geometryType = doc.Geometry.Type
		coords = doc.Geometry.Coordinates
	}
	if !strings.EqualFold(geometryType, "LineString") {
		return nil, errors.New("geojson is not a linestring")
	}
	points := make([]models.TrackPoint, 0, len(coords))
	for i, raw := range coords {
		var pair []float64
		if err := json.Unmarshal(raw, &pair); err != nil || len(pair) < 2 {
			continue
		}
		points = append(points, models.TrackPoint{Index: i, Longitude: pair[0], Latitude: pair[1]})
	}
	return points, nil
}

func normalizeTrackPoints(points []models.TrackPoint) []models.TrackPoint {
	out := make([]models.TrackPoint, 0, len(points))
	for i, p := range points {
		if !validLatLng(p.Latitude, p.Longitude) {
			continue
		}
		if p.Index == 0 {
			p.Index = i
		}
		out = append(out, p)
	}
	return out
}

func buildTrackGeoIndex(track *models.Track, points []models.TrackPoint) (*models.TrackGeoIndex, error) {
	if len(points) < 2 {
		return nil, errors.New("not enough valid track points")
	}
	minLat, maxLat := points[0].Latitude, points[0].Latitude
	minLng, maxLng := points[0].Longitude, points[0].Longitude
	var sumLat, sumLng float64
	for _, p := range points {
		if p.Latitude < minLat {
			minLat = p.Latitude
		}
		if p.Latitude > maxLat {
			maxLat = p.Latitude
		}
		if p.Longitude < minLng {
			minLng = p.Longitude
		}
		if p.Longitude > maxLng {
			maxLng = p.Longitude
		}
		sumLat += p.Latitude
		sumLng += p.Longitude
	}
	simplified := simplifyTrackPoints(points, maxSimplifiedTrackMapPoints)
	polylineJSON, err := json.Marshal(simplified)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	first := points[0]
	last := points[len(points)-1]
	return &models.TrackGeoIndex{
		TrackID:                track.ID,
		UserID:                 track.UserID,
		CityCode:               track.CityCode,
		TrackType:              defaultTrackMapType(track.TrackType),
		CoordinateSystem:       track.CoordinateSystem,
		StartLat:               first.Latitude,
		StartLng:               first.Longitude,
		EndLat:                 last.Latitude,
		EndLng:                 last.Longitude,
		CenterLat:              sumLat / float64(len(points)),
		CenterLng:              sumLng / float64(len(points)),
		MinLat:                 minLat,
		MinLng:                 minLng,
		MaxLat:                 maxLat,
		MaxLng:                 maxLng,
		Distance:               track.Distance,
		PointCount:             len(points),
		SimplifiedPolyline:     simplified,
		SimplifiedPolylineJSON: string(polylineJSON),
		CreatedAt:              now,
		UpdatedAt:              now,
	}, nil
}

func defaultTrackMapType(trackType string) string {
	trackType = strings.TrimSpace(trackType)
	if trackType == "" {
		return "徒步"
	}
	return trackType
}

func simplifyTrackPoints(points []models.TrackPoint, maxPoints int) []models.TrackPoint {
	if maxPoints <= 0 || len(points) <= maxPoints {
		return append([]models.TrackPoint(nil), points...)
	}
	out := make([]models.TrackPoint, 0, maxPoints)
	step := float64(len(points)-1) / float64(maxPoints-1)
	for i := 0; i < maxPoints; i++ {
		idx := int(math.Round(float64(i) * step))
		if idx >= len(points) {
			idx = len(points) - 1
		}
		out = append(out, points[idx])
	}
	return out
}

func validLatLng(lat, lng float64) bool {
	return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180 && !math.IsNaN(lat) && !math.IsNaN(lng)
}
