package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

type mysqlAdminDashboardRepository struct {
	db *sql.DB
}

func (r *mysqlAdminDashboardRepository) GetAdminDashboardStatistics(ctx context.Context, query models.AdminDashboardQuery) (*models.AdminDashboardStatistics, error) {
	statistics := newDashboardStatistics(query)
	if err := r.loadCoreMetrics(ctx, query, statistics); err != nil {
		return nil, err
	}
	if err := r.loadFunnel(ctx, query, statistics); err != nil {
		return nil, err
	}
	if err := r.loadReview(ctx, query, statistics); err != nil {
		return nil, err
	}
	if err := r.loadFeedback(ctx, query, statistics); err != nil {
		return nil, err
	}
	if err := r.loadContent(ctx, query, statistics); err != nil {
		return nil, err
	}
	if err := r.loadEngagement(ctx, query, statistics); err != nil {
		return nil, err
	}
	if err := r.loadCompanion(ctx, query, statistics); err != nil {
		return nil, err
	}
	if err := r.loadDaily(ctx, query, statistics); err != nil {
		return nil, err
	}
	return statistics, nil
}

func (r *mysqlAdminDashboardRepository) GetAdminCompanionStatistics(ctx context.Context, query models.AdminDashboardQuery) (*models.AdminDashboardCompanionStats, error) {
	result := &models.AdminDashboardStatistics{}
	if err := r.loadCompanion(ctx, query, result); err != nil {
		return nil, err
	}
	return &result.Companion, nil
}

func (r *mysqlAdminDashboardRepository) GetAdminUserDistributionStatistics(ctx context.Context) (*models.AdminUserDistributionStats, error) {
	var total, male, female int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*),
		COALESCE(SUM(gender='male'),0), COALESCE(SUM(gender='female'),0) FROM users`).Scan(&total, &male, &female); err != nil {
		return nil, fmt.Errorf("admin user distribution totals: %w", err)
	}
	genderCounts := map[string]int64{"male": male, "female": female, "unknown": total - male - female}

	birthdayRows, err := r.db.QueryContext(ctx, `SELECT SUBSTRING(birthday,6,5),COUNT(*) FROM users
		WHERE birthday REGEXP '^[0-9]{4}-[0-9]{2}-[0-9]{2}$'
		AND birthday >= '1900-01-01' AND birthday <= CURDATE()
		AND DATE_FORMAT(STR_TO_DATE(birthday,'%Y-%m-%d'),'%Y-%m-%d')=birthday
		GROUP BY SUBSTRING(birthday,6,5)`)
	if err != nil {
		return nil, fmt.Errorf("admin user zodiac distribution: %w", err)
	}
	birthdayCounts := make(map[string]int64)
	for birthdayRows.Next() {
		var monthDay string
		var count int64
		if err := birthdayRows.Scan(&monthDay, &count); err != nil {
			_ = birthdayRows.Close()
			return nil, err
		}
		birthdayCounts[monthDay] = count
	}
	if err := birthdayRows.Err(); err != nil {
		_ = birthdayRows.Close()
		return nil, err
	}
	_ = birthdayRows.Close()

	cityRows, err := r.db.QueryContext(ctx, `SELECT COALESCE(NULLIF(TRIM(city_code),''),'unknown') AS city_code,COUNT(DISTINCT user_id) AS user_count
		FROM track_records WHERE `+completedTrackSQL+`
		GROUP BY city_code
		ORDER BY user_count DESC, city_code`)
	if err != nil {
		return nil, fmt.Errorf("admin user track city distribution: %w", err)
	}
	cityCounts := make(map[string]int64)
	for cityRows.Next() {
		var city string
		var count int64
		if err := cityRows.Scan(&city, &count); err != nil {
			_ = cityRows.Close()
			return nil, err
		}
		cityCounts[city] = count
	}
	if err := cityRows.Err(); err != nil {
		_ = cityRows.Close()
		return nil, err
	}
	_ = cityRows.Close()
	return buildAdminUserDistributionStats(total, genderCounts, birthdayCounts, cityCounts), nil
}

func (r *mysqlAdminDashboardRepository) GetAdminUserLastLoginTimes(ctx context.Context, userIDs []int64) (map[int64]time.Time, error) {
	result := make(map[int64]time.Time, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(userIDs)), ",")
	args := make([]interface{}, len(userIDs))
	for i, userID := range userIDs {
		args[i] = userID
	}
	rows, err := r.db.QueryContext(ctx, `SELECT user_id, MAX(created_at) FROM login_log WHERE user_id IN (`+placeholders+`) GROUP BY user_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("admin user last login times: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var userID int64
		var lastLogin time.Time
		if err := rows.Scan(&userID, &lastLogin); err != nil {
			return nil, err
		}
		result[userID] = lastLogin
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

const completedTrackSQL = `deleted_at IS NULL AND status <> 0 AND is_running = 0 AND raw_track_url IS NOT NULL AND raw_track_url <> ''`
const completedTrackAliasSQL = `t.deleted_at IS NULL AND t.status <> 0 AND t.is_running = 0 AND t.raw_track_url IS NOT NULL AND t.raw_track_url <> ''`

func (r *mysqlAdminDashboardRepository) loadCoreMetrics(ctx context.Context, q models.AdminDashboardQuery, result *models.AdminDashboardStatistics) error {
	if err := r.db.QueryRowContext(ctx, `SELECT
		COUNT(DISTINCT CASE WHEN created_at >= ? AND created_at < ? THEN user_id END),
		COUNT(DISTINCT CASE WHEN created_at >= ? AND created_at < ? THEN user_id END)
		FROM login_log`, q.Start, q.End, q.PreviousStart, q.PreviousEnd).Scan(&result.Metrics.ActiveUsers.Current, &result.Metrics.ActiveUsers.Previous); err != nil {
		return fmt.Errorf("dashboard active users: %w", err)
	}
	if err := r.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(created_at >= ? AND created_at < ?),0),
		COALESCE(SUM(created_at >= ? AND created_at < ?),0)
		FROM users`, q.Start, q.End, q.PreviousStart, q.PreviousEnd).Scan(&result.Metrics.NewUsers.Current, &result.Metrics.NewUsers.Previous); err != nil {
		return fmt.Errorf("dashboard new users: %w", err)
	}
	if err := r.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(created_at >= ? AND created_at < ?),0),
		COALESCE(SUM(created_at >= ? AND created_at < ?),0),
		COUNT(DISTINCT CASE WHEN created_at >= ? AND created_at < ? THEN user_id END),
		COUNT(DISTINCT CASE WHEN created_at >= ? AND created_at < ? THEN user_id END)
		FROM track_records WHERE `+completedTrackSQL,
		q.Start, q.End, q.PreviousStart, q.PreviousEnd,
		q.Start, q.End, q.PreviousStart, q.PreviousEnd,
	).Scan(
		&result.Metrics.UploadedTracks.Current, &result.Metrics.UploadedTracks.Previous,
		&result.Metrics.UploadUsers.Current, &result.Metrics.UploadUsers.Previous,
	); err != nil {
		return fmt.Errorf("dashboard track metrics: %w", err)
	}
	if err := r.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(submitted_at >= ? AND submitted_at < ?),0),
		COALESCE(SUM(submitted_at >= ? AND submitted_at < ?),0),
		COALESCE(SUM(status = 'pending'),0)
		FROM track_submissions`, q.Start, q.End, q.PreviousStart, q.PreviousEnd).Scan(
		&result.Metrics.Submissions.Current, &result.Metrics.Submissions.Previous, &result.Metrics.PendingSubmissions,
	); err != nil {
		return fmt.Errorf("dashboard submission metrics: %w", err)
	}
	return nil
}

func (r *mysqlAdminDashboardRepository) loadFunnel(ctx context.Context, q models.AdminDashboardQuery, result *models.AdminDashboardStatistics) error {
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*),
		COALESCE(SUM(EXISTS(SELECT 1 FROM track_records t WHERE t.user_id=u.id AND t.created_at>=? AND t.created_at<? AND `+completedTrackAliasSQL+`)),0),
		COALESCE(SUM(EXISTS(SELECT 1 FROM track_records t JOIN track_submissions s ON s.track_id=t.id WHERE t.user_id=u.id AND t.created_at>=? AND t.created_at<? AND `+completedTrackAliasSQL+` AND s.submitted_at>=? AND s.submitted_at<?)),0),
		COALESCE(SUM(EXISTS(SELECT 1 FROM track_records t JOIN track_submissions s ON s.track_id=t.id WHERE t.user_id=u.id AND t.created_at>=? AND t.created_at<? AND `+completedTrackAliasSQL+` AND s.submitted_at>=? AND s.submitted_at<? AND s.status='approved' AND s.reviewed_at>=? AND s.reviewed_at<?)),0)
		FROM users u WHERE u.created_at>=? AND u.created_at<?`,
		q.Start, q.End,
		q.Start, q.End, q.Start, q.End,
		q.Start, q.End, q.Start, q.End, q.Start, q.End,
		q.Start, q.End,
	).Scan(&result.Funnel.Registered, &result.Funnel.Uploaded, &result.Funnel.Submitted, &result.Funnel.Approved)
	if err != nil {
		return fmt.Errorf("dashboard conversion funnel: %w", err)
	}
	return nil
}

func (r *mysqlAdminDashboardRepository) loadReview(ctx context.Context, q models.AdminDashboardQuery, result *models.AdminDashboardStatistics) error {
	var average sql.NullFloat64
	var oldest sql.NullTime
	err := r.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(status='pending'),0),
		COALESCE(SUM(status='pending' AND submitted_at<?),0),
		COALESCE(SUM(status='pending' AND submitted_at<?),0),
		COALESCE(SUM(status='approved' AND reviewed_at>=? AND reviewed_at<?),0),
		COALESCE(SUM(status='rejected' AND reviewed_at>=? AND reviewed_at<?),0),
		AVG(CASE WHEN status IN ('approved','rejected') AND reviewed_at>=? AND reviewed_at<? THEN TIMESTAMPDIFF(SECOND,submitted_at,reviewed_at)/3600 END),
		MIN(CASE WHEN status='pending' THEN submitted_at END)
		FROM track_submissions`,
		q.Now.Add(-24*time.Hour), q.Now.Add(-72*time.Hour),
		q.Start, q.End, q.Start, q.End, q.Start, q.End,
	).Scan(
		&result.Review.Pending, &result.Review.PendingOver24H, &result.Review.PendingOver72H,
		&result.Review.Approved, &result.Review.Rejected, &average, &oldest,
	)
	if err != nil {
		return fmt.Errorf("dashboard review metrics: %w", err)
	}
	if average.Valid {
		result.Review.AvgReviewHours = average.Float64
	}
	if oldest.Valid {
		result.Review.OldestPending = oldest.Time.Format(time.RFC3339)
	}
	reviewed := result.Review.Approved + result.Review.Rejected
	if reviewed > 0 {
		result.Review.ApprovalRate = float64(result.Review.Approved) / float64(reviewed)
	}
	result.Metrics.PendingSubmissions = result.Review.Pending
	return nil
}

func (r *mysqlAdminDashboardRepository) loadFeedback(ctx context.Context, q models.AdminDashboardQuery, result *models.AdminDashboardStatistics) error {
	var average sql.NullFloat64
	err := r.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(status='pending'),0), COALESCE(SUM(status='processing'),0), COALESCE(SUM(status='resolved'),0), COALESCE(SUM(status='ignored'),0),
		COALESCE(SUM(status IN ('pending','processing') AND created_at<?),0),
		COALESCE(SUM(status IN ('pending','processing') AND created_at<?),0),
		COALESCE(SUM(created_at>=? AND created_at<?),0), COALESCE(SUM(created_at>=? AND created_at<?),0),
		COALESCE(SUM(status='resolved' AND updated_at>=? AND updated_at<?),0),
		AVG(CASE WHEN status='resolved' AND updated_at>=? AND updated_at<? THEN TIMESTAMPDIFF(SECOND,created_at,updated_at)/3600 END)
		FROM user_feedbacks`,
		q.Now.Add(-24*time.Hour), q.Now.Add(-72*time.Hour),
		q.Start, q.End, q.PreviousStart, q.PreviousEnd,
		q.Start, q.End, q.Start, q.End,
	).Scan(
		&result.Feedback.Pending, &result.Feedback.Processing, &result.Feedback.Resolved, &result.Feedback.Ignored,
		&result.Feedback.PendingOver24H, &result.Feedback.PendingOver72H,
		&result.Feedback.Created.Current, &result.Feedback.Created.Previous,
		&result.Feedback.ResolvedInPeriod, &average,
	)
	if err != nil {
		return fmt.Errorf("dashboard feedback metrics: %w", err)
	}
	if average.Valid {
		result.Feedback.AvgResolutionHours = average.Float64
	}
	result.Metrics.PendingFeedback = result.Feedback.Pending + result.Feedback.Processing
	platforms, err := r.queryDistribution(ctx, `SELECT COALESCE(NULLIF(TRIM(platform),''),'unknown'),COUNT(*) FROM user_feedbacks WHERE created_at>=? AND created_at<? GROUP BY 1 ORDER BY 2 DESC LIMIT 6`, q.Start, q.End)
	if err != nil {
		return fmt.Errorf("dashboard feedback platforms: %w", err)
	}
	versions, err := r.queryDistribution(ctx, `SELECT COALESCE(NULLIF(TRIM(app_version),''),'unknown'),COUNT(*) FROM user_feedbacks WHERE created_at>=? AND created_at<? GROUP BY 1 ORDER BY 2 DESC LIMIT 8`, q.Start, q.End)
	if err != nil {
		return fmt.Errorf("dashboard feedback versions: %w", err)
	}
	result.Feedback.Platforms = platforms
	result.Feedback.Versions = versions
	return nil
}

func (r *mysqlAdminDashboardRepository) loadContent(ctx context.Context, q models.AdminDashboardQuery, result *models.AdminDashboardStatistics) error {
	statuses, err := r.queryDistribution(ctx, `SELECT CASE status WHEN 1 THEN 'normal' WHEN 2 THEN 'private' ELSE 'deleted' END,COUNT(*) FROM track_records WHERE created_at>=? AND created_at<? AND is_running=0 AND raw_track_url IS NOT NULL AND raw_track_url<>'' GROUP BY 1 ORDER BY 2 DESC`, q.Start, q.End)
	if err != nil {
		return fmt.Errorf("dashboard track statuses: %w", err)
	}
	types, err := r.queryDistribution(ctx, `SELECT COALESCE(NULLIF(TRIM(track_type),''),'unknown'),COUNT(*) FROM track_records WHERE created_at>=? AND created_at<? AND `+completedTrackSQL+` GROUP BY 1 ORDER BY 2 DESC LIMIT 8`, q.Start, q.End)
	if err != nil {
		return fmt.Errorf("dashboard track types: %w", err)
	}
	cities, err := r.queryDistribution(ctx, `SELECT COALESCE(NULLIF(TRIM(city_code),''),'unknown'),COUNT(*) FROM track_records WHERE created_at>=? AND created_at<? AND `+completedTrackSQL+` GROUP BY 1 ORDER BY 2 DESC LIMIT 10`, q.Start, q.End)
	if err != nil {
		return fmt.Errorf("dashboard track cities: %w", err)
	}
	result.Content.Statuses, result.Content.Types, result.Content.Cities = statuses, types, cities
	var distance, duration sql.NullFloat64
	if err := r.db.QueryRowContext(ctx, `SELECT
		AVG(CASE WHEN created_at>=? AND created_at<? AND `+completedTrackSQL+` THEN distance END),
		AVG(CASE WHEN created_at>=? AND created_at<? AND `+completedTrackSQL+` THEN duration END),
		COALESCE(SUM(deleted_at IS NULL AND status<>0 AND is_running=1 AND created_at<?),0),
		COALESCE(SUM(deleted_at IS NULL AND status<>0 AND is_running=0 AND raw_track_url IS NOT NULL AND raw_track_url<>'' AND distance<=0),0)
		FROM track_records`, q.Start, q.End, q.Start, q.End, q.Now.Add(-24*time.Hour)).Scan(
		&distance, &duration, &result.Content.StaleRunning, &result.Content.ZeroDistance,
	); err != nil {
		return fmt.Errorf("dashboard track quality: %w", err)
	}
	if distance.Valid {
		result.Content.AvgDistanceM = distance.Float64
	}
	if duration.Valid {
		result.Content.AvgDurationS = duration.Float64
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(status='failed'),0),COALESCE(SUM(status IN ('pending','processing')),0) FROM track_map_index_jobs`).Scan(&result.Content.FailedMapJobs, &result.Content.PendingMapJobs); err != nil {
		return fmt.Errorf("dashboard map jobs: %w", err)
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM track_route_groups WHERE status='active'`).Scan(&result.Content.ActiveRouteGroups); err != nil {
		return fmt.Errorf("dashboard route groups: %w", err)
	}
	return nil
}

func (r *mysqlAdminDashboardRepository) loadEngagement(ctx context.Context, q models.AdminDashboardQuery, result *models.AdminDashboardStatistics) error {
	rows, err := r.db.QueryContext(ctx, `
		SELECT 'collection',COALESCE(SUM(created_at>=? AND created_at<?),0),COALESCE(SUM(created_at>=? AND created_at<?),0) FROM track_collects
		UNION ALL SELECT 'navigation',COALESCE(SUM(created_at>=? AND created_at<?),0),COALESCE(SUM(created_at>=? AND created_at<?),0) FROM track_navigations
		UNION ALL SELECT 'like',COALESCE(SUM(created_at>=? AND created_at<?),0),COALESCE(SUM(created_at>=? AND created_at<?),0) FROM track_likes
		UNION ALL SELECT 'comment',COALESCE(SUM(created_at>=? AND created_at<? AND deleted=0),0),COALESCE(SUM(created_at>=? AND created_at<? AND deleted=0),0) FROM track_comments`,
		q.Start, q.End, q.PreviousStart, q.PreviousEnd,
		q.Start, q.End, q.PreviousStart, q.PreviousEnd,
		q.Start, q.End, q.PreviousStart, q.PreviousEnd,
		q.Start, q.End, q.PreviousStart, q.PreviousEnd,
	)
	if err != nil {
		return fmt.Errorf("dashboard engagement metrics: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var current, previous int64
		if err := rows.Scan(&kind, &current, &previous); err != nil {
			return err
		}
		switch kind {
		case "collection":
			result.Engagement.Current.Collections, result.Engagement.Previous.Collections = current, previous
		case "navigation":
			result.Engagement.Current.Navigations, result.Engagement.Previous.Navigations = current, previous
		case "like":
			result.Engagement.Current.Likes, result.Engagement.Previous.Likes = current, previous
		case "comment":
			result.Engagement.Current.Comments, result.Engagement.Previous.Comments = current, previous
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	topRows, err := r.db.QueryContext(ctx, `SELECT t.id,t.title,
		COALESCE(c.cnt,0),COALESCE(n.cnt,0),COALESCE(l.cnt,0),COALESCE(m.cnt,0),
		COALESCE(c.cnt,0)+COALESCE(n.cnt,0)+COALESCE(l.cnt,0)+COALESCE(m.cnt,0) score
		FROM track_records t
		LEFT JOIN (SELECT track_id,COUNT(*) cnt FROM track_collects GROUP BY track_id) c ON c.track_id=t.id
		LEFT JOIN (SELECT track_id,COUNT(*) cnt FROM track_navigations GROUP BY track_id) n ON n.track_id=t.id
		LEFT JOIN (SELECT track_id,COUNT(*) cnt FROM track_likes GROUP BY track_id) l ON l.track_id=t.id
		LEFT JOIN (SELECT track_id,COUNT(*) cnt FROM track_comments WHERE deleted=0 GROUP BY track_id) m ON m.track_id=t.id
		WHERE t.deleted_at IS NULL AND t.status=1 AND t.is_running=0
		ORDER BY score DESC,t.id LIMIT 10`)
	if err != nil {
		return fmt.Errorf("dashboard top tracks: %w", err)
	}
	defer topRows.Close()
	for topRows.Next() {
		var item models.AdminDashboardTopTrack
		if err := topRows.Scan(&item.TrackID, &item.Title, &item.Collections, &item.Navigations, &item.Likes, &item.Comments, &item.Score); err != nil {
			return err
		}
		result.Engagement.TopTracks = append(result.Engagement.TopTracks, item)
	}
	return topRows.Err()
}

func (r *mysqlAdminDashboardRepository) loadCompanion(ctx context.Context, q models.AdminDashboardQuery, result *models.AdminDashboardStatistics) error {
	var participants sql.NullFloat64
	err := r.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(started_at>=? AND started_at<?),0),COALESCE(SUM(started_at>=? AND started_at<?),0),
		COALESCE(SUM(status='active'),0),
		COALESCE(SUM(status='ended' AND ended_at>=? AND ended_at<?),0),
		COALESCE(SUM(status='ended' AND ended_at>=? AND ended_at<? AND end_source='auto_close'),0),
		AVG(CASE WHEN started_at>=? AND started_at<? THEN actual_participant_count END)
		FROM companion_sessions`,
		q.Start, q.End, q.PreviousStart, q.PreviousEnd,
		q.Start, q.End, q.Start, q.End, q.Start, q.End,
	).Scan(
		&result.Companion.Created.Current, &result.Companion.Created.Previous,
		&result.Companion.Active, &result.Companion.Ended, &result.Companion.AutoEnded, &participants,
	)
	if err != nil {
		return fmt.Errorf("dashboard companion metrics: %w", err)
	}
	if participants.Valid {
		result.Companion.AvgParticipants = participants.Float64
	}
	result.Metrics.ActiveCompanions = result.Companion.Active
	return nil
}

func (r *mysqlAdminDashboardRepository) loadDaily(ctx context.Context, q models.AdminDashboardQuery, result *models.AdminDashboardStatistics) error {
	rows, err := r.db.QueryContext(ctx, `
		SELECT 'new_user',DATE_FORMAT(created_at,'%Y-%m-%d'),COUNT(*) FROM users WHERE created_at>=? AND created_at<? GROUP BY 2
		UNION ALL SELECT 'active_user',DATE_FORMAT(created_at,'%Y-%m-%d'),COUNT(DISTINCT user_id) FROM login_log WHERE created_at>=? AND created_at<? GROUP BY 2
		UNION ALL SELECT 'track',DATE_FORMAT(created_at,'%Y-%m-%d'),COUNT(*) FROM track_records WHERE created_at>=? AND created_at<? AND `+completedTrackSQL+` GROUP BY 2
		UNION ALL SELECT 'collection',DATE_FORMAT(created_at,'%Y-%m-%d'),COUNT(*) FROM track_collects WHERE created_at>=? AND created_at<? GROUP BY 2
		UNION ALL SELECT 'navigation',DATE_FORMAT(created_at,'%Y-%m-%d'),COUNT(*) FROM track_navigations WHERE created_at>=? AND created_at<? GROUP BY 2
		UNION ALL SELECT 'like',DATE_FORMAT(created_at,'%Y-%m-%d'),COUNT(*) FROM track_likes WHERE created_at>=? AND created_at<? GROUP BY 2
		UNION ALL SELECT 'comment',DATE_FORMAT(created_at,'%Y-%m-%d'),COUNT(*) FROM track_comments WHERE created_at>=? AND created_at<? AND deleted=0 GROUP BY 2
		UNION ALL SELECT 'companion',DATE_FORMAT(started_at,'%Y-%m-%d'),COUNT(*) FROM companion_sessions WHERE started_at>=? AND started_at<? GROUP BY 2`,
		q.TrendStart, q.TrendEnd, q.TrendStart, q.TrendEnd, q.TrendStart, q.TrendEnd,
		q.TrendStart, q.TrendEnd, q.TrendStart, q.TrendEnd, q.TrendStart, q.TrendEnd,
		q.TrendStart, q.TrendEnd, q.TrendStart, q.TrendEnd,
	)
	if err != nil {
		return fmt.Errorf("dashboard daily metrics: %w", err)
	}
	defer rows.Close()
	index := dashboardDailyIndex(result.Trend)
	for rows.Next() {
		var kind, day string
		var count int64
		if err := rows.Scan(&kind, &day, &count); err != nil {
			return err
		}
		position, ok := index[day]
		if !ok {
			continue
		}
		item := &result.Trend[position]
		switch kind {
		case "new_user":
			item.NewUsers = count
		case "active_user":
			item.ActiveUsers = count
		case "track":
			item.UploadedTracks = count
		case "collection":
			item.Collections = count
		case "navigation":
			item.Navigations = count
		case "like":
			item.Likes = count
		case "comment":
			item.Comments = count
		case "companion":
			item.CompanionSessions = count
		}
	}
	return rows.Err()
}

func (r *mysqlAdminDashboardRepository) queryDistribution(ctx context.Context, query string, args ...interface{}) ([]models.AdminDashboardDistributionItem, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]models.AdminDashboardDistributionItem, 0)
	for rows.Next() {
		var item models.AdminDashboardDistributionItem
		if err := rows.Scan(&item.Key, &item.Count); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
