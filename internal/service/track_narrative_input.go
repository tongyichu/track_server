package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

const trackNarrativeRuleVersion = 2

type trackNarrativeInput struct {
	LearnedShapeExamples []map[string]string `json:"learned_shape_examples,omitempty"`
	Title                string              `json:"title"`
	Location             string              `json:"location,omitempty"`
	Sport                string              `json:"sport"`
	DistanceKM           float64             `json:"distance_km"`
	DurationMinutes      int                 `json:"recorded_duration_minutes"`
	Gain                 int                 `json:"recorded_gain_m"`
	// These are samples, not maxima or a distance-resampled elevation profile.
	ElevationSamples  []int                     `json:"sampled_elevation_m,omitempty"`
	ScenicName        string                    `json:"scenic_name,omitempty"`
	ScenicDescription string                    `json:"scenic_background,omitempty"`
	Submission        *trackNarrativeSubmission `json:"approved_submission,omitempty"`
}

type trackNarrativeSubmission struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Difficulty  string   `json:"difficulty"`
	Risk        string   `json:"risk"`
	Surfaces    []string `json:"surfaces"`
	Months      []int    `json:"months"`
	Transport   string   `json:"transport,omitempty"`
}

type trackNarrativeSnapshot struct {
	Track          *models.Track
	Index          *models.TrackGeoIndex
	Input          trackNarrativeInput
	Version        string
	PreviewURL     string
	PreviewVariant string
	PlaceReady     bool
	ChangedAt      time.Time
}

func narrativeText(s string, limit int) string {
	chars := []rune(strings.Join(strings.Fields(s), " "))
	if len(chars) > limit {
		chars = chars[:limit]
	}
	return string(chars)
}
func narrativeTestData(s string) bool {
	s = strings.ToLower(s)
	for _, token := range []string{"devmode", "投稿测试", "审核链路", "test submission"} {
		if strings.Contains(s, token) {
			return true
		}
	}
	return false
}

func (s *TrackNarrativeService) snapshot(ctx context.Context, id string) (*trackNarrativeSnapshot, error) {
	t, err := s.tracks.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !isTrackMapIndexEligible(t) {
		return nil, repository.ErrNotFound
	}
	index, err := s.maps.FindTrackGeoIndex(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(index.SimplifiedPolyline) < 2 {
		return nil, repository.ErrNotFound
	}
	snap := &trackNarrativeSnapshot{Track: t, Index: index, ChangedAt: t.UpdatedAt, PreviewVariant: "square_day", PreviewURL: t.PreviewImages.SquareDayURL}
	if snap.PreviewURL == "" {
		snap.PreviewURL = t.PreviewImages.SquareNightURL
		snap.PreviewVariant = "square_night"
	}
	if snap.PreviewURL == "" {
		return nil, repository.ErrNotFound
	}
	snap.Input = trackNarrativeInput{Title: narrativeText(t.Title, 80), Location: narrativeText(t.LocateAddr, 100), Sport: normalizeTrackTypeCode(t.TrackType), DistanceKM: math.Round(t.Distance/10) / 100, DurationMinutes: int(t.Duration / 60), Gain: t.ElevationGain}
	points := index.SimplifiedPolyline
	for n := 0; n < 7; n++ {
		altitude := points[n*(len(points)-1)/6].Elevation
		if math.IsNaN(altitude) || math.IsInf(altitude, 0) {
			snap.Input.ElevationSamples = nil
			break
		}
		snap.Input.ElevationSamples = append(snap.Input.ElevationSamples, int(math.Round(altitude)))
	}
	var spot *models.TrackPrimaryScenicSpot
	if s.trackSvc.places != nil {
		places, err := s.trackSvc.places.PrimaryScenicSpots(ctx, []string{id})
		if err != nil {
			return nil, err
		}
		spot = places[id]
	}
	if s.trackSvc.scenicCatalog != nil {
		spots, err := s.trackSvc.scenicCatalog.PrimarySpots(ctx, []string{id})
		if err != nil {
			return nil, err
		}
		if v := spots[id]; v != nil {
			spot = v
		}
	}
	if spot != nil {
		snap.Input.ScenicName = narrativeText(spot.Name, 80)
		if !narrativeTestData(spot.Description) {
			snap.Input.ScenicDescription = narrativeText(spot.Description, 200)
		}
	}
	snap.PlaceReady = spot != nil || !s.trackSvc.places.Enabled()
	if !snap.PlaceReady {
		record, err := s.store.GetPlaceRecord(ctx, "match:"+id)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return nil, err
		}
		if err == nil {
			var match models.TrackPlaceMatch
			if json.Unmarshal([]byte(record.Data), &match) == nil && match.SourceVersion == trackPlaceSourceVersion(index) {
				snap.PlaceReady = match.Status == "matched" || match.Status == "unmatched" || match.Status == "candidate"
			}
		}
	}
	// Read the approved record directly: no image decoration, provider lookup or public detail side effects.
	if s.trackSvc.submissions != nil {
		sub, err := s.trackSvc.submissions.repo.FindByTrackID(ctx, id)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return nil, err
		}
		if err == nil {
			if sub.UpdatedAt.After(snap.ChangedAt) {
				snap.ChangedAt = sub.UpdatedAt
			}
			if sub.Status == models.TrackSubmissionStatusApproved && !narrativeTestData(sub.Title+sub.Description+sub.TransportDescription) {
				snap.Input.Submission = &trackNarrativeSubmission{Title: narrativeText(sub.Title, 40), Description: narrativeText(sub.Description, 300), Difficulty: sub.Difficulty, Risk: sub.RiskLevel, Surfaces: sub.SurfaceTypes, Months: sub.SuitableMonths, Transport: narrativeText(sub.TransportDescription, 100)}
			}
		}
	}
	// Exclude UpdatedAt and generated outputs: saving AI content must not retrigger itself.
	raw, err := json.Marshal([]interface{}{trackNarrativeRuleVersion, s.model, snap.Input, t.RawTrackURL, t.CoordinateSystem, snap.PreviewURL, index.SimplifiedPolyline})
	if err != nil {
		return nil, err
	}
	snap.Version = fmt.Sprintf("%x", sha256.Sum256(raw))
	return snap, nil
}
