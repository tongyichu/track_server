package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

type MySQLRecommendationRepository struct{ db *sql.DB }

func NewMySQLRecommendationRepository(db *sql.DB) *MySQLRecommendationRepository {
	return &MySQLRecommendationRepository{db: db}
}

func (r *MySQLRecommendationRepository) SaveFeedSession(ctx context.Context, session *models.RecommendationFeedSession) error {
	if err := validateRecommendationFeedSession(session); err != nil {
		return err
	}
	itemsJSON, err := json.Marshal(session.Items)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO recommend_feed_sessions (request_id,user_id,city_code,strategy,items_json,created_at,expires_at)
		 VALUES (?,?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE user_id=VALUES(user_id),city_code=VALUES(city_code),strategy=VALUES(strategy),items_json=VALUES(items_json),created_at=VALUES(created_at),expires_at=VALUES(expires_at)`,
		session.RequestID, session.UserID, session.CityCode, session.Strategy, itemsJSON, session.CreatedAt, session.ExpiresAt,
	)
	return err
}

func (r *MySQLRecommendationRepository) GetFeedSession(ctx context.Context, requestID string) (*models.RecommendationFeedSession, error) {
	session := &models.RecommendationFeedSession{}
	var itemsJSON []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT request_id,user_id,city_code,strategy,items_json,created_at,expires_at FROM recommend_feed_sessions WHERE request_id=?`, requestID,
	).Scan(&session.RequestID, &session.UserID, &session.CityCode, &session.Strategy, &itemsJSON, &session.CreatedAt, &session.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(itemsJSON, &session.Items); err != nil {
		return nil, err
	}
	return session, nil
}

func (r *MySQLRecommendationRepository) DeleteExpiredFeedSessions(ctx context.Context, now time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = 1000
	}
	result, err := r.db.ExecContext(ctx, `DELETE FROM recommend_feed_sessions WHERE expires_at<=? LIMIT ?`, now, limit)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *MySQLRecommendationRepository) GetUserProfile(ctx context.Context, userID int64) (*models.RecommendationUserProfile, error) {
	var raw []byte
	err := r.db.QueryRowContext(ctx, `SELECT profile_json FROM recommend_user_profiles WHERE user_id=?`, userID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	profile := &models.RecommendationUserProfile{}
	if err := json.Unmarshal(raw, profile); err != nil {
		return nil, err
	}
	return profile, nil
}

func (r *MySQLRecommendationRepository) UpsertUserProfiles(ctx context.Context, profiles []*models.RecommendationUserProfile) error {
	if len(profiles) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO recommend_user_profiles (user_id,profile_json,data_through,generated_at) VALUES (?,?,?,?)
		 ON DUPLICATE KEY UPDATE profile_json=VALUES(profile_json),data_through=VALUES(data_through),generated_at=VALUES(generated_at)`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, profile := range profiles {
		if profile == nil {
			continue
		}
		raw, err := json.Marshal(profile)
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx, profile.UserID, raw, profile.DataThrough, profile.GeneratedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *MySQLRecommendationRepository) ListItemStats(ctx context.Context, trackIDs []string) (map[string]*models.RecommendationItemStats, error) {
	ids := uniqueNonEmptyStrings(trackIDs)
	result := make(map[string]*models.RecommendationItemStats, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	query := fmt.Sprintf(`SELECT s.track_id,s.stat_date,s.collect_count,s.navigate_count,s.impression_count,s.click_count,s.detail_view_count,s.hot_score,s.data_through,s.generated_at,s.updated_at
		FROM recommend_item_stats_daily s
		JOIN (SELECT track_id,MAX(stat_date) AS stat_date FROM recommend_item_stats_daily WHERE track_id IN (%s) GROUP BY track_id) latest
		ON latest.track_id=s.track_id AND latest.stat_date=s.stat_date`, placeholders)
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		stat := &models.RecommendationItemStats{}
		if err := rows.Scan(&stat.TrackID, &stat.StatDate, &stat.CollectCount, &stat.NavigateCount, &stat.ImpressionCount, &stat.ClickCount, &stat.DetailViewCount, &stat.HotScore, &stat.DataThrough, &stat.GeneratedAt, &stat.UpdatedAt); err != nil {
			return nil, err
		}
		result[stat.TrackID] = stat
	}
	return result, rows.Err()
}

func (r *MySQLRecommendationRepository) UpsertItemStats(ctx context.Context, stats []*models.RecommendationItemStats) error {
	if len(stats) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO recommend_item_stats_daily (track_id,stat_date,collect_count,navigate_count,impression_count,click_count,detail_view_count,hot_score,data_through,generated_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE collect_count=VALUES(collect_count),navigate_count=VALUES(navigate_count),impression_count=VALUES(impression_count),click_count=VALUES(click_count),detail_view_count=VALUES(detail_view_count),hot_score=VALUES(hot_score),data_through=VALUES(data_through),generated_at=VALUES(generated_at),updated_at=VALUES(updated_at)`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, stat := range stats {
		if stat == nil || stat.TrackID == "" {
			continue
		}
		if _, err := stmt.ExecContext(ctx, stat.TrackID, stat.StatDate, stat.CollectCount, stat.NavigateCount, stat.ImpressionCount, stat.ClickCount, stat.DetailViewCount, stat.HotScore, stat.DataThrough, stat.GeneratedAt, stat.UpdatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
