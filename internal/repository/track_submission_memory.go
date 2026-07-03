package repository

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

type InMemoryTrackSubmissionRepository struct {
	mu          sync.RWMutex
	byID        map[string]*models.TrackSubmission
	trackToID   map[string]string
	nextEventID int64
}

func NewInMemoryTrackSubmissionRepository() *InMemoryTrackSubmissionRepository {
	return &InMemoryTrackSubmissionRepository{byID: make(map[string]*models.TrackSubmission), trackToID: make(map[string]string), nextEventID: 1}
}

func cloneSubmission(in *models.TrackSubmission) *models.TrackSubmission {
	if in == nil {
		return nil
	}
	out := *in
	out.SuitableMonths = append([]int(nil), in.SuitableMonths...)
	out.SurfaceTypes = append([]string(nil), in.SurfaceTypes...)
	out.TransportModes = append([]string(nil), in.TransportModes...)
	out.Images = make([]*models.TrackSubmissionImage, 0, len(in.Images))
	for _, image := range in.Images {
		if image != nil {
			copyImage := *image
			out.Images = append(out.Images, &copyImage)
		}
	}
	out.Events = make([]*models.TrackSubmissionEvent, 0, len(in.Events))
	for _, event := range in.Events {
		if event != nil {
			copyEvent := *event
			out.Events = append(out.Events, &copyEvent)
		}
	}
	return &out
}

func (r *InMemoryTrackSubmissionRepository) appendEventLocked(sub *models.TrackSubmission, event *models.TrackSubmissionEvent) {
	if event == nil {
		return
	}
	copyEvent := *event
	copyEvent.ID = r.nextEventID
	r.nextEventID++
	sub.Events = append(sub.Events, &copyEvent)
}

func (r *InMemoryTrackSubmissionRepository) SavePending(_ context.Context, submission *models.TrackSubmission, event *models.TrackSubmissionEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existingID := r.trackToID[submission.TrackID]; existingID != "" && existingID != submission.SubmissionID {
		return ErrAlreadyExists
	}
	copySub := cloneSubmission(submission)
	if old := r.byID[submission.SubmissionID]; old != nil {
		copySub.Events = append([]*models.TrackSubmissionEvent(nil), old.Events...)
	}
	r.appendEventLocked(copySub, event)
	r.byID[copySub.SubmissionID] = copySub
	r.trackToID[copySub.TrackID] = copySub.SubmissionID
	return nil
}

func (r *InMemoryTrackSubmissionRepository) FindByTrackID(_ context.Context, trackID string) (*models.TrackSubmission, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sub := r.byID[r.trackToID[trackID]]
	if sub == nil {
		return nil, ErrNotFound
	}
	return cloneSubmission(sub), nil
}

func (r *InMemoryTrackSubmissionRepository) FindBySubmissionID(_ context.Context, submissionID string) (*models.TrackSubmission, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sub := r.byID[submissionID]
	if sub == nil {
		return nil, ErrNotFound
	}
	return cloneSubmission(sub), nil
}

func (r *InMemoryTrackSubmissionRepository) ListByTrackIDs(_ context.Context, trackIDs []string) (map[string]*models.TrackSubmission, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]*models.TrackSubmission, len(trackIDs))
	for _, trackID := range trackIDs {
		if sub := r.byID[r.trackToID[trackID]]; sub != nil {
			result[trackID] = cloneSubmission(sub)
		}
	}
	return result, nil
}

func (r *InMemoryTrackSubmissionRepository) List(_ context.Context, filter models.TrackSubmissionListFilter) ([]*models.TrackSubmission, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]*models.TrackSubmission, 0)
	for _, sub := range r.byID {
		if filter.Status != "" && sub.Status != filter.Status || filter.Difficulty != "" && sub.Difficulty != filter.Difficulty || filter.RiskLevel != "" && sub.RiskLevel != filter.RiskLevel || filter.TrackType != "" && sub.TrackType != filter.TrackType || filter.UserID > 0 && sub.UserID != filter.UserID {
			continue
		}
		items = append(items, cloneSubmission(sub))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].SubmittedAt.Equal(items[j].SubmittedAt) {
			return items[i].SubmissionID > items[j].SubmissionID
		}
		return items[i].SubmittedAt.After(items[j].SubmittedAt)
	})
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *InMemoryTrackSubmissionRepository) Review(_ context.Context, submissionID string, expectedRevision int64, status models.TrackSubmissionStatus, reviewer, reason string, now time.Time, event *models.TrackSubmissionEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	sub := r.byID[submissionID]
	if sub == nil {
		return ErrNotFound
	}
	if sub.Revision != expectedRevision || sub.Status != models.TrackSubmissionStatusPending {
		return ErrAlreadyExists
	}
	sub.Status, sub.ReviewedBy, sub.ReviewReason, sub.UpdatedAt = status, reviewer, reason, now
	sub.ReviewedAt = &now
	if status == models.TrackSubmissionStatusApproved {
		sub.ApprovedAt = &now
	} else {
		sub.ApprovedAt = nil
	}
	r.appendEventLocked(sub, event)
	return nil
}

func (r *InMemoryTrackSubmissionRepository) Withdraw(_ context.Context, trackID string, userID int64, now time.Time, event *models.TrackSubmissionEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	sub := r.byID[r.trackToID[trackID]]
	if sub == nil {
		return ErrNotFound
	}
	if sub.UserID != userID {
		return ErrForbidden
	}
	if sub.Status != models.TrackSubmissionStatusPending && sub.Status != models.TrackSubmissionStatusApproved {
		return ErrAlreadyExists
	}
	sub.Status, sub.ApprovedAt, sub.UpdatedAt = models.TrackSubmissionStatusWithdrawn, nil, now
	r.appendEventLocked(sub, event)
	return nil
}

func (r *InMemoryTrackSubmissionRepository) Invalidate(_ context.Context, trackID, reason string, now time.Time, event *models.TrackSubmissionEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	sub := r.byID[r.trackToID[trackID]]
	if sub == nil {
		return ErrNotFound
	}
	if sub.Status != models.TrackSubmissionStatusApproved {
		return nil
	}
	sub.Status, sub.ApprovedAt, sub.ReviewReason, sub.UpdatedAt = models.TrackSubmissionStatusInvalidated, nil, reason, now
	r.appendEventLocked(sub, event)
	return nil
}

func (r *InMemoryTrackSubmissionRepository) ListEvents(_ context.Context, submissionID string) ([]*models.TrackSubmissionEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sub := r.byID[submissionID]
	if sub == nil {
		return nil, ErrNotFound
	}
	result := make([]*models.TrackSubmissionEvent, len(sub.Events))
	for i, event := range sub.Events {
		copyEvent := *event
		result[i] = &copyEvent
	}
	return result, nil
}
