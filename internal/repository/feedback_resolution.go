package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
)

type correctionStorage struct {
	*models.FeedbackCorrection
	HandledBy   string                      `json:"handled_by,omitempty"`
	TrackChange *models.FeedbackTrackChange `json:"track_change,omitempty"`
}

func marshalCorrectionStorage(c *models.FeedbackCorrection) ([]byte, error) {
	return json.Marshal(correctionStorage{c, c.HandledBy, c.TrackChange})
}

func unmarshalCorrectionStorage(data []byte) (*models.FeedbackCorrection, error) {
	if string(data) == "null" {
		return nil, nil
	}
	stored := correctionStorage{FeedbackCorrection: &models.FeedbackCorrection{}}
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, err
	}
	stored.FeedbackCorrection.HandledBy = stored.HandledBy
	stored.FeedbackCorrection.TrackChange = stored.TrackChange
	return stored.FeedbackCorrection, nil
}

func (r *MySQLFeedbackRepository) UpdateCorrectionStatus(ctx context.Context, id string, status models.FeedbackStatus, reply string, c *models.FeedbackCorrection, expected int64) error {
	data, err := marshalCorrectionStorage(c)
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `UPDATE user_feedbacks SET status=?, reply=?, correction_json=?, updated_at=? WHERE feedback_id=? AND correction_json IS NOT NULL AND COALESCE(JSON_EXTRACT(correction_json,'$.revision'),0)=?`, status, nullableStringValue(reply), string(data), time.Now(), id, expected)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return ErrAlreadyExists
	}
	return err
}

func (r *MongoFeedbackRepository) UpdateCorrectionStatus(ctx context.Context, id string, status models.FeedbackStatus, reply string, c *models.FeedbackCorrection, expected int64) error {
	query := bson.M{"feedback_id": id, "correction": bson.M{"$type": "object"}}
	if expected == 0 {
		query["$or"] = bson.A{bson.M{"correction.revision": 0}, bson.M{"correction.revision": bson.M{"$exists": false}}}
	} else {
		query["correction.revision"] = expected
	}
	result, err := r.collection.UpdateOne(ctx, query, bson.M{"$set": bson.M{"status": status, "reply": reply, "correction": c, "updated_at": time.Now()}})
	if err == nil && result.MatchedCount == 0 {
		return ErrAlreadyExists
	}
	return err
}

func (r *InMemoryFeedbackRepository) UpdateCorrectionStatus(_ context.Context, id string, status models.FeedbackStatus, reply string, c *models.FeedbackCorrection, expected int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.feedbacks[id]
	if !ok {
		return ErrNotFound
	}
	if item.Correction == nil || item.Correction.Revision != expected {
		return ErrAlreadyExists
	}
	item.Status, item.Reply, item.UpdatedAt = status, reply, time.Now()
	item.Correction = cloneFeedback(&models.Feedback{Correction: c}).Correction
	return nil
}
