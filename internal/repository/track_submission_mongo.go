package repository

import (
	"context"
	"errors"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoTrackSubmissionRepository struct{ collection *mongo.Collection }

func NewMongoTrackSubmissionRepository(collection *mongo.Collection) *MongoTrackSubmissionRepository {
	if collection != nil {
		_, _ = collection.Indexes().CreateOne(context.Background(), mongo.IndexModel{Keys: bson.D{{Key: "track_id", Value: 1}}, Options: options.Index().SetUnique(true).SetName("uk_track_submission_track")})
	}
	return &MongoTrackSubmissionRepository{collection: collection}
}

func (r *MongoTrackSubmissionRepository) SavePending(ctx context.Context, submission *models.TrackSubmission, event *models.TrackSubmissionEvent) error {
	var old models.TrackSubmission
	err := r.collection.FindOne(ctx, bson.M{"track_id": submission.TrackID}).Decode(&old)
	if err == nil {
		if old.SubmissionID != submission.SubmissionID {
			return ErrAlreadyExists
		}
		submission.Events = append([]*models.TrackSubmissionEvent(nil), old.Events...)
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return err
	}
	if event != nil {
		copyEvent := *event
		copyEvent.ID = time.Now().UnixNano()
		submission.Events = append(submission.Events, &copyEvent)
	}
	_, err = r.collection.ReplaceOne(ctx, bson.M{"_id": submission.SubmissionID}, submission, options.Replace().SetUpsert(true))
	if mongo.IsDuplicateKeyError(err) {
		return ErrAlreadyExists
	}
	return err
}

func (r *MongoTrackSubmissionRepository) findOne(ctx context.Context, filter interface{}) (*models.TrackSubmission, error) {
	var sub models.TrackSubmission
	if err := r.collection.FindOne(ctx, filter).Decode(&sub); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &sub, nil
}

func (r *MongoTrackSubmissionRepository) FindByTrackID(ctx context.Context, trackID string) (*models.TrackSubmission, error) {
	return r.findOne(ctx, bson.M{"track_id": trackID})
}

func (r *MongoTrackSubmissionRepository) FindBySubmissionID(ctx context.Context, submissionID string) (*models.TrackSubmission, error) {
	return r.findOne(ctx, bson.M{"_id": submissionID})
}

func (r *MongoTrackSubmissionRepository) ListByTrackIDs(ctx context.Context, trackIDs []string) (map[string]*models.TrackSubmission, error) {
	result := make(map[string]*models.TrackSubmission)
	if len(trackIDs) == 0 {
		return result, nil
	}
	cur, err := r.collection.Find(ctx, bson.M{"track_id": bson.M{"$in": trackIDs}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var sub models.TrackSubmission
		if err := cur.Decode(&sub); err != nil {
			return nil, err
		}
		result[sub.TrackID] = &sub
	}
	return result, cur.Err()
}

func (r *MongoTrackSubmissionRepository) List(ctx context.Context, filter models.TrackSubmissionListFilter) ([]*models.TrackSubmission, error) {
	query := bson.M{}
	if filter.Status != "" {
		query["status"] = filter.Status
	}
	if filter.Difficulty != "" {
		query["difficulty"] = filter.Difficulty
	}
	if filter.RiskLevel != "" {
		query["risk_level"] = filter.RiskLevel
	}
	if filter.TrackType != "" {
		query["track_type"] = filter.TrackType
	}
	if filter.UserID > 0 {
		query["user_id"] = filter.UserID
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	cur, err := r.collection.Find(ctx, query, options.Find().SetSort(bson.D{{Key: "submitted_at", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	items := make([]*models.TrackSubmission, 0)
	for cur.Next(ctx) {
		var sub models.TrackSubmission
		if err := cur.Decode(&sub); err != nil {
			return nil, err
		}
		items = append(items, &sub)
	}
	return items, cur.Err()
}

func mongoEvent(event *models.TrackSubmissionEvent) interface{} {
	if event == nil {
		return bson.M{}
	}
	copyEvent := *event
	copyEvent.ID = time.Now().UnixNano()
	return copyEvent
}

func (r *MongoTrackSubmissionRepository) Review(ctx context.Context, submissionID string, expectedRevision int64, status models.TrackSubmissionStatus, reviewer, reason string, now time.Time, event *models.TrackSubmissionEvent) error {
	set := bson.M{"status": status, "reviewed_by": reviewer, "review_reason": reason, "reviewed_at": now, "updated_at": now, "approved_at": nil}
	if status == models.TrackSubmissionStatusApproved {
		set["approved_at"] = now
	}
	res, err := r.collection.UpdateOne(ctx, bson.M{"_id": submissionID, "revision": expectedRevision, "status": models.TrackSubmissionStatusPending}, bson.M{"$set": set, "$push": bson.M{"events": mongoEvent(event)}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		if _, err := r.FindBySubmissionID(ctx, submissionID); err != nil {
			return err
		}
		return ErrAlreadyExists
	}
	return nil
}

func (r *MongoTrackSubmissionRepository) Withdraw(ctx context.Context, trackID string, userID int64, now time.Time, event *models.TrackSubmissionEvent) error {
	res, err := r.collection.UpdateOne(ctx, bson.M{"track_id": trackID, "user_id": userID, "status": bson.M{"$in": []models.TrackSubmissionStatus{models.TrackSubmissionStatusPending, models.TrackSubmissionStatusApproved}}}, bson.M{"$set": bson.M{"status": models.TrackSubmissionStatusWithdrawn, "approved_at": nil, "updated_at": now}, "$push": bson.M{"events": mongoEvent(event)}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		if sub, findErr := r.FindByTrackID(ctx, trackID); findErr != nil {
			return findErr
		} else if sub.UserID != userID {
			return ErrForbidden
		}
		return ErrAlreadyExists
	}
	return nil
}

func (r *MongoTrackSubmissionRepository) Invalidate(ctx context.Context, trackID, reason string, now time.Time, event *models.TrackSubmissionEvent) error {
	_, err := r.collection.UpdateOne(ctx, bson.M{"track_id": trackID, "status": models.TrackSubmissionStatusApproved}, bson.M{"$set": bson.M{"status": models.TrackSubmissionStatusInvalidated, "approved_at": nil, "review_reason": reason, "updated_at": now}, "$push": bson.M{"events": mongoEvent(event)}})
	return err
}

func (r *MongoTrackSubmissionRepository) ListEvents(ctx context.Context, submissionID string) ([]*models.TrackSubmissionEvent, error) {
	sub, err := r.FindBySubmissionID(ctx, submissionID)
	if err != nil {
		return nil, err
	}
	return sub.Events, nil
}
