package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

var ErrPreviewSourceChanged = errors.New("track preview source changed; refresh the task")
var ErrDescriptionSourceChanged = errors.New("track description source changed; refresh the task")
var previewUploadIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

type PreviewBackfillTask struct {
	TrackID          string  `json:"track_id"`
	Title            string  `json:"title"`
	TrackType        string  `json:"track_type"`
	CoordinateSystem string  `json:"coordinate_system"`
	SourceVersion    string  `json:"source_version"`
	LocateAddr       string  `json:"locate_addr"`
	StartTime        string  `json:"start_time"`
	EndTime          string  `json:"end_time"`
	Distance         float64 `json:"distance"`
	Duration         uint32  `json:"duration"`
}

type PreviewBackfillPage struct {
	Items           []PreviewBackfillTask `json:"items"`
	NextCursor      string                `json:"next_cursor"`
	HasMore         bool                  `json:"has_more"`
	PendingCount    int64                 `json:"pending_count"`
	WithoutRawCount int64                 `json:"without_raw_count"`
}

type PreviewBackfillInput struct {
	SourceVersion string `json:"source_version"`
	UploadID      string `json:"upload_id"`
}

type PreviewBackfillPoint struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type PreviewBackfillData struct {
	State  string                 `json:"state"`
	Task   PreviewBackfillTask    `json:"task"`
	Points []PreviewBackfillPoint `json:"points"`
}

type PreviewBackfillUpload struct {
	State      string            `json:"state"`
	UploadURLs map[string]string `json:"upload_urls,omitempty"`
}

type PreviewBackfillResult struct {
	State         string                    `json:"state"`
	PreviewImages models.TrackPreviewImages `json:"preview_images"`
}

func (s *TrackService) SetPreviewBackfillStore(store *OSSTokenService) { s.previewBackfill = store }

func previewSourceVersion(t *models.Track) string {
	source := t.RawTrackURL + "\x00" + t.CoordinateSystem + "\x00" + t.TrackType
	if t.PreviewImages.ResetVersion != "" {
		source += "\x00" + t.PreviewImages.ResetVersion
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(source)))
}

func previewBackfillTask(t *models.Track) PreviewBackfillTask {
	return PreviewBackfillTask{TrackID: t.ID, Title: t.Title, TrackType: t.TrackType,
		CoordinateSystem: t.CoordinateSystem, SourceVersion: previewSourceVersion(t), LocateAddr: t.LocateAddr,
		StartTime: t.StartTime.Format(time.RFC3339), EndTime: t.EndTime.Format(time.RFC3339), Distance: t.Distance, Duration: t.Duration}
}

func descriptionBackfillTask(t *models.Track) PreviewBackfillTask {
	task := previewBackfillTask(t)
	source := fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%.3f\x00%d", previewSourceVersion(t), t.Title,
		t.LocateAddr, t.StartTime.Format(time.RFC3339Nano), t.EndTime.Format(time.RFC3339Nano), t.Distance, t.Duration)
	task.SourceVersion = fmt.Sprintf("%x", sha256.Sum256([]byte(source)))
	return task
}

// DevMode uses ordinary business JWTs; cross-user access here is intentional and limited to backfilling previews.
func (s *TrackService) ListPreviewBackfill(ctx context.Context, rawCursor string, limit int) (*PreviewBackfillPage, error) {
	return s.listPreviewBackfill(ctx, rawCursor, limit, false)
}

func (s *TrackService) ListDescriptionBackfill(ctx context.Context, rawCursor string, limit int) (*PreviewBackfillPage, error) {
	return s.listPreviewBackfill(ctx, rawCursor, limit, true)
}

func (s *TrackService) listPreviewBackfill(ctx context.Context, rawCursor string, limit int, descriptions bool) (*PreviewBackfillPage, error) {
	cursor, err := decodeTrackListCursor(rawCursor)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	var tracks []*models.Track
	if descriptions {
		tracks, err = s.tracks.ListDescriptionBackfill(ctx, cursor, limit+1)
	} else {
		tracks, err = s.tracks.ListPreviewBackfill(ctx, cursor, limit+1)
	}
	if err != nil {
		return nil, err
	}
	var pending, withoutRaw int64
	if descriptions {
		pending, withoutRaw, err = s.tracks.DescriptionBackfillCounts(ctx)
	} else {
		pending, withoutRaw, err = s.tracks.PreviewBackfillCounts(ctx)
	}
	if err != nil {
		return nil, err
	}
	page := &PreviewBackfillPage{Items: make([]PreviewBackfillTask, 0, len(tracks)), PendingCount: pending, WithoutRawCount: withoutRaw}
	if len(tracks) > limit {
		page.HasMore = true
		tracks = tracks[:limit]
	}
	for _, track := range tracks {
		if descriptions {
			page.Items = append(page.Items, descriptionBackfillTask(track))
		} else {
			page.Items = append(page.Items, previewBackfillTask(track))
		}
	}
	if page.HasMore {
		last := tracks[len(tracks)-1]
		page.NextCursor, err = encodeTrackListCursor(last.StartTime, last.ID)
	}
	return page, err
}

func (s *TrackService) DescriptionBackfillData(ctx context.Context, id, version string) (*PreviewBackfillData, error) {
	if version == "" {
		return nil, invalidArg("source_version is required")
	}
	t, err := s.previewBackfillTrack(ctx, id, "")
	if err != nil {
		return nil, err
	}
	if version != descriptionBackfillTask(t).SourceVersion {
		return nil, ErrDescriptionSourceChanged
	}
	if strings.TrimSpace(t.Description) != "" {
		return &PreviewBackfillData{State: "already_complete", Task: descriptionBackfillTask(t)}, nil
	}
	data, err := s.previewBackfillDataForTrack(ctx, t)
	if err == nil {
		data.Task = descriptionBackfillTask(t)
	}
	return data, err
}

func (s *TrackService) previewBackfillTrack(ctx context.Context, id, version string) (*models.Track, error) {
	t, err := s.tracks.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.Status == models.TrackStatusDeleted || t.IsRunning || strings.TrimSpace(t.RawTrackURL) == "" {
		return nil, repository.ErrNotFound
	}
	if version != "" && version != previewSourceVersion(t) {
		return nil, ErrPreviewSourceChanged
	}
	return t, nil
}

func (s *TrackService) PreviewBackfillData(ctx context.Context, id, version string) (*PreviewBackfillData, error) {
	t, err := s.previewBackfillTrack(ctx, id, version)
	if err != nil {
		return nil, err
	}
	if t.PreviewImages.Complete() {
		return &PreviewBackfillData{State: "already_complete", Task: previewBackfillTask(t)}, nil
	}
	return s.previewBackfillDataForTrack(ctx, t)
}

func (s *TrackService) previewBackfillDataForTrack(ctx context.Context, t *models.Track) (*PreviewBackfillData, error) {
	result := &PreviewBackfillData{State: "pending", Task: previewBackfillTask(t), Points: []PreviewBackfillPoint{}}
	if s.rawTrackCache == nil {
		return nil, errors.New("raw track cache unavailable")
	}
	file, err := s.rawTrackCache.EnsureCachedFile(ctx, t.UserID, t.ID+"_preview_source_"+result.Task.SourceVersion, t.RawTrackURL)
	if err != nil {
		return nil, err
	}
	// Reuse the full-resolution JSON/GeoJSON/KML/KMZ reader; rendering and coordinate conversion remain on Android.
	points, err := parseTrackPointsFile(file)
	if err != nil {
		return nil, invalidArg(fmt.Sprintf("cannot parse raw GPS: %v", err))
	}
	if len(points) == 0 {
		return nil, invalidArg("raw track has no GPS points")
	}
	for _, point := range points {
		result.Points = append(result.Points, PreviewBackfillPoint{Latitude: point.Latitude, Longitude: point.Longitude})
	}
	return result, nil
}

type DescriptionBackfillInput struct {
	SourceVersion string `json:"source_version"`
	Description   string `json:"description"`
}
type DescriptionBackfillResult struct {
	State       string `json:"state"`
	Description string `json:"description"`
}

func (s *TrackService) CompleteDescriptionBackfill(ctx context.Context, id string, input DescriptionBackfillInput) (*DescriptionBackfillResult, error) {
	if input.SourceVersion == "" {
		return nil, invalidArg("source_version is required")
	}
	description := strings.TrimSpace(input.Description)
	if description == "" || len([]rune(description)) > 1000 {
		return nil, invalidArg("description must contain 1 to 1000 characters")
	}
	t, err := s.previewBackfillTrack(ctx, id, "")
	if err != nil {
		return nil, err
	}
	if input.SourceVersion != descriptionBackfillTask(t).SourceVersion {
		return nil, ErrDescriptionSourceChanged
	}
	if strings.TrimSpace(t.Description) != "" {
		return &DescriptionBackfillResult{State: "already_complete", Description: t.Description}, nil
	}
	updated, err := s.tracks.FillMissingDescription(ctx, t, description)
	if err != nil {
		return nil, err
	}
	if !updated {
		latest, e := s.tracks.FindByID(ctx, id)
		if e != nil {
			return nil, e
		}
		if latest.Status != models.TrackStatusDeleted && strings.TrimSpace(latest.Description) != "" {
			return &DescriptionBackfillResult{State: "already_complete", Description: latest.Description}, nil
		}
		return nil, ErrDescriptionSourceChanged
	}
	return &DescriptionBackfillResult{State: "completed", Description: description}, nil
}

func (s *TrackService) PreparePreviewBackfill(ctx context.Context, id string, input PreviewBackfillInput) (*PreviewBackfillUpload, error) {
	if input.SourceVersion == "" {
		return nil, invalidArg("source_version is required")
	}
	t, err := s.previewBackfillTrack(ctx, id, input.SourceVersion)
	if err != nil {
		return nil, err
	}
	if t.PreviewImages.Complete() {
		return &PreviewBackfillUpload{State: "already_complete"}, nil
	}
	if !previewUploadIDPattern.MatchString(input.UploadID) {
		return nil, invalidArg("invalid upload_id")
	}
	if s.previewBackfill == nil {
		return nil, errors.New("preview upload storage unavailable")
	}
	urls, err := s.previewBackfill.PreviewBackfillUploadURLs(ctx, t, input)
	if err != nil {
		return nil, err
	}
	return &PreviewBackfillUpload{State: "pending", UploadURLs: urls}, nil
}

func (s *TrackService) CompletePreviewBackfill(ctx context.Context, id string, input PreviewBackfillInput) (*PreviewBackfillResult, error) {
	if input.SourceVersion == "" {
		return nil, invalidArg("source_version is required")
	}
	t, err := s.previewBackfillTrack(ctx, id, input.SourceVersion)
	if err != nil {
		return nil, err
	}
	if t.PreviewImages.Complete() {
		return &PreviewBackfillResult{State: "already_complete", PreviewImages: trackPreviewURLs(t.ID, t.PreviewImages)}, nil
	}
	if !previewUploadIDPattern.MatchString(input.UploadID) {
		return nil, invalidArg("invalid upload_id")
	}
	if s.previewBackfill == nil {
		return nil, errors.New("preview upload storage unavailable")
	}
	images, err := s.previewBackfill.VerifyPreviewBackfill(ctx, t, input)
	if err != nil {
		return nil, err
	}
	images.ResetVersion = t.PreviewImages.ResetVersion
	updated, err := s.tracks.FillMissingPreviewImages(ctx, t, images)
	if err != nil {
		return nil, err
	}
	if !updated {
		latest, err := s.tracks.FindByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if latest.Status != models.TrackStatusDeleted && latest.PreviewImages.Complete() {
			return &PreviewBackfillResult{State: "already_complete", PreviewImages: trackPreviewURLs(id, latest.PreviewImages)}, nil
		}
		return nil, ErrPreviewSourceChanged
	}
	return &PreviewBackfillResult{State: "completed", PreviewImages: trackPreviewURLs(id, images)}, nil
}

type PreviewBackfillClearInput struct {
	All     bool   `json:"all"`
	TrackID string `json:"track_id"`
}

type PreviewBackfillClearResult struct {
	AffectedCount int64  `json:"affected_count"`
	All           bool   `json:"all"`
	TrackID       string `json:"track_id,omitempty"`
}

func (s *TrackService) ClearPreviewBackfill(ctx context.Context, input PreviewBackfillClearInput) (*PreviewBackfillClearResult, error) {
	id := strings.TrimSpace(input.TrackID)
	if input.All == (id != "") || len(id) > 64 {
		return nil, invalidArg("specify either all=true or a track_id")
	}
	var generation [16]byte
	if _, err := rand.Read(generation[:]); err != nil {
		return nil, err
	}
	affected, err := s.tracks.ClearPreviewImages(ctx, id, fmt.Sprintf("%x", generation))
	if err != nil {
		return nil, err
	}
	if id != "" && affected == 0 {
		return nil, repository.ErrNotFound
	}
	return &PreviewBackfillClearResult{AffectedCount: affected, All: input.All, TrackID: id}, nil
}
