package repository

import (
	"context"
	"time"
)

func (r *MySQLUserRepository) UpdateInternal(ctx context.Context, userID int64, isInternal bool) error {
	result, err := r.db.ExecContext(ctx, `UPDATE users SET is_internal=?, updated_at=? WHERE id=?`, isInternal, time.Now(), userID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		_, err = r.FindByID(ctx, userID)
		return err
	}
	return nil
}
