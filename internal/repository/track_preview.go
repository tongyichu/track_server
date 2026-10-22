package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/tongyichu/track_server/internal/models"
)

func ensureMySQLTrackPreviewImagesColumn(ctx context.Context, db *sql.DB) error {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'track_records' AND COLUMN_NAME = 'preview_images'`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := db.ExecContext(ctx, `ALTER TABLE track_records ADD COLUMN preview_images JSON NULL COMMENT '四种尺寸与主题的静态轨迹预览' AFTER track_no_map_bg_screenshot_url`)
	return err
}

func encodeTrackPreviewImages(images models.TrackPreviewImages) string {
	data, _ := json.Marshal(images)
	return string(data)
}

func decodeTrackPreviewImages(data sql.NullString, images *models.TrackPreviewImages) error {
	if !data.Valid || data.String == "" || data.String == "null" {
		return nil
	}
	if err := json.Unmarshal([]byte(data.String), images); err != nil {
		return fmt.Errorf("decode track preview images: %w", err)
	}
	return nil
}
