package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

const narrativeScanKey = "narrative:scan"

// Keep the budget key within the existing MySQL provider VARCHAR(16).
const narrativeBudgetProvider = "kimi_track_narra"

type narrativeScan struct {
	ActivatedAt time.Time `json:"activated_at"`
	Cursor      string    `json:"cursor"`
}

type narrativeState struct {
	Version     string                `json:"version"`
	Status      string                `json:"status"`
	FirstSeen   time.Time             `json:"first_seen"`
	NextAttempt time.Time             `json:"next_attempt"`
	Attempts    int                   `json:"attempts"`
	Result      *TrackNarrativeResult `json:"result,omitempty"`
	OwnsShape   bool                  `json:"owns_shape"`
	LastShape   string                `json:"last_shape"`
}

type TrackNarrativeService struct {
	tracks    repository.TrackRepository
	writer    repository.TrackNarrativeWriter
	maps      repository.TrackMapRepository
	indexes   repository.TrackPlaceIndexReader
	store     repository.TrackPlaceRepository
	trackSvc  *TrackService
	generator TrackNarrativeGenerator
	model     string
	budget    int
	startedAt time.Time
	mu        sync.Mutex
}

type TrackNarrativeRunResult struct{ Scanned, Generated, Skipped, Deferred, Failed int }

func NewTrackNarrativeService(tracks repository.TrackRepository, maps repository.TrackMapRepository, store repository.TrackPlaceRepository, trackSvc *TrackService, generator TrackNarrativeGenerator, model string, budget int) *TrackNarrativeService {
	writer, ok := tracks.(repository.TrackNarrativeWriter)
	if !ok {
		return nil
	}
	indexes, ok := maps.(repository.TrackPlaceIndexReader)
	if !ok || store == nil || generator == nil || trackSvc == nil {
		return nil
	}
	if budget <= 0 {
		budget = 100
	}
	return &TrackNarrativeService{tracks: tracks, writer: writer, maps: maps, indexes: indexes, store: store, trackSvc: trackSvc, generator: generator, model: model, budget: budget, startedAt: time.Now()}
}

func (s *TrackNarrativeService) readState(ctx context.Context, key string, target interface{}) error {
	record, err := s.store.GetPlaceRecord(ctx, key)
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(record.Data), target)
}
func (s *TrackNarrativeService) saveState(ctx context.Context, key string, value interface{}, owner string) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	saved, err := s.store.SavePlaceRecord(ctx, &models.TrackPlaceRecord{Key: key, Data: string(raw)}, owner)
	if err != nil {
		return err
	}
	if !saved {
		return errors.New("narrative lease lost")
	}
	return nil
}

// RunOnce discovers changes from persisted geo indexes. It never downloads raw tracks.
// The initial activation watermark excludes untouched historical tracks from automatic billing.
func (s *TrackNarrativeService) RunOnce(parent context.Context) (*TrackNarrativeRunResult, error) {
	out := &TrackNarrativeRunResult{}
	if s == nil || !s.mu.TryLock() {
		return out, nil
	}
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(parent, 8*time.Minute)
	defer cancel()
	owner, err := submissionID("NAR-")
	if err != nil {
		return out, err
	}
	now := time.Now()
	acquired, err := s.store.LeasePlaceRecord(ctx, narrativeScanKey, owner, now, now.Add(10*time.Minute))
	if err != nil || !acquired {
		return out, err
	}
	var scan narrativeScan
	if err = s.readState(ctx, narrativeScanKey, &scan); err != nil {
		return out, err
	}
	if scan.ActivatedAt.IsZero() {
		scan.ActivatedAt = s.startedAt
	}
	// Keep the lease while checkpointing; owned save below releases it at completion.
	if err = s.saveState(ctx, narrativeScanKey, scan, ""); err != nil {
		return out, err
	}
	defer func() {
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer releaseCancel()
		if err := s.saveState(releaseCtx, narrativeScanKey, scan, owner); err != nil {
			log.Printf("[TrackNarrative] cursor_save_failed: %v", err)
		}
	}()
	indexes, err := s.indexes.ListPlaceGeoIndexes(ctx, scan.Cursor, 200)
	if err != nil {
		return out, err
	}
	if len(indexes) == 0 {
		scan.Cursor = ""
		return out, nil
	}
	calls := 0
	for _, index := range indexes {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		out.Scanned++
		called, err := s.process(ctx, index.TrackID, scan.ActivatedAt, out)
		if err != nil {
			out.Failed++
			log.Printf("[TrackNarrative] track_id=%s failed: %v", index.TrackID, err)
		}
		scan.Cursor = index.TrackID
		if called {
			calls++
		}
		// Bound each round's remote work; resume after the last visited ID on the next tick.
		if calls >= 3 {
			break
		}
	}
	if len(indexes) < 200 && scan.Cursor == indexes[len(indexes)-1].TrackID {
		// A short final page is already exhausted; avoid spending another tick resetting the cursor.
		scan.Cursor = ""
	}
	return out, nil
}

func (s *TrackNarrativeService) process(ctx context.Context, id string, activated time.Time, out *TrackNarrativeRunResult) (bool, error) {
	snap, err := s.snapshot(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		out.Skipped++
		return false, nil
	}
	if err != nil {
		return false, err
	}
	key := "narrative:track:" + id
	var state narrativeState
	if err = s.readState(ctx, key, &state); err != nil {
		return false, err
	}
	if state.Version == "" && snap.ChangedAt.Before(activated) {
		out.Skipped++
		return false, nil
	}
	now := time.Now()
	if state.Version != snap.Version {
		state = narrativeState{Version: snap.Version, Status: "waiting", FirstSeen: now, NextAttempt: now.Add(time.Minute), OwnsShape: state.OwnsShape, LastShape: state.LastShape}
		out.Deferred++
		return false, s.saveState(ctx, key, state, "")
	}
	if state.Status == "complete" || state.Status == "failed" {
		out.Skipped++
		return false, nil
	}
	if state.NextAttempt.After(now) || (!snap.PlaceReady && now.Sub(state.FirstSeen) < 10*time.Minute) {
		out.Deferred++
		return false, nil
	}
	t := snap.Track
	writeDescription := t.DescriptionSource == "" || t.DescriptionSource == "auto" || t.DescriptionSource == "ai"
	writeShape := (state.OwnsShape && t.ShapeLabel == state.LastShape) || (!state.OwnsShape && t.ShapeLabel == "")
	if !writeDescription && !writeShape {
		state.Status = "complete"
		out.Skipped++
		return false, s.saveState(ctx, key, state, "")
	}
	called := false
	if state.Result == nil {
		if state.Attempts >= 3 {
			state.Status = "failed"
			return false, s.saveState(ctx, key, state, "")
		}
		// Fetch exactly one private preview through the existing internal OSS cache.
		imagePath, err := s.trackSvc.TrackPreviewFile(ctx, t.UserID, t.ID, snap.PreviewVariant, previewVersion(snap.PreviewURL))
		if err != nil {
			return false, s.fail(ctx, key, &state, err)
		}
		day := now.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02")
		reserved, err := s.store.ReservePlaceAPICall(ctx, narrativeBudgetProvider, day, s.budget)
		if err != nil {
			return false, err
		}
		if !reserved {
			out.Deferred++
			return false, nil
		}
		state.Attempts++
		state.Status = "running"
		state.NextAttempt = now.Add(5 * time.Minute)
		if err = s.saveState(ctx, key, state, ""); err != nil {
			return false, err
		}
		raw, _ := json.Marshal(snap.Input)
		called = true
		state.Result, err = s.generator.Generate(ctx, string(raw), imagePath)
		if err != nil {
			return true, s.fail(ctx, key, &state, err)
		}
		// Cache the paid result before CAS. Restart/concurrent edits do not require a second call.
		state.Status = "ready"
		if err = s.saveState(ctx, key, state, ""); err != nil {
			return true, err
		}
	}
	current, err := s.snapshot(ctx, id)
	if err != nil {
		return called, err
	}
	if current.Version != snap.Version {
		out.Deferred++
		return called, nil
	}
	t = current.Track
	writeDescription = t.DescriptionSource == "" || t.DescriptionSource == "auto" || t.DescriptionSource == "ai"
	writeShape = (state.OwnsShape && t.ShapeLabel == state.LastShape) || (!state.OwnsShape && t.ShapeLabel == "")
	applied, err := s.writer.ApplyTrackNarrative(ctx, t, state.Result.Description, state.Result.ShapeLabel, writeDescription, writeShape)
	if err != nil {
		return called, err
	}
	if !applied {
		out.Deferred++
		return called, nil
	}
	if writeShape {
		state.OwnsShape = true
		state.LastShape = state.Result.ShapeLabel
	}
	state.Status = "complete"
	state.Result = nil
	if err = s.saveState(ctx, key, state, ""); err != nil {
		return called, err
	}
	out.Generated++
	return called, nil
}

func (s *TrackNarrativeService) fail(ctx context.Context, key string, state *narrativeState, cause error) error {
	// Image failures also have bounded retries, but do not consume the model-call budget.
	if state.Status != "running" {
		state.Attempts++
	}
	state.Status = "retry"
	if state.Attempts >= 3 {
		state.Status = "failed"
	}
	state.NextAttempt = time.Now().Add(time.Duration(state.Attempts*5) * time.Minute)
	if err := s.saveState(ctx, key, state, ""); err != nil {
		return err
	}
	return fmt.Errorf("generation attempt %d failed: %w", state.Attempts, cause)
}
