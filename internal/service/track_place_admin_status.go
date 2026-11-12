package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

// AdminRecognitionStatus projects saved work only; opening an editor never
// queues recognition or calls a provider. Unknown/expired work awaits a retry.
func (s *TrackPlaceService) AdminRecognitionStatus(ctx context.Context, trackID string, index *models.TrackGeoIndex) (string, error) {
	if s == nil || s.store == nil || index == nil {
		return "waiting", nil
	}
	record, err := s.store.GetPlaceRecord(ctx, "match:"+trackID)
	if errors.Is(err, repository.ErrNotFound) {
		return "waiting", nil
	}
	if err != nil {
		return "", err
	}
	if record.LeaseOwner != "" && record.LeaseUntil.After(time.Now()) {
		return "running", nil
	}
	if !trackPlaceRecordFresh(record) {
		return "waiting", nil
	}
	var match models.TrackPlaceMatch
	if err := json.Unmarshal([]byte(record.Data), &match); err != nil {
		return "", err
	}
	if match.SourceVersion != trackPlaceSourceVersion(index) || match.RuleVersion != trackPlaceRuleVersion {
		return "waiting", nil
	}
	switch match.Status {
	case "matched":
		return "found", nil
	case "candidate", "unmatched":
		return "not_found", nil
	default:
		return "waiting", nil
	}
}
