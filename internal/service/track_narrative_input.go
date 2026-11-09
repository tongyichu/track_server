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

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

const trackNarrativeRuleVersion = 4

type trackNarrativeInput struct {
	LearnedShapeExamples []map[string]string   `json:"learned_shape_examples,omitempty"`
	Title                string                `json:"title"`
	Location             string                `json:"location,omitempty"`
	City                 string                `json:"city,omitempty"`
	Labels               []trackNarrativeLabel `json:"labels"`
	ShapeLocked          bool                  `json:"shape_locked"`
	Sport                string                `json:"sport"`
	DistanceKM           float64               `json:"distance_km"`
	DurationMinutes      int                   `json:"recorded_duration_minutes"`
	Gain                 int                   `json:"recorded_gain_m"`
	// These are samples, not maxima or a distance-resampled elevation profile.
	ElevationSamples  []int                     `json:"sampled_elevation_m,omitempty"`
	ScenicName        string                    `json:"scenic_name,omitempty"`
	ScenicDescription string                    `json:"scenic_background,omitempty"`
	ScenicCategory    string                    `json:"scenic_category,omitempty"`
	Submission        *trackNarrativeSubmission `json:"approved_submission,omitempty"`
}

type trackNarrativeLabel struct {
	Type string `json:"type"`
	Code string `json:"code,omitempty"`
	Name string `json:"name"`
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
	FactsVersion   string
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
	snap.Input = trackNarrativeInput{Title: narrativeText(t.Title, 80), Location: narrativeText(t.LocateAddr, 100), City: narrativeText(config.CityNameByCode(t.CityCode), 80), Sport: normalizeTrackTypeCode(t.TrackType), DistanceKM: math.Round(t.Distance/10) / 100, DurationMinutes: int(t.Duration / 60), Gain: t.ElevationGain}
	var state narrativeState
	if err := s.readState(ctx, "narrative:track:"+id, &state); err != nil {
		return nil, err
	}
	manualShape, err := s.adminOwnsShape(ctx, t)
	if err != nil {
		return nil, err
	}
	snap.Input.ShapeLocked = manualShape || (state.OwnsShape && t.ShapeLabel != state.LastShape) || (!state.OwnsShape && t.ShapeLabel != "")
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
			if strings.TrimSpace(v.Name) == "" && spot != nil {
				v.Name = spot.Name
			}
			spot = v
		}
	}
	if spot != nil {
		snap.Input.ScenicName = narrativeText(spot.Name, 80)
		snap.Input.ScenicCategory = narrativeText(spot.Category, 80)
		if !narrativeTestData(spot.Description) {
			snap.Input.ScenicDescription = narrativeText(spot.Description, 200)
		}
	}
	snap.PlaceReady = spot != nil || !s.trackSvc.places.Enabled()
	record, err := s.store.GetPlaceRecord(ctx, "match:"+id)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	if err == nil {
		var match models.TrackPlaceMatch
		if json.Unmarshal([]byte(record.Data), &match) == nil && match.SourceVersion == trackPlaceSourceVersion(index) {
			snap.PlaceReady = snap.PlaceReady || match.Status == "matched" || match.Status == "unmatched" || match.Status == "candidate"
			if match.UpdatedAt.After(snap.ChangedAt) {
				snap.ChangedAt = match.UpdatedAt
			}
		}
	}
	if s.trackSvc.scenicCatalog != nil && s.trackSvc.scenicCatalog.Enabled() {
		links, err := s.trackSvc.scenicCatalog.spots.GetTrackScenicSpotLinks(ctx, []string{id})
		if err != nil {
			return nil, err
		}
		if link := links[id]; link != nil && link.UpdatedAt.After(snap.ChangedAt) {
			snap.ChangedAt = link.UpdatedAt
		}
	}
	// Read the approved record directly: no image decoration, provider lookup or public detail side effects.
	featured := false
	if s.trackSvc.submissions != nil {
		sub, err := s.trackSvc.submissions.repo.FindByTrackID(ctx, id)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return nil, err
		}
		if err == nil {
			featured = sub.Status == models.TrackSubmissionStatusApproved
			if sub.UpdatedAt.After(snap.ChangedAt) {
				snap.ChangedAt = sub.UpdatedAt
			}
			if sub.Status == models.TrackSubmissionStatusApproved && !narrativeTestData(sub.Title+sub.Description+sub.TransportDescription) {
				snap.Input.Submission = &trackNarrativeSubmission{Title: narrativeText(sub.Title, 40), Description: narrativeText(sub.Description, 300), Difficulty: sub.Difficulty, Risk: sub.RiskLevel, Surfaces: sub.SurfaceTypes, Months: sub.SuitableMonths, Transport: narrativeText(sub.TransportDescription, 100)}
			}
		}
	}
	snap.Input.Labels = make([]trackNarrativeLabel, 0, 4)
	for _, label := range buildTrackLabels(t.ShapeLabel, t.TrackType, spot, featured) {
		snap.Input.Labels = append(snap.Input.Labels, trackNarrativeLabel{Type: label.Type, Code: label.Code, Name: narrativeText(label.Name, 100)})
	}
	// Pass all labels to the model, but exclude our own shape output from change detection.
	versionInput := snap.Input
	versionInput.Labels = make([]trackNarrativeLabel, 0, len(snap.Input.Labels))
	for _, label := range snap.Input.Labels {
		if label.Type != "shape" || snap.Input.ShapeLocked {
			versionInput.Labels = append(versionInput.Labels, label)
		}
	}
	raw, err := json.Marshal([]interface{}{versionInput, t.RawTrackURL, t.CoordinateSystem, snap.PreviewURL, index.SimplifiedPolyline})
	if err != nil {
		return nil, err
	}
	snap.FactsVersion = fmt.Sprintf("%x", sha256.Sum256(raw))
	snap.Version = fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d:%s:%s", trackNarrativeRuleVersion, s.model, snap.FactsVersion))))
	return snap, nil
}
