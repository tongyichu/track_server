package repository

import (
	"context"
	"database/sql"
	"fmt"
)

func ensureMySQLTrackStepCountColumn(ctx context.Context, db *sql.DB) error {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.COLUMNS
        WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='track_records' AND COLUMN_NAME='step_count'`).Scan(&count); err != nil {
		return fmt.Errorf("check track_records.step_count: %w", err)
	}
	if count > 0 {
		return nil
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE track_records ADD COLUMN step_count BIGINT NULL DEFAULT NULL AFTER elevation_gain`); err != nil {
		return fmt.Errorf("add track_records.step_count: %w", err)
	}
	return nil
}
