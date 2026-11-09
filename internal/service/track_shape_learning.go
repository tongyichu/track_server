package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/tongyichu/track_server/internal/ai"
	"github.com/tongyichu/track_server/internal/models"
)

const shapeMemoryKey = "narrative:shape-memory"

type shapeFeedback struct {
	TaskID        string       `json:"task_id,omitempty"`
	Revision      string       `json:"revision"`
	TrackID       string       `json:"track_id"`
	Before        string       `json:"before"`
	Label         string       `json:"label"`
	PreviousAI    string       `json:"previous_ai"`
	HasPreviousAI bool         `json:"has_previous_ai"`
	SourceVersion string       `json:"source_version"`
	Model         string       `json:"model"`
	RuleVersion   int          `json:"rule_version"`
	Preview       string       `json:"preview"`
	Title         string       `json:"title"`
	Operator      string       `json:"operator"`
	CreatedAt     time.Time    `json:"created_at"`
	Status        string       `json:"status"`
	Attempts      int          `json:"attempts"`
	NextAttempt   time.Time    `json:"next_attempt"`
	Review        *shapeReview `json:"review,omitempty"`
}

type shapeReview struct {
	Supported bool   `json:"supported"`
	Evidence  string `json:"evidence"`
	Lesson    string `json:"lesson"`
}

type shapeMemory struct {
	Items []shapeFeedback `json:"items"`
}

func shapePreview(t *models.Track) string {
	if t.PreviewImages.SquareDayURL != "" {
		return t.PreviewImages.SquareDayURL
	}
	return t.PreviewImages.SquareNightURL
}

// A short record lease prevents an old review from overwriting a newer admin correction.
func (s *TrackNarrativeService) editShapeFeedback(ctx context.Context, id string, edit func(*shapeFeedback) error) error {
	key := "narrative:shape-feedback:" + id
	owner, err := submissionID("SHP-")
	if err != nil {
		return err
	}
	now := time.Now()
	ok, err := s.store.LeasePlaceRecord(ctx, key, owner, now, now.Add(15*time.Second))
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("shape feedback busy")
	}
	var f shapeFeedback
	if err = s.readState(ctx, key, &f); err != nil {
		return err
	}
	if err = edit(&f); err != nil {
		_ = s.saveState(ctx, key, f, owner)
		return err
	}
	return s.saveState(ctx, key, f, owner)
}

// RecordShapeCorrection records only a successful admin save, without calling AI in the request.
func (s *TrackNarrativeService) RecordShapeCorrection(ctx context.Context, before, after *models.Track, operator string) error {
	revision, err := submissionID("SHF-")
	if err != nil {
		return err
	}
	var previous narrativeState
	if err = s.readState(ctx, "narrative:track:"+after.ID, &previous); err != nil {
		return err
	}
	return s.editShapeFeedback(ctx, after.ID, func(f *shapeFeedback) error {
		// Re-read to avoid queuing an already superseded save.
		current, err := s.tracks.FindByID(ctx, after.ID)
		if err != nil {
			return err
		}
		if current.ShapeLabel != after.ShapeLabel || shapePreview(current) != shapePreview(after) {
			return errors.New("shape correction superseded")
		}
		*f = shapeFeedback{Revision: revision, TrackID: after.ID, Before: before.ShapeLabel, Label: after.ShapeLabel, PreviousAI: previous.LastShape, HasPreviousAI: previous.OwnsShape, SourceVersion: previous.Version, Model: s.model, RuleVersion: trackNarrativeRuleVersion, Preview: shapePreview(after), Title: after.Title, Operator: operator, CreatedAt: time.Now(), Status: "pending"}
		return nil
	})
}

// learnedShapeExamples are bounded reference data, never system instructions. Recheck sources
// so clearing/changing a correction, replacing the image or hiding a track retracts its lesson.
func (s *TrackNarrativeService) learnedShapeExamples(ctx context.Context) ([]map[string]string, error) {
	var memory shapeMemory
	if err := s.readState(ctx, shapeMemoryKey, &memory); err != nil {
		return nil, err
	}
	var out []map[string]string
	for _, item := range memory.Items {
		if len(out) >= 4 {
			break
		}
		var current shapeFeedback
		if err := s.readState(ctx, "narrative:shape-feedback:"+item.TrackID, &current); err != nil {
			return nil, err
		}
		if current.Revision != item.Revision || current.Status != "learned" || item.Review == nil {
			continue
		}
		t, err := s.tracks.FindByID(ctx, item.TrackID)
		if err != nil || !isTrackMapIndexEligible(t) || t.ShapeLabel != item.Label || shapePreview(t) != item.Preview {
			continue
		}
		out = append(out, map[string]string{"label": item.Label, "previous_label": item.Before, "visual_evidence": item.Review.Evidence, "lesson": item.Review.Lesson})
	}
	return out, nil
}

// Called only under the narrative scan lease; at most one learning request per scan.
func (s *TrackNarrativeService) learnShapeCorrection(ctx context.Context, id string) (called bool, err error) {
	var f shapeFeedback
	key := "narrative:shape-feedback:" + id
	if err := s.readState(ctx, key, &f); err != nil {
		return false, err
	}
	if f.Revision == "" || (f.Status != "pending" && f.Status != "retry" && f.Status != "running" && f.Status != "reviewed") || f.NextAttempt.After(time.Now()) {
		return false, nil
	}
	t, err := s.tracks.FindByID(ctx, id)
	if err != nil {
		return false, err
	}
	if !isTrackMapIndexEligible(t) || t.ShapeLabel != f.Label || shapePreview(t) != f.Preview {
		return false, s.editShapeFeedback(ctx, id, func(v *shapeFeedback) error {
			if v.Revision == f.Revision {
				v.Status = "stale"
			}
			return nil
		})
	}
	if allowed := s.ai.AutomaticAllowed(ctx, "shape_learning", id, f.Revision); allowed != nil {
		return false, nil
	}
	workCtx, cancelWork := context.WithTimeout(ctx, 270*time.Second)
	defer cancelWork()
	release, leaseErr := s.leaseNarrativeTrack(workCtx, id)
	if errors.Is(leaseErr, errNarrativeTrackBusy) {
		return false, nil
	}
	if leaseErr != nil {
		return false, leaseErr
	}
	defer release()
	ctx = workCtx
	var run *ai.Run
	defer func() {
		if run.CancellationRequested() {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.editShapeFeedback(cleanup, id, func(v *shapeFeedback) error {
				if v.Revision == f.Revision {
					v.Status = "cancelled"
					v.Review = nil
					v.NextAttempt = time.Time{}
				}
				return nil
			})
		}
		run.Finish(err)
	}()
	if f.Review == nil {
		if allowed := s.ai.AutomaticAllowed(ctx, "shape_learning", id, f.Revision); allowed != nil {
			return false, nil
		}

		if f.Attempts >= 3 {
			return false, s.editShapeFeedback(ctx, id, func(v *shapeFeedback) error {
				if v.Revision == f.Revision {
					v.Status = "failed"
				}
				return nil
			})
		}
		variant := "square_day"
		if t.PreviewImages.SquareDayURL == "" {
			variant = "square_night"
		}
		path, err := s.trackSvc.TrackPreviewFile(ctx, t.UserID, id, variant, previewVersion(f.Preview))
		if err != nil {
			return false, s.shapeLearningFailure(ctx, &f)
		}
		ctx, run, err = s.ai.Begin(ctx, ai.Spec{Feature: "shape_learning", Provider: "moonshot", Model: s.model, ObjectID: id, ObjectName: t.Title, Reason: fmt.Sprintf("人工改标后的学习复核（第%d次尝试）", f.Attempts+1), RetryOf: f.TaskID, InputVersion: f.Revision, RuleVersion: fmt.Sprint(trackNarrativeRuleVersion), InputSummary: "复核人工图案修正及轨迹方图，不修改人工标签"})
		if err != nil {
			return false, err
		}
		f.TaskID = ai.TaskID(ctx)
		f.Attempts++
		f.Status = "running"
		f.NextAttempt = time.Now().Add(5 * time.Minute)
		err = s.editShapeFeedback(ctx, id, func(v *shapeFeedback) error {
			if v.Revision != f.Revision {
				return errors.New("shape feedback changed")
			}
			*v = f
			return nil
		})
		if err != nil {
			return false, err
		}
		facts, _ := json.Marshal(map[string]interface{}{"title": f.Title, "shape_correction": map[string]interface{}{"before": f.Before, "admin_label": f.Label, "previous_ai": f.PreviousAI, "has_previous_ai": f.HasPreviousAI}})
		result, err := s.generator.Generate(ctx, string(facts), path)
		called = true
		if e := run.Check(); e != nil {
			return true, e
		}
		if ai.IsControlError(err) {
			f.Attempts--
			f.Status = "pending"
			f.NextAttempt = time.Now().Add(time.Minute)
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.editShapeFeedback(cleanup, id, func(v *shapeFeedback) error {
				if v.Revision == f.Revision {
					*v = f
				}
				return nil
			})
			return true, err
		}
		if err != nil || result.ShapeReview == nil {
			cause := err
			if cause == nil {
				cause = errors.New("AI 未返回有效学习复核结果")
			}
			if saveErr := s.shapeLearningFailure(ctx, &f); saveErr != nil {
				return true, saveErr
			}
			return true, cause
		}
		if err = run.Result(result.ShapeReview, "pending"); err != nil {
			return true, err
		}
		if err = run.BeforeWrite(); err != nil {
			return true, err
		}
		f.Review = result.ShapeReview
		if f.Review.Supported && (result.ShapeLabel != f.Label || (f.Label != "" && (result.ShapeAppropriate == nil || !*result.ShapeAppropriate || result.ShapeConfident == nil || !*result.ShapeConfident))) {
			f.Review.Supported = false
		}
		f.Status = "reviewed"
		f.NextAttempt = time.Time{}
		err = s.editShapeFeedback(ctx, id, func(v *shapeFeedback) error {
			if v.Revision != f.Revision {
				return errors.New("shape feedback changed")
			}
			*v = f
			return nil
		})
		if err != nil {
			return true, err
		}
	}
	if !f.Review.Supported {
		_ = run.Adoption("not_applied")
		return called, s.editShapeFeedback(ctx, id, func(v *shapeFeedback) error {
			if v.Revision == f.Revision {
				v.Status = "disputed"
			}
			return nil
		})
	}
	if err = run.BeforeWrite(); err != nil {
		return called, err
	}
	// Idempotent compact memory update. Reviewed results survive a failed persistence/restart.
	var memory shapeMemory
	if err = s.readState(ctx, shapeMemoryKey, &memory); err != nil {
		return called, err
	}
	items := []shapeFeedback{f}
	for _, item := range memory.Items {
		if item.TrackID != id && len(items) < 8 {
			items = append(items, item)
		}
	}
	memory.Items = items
	if err = s.saveState(ctx, shapeMemoryKey, memory, ""); err != nil {
		return called, err
	}
	err = s.editShapeFeedback(ctx, id, func(v *shapeFeedback) error {
		if v.Revision == f.Revision {
			v.Status = "learned"
		}
		return nil
	})
	if err == nil {
		_ = run.Adoption("applied")
		if run == nil && s.ai != nil && f.TaskID != "" {
			_ = s.ai.MarkAdopted(ctx, f.TaskID, "shape_learning", id)
		}
	}
	return called, err
}

func (s *TrackNarrativeService) shapeLearningFailure(ctx context.Context, f *shapeFeedback) error {
	return s.editShapeFeedback(ctx, f.TrackID, func(v *shapeFeedback) error {
		if v.Revision != f.Revision {
			return nil
		}
		if v.Attempts < f.Attempts {
			v.Attempts = f.Attempts
		} else if v.Status != "running" {
			v.Attempts++
		}
		v.Status = "retry"
		if v.Attempts >= 3 {
			v.Status = "failed"
		}
		v.NextAttempt = time.Now().Add(time.Duration(v.Attempts*5) * time.Minute)
		return nil
	})
}

func (s *TrackNarrativeService) shapeLearningStatus(ctx context.Context, id string) interface{} {
	var f shapeFeedback
	if err := s.readState(ctx, "narrative:shape-feedback:"+id, &f); err != nil {
		return map[string]string{"status": "unavailable"}
	}
	if f.Status == "" {
		return nil
	}
	if t, err := s.tracks.FindByID(ctx, id); err != nil || t.ShapeLabel != f.Label || shapePreview(t) != f.Preview || !isTrackMapIndexEligible(t) {
		f.Status = "stale"
	}
	return map[string]interface{}{"status": f.Status, "attempts": f.Attempts, "label": f.Label, "review": f.Review}
}

func (s *TrackNarrativeService) adminOwnsShape(ctx context.Context, t *models.Track) (bool, error) {
	var f shapeFeedback
	if err := s.readState(ctx, "narrative:shape-feedback:"+t.ID, &f); err != nil {
		return false, err
	}
	return f.Revision != "" && t.ShapeLabel == f.Label, nil
}
