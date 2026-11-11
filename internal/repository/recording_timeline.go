package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/tongyichu/track_server/internal/models"
)

func ensureMySQLRecordingTimelineColumn(ctx context.Context, db *sql.DB) error {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='track_records' AND COLUMN_NAME='recording_timeline'`).Scan(&count); err != nil {
		return fmt.Errorf("check track_records.recording_timeline: %w", err)
	}
	if count == 0 {
		if _, err := db.ExecContext(ctx, `ALTER TABLE track_records ADD COLUMN recording_timeline JSON NULL`); err != nil {
			return fmt.Errorf("add track_records.recording_timeline: %w", err)
		}
	}
	return nil
}

func encodeRecordingTimeline(timeline *models.RecordingTimeline) any {
	if timeline == nil {
		return nil
	}
	value, _ := json.Marshal(timeline)
	return string(value)
}

func decodeRecordingTimeline(value sql.NullString, destination **models.RecordingTimeline) error {
	if !value.Valid || value.String == "" || value.String == "null" {
		return nil
	}
	return json.Unmarshal([]byte(value.String), destination)
}
