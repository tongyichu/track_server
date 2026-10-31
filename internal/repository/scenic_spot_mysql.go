package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/tongyichu/track_server/internal/models"
)

type MySQLScenicSpotRepository struct{ db *sql.DB }

func NewMySQLScenicSpotRepository(db *sql.DB) *MySQLScenicSpotRepository {
	return &MySQLScenicSpotRepository{db: db}
}

func (r *MySQLScenicSpotRepository) CreateScenicSpot(ctx context.Context, spot *models.ScenicSpot) error {
	data, err := json.Marshal(spot)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO scenic_spots(spot_id,name,status,merged_into,version,data_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, spot.ID, spot.Name, spot.Status, spot.MergedInto, spot.Version, string(data), spot.CreatedAt, spot.UpdatedAt)
	return mapMySQLConflict(err)
}
func (r *MySQLScenicSpotRepository) GetScenicSpot(ctx context.Context, id string) (*models.ScenicSpot, error) {
	var data string
	err := r.db.QueryRowContext(ctx, `SELECT data_json FROM scenic_spots WHERE spot_id=?`, id).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var spot models.ScenicSpot
	if err := json.Unmarshal([]byte(data), &spot); err != nil {
		return nil, err
	}
	return &spot, nil
}
func (r *MySQLScenicSpotRepository) GetScenicSpots(ctx context.Context, ids []string) (map[string]*models.ScenicSpot, error) {
	out := map[string]*models.ScenicSpot{}
	ids = uniqueNonEmptyStrings(ids)
	if len(ids) == 0 {
		return out, nil
	}
	p := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := r.db.QueryContext(ctx, `SELECT data_json FROM scenic_spots WHERE spot_id IN (`+p+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var v models.ScenicSpot
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, err
		}
		out[v.ID] = &v
	}
	return out, rows.Err()
}
func (r *MySQLScenicSpotRepository) FindScenicSpotBySource(ctx context.Context, provider, providerID string) (*models.ScenicSpot, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT data_json FROM scenic_spots WHERE status<>'merged'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var spot models.ScenicSpot
		if json.Unmarshal([]byte(raw), &spot) != nil {
			continue
		}
		for _, source := range spot.Sources {
			if source.Provider == provider && source.ProviderID == providerID {
				return &spot, nil
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nil, ErrNotFound
}
func (r *MySQLScenicSpotRepository) ListScenicSpots(ctx context.Context, f ScenicSpotListFilter) ([]*models.ScenicSpot, int64, error) {
	where := ` WHERE 1=1`
	args := []any{}
	if f.VisibleOnly {
		where += ` AND status NOT IN ('archived','merged')`
	}
	if f.Missing != "" {
		where += ` AND (` + scenicSpotMissingSQL(f.Missing) + `)`
	}
	if f.Query != "" {
		where += ` AND name LIKE ?`
		args = append(args, "%"+strings.ReplaceAll(f.Query, "%", "\\%")+"%")
	}
	if f.Status != "" {
		where += ` AND status=?`
		args = append(args, f.Status)
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM scenic_spots`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if f.Cursor != "" {
		where += ` AND spot_id>?`
		args = append(args, f.Cursor)
	}
	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, `SELECT data_json FROM scenic_spots`+where+` ORDER BY spot_id LIMIT ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []*models.ScenicSpot{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, 0, err
		}
		var spot models.ScenicSpot
		if err := json.Unmarshal([]byte(raw), &spot); err != nil {
			return nil, 0, err
		}
		out = append(out, &spot)
	}
	return out, total, rows.Err()
}
func (r *MySQLScenicSpotRepository) UpdateScenicSpot(ctx context.Context, spot *models.ScenicSpot, expected int64) error {
	data, err := json.Marshal(spot)
	if err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx, `UPDATE scenic_spots SET name=?,status=?,merged_into=?,version=?,data_json=?,updated_at=? WHERE spot_id=? AND version=?`, spot.Name, spot.Status, spot.MergedInto, spot.Version, string(data), spot.UpdatedAt, spot.ID, expected)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrConflict
	}
	return nil
}
func (r *MySQLScenicSpotRepository) GetTrackScenicSpotLink(ctx context.Context, trackID string) (*models.TrackScenicSpotLink, error) {
	var data string
	err := r.db.QueryRowContext(ctx, `SELECT data_json FROM track_scenic_spot_links WHERE track_id=?`, trackID).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var link models.TrackScenicSpotLink
	if err := json.Unmarshal([]byte(data), &link); err != nil {
		return nil, err
	}
	return &link, nil
}
func (r *MySQLScenicSpotRepository) GetTrackScenicSpotLinks(ctx context.Context, ids []string) (map[string]*models.TrackScenicSpotLink, error) {
	out := map[string]*models.TrackScenicSpotLink{}
	ids = uniqueNonEmptyStrings(ids)
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := r.db.QueryContext(ctx, `SELECT data_json FROM track_scenic_spot_links WHERE track_id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var v models.TrackScenicSpotLink
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, err
		}
		out[v.TrackID] = &v
	}
	return out, rows.Err()
}
func (r *MySQLScenicSpotRepository) SetTrackScenicSpotLink(ctx context.Context, link *models.TrackScenicSpotLink, expected string, absent bool) error {
	data, err := json.Marshal(link)
	if err != nil {
		return err
	}
	if absent {
		_, err = r.db.ExecContext(ctx, `INSERT INTO track_scenic_spot_links(track_id,spot_id,data_json,updated_at) VALUES(?,?,?,?)`, link.TrackID, link.SpotID, string(data), link.UpdatedAt)
		return mapMySQLConflict(err)
	}
	res, err := r.db.ExecContext(ctx, `UPDATE track_scenic_spot_links SET spot_id=?,data_json=?,updated_at=? WHERE track_id=? AND spot_id=?`, link.SpotID, string(data), link.UpdatedAt, link.TrackID, expected)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrConflict
	}
	return nil
}
func (r *MySQLScenicSpotRepository) DeleteTrackScenicSpotLink(ctx context.Context, trackID, expected string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM track_scenic_spot_links WHERE track_id=? AND spot_id=?`, trackID, expected)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrConflict
	}
	return nil
}
func (r *MySQLScenicSpotRepository) ListTrackScenicSpotLinks(ctx context.Context, spotID, cursor string, limit int) ([]*models.TrackScenicSpotLink, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	q := `SELECT data_json FROM track_scenic_spot_links WHERE spot_id=?`
	args := []any{spotID}
	if cursor != "" {
		q += ` AND track_id>?`
		args = append(args, cursor)
	}
	q += ` ORDER BY track_id LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*models.TrackScenicSpotLink{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var v models.TrackScenicSpotLink
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, err
		}
		out = append(out, &v)
	}
	return out, rows.Err()
}
func (r *MySQLScenicSpotRepository) AppendScenicSpotAudit(ctx context.Context, a *models.ScenicSpotAudit) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO scenic_spot_audit_logs(audit_id,spot_id,action,operator,details,before_json,after_json,created_at) VALUES(?,?,?,?,?,?,?,?)`, a.ID, a.SpotID, a.Action, a.Operator, a.Details, a.Before, a.After, a.CreatedAt)
	return err
}
func (r *MySQLScenicSpotRepository) ListScenicSpotAudit(ctx context.Context, spotID, cursor string, limit int) ([]*models.ScenicSpotAudit, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	q := `SELECT audit_id,spot_id,action,operator,details,before_json,after_json,created_at FROM scenic_spot_audit_logs WHERE spot_id=?`
	args := []any{spotID}
	if cursor != "" {
		q += ` AND audit_id<?`
		args = append(args, cursor)
	}
	q += ` ORDER BY audit_id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*models.ScenicSpotAudit{}
	for rows.Next() {
		v := &models.ScenicSpotAudit{}
		if err := rows.Scan(&v.ID, &v.SpotID, &v.Action, &v.Operator, &v.Details, &v.Before, &v.After, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *MySQLScenicSpotRepository) IncrementScenicSpotProviderUsage(ctx context.Context, spotID, provider, day, operation string, succeeded bool) error {
	success := 0
	if succeeded {
		success = 1
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO scenic_spot_provider_usage(spot_id,provider,usage_day,operation,request_count,success_count,failure_count) VALUES(?,?,?,?,1,?,?) ON DUPLICATE KEY UPDATE request_count=request_count+1,success_count=success_count+VALUES(success_count),failure_count=failure_count+VALUES(failure_count)`, spotID, provider, day, operation, success, 1-success)
	return err
}
func (r *MySQLScenicSpotRepository) ListScenicSpotProviderUsage(ctx context.Context, fromDay, toDay string) ([]*models.ScenicSpotProviderUsage, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT spot_id,provider,DATE_FORMAT(usage_day,'%Y-%m-%d'),operation,request_count,success_count,failure_count FROM scenic_spot_provider_usage WHERE usage_day BETWEEN ? AND ?`, fromDay, toDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*models.ScenicSpotProviderUsage{}
	for rows.Next() {
		v := &models.ScenicSpotProviderUsage{}
		if err := rows.Scan(&v.SpotID, &v.Provider, &v.UsageDay, &v.Operation, &v.Requests, &v.Successes, &v.Failures); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *MySQLScenicSpotRepository) CountScenicSpotLinksBySpotIDs(ctx context.Context, spotIDs []string) (map[string]int64, error) {
	ids := uniqueNonEmptyStrings(spotIDs)
	out := make(map[string]int64, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	p := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := r.db.QueryContext(ctx, `SELECT spot_id,COUNT(*) FROM track_scenic_spot_links WHERE spot_id IN (`+p+`) GROUP BY spot_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var count int64
		if err := rows.Scan(&id, &count); err != nil {
			return nil, err
		}
		out[id] = count
	}
	return out, rows.Err()
}

func mapMySQLConflict(err error) error {
	if err == nil {
		return nil
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return ErrAlreadyExists
	}
	return err
}

const scenicSpotSchema = `CREATE TABLE IF NOT EXISTS scenic_spots (
	spot_id VARCHAR(64) NOT NULL PRIMARY KEY,
	name VARCHAR(255) NOT NULL,
	status VARCHAR(16) NOT NULL,
	merged_into VARCHAR(64) NOT NULL DEFAULT '',
	version BIGINT NOT NULL DEFAULT 1,
	data_json JSON NOT NULL,
	created_at DATETIME(6) NOT NULL,
	updated_at DATETIME(6) NOT NULL,
	KEY idx_scenic_spot_status_name (status,name),
	KEY idx_scenic_spot_merged_into (merged_into)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

const trackScenicSpotLinkSchema = `CREATE TABLE IF NOT EXISTS track_scenic_spot_links (
	track_id VARCHAR(64) NOT NULL PRIMARY KEY,
	spot_id VARCHAR(64) NOT NULL,
	data_json JSON NOT NULL,
	updated_at DATETIME(6) NOT NULL,
	KEY idx_track_scenic_spot_spot_track (spot_id,track_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

const scenicSpotAuditSchema = `CREATE TABLE IF NOT EXISTS scenic_spot_audit_logs (
	audit_id VARCHAR(64) NOT NULL PRIMARY KEY,
	spot_id VARCHAR(64) NOT NULL,
	action VARCHAR(32) NOT NULL,
	operator VARCHAR(64) NOT NULL,
	details TEXT,
	before_json JSON NULL,
	after_json JSON NULL,
	created_at DATETIME(6) NOT NULL,
	KEY idx_scenic_spot_audit_spot_id (spot_id,audit_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

const scenicSpotProviderUsageSchema = `CREATE TABLE IF NOT EXISTS scenic_spot_provider_usage (
	spot_id VARCHAR(64) NOT NULL,
	provider VARCHAR(16) NOT NULL,
	usage_day DATE NOT NULL,
	operation VARCHAR(16) NOT NULL,
	request_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
	success_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
	failure_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
	PRIMARY KEY (spot_id,provider,usage_day,operation),
	KEY idx_scenic_provider_usage_day (usage_day,provider,spot_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

var _ ScenicSpotRepository = (*MySQLScenicSpotRepository)(nil)
