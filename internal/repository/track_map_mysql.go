package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
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
				WHEN status = 'pending' THEN LEAST(next_run_at, VALUES(next_run_at))
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

// CleanupDeletedTrack removes map-index side records for a deleted track.
func (r *MySQLTrackMapRepository) CleanupDeletedTrack(ctx context.Context, trackID string) error {
	trackID = strings.TrimSpace(trackID)
	if trackID == "" {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	groups, err := mysqlRouteGroupsForTrackTx(ctx, tx, trackID)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM track_map_index_jobs WHERE track_id=?`, trackID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM track_route_group_members WHERE track_id=?`, trackID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM track_geo_indexes WHERE track_id=?`, trackID); err != nil {
		return err
	}
	if err := mysqlCleanupRouteGroupsAfterTrackDeleteTx(ctx, tx, groups, time.Now()); err != nil {
		return err
	}
	return tx.Commit()
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

func (r *MySQLTrackMapRepository) ListAllTrackGeoIndexes(ctx context.Context) ([]*models.TrackGeoIndex, error) {
	rows, err := r.db.QueryContext(ctx, trackGeoIndexSelectSQL()+` ORDER BY track_type ASC, center_lat ASC, center_lng ASC, track_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*models.TrackGeoIndex, 0)
	for rows.Next() {
		index, err := scanTrackGeoIndex(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, index)
	}
	return items, rows.Err()
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

func (r *MySQLTrackMapRepository) FindRouteGroup(ctx context.Context, groupID string) (*models.TrackRouteGroup, error) {
	row := r.db.QueryRowContext(ctx, trackRouteGroupSelectSQL()+` WHERE group_id = ? AND status = ?`, groupID, models.TrackRouteGroupStatusActive)
	group, err := scanTrackRouteGroup(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return group, nil
}

func (r *MySQLTrackMapRepository) ListRouteGroups(ctx context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackRouteGroup, error) {
	limit := normalizeTrackMapQueryLimit(filter.Limit)
	where, args := buildTrackRouteGroupWhere(filter)
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, trackRouteGroupSelectSQL()+where+` ORDER BY member_count DESC, updated_at DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*models.TrackRouteGroup, 0, limit)
	for rows.Next() {
		group, err := scanTrackRouteGroup(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, group)
	}
	return items, rows.Err()
}

func (r *MySQLTrackMapRepository) ListRouteGroupSummaries(ctx context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackRouteGroup, error) {
	limit := normalizeTrackMapQueryLimit(filter.Limit)
	where, args := buildTrackRouteGroupWhere(filter)
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, `SELECT `+trackRouteGroupSummaryColumns("track_route_groups")+` FROM track_route_groups`+where+` ORDER BY member_count DESC, updated_at DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*models.TrackRouteGroup, 0, limit)
	for rows.Next() {
		group, err := scanTrackRouteGroupSummary(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, group)
	}
	return items, rows.Err()
}

func (r *MySQLTrackMapRepository) ListRouteGroupCandidates(ctx context.Context, index *models.TrackGeoIndex, limit int) ([]*models.TrackRouteGroupCandidate, error) {
	if index == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+trackRouteGroupColumns("rg")+`, `+trackGeoIndexColumns("gi")+`
		FROM track_route_groups rg
		JOIN track_geo_indexes gi ON gi.track_id = rg.representative_track_id
		WHERE rg.status = ?
		  AND rg.track_type = ?
		  AND rg.max_lat >= ? AND rg.min_lat <= ? AND rg.max_lng >= ? AND rg.min_lng <= ?
		ORDER BY rg.member_count DESC, rg.updated_at DESC
		LIMIT ?
	`, models.TrackRouteGroupStatusActive, index.TrackType, index.MinLat-0.02, index.MaxLat+0.02, index.MinLng-0.02, index.MaxLng+0.02, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*models.TrackRouteGroupCandidate, 0, limit)
	for rows.Next() {
		group, geoIndex, err := scanTrackRouteGroupCandidate(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, &models.TrackRouteGroupCandidate{Group: group, Index: geoIndex})
	}
	return items, rows.Err()
}

func (r *MySQLTrackMapRepository) ListGeoIndexesWithoutRouteGroup(ctx context.Context, limit int) ([]*models.TrackGeoIndex, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, trackGeoIndexSelectSQL()+`
		WHERE NOT EXISTS (
			SELECT 1 FROM track_route_group_members m WHERE m.track_id = track_geo_indexes.track_id
		)
		ORDER BY created_at ASC
		LIMIT ?
	`, limit)
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
	return items, rows.Err()
}

func (r *MySQLTrackMapRepository) UpsertRouteGroup(ctx context.Context, group *models.TrackRouteGroup) error {
	if group == nil || group.GroupID == "" {
		return errors.New("route group is required")
	}
	cityCodesJSON, err := json.Marshal(group.CityCodes)
	if err != nil {
		return err
	}
	now := time.Now()
	if group.CreatedAt.IsZero() {
		group.CreatedAt = now
	}
	group.UpdatedAt = now
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO track_route_groups
			(group_id, name, track_type, status, city_codes_json, area_id, coordinate_system,
			 center_lat, center_lng, radius_m, min_lat, min_lng, max_lat, max_lng, distance,
			 representative_track_id, member_count, source, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			name = VALUES(name),
			track_type = VALUES(track_type),
			status = VALUES(status),
			city_codes_json = VALUES(city_codes_json),
			area_id = VALUES(area_id),
			coordinate_system = VALUES(coordinate_system),
			center_lat = VALUES(center_lat),
			center_lng = VALUES(center_lng),
			radius_m = VALUES(radius_m),
			min_lat = VALUES(min_lat),
			min_lng = VALUES(min_lng),
			max_lat = VALUES(max_lat),
			max_lng = VALUES(max_lng),
			distance = VALUES(distance),
			representative_track_id = VALUES(representative_track_id),
			member_count = VALUES(member_count),
			source = VALUES(source),
			updated_at = VALUES(updated_at)
	`, group.GroupID, group.Name, group.TrackType, group.Status, string(cityCodesJSON), group.AreaID, group.CoordinateSystem,
		group.CenterLat, group.CenterLng, group.RadiusM, group.MinLat, group.MinLng, group.MaxLat, group.MaxLng, group.Distance,
		group.RepresentativeTrackID, group.MemberCount, group.Source, group.CreatedAt, group.UpdatedAt)
	return err
}

func (r *MySQLTrackMapRepository) UpsertRouteGroupMember(ctx context.Context, member *models.TrackRouteGroupMember) error {
	if member == nil || member.GroupID == "" || member.TrackID == "" {
		return errors.New("route group member is required")
	}
	now := time.Now()
	if member.CreatedAt.IsZero() {
		member.CreatedAt = now
	}
	member.UpdatedAt = now
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO track_route_group_members
			(group_id, track_id, similarity_score, match_direction, role, source, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			similarity_score = VALUES(similarity_score),
			match_direction = VALUES(match_direction),
			role = VALUES(role),
			source = VALUES(source),
			updated_at = VALUES(updated_at)
	`, member.GroupID, member.TrackID, member.SimilarityScore, member.MatchDirection, member.Role, member.Source, member.CreatedAt, member.UpdatedAt)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		UPDATE track_route_groups
		SET member_count = (SELECT COUNT(*) FROM track_route_group_members WHERE group_id = ?), updated_at = ?
		WHERE group_id = ?
	`, member.GroupID, now, member.GroupID)
	return err
}

func (r *MySQLTrackMapRepository) ReplaceRouteGroups(ctx context.Context, groups []*models.TrackRouteGroup, members []*models.TrackRouteGroupMember) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM track_route_group_members`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM track_route_groups`); err != nil {
		return err
	}
	for _, group := range groups {
		if group == nil || group.GroupID == "" {
			continue
		}
		cityCodesJSON, err := json.Marshal(group.CityCodes)
		if err != nil {
			return err
		}
		now := time.Now()
		if group.CreatedAt.IsZero() {
			group.CreatedAt = now
		}
		if group.UpdatedAt.IsZero() {
			group.UpdatedAt = now
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO track_route_groups
				(group_id, name, track_type, status, city_codes_json, area_id, coordinate_system,
				 center_lat, center_lng, radius_m, min_lat, min_lng, max_lat, max_lng, distance,
				 representative_track_id, member_count, source, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, group.GroupID, group.Name, group.TrackType, group.Status, string(cityCodesJSON), group.AreaID, group.CoordinateSystem,
			group.CenterLat, group.CenterLng, group.RadiusM, group.MinLat, group.MinLng, group.MaxLat, group.MaxLng, group.Distance,
			group.RepresentativeTrackID, group.MemberCount, group.Source, group.CreatedAt, group.UpdatedAt); err != nil {
			return err
		}
	}
	for _, member := range members {
		if member == nil || member.GroupID == "" || member.TrackID == "" {
			continue
		}
		now := time.Now()
		if member.CreatedAt.IsZero() {
			member.CreatedAt = now
		}
		if member.UpdatedAt.IsZero() {
			member.UpdatedAt = now
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO track_route_group_members
				(group_id, track_id, similarity_score, match_direction, role, source, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, member.GroupID, member.TrackID, member.SimilarityScore, member.MatchDirection, member.Role, member.Source, member.CreatedAt, member.UpdatedAt); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE track_route_introductions i
		LEFT JOIN track_route_group_members m ON m.track_id = i.anchor_track_id
		SET i.current_group_id = COALESCE(m.group_id, ''), i.updated_at = ?
	`, time.Now()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE track_route_introductions newer
		JOIN track_route_introductions older
		  ON older.current_group_id = newer.current_group_id AND older.id < newer.id
		SET newer.current_group_id = '', newer.status = 'archived', newer.updated_at = ?
		WHERE newer.current_group_id <> ''
	`, time.Now()); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *MySQLTrackMapRepository) FindRouteIntroductionByGroupID(ctx context.Context, groupID string) (*models.TrackRouteIntroduction, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, anchor_track_id, current_group_id, status, content_zh_json, content_en_json,
		       difficulty, estimated_duration_min, estimated_duration_max, best_seasons_json,
		       content_version, created_at, updated_at, published_at
		FROM track_route_introductions WHERE current_group_id=? LIMIT 1
	`, strings.TrimSpace(groupID))
	return scanMySQLRouteIntroduction(row)
}

func (r *MySQLTrackMapRepository) ListPublishedRouteIntroductions(ctx context.Context, groupIDs []string) (map[string]*models.TrackRouteIntroduction, error) {
	result := make(map[string]*models.TrackRouteIntroduction)
	if len(groupIDs) == 0 {
		return result, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(groupIDs)), ",")
	args := make([]interface{}, 0, len(groupIDs)+1)
	args = append(args, models.TrackRouteIntroductionStatusPublished)
	for _, groupID := range groupIDs {
		args = append(args, strings.TrimSpace(groupID))
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, anchor_track_id, current_group_id, status, content_zh_json, content_en_json,
		       difficulty, estimated_duration_min, estimated_duration_max, best_seasons_json,
		       content_version, created_at, updated_at, published_at
		FROM track_route_introductions WHERE status=? AND current_group_id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		introduction, err := scanMySQLRouteIntroduction(rows)
		if err != nil {
			return nil, err
		}
		result[introduction.CurrentGroupID] = introduction
	}
	return result, rows.Err()
}

type routeIntroductionScanner interface {
	Scan(dest ...interface{}) error
}

func scanMySQLRouteIntroduction(scanner routeIntroductionScanner) (*models.TrackRouteIntroduction, error) {
	var introduction models.TrackRouteIntroduction
	var zhJSON, enJSON, seasonsJSON []byte
	var publishedAt sql.NullTime
	err := scanner.Scan(&introduction.ID, &introduction.AnchorTrackID, &introduction.CurrentGroupID, &introduction.Status,
		&zhJSON, &enJSON, &introduction.Difficulty, &introduction.EstimatedDurationMin, &introduction.EstimatedDurationMax,
		&seasonsJSON, &introduction.ContentVersion, &introduction.CreatedAt, &introduction.UpdatedAt, &publishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(zhJSON, &introduction.Chinese); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(enJSON, &introduction.English); err != nil {
		return nil, err
	}
	if len(seasonsJSON) > 0 {
		if err := json.Unmarshal(seasonsJSON, &introduction.BestSeasons); err != nil {
			return nil, err
		}
	}
	if publishedAt.Valid {
		introduction.PublishedAt = &publishedAt.Time
	}
	return &introduction, nil
}

func (r *MySQLTrackMapRepository) UpsertRouteIntroduction(ctx context.Context, introduction *models.TrackRouteIntroduction) error {
	if introduction == nil || strings.TrimSpace(introduction.AnchorTrackID) == "" {
		return errors.New("route introduction is required")
	}
	zhJSON, err := json.Marshal(introduction.Chinese)
	if err != nil {
		return err
	}
	enJSON, err := json.Marshal(introduction.English)
	if err != nil {
		return err
	}
	seasonsJSON, err := json.Marshal(introduction.BestSeasons)
	if err != nil {
		return err
	}
	now := time.Now()
	if introduction.CreatedAt.IsZero() {
		introduction.CreatedAt = now
	}
	introduction.UpdatedAt = now
	if introduction.ID > 0 {
		_, err = r.db.ExecContext(ctx, `
			UPDATE track_route_introductions SET anchor_track_id=?,current_group_id=?,status=?,content_zh_json=?,content_en_json=?,difficulty=?,
				estimated_duration_min=?,estimated_duration_max=?,best_seasons_json=?,content_version=?,updated_at=?,published_at=? WHERE id=?
		`, introduction.AnchorTrackID, introduction.CurrentGroupID, introduction.Status, zhJSON, enJSON, introduction.Difficulty,
			introduction.EstimatedDurationMin, introduction.EstimatedDurationMax, seasonsJSON, introduction.ContentVersion,
			introduction.UpdatedAt, introduction.PublishedAt, introduction.ID)
		return err
	}
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO track_route_introductions
			(anchor_track_id,current_group_id,status,content_zh_json,content_en_json,difficulty,
			 estimated_duration_min,estimated_duration_max,best_seasons_json,content_version,created_at,updated_at,published_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE current_group_id=VALUES(current_group_id),status=VALUES(status),
			content_zh_json=VALUES(content_zh_json),content_en_json=VALUES(content_en_json),difficulty=VALUES(difficulty),
			estimated_duration_min=VALUES(estimated_duration_min),estimated_duration_max=VALUES(estimated_duration_max),
			id=LAST_INSERT_ID(id),best_seasons_json=VALUES(best_seasons_json),content_version=VALUES(content_version),updated_at=VALUES(updated_at),published_at=VALUES(published_at)
	`, introduction.AnchorTrackID, introduction.CurrentGroupID, introduction.Status, zhJSON, enJSON, introduction.Difficulty,
		introduction.EstimatedDurationMin, introduction.EstimatedDurationMax, seasonsJSON, introduction.ContentVersion,
		introduction.CreatedAt, introduction.UpdatedAt, introduction.PublishedAt)
	if err != nil {
		return err
	}
	introduction.ID, _ = result.LastInsertId()
	return nil
}

func (r *MySQLTrackMapRepository) DeleteRouteGroupMember(ctx context.Context, groupID, trackID string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM track_route_group_members WHERE group_id = ? AND track_id = ?`, groupID, trackID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err == nil && affected == 0 {
		return ErrNotFound
	}
	now := time.Now()
	_, err = r.db.ExecContext(ctx, `
		UPDATE track_route_groups
		SET member_count = (SELECT COUNT(*) FROM track_route_group_members WHERE group_id = ?), updated_at = ?
		WHERE group_id = ?
	`, groupID, now, groupID)
	return err
}

func (r *MySQLTrackMapRepository) ArchiveRouteGroup(ctx context.Context, groupID string, now time.Time) error {
	res, err := r.db.ExecContext(ctx, `UPDATE track_route_groups SET status = ?, updated_at = ? WHERE group_id = ?`, models.TrackRouteGroupStatusArchived, now, groupID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err == nil && affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MySQLTrackMapRepository) ListRouteGroupMembers(ctx context.Context, groupID string, limit int) ([]*models.TrackRouteGroupMember, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT group_id, track_id, similarity_score, match_direction, role, source, created_at, updated_at
		FROM track_route_group_members
		WHERE group_id = ?
		ORDER BY role = 'representative' DESC, created_at DESC
		LIMIT ?
	`, groupID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*models.TrackRouteGroupMember, 0, limit)
	for rows.Next() {
		member, err := scanTrackRouteGroupMember(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, member)
	}
	return items, rows.Err()
}

func (r *MySQLTrackMapRepository) FindRouteGroupByTrackID(ctx context.Context, trackID string) (*models.TrackRouteGroup, error) {
	row := r.db.QueryRowContext(ctx, trackRouteGroupSelectSQL()+`
		JOIN track_route_group_members m ON m.group_id = track_route_groups.group_id
		WHERE m.track_id = ? AND track_route_groups.status = ?
		LIMIT 1
	`, trackID, models.TrackRouteGroupStatusActive)
	group, err := scanTrackRouteGroup(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return group, nil
}

func (r *MySQLTrackMapRepository) CountRouteGroupsByCity(ctx context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackMapClusterItem, error) {
	groups, err := r.ListRouteGroups(ctx, filter)
	if err != nil {
		return nil, err
	}
	accs := make(map[string]*trackMapClusterAcc)
	for _, group := range groups {
		codes := group.CityCodes
		if len(codes) == 0 {
			codes = []string{""}
		}
		for _, code := range codes {
			key := code
			if key == "" {
				key = "unknown"
			}
			a := accs[key]
			if a == nil {
				a = &trackMapClusterAcc{}
				accs[key] = a
			}
			a.addRouteGroup(group)
		}
	}
	return sortedClusterItems(accs, "city_cluster", filter.TrackType, filter.Limit), nil
}

func (r *MySQLTrackMapRepository) CountRouteGroupsByArea(ctx context.Context, filter models.TrackMapQueryFilter) ([]*models.TrackMapClusterItem, error) {
	limit := normalizeTrackMapQueryLimit(filter.Limit)
	where, args := buildTrackRouteGroupWhere(filter)
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, `
		SELECT area_id,
		       CASE WHEN area_id = '' THEN FLOOR(center_lat * 10) / 10 ELSE 0 END AS lat_cell,
		       CASE WHEN area_id = '' THEN FLOOR(center_lng * 10) / 10 ELSE 0 END AS lng_cell,
		       track_type, COUNT(*) AS route_count,
		       AVG(center_lat), AVG(center_lng), MIN(min_lat), MIN(min_lng), MAX(max_lat), MAX(max_lng)
		FROM track_route_groups`+where+`
		GROUP BY area_id, lat_cell, lng_cell, track_type
		ORDER BY route_count DESC
		LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*models.TrackMapClusterItem, 0, limit)
	for rows.Next() {
		var latCell, lngCell float64
		item := &models.TrackMapClusterItem{Type: "area_cluster"}
		if err := rows.Scan(&item.AreaID, &latCell, &lngCell, &item.TrackType, &item.RouteCount, &item.Center.Latitude, &item.Center.Longitude, &item.BBox.MinLatitude, &item.BBox.MinLongitude, &item.BBox.MaxLatitude, &item.BBox.MaxLongitude); err != nil {
			return nil, err
		}
		if item.AreaID != "" {
			item.ClusterID = "area_" + item.AreaID
		} else {
			item.ClusterID = "cell_" + strings.TrimRight(strings.TrimRight(strconvFormatFloat(latCell), "0"), ".") + "_" + strings.TrimRight(strings.TrimRight(strconvFormatFloat(lngCell), "0"), ".")
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func trackRouteGroupSelectSQL() string {
	return `SELECT ` + trackRouteGroupColumns("track_route_groups") + ` FROM track_route_groups`
}

func trackRouteGroupColumns(alias string) string {
	p := alias + "."
	return p + `group_id, ` + p + `name, ` + p + `track_type, ` + p + `status, ` + p + `city_codes_json, ` + p + `area_id, ` + p + `coordinate_system,
		` + p + `center_lat, ` + p + `center_lng, ` + p + `radius_m, ` + p + `min_lat, ` + p + `min_lng, ` + p + `max_lat, ` + p + `max_lng,
		` + p + `distance, ` + p + `representative_track_id, ` + p + `member_count,
		` + p + `source, ` + p + `created_at, ` + p + `updated_at`
}

func trackRouteGroupSummaryColumns(alias string) string {
	p := alias + "."
	return p + `group_id, ` + p + `name, ` + p + `track_type, ` + p + `status, ` + p + `city_codes_json, ` + p + `area_id, ` + p + `coordinate_system,
		` + p + `center_lat, ` + p + `center_lng, ` + p + `radius_m, ` + p + `min_lat, ` + p + `min_lng, ` + p + `max_lat, ` + p + `max_lng,
		` + p + `distance, ` + p + `representative_track_id, ` + p + `member_count,
		` + p + `source, ` + p + `created_at, ` + p + `updated_at`
}

func trackGeoIndexColumns(alias string) string {
	p := alias + "."
	return p + `track_id, ` + p + `user_id, ` + p + `city_code, ` + p + `track_type, ` + p + `coordinate_system,
		` + p + `start_lat, ` + p + `start_lng, ` + p + `end_lat, ` + p + `end_lng, ` + p + `center_lat, ` + p + `center_lng,
		` + p + `min_lat, ` + p + `min_lng, ` + p + `max_lat, ` + p + `max_lng, ` + p + `distance, ` + p + `point_count,
		` + p + `simplified_polyline_json, ` + p + `created_at, ` + p + `updated_at`
}

func buildTrackRouteGroupWhere(filter models.TrackMapQueryFilter) (string, []interface{}) {
	conds := []string{"status = ?"}
	args := []interface{}{models.TrackRouteGroupStatusActive}
	if filter.TrackType != "" {
		conds = append(conds, "track_type = ?")
		args = append(args, filter.TrackType)
	}
	if filter.CityCode != "" {
		conds = append(conds, "city_codes_json LIKE ?")
		args = append(args, "%\""+filter.CityCode+"\"%")
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
	return " WHERE " + strings.Join(conds, " AND "), args
}

func scanTrackRouteGroup(row trackGeoIndexScanner) (*models.TrackRouteGroup, error) {
	var group models.TrackRouteGroup
	var cityCodesJSON sql.NullString
	if err := row.Scan(&group.GroupID, &group.Name, &group.TrackType, &group.Status, &cityCodesJSON, &group.AreaID, &group.CoordinateSystem,
		&group.CenterLat, &group.CenterLng, &group.RadiusM, &group.MinLat, &group.MinLng, &group.MaxLat, &group.MaxLng,
		&group.Distance, &group.RepresentativeTrackID, &group.MemberCount,
		&group.Source, &group.CreatedAt, &group.UpdatedAt); err != nil {
		return nil, err
	}
	if cityCodesJSON.Valid {
		group.CityCodesJSON = cityCodesJSON.String
		_ = json.Unmarshal([]byte(cityCodesJSON.String), &group.CityCodes)
	}
	return &group, nil
}

func scanTrackRouteGroupSummary(row trackGeoIndexScanner) (*models.TrackRouteGroup, error) {
	var group models.TrackRouteGroup
	var cityCodesJSON sql.NullString
	if err := row.Scan(&group.GroupID, &group.Name, &group.TrackType, &group.Status, &cityCodesJSON, &group.AreaID, &group.CoordinateSystem,
		&group.CenterLat, &group.CenterLng, &group.RadiusM, &group.MinLat, &group.MinLng, &group.MaxLat, &group.MaxLng,
		&group.Distance, &group.RepresentativeTrackID, &group.MemberCount,
		&group.Source, &group.CreatedAt, &group.UpdatedAt); err != nil {
		return nil, err
	}
	if cityCodesJSON.Valid {
		group.CityCodesJSON = cityCodesJSON.String
		_ = json.Unmarshal([]byte(cityCodesJSON.String), &group.CityCodes)
	}
	return &group, nil
}

func scanTrackRouteGroupCandidate(row trackGeoIndexScanner) (*models.TrackRouteGroup, *models.TrackGeoIndex, error) {
	var group models.TrackRouteGroup
	var cityCodesJSON, indexPolylineJSON sql.NullString
	var index models.TrackGeoIndex
	if err := row.Scan(&group.GroupID, &group.Name, &group.TrackType, &group.Status, &cityCodesJSON, &group.AreaID, &group.CoordinateSystem,
		&group.CenterLat, &group.CenterLng, &group.RadiusM, &group.MinLat, &group.MinLng, &group.MaxLat, &group.MaxLng,
		&group.Distance, &group.RepresentativeTrackID, &group.MemberCount,
		&group.Source, &group.CreatedAt, &group.UpdatedAt,
		&index.TrackID, &index.UserID, &index.CityCode, &index.TrackType, &index.CoordinateSystem,
		&index.StartLat, &index.StartLng, &index.EndLat, &index.EndLng, &index.CenterLat, &index.CenterLng,
		&index.MinLat, &index.MinLng, &index.MaxLat, &index.MaxLng, &index.Distance, &index.PointCount,
		&indexPolylineJSON, &index.CreatedAt, &index.UpdatedAt); err != nil {
		return nil, nil, err
	}
	if cityCodesJSON.Valid {
		group.CityCodesJSON = cityCodesJSON.String
		_ = json.Unmarshal([]byte(cityCodesJSON.String), &group.CityCodes)
	}
	if indexPolylineJSON.Valid {
		index.SimplifiedPolylineJSON = indexPolylineJSON.String
		_ = json.Unmarshal([]byte(indexPolylineJSON.String), &index.SimplifiedPolyline)
	}
	return &group, &index, nil
}

func scanTrackRouteGroupMember(row trackGeoIndexScanner) (*models.TrackRouteGroupMember, error) {
	var member models.TrackRouteGroupMember
	if err := row.Scan(&member.GroupID, &member.TrackID, &member.SimilarityScore, &member.MatchDirection, &member.Role, &member.Source, &member.CreatedAt, &member.UpdatedAt); err != nil {
		return nil, err
	}
	return &member, nil
}

func strconvFormatFloat(v float64) string {
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(v, 'f', 1, 64), "0"), ".")
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
