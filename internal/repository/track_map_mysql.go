package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

// MySQLTrackMapRepository implements TrackMapRepository using MySQL.
type MySQLTrackMapRepository struct{ db *sql.DB }

func NewMySQLTrackMapRepository(db *sql.DB) *MySQLTrackMapRepository {
	return &MySQLTrackMapRepository{db: db}
}

func (r *MySQLTrackMapRepository) EnqueueIndexJob(ctx context.Context, trackID string, runAt time.Time) error {
	trackID = strings.TrimSpace(trackID)
	if trackID == "" {
		return errors.New("track id is required")
	}
	now := time.Now()
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO track_map_index_jobs
			(track_id, status, attempts, last_error, next_run_at, locked_by, created_at, updated_at)
		VALUES (?, 'pending', 0, '', ?, '', ?, ?)
		ON DUPLICATE KEY UPDATE
			status = CASE
				WHEN status = 'succeeded' THEN status
				WHEN status = 'processing' THEN status
				ELSE 'pending'
			END,
			next_run_at = CASE
				WHEN status IN ('succeeded', 'processing') THEN next_run_at
				ELSE VALUES(next_run_at)
			END,
			updated_at = VALUES(updated_at)
	`, trackID, runAt, now, now)
	return err
}

func (r *MySQLTrackMapRepository) ClaimPendingIndexJobs(ctx context.Context, workerID string, now time.Time, limit int) ([]*models.TrackMapIndexJob, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT track_id, status, attempts, last_error, next_run_at, locked_at, locked_by, created_at, updated_at, succeeded_at, last_failed_at
		FROM track_map_index_jobs
		WHERE (status = 'pending' AND next_run_at <= ?)
		   OR (status = 'processing' AND locked_at IS NOT NULL AND locked_at < ?)
		ORDER BY created_at ASC
		LIMIT ?
	`, now, now.Add(-30*time.Minute), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := make([]*models.TrackMapIndexJob, 0, limit)
	for rows.Next() {
		job, err := scanTrackMapIndexJob(rows)
		if err != nil {
			return nil, err
		}
		res, err := r.db.ExecContext(ctx, `
			UPDATE track_map_index_jobs
			SET status = 'processing', locked_at = ?, locked_by = ?, updated_at = ?
			WHERE track_id = ?
			  AND (
			    status = 'pending'
			    OR (status = 'processing' AND locked_at IS NOT NULL AND locked_at < ?)
			  )
		`, now, workerID, now, job.TrackID, now.Add(-30*time.Minute))
		if err != nil {
			return nil, err
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			continue
		}
		job.Status = models.TrackMapIndexJobProcessing
		job.LockedAt = now
		job.LockedBy = workerID
		job.UpdatedAt = now
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (r *MySQLTrackMapRepository) MarkIndexJobSucceeded(ctx context.Context, trackID string, now time.Time) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE track_map_index_jobs
		SET status = 'succeeded', last_error = '', succeeded_at = ?, updated_at = ?
		WHERE track_id = ?
	`, now, now, trackID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MySQLTrackMapRepository) MarkIndexJobFailed(ctx context.Context, trackID, errMsg string, nextRunAt time.Time, now time.Time) error {
	if len(errMsg) > 512 {
		errMsg = errMsg[:512]
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE track_map_index_jobs
		SET status = 'pending', attempts = attempts + 1, last_error = ?, last_failed_at = ?, next_run_at = ?, updated_at = ?
		WHERE track_id = ?
	`, errMsg, now, nextRunAt, now, trackID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MySQLTrackMapRepository) UpsertTrackGeoIndex(ctx context.Context, index *models.TrackGeoIndex) error {
	if index == nil || index.TrackID == "" {
		return errors.New("track geo index is required")
	}
	now := time.Now()
	if index.CreatedAt.IsZero() {
		index.CreatedAt = now
	}
	index.UpdatedAt = now
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO track_geo_indexes
			(track_id, user_id, city_code, track_type, coordinate_system,
			 start_lat, start_lng, end_lat, end_lng, center_lat, center_lng,
			 min_lat, min_lng, max_lat, max_lng, distance, point_count,
			 simplified_polyline_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			user_id = VALUES(user_id),
			city_code = VALUES(city_code),
			track_type = VALUES(track_type),
			coordinate_system = VALUES(coordinate_system),
			start_lat = VALUES(start_lat),
			start_lng = VALUES(start_lng),
			end_lat = VALUES(end_lat),
			end_lng = VALUES(end_lng),
			center_lat = VALUES(center_lat),
			center_lng = VALUES(center_lng),
			min_lat = VALUES(min_lat),
			min_lng = VALUES(min_lng),
			max_lat = VALUES(max_lat),
			max_lng = VALUES(max_lng),
			distance = VALUES(distance),
			point_count = VALUES(point_count),
			simplified_polyline_json = VALUES(simplified_polyline_json),
			updated_at = VALUES(updated_at)
	`, index.TrackID, index.UserID, index.CityCode, index.TrackType, index.CoordinateSystem,
		index.StartLat, index.StartLng, index.EndLat, index.EndLng, index.CenterLat, index.CenterLng,
		index.MinLat, index.MinLng, index.MaxLat, index.MaxLng, index.Distance, index.PointCount,
		index.SimplifiedPolylineJSON, index.CreatedAt, index.UpdatedAt)
	return err
}

func (r *MySQLTrackMapRepository) HasTrackGeoIndex(ctx context.Context, trackID string) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(ctx, `SELECT 1 FROM track_geo_indexes WHERE track_id = ? LIMIT 1`, trackID).Scan(&exists)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *MySQLTrackMapRepository) ListCompletedTracksMissingGeoIndex(ctx context.Context, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT tr.id
		FROM track_records tr
		LEFT JOIN track_geo_indexes gi ON gi.track_id = tr.id
		WHERE tr.status = ? AND tr.is_running = 0 AND tr.raw_track_url <> '' AND gi.track_id IS NULL
		ORDER BY tr.created_at ASC
		LIMIT ?
	`, models.TrackStatusNormal, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	trackRepo := NewMySQLTrackRepository(r.db)
	var tracks []*models.Track
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		t, err := trackRepo.FindByID(ctx, id)
		if err != nil {
			return nil, err
		}
		tracks = append(tracks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tracks, nil
}

type trackMapJobScanner interface {
	Scan(dest ...any) error
}

func scanTrackMapIndexJob(row trackMapJobScanner) (*models.TrackMapIndexJob, error) {
	var job models.TrackMapIndexJob
	var lockedAt, succeededAt, lastFailedAt sql.NullTime
	if err := row.Scan(
		&job.TrackID,
		&job.Status,
		&job.Attempts,
		&job.LastError,
		&job.NextRunAt,
		&lockedAt,
		&job.LockedBy,
		&job.CreatedAt,
		&job.UpdatedAt,
		&succeededAt,
		&lastFailedAt,
	); err != nil {
		return nil, err
	}
	if lockedAt.Valid {
		job.LockedAt = lockedAt.Time
	}
	if succeededAt.Valid {
		job.SucceededAt = succeededAt.Time
	}
	if lastFailedAt.Valid {
		job.LastFailedAt = lastFailedAt.Time
	}
	return &job, nil
}

func (r *MySQLTrackMapRepository) FindTrackGeoIndex(ctx context.Context, trackID string) (*models.TrackGeoIndex, error) {
	row := r.db.QueryRowContext(ctx, trackGeoIndexSelectSQL()+` WHERE track_id = ? LIMIT 1`, trackID)
	index, err := scanTrackGeoIndex(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return index, nil
}

func (r *MySQLTrackMapRepository) ListTrackGeoIndexes(ctx context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackGeoIndex, error) {
	limit := normalizeTrackMapQueryLimit(filter.Limit)
	where, args := buildTrackGeoIndexWhere(filter)
	query := trackGeoIndexSelectSQL() + where + ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*models.TrackGeoIndex, 0, limit)
	for rows.Next() {
		index, err := scanTrackGeoIndex(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, index)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *MySQLTrackMapRepository) CountTrackGeoIndexesByCity(ctx context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackMapClusterItem, error) {
	limit := normalizeTrackMapQueryLimit(filter.Limit)
	where, args := buildTrackGeoIndexWhere(filter)
	query := `
		SELECT city_code, track_type, COUNT(*) AS route_count,
		       AVG(center_lat) AS center_lat, AVG(center_lng) AS center_lng,
		       MIN(min_lat) AS min_lat, MIN(min_lng) AS min_lng,
		       MAX(max_lat) AS max_lat, MAX(max_lng) AS max_lng
		FROM track_geo_indexes` + where + `
		GROUP BY city_code, track_type
		ORDER BY route_count DESC
		LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*models.TrackMapClusterItem, 0, limit)
	for rows.Next() {
		item := &models.TrackMapClusterItem{Type: "city_cluster"}
		if err := rows.Scan(
			&item.CityCode,
			&item.TrackType,
			&item.RouteCount,
			&item.Center.Latitude,
			&item.Center.Longitude,
			&item.BBox.MinLatitude,
			&item.BBox.MinLongitude,
			&item.BBox.MaxLatitude,
			&item.BBox.MaxLongitude,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *MySQLTrackMapRepository) CountTrackGeoIndexesByArea(ctx context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackMapClusterItem, error) {
	limit := normalizeTrackMapQueryLimit(filter.Limit)
	where, args := buildTrackGeoIndexWhere(filter)
	query := `
		SELECT CONCAT('cell_', ROUND(center_lat, 1), '_', ROUND(center_lng, 1)) AS cluster_id,
		       track_type, COUNT(*) AS route_count,
		       AVG(center_lat) AS center_lat, AVG(center_lng) AS center_lng,
		       MIN(min_lat) AS min_lat, MIN(min_lng) AS min_lng,
		       MAX(max_lat) AS max_lat, MAX(max_lng) AS max_lng
		FROM track_geo_indexes` + where + `
		GROUP BY cluster_id, track_type
		ORDER BY route_count DESC
		LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*models.TrackMapClusterItem, 0, limit)
	for rows.Next() {
		item := &models.TrackMapClusterItem{Type: "area_cluster"}
		if err := rows.Scan(
			&item.ClusterID,
			&item.TrackType,
			&item.RouteCount,
			&item.Center.Latitude,
			&item.Center.Longitude,
			&item.BBox.MinLatitude,
			&item.BBox.MinLongitude,
			&item.BBox.MaxLatitude,
			&item.BBox.MaxLongitude,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func trackGeoIndexSelectSQL() string {
	return `SELECT track_id, user_id, city_code, track_type, coordinate_system,
		start_lat, start_lng, end_lat, end_lng, center_lat, center_lng,
		min_lat, min_lng, max_lat, max_lng, distance, point_count,
		simplified_polyline_json, created_at, updated_at FROM track_geo_indexes`
}

func buildTrackGeoIndexWhere(filter models.TrackMapQueryFilter) (string, []interface{}) {
	conds := make([]string, 0, 6)
	args := make([]interface{}, 0, 12)
	if filter.TrackType != "" {
		conds = append(conds, "track_type = ?")
		args = append(args, filter.TrackType)
	}
	if filter.CityCode != "" {
		conds = append(conds, "city_code = ?")
		args = append(args, filter.CityCode)
	}
	if filter.BBox != nil {
		conds = append(conds, "max_lat >= ? AND min_lat <= ? AND max_lng >= ? AND min_lng <= ?")
		args = append(args, filter.BBox.MinLatitude, filter.BBox.MaxLatitude, filter.BBox.MinLongitude, filter.BBox.MaxLongitude)
	}
	if filter.Center != nil && filter.RadiusM > 0 {
		deg := float64(filter.RadiusM) / 111000.0
		conds = append(conds, "center_lat BETWEEN ? AND ? AND center_lng BETWEEN ? AND ?")
		args = append(args, filter.Center.Latitude-deg, filter.Center.Latitude+deg, filter.Center.Longitude-deg, filter.Center.Longitude+deg)
	}
	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

type trackGeoIndexScanner interface {
	Scan(dest ...any) error
}

func scanTrackGeoIndex(row trackGeoIndexScanner) (*models.TrackGeoIndex, error) {
	var index models.TrackGeoIndex
	var polylineJSON sql.NullString
	if err := row.Scan(
		&index.TrackID,
		&index.UserID,
		&index.CityCode,
		&index.TrackType,
		&index.CoordinateSystem,
		&index.StartLat,
		&index.StartLng,
		&index.EndLat,
		&index.EndLng,
		&index.CenterLat,
		&index.CenterLng,
		&index.MinLat,
		&index.MinLng,
		&index.MaxLat,
		&index.MaxLng,
		&index.Distance,
		&index.PointCount,
		&polylineJSON,
		&index.CreatedAt,
		&index.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if polylineJSON.Valid {
		index.SimplifiedPolylineJSON = polylineJSON.String
		_ = json.Unmarshal([]byte(polylineJSON.String), &index.SimplifiedPolyline)
	}
	return &index, nil
}

func normalizeTrackMapQueryLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 200 {
		return 200
	}
	return limit
}
