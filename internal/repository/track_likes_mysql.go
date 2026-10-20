package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

const trackLikesSchema = `CREATE TABLE IF NOT EXISTS track_likes (
 user_id BIGINT NOT NULL,
 track_id VARCHAR(64) NOT NULL,
 created_at DATETIME(6) NOT NULL,
 PRIMARY KEY (user_id, track_id),
 KEY idx_track_likes_track (track_id),
 KEY idx_track_likes_track_time (track_id, created_at, user_id),
 KEY idx_track_likes_time (created_at, track_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`

type MySQLTrackLikeRepository struct{ db *sql.DB }

func NewMySQLTrackLikeRepository(db *sql.DB) *MySQLTrackLikeRepository {
	return &MySQLTrackLikeRepository{db: db}
}

func (r *MySQLTrackLikeRepository) SetLike(ctx context.Context, userID int64, trackID string, liked bool) error {
	if !liked {
		_, err := r.db.ExecContext(ctx, `DELETE FROM track_likes WHERE user_id=? AND track_id=?`, userID, trackID)
		return err
	}
	_, err := r.db.ExecContext(ctx, `INSERT IGNORE INTO track_likes(user_id,track_id,created_at) VALUES(?,?,?)`, userID, trackID, time.Now().UTC())
	return err
}

func (r *MySQLTrackLikeRepository) Stats(ctx context.Context, userID int64, ids []string) (map[string]models.TrackLikeStats, error) {
	result := make(map[string]models.TrackLikeStats)
	if len(ids) == 0 {
		return result, nil
	}
	args := []interface{}{userID}
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx, `SELECT track_id, COUNT(*), MAX(user_id=?) FROM track_likes WHERE track_id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")+`) GROUP BY track_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var s models.TrackLikeStats
		if err := rows.Scan(&id, &s.LikeCount, &s.Liked); err != nil {
			return nil, err
		}
		result[id] = s
	}
	return result, rows.Err()
}

const receivedLikeWhere = ` FROM track_likes l JOIN track_records t ON t.id=l.track_id WHERE t.user_id=? AND l.user_id<>t.user_id AND t.status=1 AND t.is_running=0 AND TRIM(COALESCE(t.raw_track_url,''))<>''`

func (r *MySQLTrackLikeRepository) CountReceived(ctx context.Context, ownerID int64) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*)`+receivedLikeWhere, ownerID).Scan(&count)
	return count, err
}

func ensureMySQLTrackLikeIndex(ctx context.Context, db *sql.DB) error {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='track_likes' AND INDEX_NAME='idx_track_likes_track_time'`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := db.ExecContext(ctx, `ALTER TABLE track_likes ADD INDEX idx_track_likes_track_time(track_id,created_at,user_id)`)
	return err
}
