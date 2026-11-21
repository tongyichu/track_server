package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

func workbenchSQL(ctx context.Context, db *sql.DB, query string, args []any, visit func(*sql.Rows) error) error {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := visit(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (r *mysqlAdminDashboardRepository) WorkbenchCities(ctx context.Context, q models.AdminDashboardQuery) ([]models.WorkbenchCity, error) {
	result := newWorkbenchCities(q)
	db := r.db
	err := workbenchSQL(ctx, db, "SELECT id,city_code,created_at FROM track_records WHERE status=1 AND "+completedTrackSQL+" AND city_code<>''", nil, func(row *sql.Rows) error {
		var id, code string
		var created time.Time
		if err := row.Scan(&id, &code, &created); err != nil {
			return err
		}
		result.addTrack(id, code, created)
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = workbenchSQL(ctx, db, "SELECT track_id,user_id FROM track_collects WHERE created_at>=? AND created_at<?", []any{q.Start, q.End}, func(row *sql.Rows) error {
		var id string
		var user int64
		if err := row.Scan(&id, &user); err != nil {
			return err
		}
		result.addCollect(id, user)
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = workbenchSQL(ctx, db, "SELECT track_id,created_at FROM track_navigations WHERE (created_at>=? AND created_at<?) OR (created_at>=? AND created_at<?)", []any{q.Start, q.End, q.PreviousStart, q.PreviousEnd}, func(row *sql.Rows) error {
		var id string
		var created time.Time
		if err := row.Scan(&id, &created); err != nil {
			return err
		}
		result.addNavigation(id, created)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result.rows(), nil
}

func (r *mysqlAdminDashboardRepository) WorkbenchCohorts(ctx context.Context, now time.Time) ([]models.WorkbenchCohort, error) {
	result := newWorkbenchCohorts(now)
	start := workbenchCohortStart(now)
	err := workbenchSQL(ctx, r.db, "SELECT id,created_at FROM users WHERE created_at>=? AND created_at<?", []any{start, now}, func(row *sql.Rows) error {
		var user int64
		var created time.Time
		if err := row.Scan(&user, &created); err != nil {
			return err
		}
		result.addUser(user, created)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(result.users) == 0 {
		return result.rows(), nil
	}
	err = workbenchSQL(ctx, r.db, "SELECT t.user_id,MIN(t.created_at) FROM users u JOIN track_records t ON t.user_id=u.id AND t.created_at>=u.created_at WHERE u.created_at>=? AND u.created_at<? AND t.created_at<? AND "+completedTrackAliasSQL+" GROUP BY t.user_id", []any{start, now, now}, func(row *sql.Rows) error {
		var user int64
		var created time.Time
		if err := row.Scan(&user, &created); err != nil {
			return err
		}
		result.addTrack(user, created)
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = workbenchSQL(ctx, r.db, "SELECT l.user_id,l.created_at FROM users u JOIN login_log l ON l.user_id=u.id WHERE u.created_at>=? AND u.created_at<? AND l.created_at>=? AND l.created_at<?", []any{start, now, start, now}, func(row *sql.Rows) error {
		var user int64
		var at time.Time
		if err := row.Scan(&user, &at); err != nil {
			return err
		}
		result.addLogin(user, at)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result.rows(), nil
}

func (r *mysqlAdminDashboardRepository) WorkbenchVersions(ctx context.Context, since time.Time, includeFeedback bool) (models.WorkbenchVersionCounts, error) {
	result := models.WorkbenchVersionCounts{Devices: make(map[string]int64)}
	add := func(target map[string]int64) func(*sql.Rows) error {
		return func(row *sql.Rows) error {
			var name string
			var count int64
			if err := row.Scan(&name, &count); err != nil {
				return err
			}
			target[workbenchVersionName(name)] += count
			return nil
		}
	}
	err := workbenchSQL(ctx, r.db, "SELECT app_version,COUNT(*) FROM app_client_devices WHERE platform='android' AND last_seen_at>=? GROUP BY app_version", []any{since}, add(result.Devices))
	if err != nil || !includeFeedback {
		return result, err
	}
	result.Feedback = make(map[string]int64)
	err = workbenchSQL(ctx, r.db, "SELECT app_version,COUNT(*) FROM user_feedbacks WHERE platform='android' AND created_at>=? GROUP BY app_version", []any{since}, add(result.Feedback))
	return result, err
}

func (r *mysqlAdminDashboardRepository) WorkbenchCompanions(ctx context.Context, q models.AdminDashboardQuery) ([]models.WorkbenchCompanion, error) {
	result := newWorkbenchCompanions()
	window := []any{q.Start, q.End}
	roomRange := " WHERE s.status='ended' AND s.ended_at>=? AND s.ended_at<?"
	err := workbenchSQL(ctx, r.db, "SELECT s.session_id,s.track_type FROM companion_sessions s"+roomRange, window, func(row *sql.Rows) error {
		var id, kind string
		if err := row.Scan(&id, &kind); err != nil {
			return err
		}
		result.addRoom(id, kind)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(result.rooms) == 0 {
		return result.rows(), nil
	}
	tracks := make(map[string]*models.Track)
	err = workbenchSQL(ctx, r.db, "SELECT t.id,t.user_id,t.session_id,t.status,t.deleted_at FROM companion_sessions s JOIN track_records t ON t.session_id=s.session_id"+roomRange, window, func(row *sql.Rows) error {
		track := &models.Track{}
		var deleted sql.NullTime
		if err := row.Scan(&track.ID, &track.UserID, &track.SessionID, &track.Status, &deleted); err != nil {
			return err
		}
		track.DeletedAt = deleted.Time
		tracks[track.ID] = track
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = workbenchSQL(ctx, r.db, `SELECT m.session_id,m.user_id,c.data FROM companion_sessions s
		JOIN companion_session_members m ON m.session_id=s.session_id
		LEFT JOIN companion_track_chunks c ON c.session_id=m.session_id AND c.user_id=m.user_id AND c.chunk_index=0`+roomRange, window, func(row *sql.Rows) error {
		var room string
		var user int64
		var payload sql.NullString
		if err := row.Scan(&room, &user, &payload); err != nil {
			return err
		}
		result.addMember(room, payload.String, payload.Valid, user, tracks)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result.rows(), nil
}

func (r *mysqlAdminDashboardRepository) WorkbenchBacklogs(ctx context.Context, now time.Time, scopes []string) ([]models.WorkbenchBacklog, error) {
	specs := []struct{ scope, key, source, opened, due string }{
		{backlogReview, "review", "track_submissions WHERE status='pending'", "submitted_at", "submitted_at"},
		{backlogFeedback, "feedback", "user_feedbacks WHERE status IN ('pending','processing')", "created_at", "created_at"},
		{backlogIndex, "index", "track_map_index_jobs WHERE status IN ('pending','processing')", "created_at", "next_run_at"},
		{backlogIndex, "index_failed", "track_map_index_jobs WHERE status='failed'", "created_at", ""},
		{backlogDeletion, "deletion_records", "user_account_deletions WHERE completed_at IS NULL AND assets IS NULL", "requested_at", "requested_at"},
		{backlogDeletion, "deletion_files", "user_account_deletions WHERE completed_at IS NULL AND assets IS NOT NULL", "requested_at", "objects_after"},
	}
	rows := make([]models.WorkbenchBacklog, 0, len(specs))
	for _, spec := range specs {
		if !hasWorkbenchBacklog(scopes, spec.scope) {
			continue
		}
		row := newWorkbenchBacklog(spec.key, spec.due != "")
		var oldest sql.NullTime
		var err error
		if row.Overdue == nil {
			err = r.db.QueryRowContext(ctx, "SELECT COUNT(*),MIN("+spec.opened+") FROM "+spec.source).Scan(&row.Open, &oldest)
		} else {
			err = r.db.QueryRowContext(ctx, "SELECT COUNT(*),MIN("+spec.opened+"),COALESCE(SUM("+spec.due+"<=?),0) FROM "+spec.source, now.Add(-24*time.Hour)).Scan(&row.Open, &oldest, row.Overdue)
		}
		if err != nil {
			return nil, err
		}
		if oldest.Valid {
			row.Oldest = &oldest.Time
		}
		rows = append(rows, row)
	}
	return rows, nil
}
