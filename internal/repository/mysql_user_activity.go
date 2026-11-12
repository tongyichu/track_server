package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

var _ UserActivityRepository = (*MySQLUserRepository)(nil)

const userActivityDaysSchema = `CREATE TABLE IF NOT EXISTS user_activity_days (
	user_id BIGINT NOT NULL,
	active_date DATE NOT NULL COMMENT '北京时间自然日',
	PRIMARY KEY (user_id, active_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户近七天活跃日期';`

func (r *MySQLUserRepository) TouchUserActivity(ctx context.Context, userID int64, at time.Time, appVersion string) error {
	first, day := userActivityDateWindow(at)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE users SET last_active_at=?,last_active_app_version=? WHERE id=? AND (last_active_at IS NULL OR last_active_at<?)`, at, appVersion, userID, at); err != nil {
		return err
	}
	// SELECT prevents activity rows from creating nonexistent users; the key deduplicates devices/retries.
	result, err := tx.ExecContext(ctx, `INSERT INTO user_activity_days (user_id, active_date)
		SELECT id, ? FROM users WHERE id=? ON DUPLICATE KEY UPDATE active_date=VALUES(active_date)`, day, userID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM user_activity_days WHERE user_id=? AND active_date<?`, userID, first); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *MySQLUserRepository) GetUserActivityStats(ctx context.Context, userIDs []int64, now time.Time) (map[int64]models.UserActivityStats, error) {
	result := make(map[int64]models.UserActivityStats, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}
	first, today := userActivityDateWindow(now)
	args := make([]interface{}, 0, len(userIDs)+2)
	args = append(args, first, today)
	for _, id := range userIDs {
		args = append(args, id)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(userIDs)), ",")
	rows, err := r.db.QueryContext(ctx, `SELECT u.id,u.last_active_at,u.last_active_app_version,COUNT(d.user_id)
		FROM users u LEFT JOIN user_activity_days d ON d.user_id=u.id AND d.active_date>=? AND d.active_date<=?
		WHERE u.id IN (`+placeholders+`) GROUP BY u.id,u.last_active_at,u.last_active_app_version`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var last sql.NullTime
		var stats models.UserActivityStats
		if err := rows.Scan(&id, &last, &stats.LastActiveAppVersion, &stats.ActiveDays7D); err != nil {
			return nil, err
		}
		if last.Valid && !last.Time.IsZero() {
			stats.LastActiveAt = &last.Time
		}
		result[id] = stats
	}
	return result, rows.Err()
}

func (r *MySQLUserRepository) GetUserLastActiveTimes(ctx context.Context, userIDs []int64) (map[int64]time.Time, error) {
	result := make(map[int64]time.Time, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}
	args := make([]interface{}, len(userIDs))
	for i, id := range userIDs {
		args[i] = id
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(userIDs)), ",")
	rows, err := r.db.QueryContext(ctx, `SELECT id, last_active_at FROM users WHERE id IN (`+placeholders+`) AND last_active_at IS NOT NULL`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var at time.Time
		if err := rows.Scan(&id, &at); err != nil {
			return nil, err
		}
		result[id] = at
	}
	return result, rows.Err()
}
