package repository

import (
	"context"
	"strings"
	"time"
)

var _ UserActivityRepository = (*MySQLUserRepository)(nil)

func (r *MySQLUserRepository) TouchUserActivity(ctx context.Context, userID int64, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET last_active_at=? WHERE id=? AND (last_active_at IS NULL OR last_active_at<?)`, at, userID, at)
	return err
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
