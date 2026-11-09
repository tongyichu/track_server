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

const aiTasksSchema = `CREATE TABLE IF NOT EXISTS ai_usage_tasks (
 id VARCHAR(80) NOT NULL PRIMARY KEY, revision BIGINT NOT NULL,
 feature VARCHAR(48) NOT NULL, provider VARCHAR(40) NOT NULL, model VARCHAR(100) NOT NULL,
 object_id VARCHAR(100) NOT NULL, object_name VARCHAR(200) NOT NULL, actor VARCHAR(100) NOT NULL,
 mode VARCHAR(16) NOT NULL, status VARCHAR(24) NOT NULL, adoption VARCHAR(24) NOT NULL,
 created_at DATETIME(6) NOT NULL, payload JSON NOT NULL, summary JSON NOT NULL,
 INDEX idx_ai_created(created_at,id), INDEX idx_ai_status(status,id), INDEX idx_ai_feature(feature,created_at),
 INDEX idx_ai_model(provider,model,created_at), INDEX idx_ai_object(object_id,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`
const aiStateSchema = `CREATE TABLE IF NOT EXISTS ai_usage_state (
 state_key VARCHAR(160) NOT NULL PRIMARY KEY, revision BIGINT NOT NULL, payload JSON NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

type MySQLAIUsageRepository struct{ db *sql.DB }

func NewMySQLAIUsageRepository(ctx context.Context, db *sql.DB) (*MySQLAIUsageRepository, error) {
	for _, q := range []string{aiTasksSchema, aiStateSchema} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return nil, err
		}
	}
	return &MySQLAIUsageRepository{db}, nil
}
func aiMySQLError(err error) error {
	var e *mysql.MySQLError
	if errors.As(err, &e) && e.Number == 1062 {
		return ErrAlreadyExists
	}
	return err
}
func (r *MySQLAIUsageRepository) GetAITask(ctx context.Context, id string) (*models.AITask, error) {
	var raw []byte
	err := r.db.QueryRowContext(ctx, "SELECT payload FROM ai_usage_tasks WHERE id=?", id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var t models.AITask
	err = json.Unmarshal(raw, &t)
	return &t, err
}
func (r *MySQLAIUsageRepository) SaveAITask(ctx context.Context, t *models.AITask, expected int64) error {
	raw, err := json.Marshal(t)
	if err != nil {
		return err
	}
	summary, err := json.Marshal(aiSummary(t))
	if err != nil {
		return err
	}
	args := []any{t.Revision, t.Feature, t.Provider, t.Model, t.ObjectID, t.ObjectName, t.Actor, t.Mode, t.Status, t.Adoption, t.CreatedAt.UTC(), string(raw), string(summary)}
	if expected == 0 {
		_, err = r.db.ExecContext(ctx, `INSERT INTO ai_usage_tasks (revision,feature,provider,model,object_id,object_name,actor,mode,status,adoption,created_at,payload,summary,id) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, append(args, t.ID)...)
		return aiMySQLError(err)
	}
	res, err := r.db.ExecContext(ctx, `UPDATE ai_usage_tasks SET revision=?,feature=?,provider=?,model=?,object_id=?,object_name=?,actor=?,mode=?,status=?,adoption=?,created_at=?,payload=?,summary=? WHERE id=? AND revision=?`, append(args, t.ID, expected)...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrAlreadyExists
	}
	return err
}
func aiSQLFilter(f models.AIFilter) (string, []any) {
	var q []string
	var args []any
	for _, p := range []struct{ k, v string }{{"object_id", f.ObjectID}, {"feature", f.Feature}, {"provider", f.Provider}, {"model", f.Model}, {"status", f.Status}, {"mode", f.Mode}, {"adoption", f.Adoption}} {
		if p.v != "" {
			q = append(q, p.k+"=?")
			args = append(args, p.v)
		}
	}
	if !f.From.IsZero() {
		q = append(q, "created_at>=?")
		args = append(args, f.From.UTC())
	}
	if !f.To.IsZero() {
		q = append(q, "created_at<?")
		args = append(args, f.To.UTC())
	}
	if f.InputVersion != "" {
		q = append(q, "JSON_UNQUOTE(JSON_EXTRACT(payload,'$.input_version'))=?")
		args = append(args, f.InputVersion)
	}
	if f.Active {
		q = append(q, "status IN ('queued','running','cancelling')")
	}
	if f.Query != "" {
		q = append(q, "(LOCATE(LOWER(?),LOWER(CONCAT(id,' ',object_id,' ',object_name,' ',actor)))>0)")
		args = append(args, f.Query)
	}
	if len(q) == 0 {
		return "1=1", args
	}
	return strings.Join(q, " AND "), args
}
func (r *MySQLAIUsageRepository) ListAITasks(ctx context.Context, f models.AIFilter) (*models.AIPage, error) {
	where, args := aiSQLFilter(f)
	out := &models.AIPage{Items: []*models.AITask{}}
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_usage_tasks WHERE "+where, args...).Scan(&out.Total); err != nil {
		return nil, err
	}
	if f.Cursor != "" {
		where += " AND id<?"
		args = append(args, f.Cursor)
	}
	limit := aiLimit(f)
	args = append(args, limit+1)
	rows, err := r.db.QueryContext(ctx, "SELECT summary FROM ai_usage_tasks WHERE "+where+" ORDER BY id DESC LIMIT ?", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var b []byte
		var t models.AITask
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, &t); err != nil {
			return nil, err
		}
		out.Items = append(out.Items, &t)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = out.Items[limit-1].ID
	}
	return out, nil
}
func (r *MySQLAIUsageRepository) AIStatistics(ctx context.Context, f models.AIFilter) (*models.AIStatistics, error) {
	a := newAIAggregator()
	var first sql.NullTime
	if err := r.db.QueryRowContext(ctx, "SELECT MIN(created_at) FROM ai_usage_tasks").Scan(&first); err != nil {
		return nil, err
	}
	if first.Valid {
		v := first.Time.UTC()
		a.out.FirstRecordedAt = &v
	}
	where, args := aiSQLFilter(f)
	rows, err := r.db.QueryContext(ctx, "SELECT summary FROM ai_usage_tasks WHERE "+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var b []byte
		var t models.AITask
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, &t); err != nil {
			return nil, err
		}
		a.add(&t)
	}
	return a.result(), rows.Err()
}
func (r *MySQLAIUsageRepository) LoadAIState(ctx context.Context, key string) (int64, []byte, error) {
	var v int64
	var b []byte
	err := r.db.QueryRowContext(ctx, "SELECT revision,payload FROM ai_usage_state WHERE state_key=?", key).Scan(&v, &b)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return v, b, err
}
func (r *MySQLAIUsageRepository) SaveAIState(ctx context.Context, key string, expected int64, b []byte) error {
	if expected == 0 {
		_, err := r.db.ExecContext(ctx, "INSERT INTO ai_usage_state(state_key,revision,payload)VALUES(?,1,?)", key, string(b))
		return aiMySQLError(err)
	}
	res, err := r.db.ExecContext(ctx, "UPDATE ai_usage_state SET revision=?,payload=? WHERE state_key=? AND revision=?", expected+1, string(b), key, expected)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrAlreadyExists
	}
	return err
}

var _ AIUsageRepository = (*MySQLAIUsageRepository)(nil)
