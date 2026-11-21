package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

const accountDeletionSchema = `CREATE TABLE IF NOT EXISTS user_account_deletions (
 user_id BIGINT NOT NULL PRIMARY KEY,
 requested_at DATETIME(6) NOT NULL,
 next_attempt_at DATETIME(6) NOT NULL,
 objects_after DATETIME(6) NOT NULL,
 attempt BIGINT NOT NULL DEFAULT 0,
 assets JSON DEFAULT NULL,
 completed_at DATETIME(6) DEFAULT NULL,
 KEY idx_user_account_deletions_due (completed_at, next_attempt_at, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

type MySQLAccountDeletionStore struct{ db *sql.DB }

type AccountDeletionJob struct {
	UserID       int64
	Attempt      int64
	ObjectsAfter time.Time
}

type AccountDeletionAssets struct {
	Objects            []string
	Tracks             []AccountDeletionTrackAssets
	CompanionCovers    []AccountDeletionCompanionCover
	SubmissionImageIDs []string
	CommentImageIDs    []string
}

type AccountDeletionTrackAssets struct {
	TrackID string
	Preview map[string]string
}

type AccountDeletionCompanionCover struct {
	SessionID string
	Version   string
}

func NewMySQLAccountDeletionStore(ctx context.Context, users UserRepository) (*MySQLAccountDeletionStore, error) {
	userRepo, ok := users.(*MySQLUserRepository)
	if !ok {
		return nil, nil
	}
	store := &MySQLAccountDeletionStore{db: userRepo.db}
	if _, err := userRepo.db.ExecContext(ctx, accountDeletionSchema); err != nil {
		if _, lookupErr := store.Blocked(ctx, 0); lookupErr != nil {
			return store, err
		}
	}
	return store, nil
}

func (r *MySQLAccountDeletionStore) Blocked(ctx context.Context, userID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_account_deletions WHERE user_id=?)`, userID).Scan(&exists)
	return exists, err
}

func (r *MySQLAccountDeletionStore) ActiveTokenVersion(ctx context.Context, userID int64) (int64, error) {
	var version int64
	err := r.db.QueryRowContext(ctx, `SELECT u.token_version FROM users u WHERE u.id=?
		AND NOT EXISTS(SELECT 1 FROM user_account_deletions d WHERE d.user_id=u.id)`, userID).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return version, err
}

func (r *MySQLAccountDeletionStore) HasWechatLogin(ctx context.Context, userID int64) bool {
	var exists bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM login_log WHERE user_id=? AND login_type='wechat')`, userID).Scan(&exists)
	return err == nil && exists
}

func (r *MySQLAccountDeletionStore) Request(ctx context.Context, userID int64, now, firstAttempt, objectsAfter time.Time) error {
	result, err := r.db.ExecContext(ctx, `INSERT INTO user_account_deletions (user_id,requested_at,next_attempt_at,objects_after)
		SELECT id,?,?,? FROM users WHERE id=? ON DUPLICATE KEY UPDATE attempt=user_account_deletions.attempt`, now, firstAttempt, objectsAfter, userID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected > 0 {
		return err
	}
	blocked, err := r.Blocked(ctx, userID)
	if err == nil && !blocked {
		return ErrNotFound
	}
	return err
}

func (r *MySQLAccountDeletionStore) Claim(ctx context.Context, now time.Time, lease time.Duration) (*AccountDeletionJob, error) {
	var job AccountDeletionJob
	err := r.db.QueryRowContext(ctx, `SELECT user_id,attempt,objects_after FROM user_account_deletions
		WHERE completed_at IS NULL AND next_attempt_at<=? ORDER BY next_attempt_at,user_id LIMIT 1`, now).Scan(&job.UserID, &job.Attempt, &job.ObjectsAfter)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result, err := r.db.ExecContext(ctx, `UPDATE user_account_deletions SET attempt=attempt+1,next_attempt_at=?
		WHERE user_id=? AND attempt=? AND completed_at IS NULL AND next_attempt_at<=?`, now.Add(lease), job.UserID, job.Attempt, now)
	if err != nil {
		return nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected == 0 {
		return nil, err
	}
	job.Attempt++
	return &job, nil
}

func (r *MySQLAccountDeletionStore) Finish(ctx context.Context, job AccountDeletionJob, now time.Time) error {
	result, err := r.db.ExecContext(ctx, `UPDATE user_account_deletions SET completed_at=?,assets=NULL
		WHERE user_id=? AND attempt=? AND assets IS NOT NULL AND completed_at IS NULL`, now, job.UserID, job.Attempt)
	return accountDeletionUpdated(result, err)
}

func (r *MySQLAccountDeletionStore) WaitForObjectCredentials(ctx context.Context, job AccountDeletionJob) error {
	result, err := r.db.ExecContext(ctx, `UPDATE user_account_deletions SET next_attempt_at=objects_after
		WHERE user_id=? AND attempt=? AND assets IS NOT NULL AND completed_at IS NULL`, job.UserID, job.Attempt)
	return accountDeletionUpdated(result, err)
}

func (r *MySQLAccountDeletionStore) SaveAssets(ctx context.Context, job AccountDeletionJob, assets AccountDeletionAssets) error {
	payload, err := json.Marshal(assets)
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `UPDATE user_account_deletions SET assets=?
		WHERE user_id=? AND attempt=? AND completed_at IS NULL`, string(payload), job.UserID, job.Attempt)
	return accountDeletionUpdated(result, err)
}

func accountDeletionUpdated(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrConflict
	}
	return nil
}

func (r *MySQLAccountDeletionStore) PurgeData(ctx context.Context, job AccountDeletionJob, objectKey func(string) string) (AccountDeletionAssets, error) {
	var assets AccountDeletionAssets
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return assets, err
	}
	defer tx.Rollback()
	var snapshot []byte
	if err := tx.QueryRowContext(ctx, `SELECT assets FROM user_account_deletions WHERE user_id=? AND attempt=?
		AND completed_at IS NULL FOR UPDATE`, job.UserID, job.Attempt).Scan(&snapshot); err != nil {
		return assets, err
	}
	if snapshot != nil {
		err := json.Unmarshal(snapshot, &assets)
		return assets, err
	}
	assets, err = accountDeletionAssets(ctx, tx, job.UserID, objectKey)
	if err != nil {
		return assets, err
	}
	if err := purgeAccountData(ctx, tx, job.UserID); err != nil {
		return assets, err
	}
	payload, err := json.Marshal(assets)
	if err != nil {
		return assets, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE user_account_deletions SET assets=? WHERE user_id=?`, string(payload), job.UserID); err != nil {
		return assets, err
	}
	return assets, tx.Commit()
}
