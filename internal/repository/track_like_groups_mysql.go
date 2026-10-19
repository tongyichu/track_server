package repository

import (
	"context"
	"database/sql"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
)

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
func (r *MySQLTrackLikeRepository) ListReceivedGroups(ctx context.Context, owner int64, cursor *models.ReceivedLikeTrackCursor, limit int) ([]*models.ReceivedLikeTrack, error) {
	query := `SELECT l.track_id,COUNT(*),MAX(l.created_at)` + receivedLikeWhere + ` GROUP BY l.track_id`
	args := []interface{}{owner}
	if cursor != nil {
		query += ` HAVING MAX(l.created_at)<? OR (MAX(l.created_at)=? AND l.track_id<?)`
		args = append(args, cursor.LastLikedAt, cursor.LastLikedAt, cursor.TrackID)
	}
	query += ` ORDER BY MAX(l.created_at) DESC,l.track_id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	groups := make([]*models.ReceivedLikeTrack, 0)
	for rows.Next() {
		g := &models.ReceivedLikeTrack{RecentLikes: make([]*models.TrackLike, 0, 3)}
		if err := rows.Scan(&g.TrackID, &g.LikeCount, &g.LastLikedAt); err != nil {
			rows.Close()
			return nil, err
		}
		groups = append(groups, g)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return groups, nil
	}
	// Bounded per-track index scans in one round trip; never materialize every liker of a popular track.
	parts := make([]string, 0, len(groups))
	args = nil
	byID := make(map[string]*models.ReceivedLikeTrack, len(groups))
	for _, g := range groups {
		parts = append(parts, `(SELECT user_id,track_id,created_at FROM track_likes WHERE track_id=? AND user_id<>? ORDER BY created_at DESC,user_id DESC LIMIT 3)`)
		args = append(args, g.TrackID, owner)
		byID[g.TrackID] = g
	}
	query = strings.Join(parts, " UNION ALL ")
	if len(parts) == 1 {
		query = strings.TrimSuffix(strings.TrimPrefix(query, "("), ")")
	}
	recent, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer recent.Close()
	for recent.Next() {
		like := &models.TrackLike{}
		if err := recent.Scan(&like.UserID, &like.TrackID, &like.CreatedAt); err != nil {
			return nil, err
		}
		byID[like.TrackID].RecentLikes = append(byID[like.TrackID].RecentLikes, like)
	}
	return groups, recent.Err()
}
func (r *MySQLTrackLikeRepository) CountReceivedGroups(ctx context.Context, owner int64) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT l.track_id)`+receivedLikeWhere, owner).Scan(&count)
	return count, err
}
func (r *MySQLTrackLikeRepository) ListReceivedTrackLikers(ctx context.Context, owner int64, track string, cursor *models.TrackLikeCursor, limit int) ([]*models.TrackLike, error) {
	query := `SELECT l.user_id,l.track_id,l.created_at` + receivedLikeWhere + ` AND l.track_id=?`
	args := []interface{}{owner, track}
	if cursor != nil {
		query += ` AND (l.created_at<? OR (l.created_at=? AND l.user_id<?))`
		args = append(args, cursor.CreatedAt, cursor.CreatedAt, cursor.UserID)
	}
	query += ` ORDER BY l.created_at DESC,l.user_id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]*models.TrackLike, 0)
	for rows.Next() {
		row := &models.TrackLike{}
		if err := rows.Scan(&row.UserID, &row.TrackID, &row.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
