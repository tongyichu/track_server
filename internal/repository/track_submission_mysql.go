package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

type MySQLTrackSubmissionRepository struct{ db *sql.DB }

func NewMySQLTrackSubmissionRepository(db *sql.DB) *MySQLTrackSubmissionRepository {
	return &MySQLTrackSubmissionRepository{db: db}
}

func submissionJSON(value interface{}) string {
	buf, _ := json.Marshal(value)
	return string(buf)
}

func insertSubmissionEvent(ctx context.Context, tx *sql.Tx, event *models.TrackSubmissionEvent) error {
	if event == nil {
		return nil
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO track_submission_events
		(submission_id,revision,event_type,from_status,to_status,operator_type,operator,reason,snapshot_json,created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`, event.SubmissionID, event.Revision, event.EventType, event.FromStatus, event.ToStatus, event.OperatorType, event.Operator, event.Reason, event.SnapshotJSON, event.CreatedAt)
	return err
}

func (r *MySQLTrackSubmissionRepository) SavePending(ctx context.Context, sub *models.TrackSubmission, event *models.TrackSubmissionEvent) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO track_submissions
		(submission_id,track_id,user_id,track_type,title,description,difficulty,risk_level,suitable_months_json,surface_types_json,transport_modes_json,transport_description,status,revision,submitted_at,approved_at,reviewed_at,reviewed_by,review_reason,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE user_id=VALUES(user_id),track_type=VALUES(track_type),title=VALUES(title),description=VALUES(description),difficulty=VALUES(difficulty),risk_level=VALUES(risk_level),suitable_months_json=VALUES(suitable_months_json),surface_types_json=VALUES(surface_types_json),transport_modes_json=VALUES(transport_modes_json),transport_description=VALUES(transport_description),status=VALUES(status),revision=VALUES(revision),submitted_at=VALUES(submitted_at),approved_at=VALUES(approved_at),reviewed_at=VALUES(reviewed_at),reviewed_by=VALUES(reviewed_by),review_reason=VALUES(review_reason),updated_at=VALUES(updated_at)`,
		sub.SubmissionID, sub.TrackID, sub.UserID, sub.TrackType, sub.Title, sub.Description, sub.Difficulty, sub.RiskLevel, submissionJSON(sub.SuitableMonths), submissionJSON(sub.SurfaceTypes), submissionJSON(sub.TransportModes), sub.TransportDescription, sub.Status, sub.Revision, sub.SubmittedAt, sub.ApprovedAt, sub.ReviewedAt, sub.ReviewedBy, sub.ReviewReason, sub.CreatedAt, sub.UpdatedAt)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM track_submission_images WHERE submission_id=?`, sub.SubmissionID); err != nil {
		return err
	}
	for _, image := range sub.Images {
		if image == nil {
			continue
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO track_submission_images (image_id,submission_id,oss_url,caption,sort_order,created_at,updated_at) VALUES (?,?,?,?,?,?,?)`, image.ImageID, sub.SubmissionID, image.OSSURL, image.Caption, image.SortOrder, image.CreatedAt, image.UpdatedAt); err != nil {
			return err
		}
	}
	if err = insertSubmissionEvent(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit()
}

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanTrackSubmission(row scanner) (*models.TrackSubmission, error) {
	var sub models.TrackSubmission
	var months, surfaces, transports string
	var approvedAt, reviewedAt sql.NullTime
	err := row.Scan(&sub.SubmissionID, &sub.TrackID, &sub.UserID, &sub.TrackType, &sub.Title, &sub.Description, &sub.Difficulty, &sub.RiskLevel, &months, &surfaces, &transports, &sub.TransportDescription, &sub.Status, &sub.Revision, &sub.SubmittedAt, &approvedAt, &reviewedAt, &sub.ReviewedBy, &sub.ReviewReason, &sub.CreatedAt, &sub.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(months), &sub.SuitableMonths)
	_ = json.Unmarshal([]byte(surfaces), &sub.SurfaceTypes)
	_ = json.Unmarshal([]byte(transports), &sub.TransportModes)
	if approvedAt.Valid {
		sub.ApprovedAt = &approvedAt.Time
	}
	if reviewedAt.Valid {
		sub.ReviewedAt = &reviewedAt.Time
	}
	return &sub, nil
}

const submissionColumns = `submission_id,track_id,user_id,track_type,title,description,difficulty,risk_level,suitable_months_json,surface_types_json,transport_modes_json,transport_description,status,revision,submitted_at,approved_at,reviewed_at,reviewed_by,review_reason,created_at,updated_at`

func (r *MySQLTrackSubmissionRepository) loadImages(ctx context.Context, sub *models.TrackSubmission) error {
	rows, err := r.db.QueryContext(ctx, `SELECT image_id,submission_id,oss_url,caption,sort_order,created_at,updated_at FROM track_submission_images WHERE submission_id=? ORDER BY sort_order,image_id`, sub.SubmissionID)
	if err != nil {
		return err
	}
	defer rows.Close()
	sub.Images = make([]*models.TrackSubmissionImage, 0)
	for rows.Next() {
		var image models.TrackSubmissionImage
		if err := rows.Scan(&image.ImageID, &image.SubmissionID, &image.OSSURL, &image.Caption, &image.SortOrder, &image.CreatedAt, &image.UpdatedAt); err != nil {
			return err
		}
		sub.Images = append(sub.Images, &image)
	}
	return rows.Err()
}

func (r *MySQLTrackSubmissionRepository) find(ctx context.Context, where string, arg interface{}) (*models.TrackSubmission, error) {
	sub, err := scanTrackSubmission(r.db.QueryRowContext(ctx, `SELECT `+submissionColumns+` FROM track_submissions WHERE `+where, arg))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := r.loadImages(ctx, sub); err != nil {
		return nil, err
	}
	return sub, nil
}

func (r *MySQLTrackSubmissionRepository) FindByTrackID(ctx context.Context, trackID string) (*models.TrackSubmission, error) {
	return r.find(ctx, "track_id=?", trackID)
}

func (r *MySQLTrackSubmissionRepository) FindBySubmissionID(ctx context.Context, submissionID string) (*models.TrackSubmission, error) {
	return r.find(ctx, "submission_id=?", submissionID)
}

func (r *MySQLTrackSubmissionRepository) ListByTrackIDs(ctx context.Context, trackIDs []string) (map[string]*models.TrackSubmission, error) {
	result := make(map[string]*models.TrackSubmission)
	if len(trackIDs) == 0 {
		return result, nil
	}
	args := make([]interface{}, len(trackIDs))
	for i := range trackIDs {
		args[i] = trackIDs[i]
	}
	query := `SELECT ` + submissionColumns + ` FROM track_submissions WHERE track_id IN (` + strings.TrimSuffix(strings.Repeat("?,", len(trackIDs)), ",") + `)`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	items := make([]*models.TrackSubmission, 0)
	for rows.Next() {
		sub, err := scanTrackSubmission(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, sub)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, sub := range items {
		if err := r.loadImages(ctx, sub); err != nil {
			return nil, err
		}
		result[sub.TrackID] = sub
	}
	return result, nil
}

func (r *MySQLTrackSubmissionRepository) List(ctx context.Context, filter models.TrackSubmissionListFilter) ([]*models.TrackSubmission, error) {
	query := `SELECT ` + submissionColumns + ` FROM track_submissions WHERE 1=1`
	args := make([]interface{}, 0)
	if filter.Status != "" {
		query += ` AND status=?`
		args = append(args, filter.Status)
	}
	if filter.Difficulty != "" {
		query += ` AND difficulty=?`
		args = append(args, filter.Difficulty)
	}
	if filter.RiskLevel != "" {
		query += ` AND risk_level=?`
		args = append(args, filter.RiskLevel)
	}
	if filter.TrackType != "" {
		query += ` AND track_type=?`
		args = append(args, filter.TrackType)
	}
	if filter.UserID > 0 {
		query += ` AND user_id=?`
		args = append(args, filter.UserID)
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query += ` ORDER BY submitted_at DESC,submission_id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	items := make([]*models.TrackSubmission, 0)
	for rows.Next() {
		sub, err := scanTrackSubmission(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, sub)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, sub := range items {
		if err := r.loadImages(ctx, sub); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (r *MySQLTrackSubmissionRepository) Review(ctx context.Context, submissionID string, expectedRevision int64, status models.TrackSubmissionStatus, reviewer, reason string, now time.Time, event *models.TrackSubmissionEvent) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var approved interface{}
	if status == models.TrackSubmissionStatusApproved {
		approved = now
	}
	res, err := tx.ExecContext(ctx, `UPDATE track_submissions SET status=?,approved_at=?,reviewed_at=?,reviewed_by=?,review_reason=?,updated_at=? WHERE submission_id=? AND revision=? AND status='pending'`, status, approved, now, reviewer, reason, now, submissionID, expectedRevision)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM track_submissions WHERE submission_id=?`, submissionID).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			return ErrNotFound
		}
		return ErrAlreadyExists
	}
	if err := insertSubmissionEvent(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *MySQLTrackSubmissionRepository) Withdraw(ctx context.Context, trackID string, userID int64, now time.Time, event *models.TrackSubmissionEvent) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE track_submissions SET status='withdrawn',approved_at=NULL,updated_at=? WHERE track_id=? AND user_id=? AND status IN ('pending','approved')`, now, trackID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var owner int64
		if err := tx.QueryRowContext(ctx, `SELECT user_id FROM track_submissions WHERE track_id=?`, trackID).Scan(&owner); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if owner != userID {
			return ErrForbidden
		}
		return ErrAlreadyExists
	}
	if err := insertSubmissionEvent(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *MySQLTrackSubmissionRepository) Invalidate(ctx context.Context, trackID, reason string, now time.Time, event *models.TrackSubmissionEvent) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE track_submissions SET status='invalidated',approved_at=NULL,review_reason=?,updated_at=? WHERE track_id=? AND status='approved'`, reason, now, trackID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		if err := insertSubmissionEvent(ctx, tx, event); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *MySQLTrackSubmissionRepository) ListEvents(ctx context.Context, submissionID string) ([]*models.TrackSubmissionEvent, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,submission_id,revision,event_type,from_status,to_status,operator_type,operator,reason,snapshot_json,created_at FROM track_submission_events WHERE submission_id=? ORDER BY id`, submissionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*models.TrackSubmissionEvent, 0)
	for rows.Next() {
		var event models.TrackSubmissionEvent
		if err := rows.Scan(&event.ID, &event.SubmissionID, &event.Revision, &event.EventType, &event.FromStatus, &event.ToStatus, &event.OperatorType, &event.Operator, &event.Reason, &event.SnapshotJSON, &event.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, &event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		if _, err := r.FindBySubmissionID(ctx, submissionID); err != nil {
			return nil, err
		}
	}
	return items, nil
}

var _ TrackSubmissionRepository = (*MySQLTrackSubmissionRepository)(nil)
