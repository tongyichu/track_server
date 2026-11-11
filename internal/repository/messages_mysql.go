package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/go-sql-driver/mysql"
	"github.com/tongyichu/track_server/internal/models"
	"strings"
	"time"
)

const MessageRecordsSchema = `CREATE TABLE IF NOT EXISTS message_records (
 id VARCHAR(160) NOT NULL PRIMARY KEY, kind VARCHAR(24) NOT NULL,
 user_id BIGINT NOT NULL DEFAULT 0, peer_id BIGINT NOT NULL DEFAULT 0,
 thread_id VARCHAR(100) NOT NULL DEFAULT '', seq BIGINT NOT NULL DEFAULT 0,
 created_at DATETIME(6) NOT NULL, active BOOLEAN NOT NULL DEFAULT TRUE,
 pending BOOLEAN NOT NULL DEFAULT FALSE, flag_a BOOLEAN NOT NULL DEFAULT FALSE,
 flag_b BOOLEAN NOT NULL DEFAULT FALSE, clear_a BIGINT NOT NULL DEFAULT 0,
 clear_b BIGINT NOT NULL DEFAULT 0, targets JSON NOT NULL,
 revision BIGINT NOT NULL, payload JSON NOT NULL,
 INDEX idx_messages_kind(kind,created_at,id),
 INDEX idx_messages_user(kind,user_id,created_at,id),
 INDEX idx_messages_peer(kind,peer_id,created_at,id),
 INDEX idx_messages_thread(kind,thread_id,seq),
 INDEX idx_messages_delivery(pending,created_at,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

type MySQLMessageRepository struct{ db *sql.DB }

func (r *MySQLMessageRepository) MarkMessageDelivered(ctx context.Context, id string, revision int64) error {
	result, err := r.db.ExecContext(ctx, "UPDATE message_records SET pending=FALSE WHERE id=? AND revision=?", id, revision)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil || n > 0 {
		return err
	}
	var current int64
	if err = r.db.QueryRowContext(ctx, "SELECT revision FROM message_records WHERE id=?", id).Scan(&current); errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	} else if err != nil {
		return err
	}
	if current != revision {
		return ErrConflict
	}
	return nil
}
func NewMySQLMessageRepository(ctx context.Context, db *sql.DB) (*MySQLMessageRepository, error) {
	if _, err := db.ExecContext(ctx, MessageRecordsSchema); err != nil {
		return nil, err
	}
	for _, column := range []string{"clear_a", "clear_b"} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='message_records' AND COLUMN_NAME=?`, column).Scan(&count); err != nil {
			return nil, err
		}
		if count == 0 {
			if _, err := db.ExecContext(ctx, "ALTER TABLE message_records ADD COLUMN "+column+" BIGINT NOT NULL DEFAULT 0"); err != nil {
				var duplicate *mysql.MySQLError
				if !errors.As(err, &duplicate) || duplicate.Number != 1060 {
					return nil, err
				}
			}
		}
	}
	return &MySQLMessageRepository{db}, nil
}

const messageColumns = "m.id,m.kind,m.user_id,m.peer_id,m.thread_id,m.seq,m.created_at,m.active,m.pending,m.flag_a,m.flag_b,m.clear_a,m.clear_b,m.targets,m.revision,m.payload"

type messageScanner interface{ Scan(...any) error }

func scanMessageRecord(row messageScanner) (*models.MessageRecord, error) {
	v := &models.MessageRecord{}
	var targets, raw []byte
	err := row.Scan(&v.ID, &v.Kind, &v.UserID, &v.PeerID, &v.ThreadID, &v.Seq, &v.CreatedAt, &v.Active, &v.Pending, &v.FlagA, &v.FlagB, &v.ClearA, &v.ClearB, &targets, &v.Revision, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	v.CreatedAt = v.CreatedAt.UTC()
	v.Payload = raw
	err = json.Unmarshal(targets, &v.Targets)
	return v, err
}
func (r *MySQLMessageRepository) GetMessageRecord(ctx context.Context, id string) (*models.MessageRecord, error) {
	return scanMessageRecord(r.db.QueryRowContext(ctx, "SELECT "+messageColumns+" FROM message_records m WHERE m.id=?", id))
}
func (r *MySQLMessageRepository) GetMessageRecords(ctx context.Context, ids []string) (map[string]*models.MessageRecord, error) {
	out := map[string]*models.MessageRecord{}
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := r.db.QueryContext(ctx, "SELECT "+messageColumns+" FROM message_records m WHERE m.id IN ("+strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		v, err := scanMessageRecord(rows)
		if err != nil {
			return nil, err
		}
		out[v.ID] = v
	}
	return out, rows.Err()
}
func (r *MySQLMessageRepository) PutMessageRecord(ctx context.Context, v *models.MessageRecord, expected int64) error {
	if v.Revision != expected+1 {
		return ErrConflict
	}
	targets := v.Targets
	if targets == nil {
		targets = []string{}
	}
	raw, err := json.Marshal(targets)
	if err != nil {
		return err
	}
	args := []any{v.Kind, v.UserID, v.PeerID, v.ThreadID, v.Seq, v.CreatedAt.UTC(), v.Active, v.Pending, v.FlagA, v.FlagB, v.ClearA, v.ClearB, string(raw), v.Revision, string(v.Payload), v.ID}
	if expected == 0 {
		_, err = r.db.ExecContext(ctx, `INSERT INTO message_records(kind,user_id,peer_id,thread_id,seq,created_at,active,pending,flag_a,flag_b,clear_a,clear_b,targets,revision,payload,id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, args...)
		var duplicate *mysql.MySQLError
		if errors.As(err, &duplicate) && duplicate.Number == 1062 {
			return ErrConflict
		}
		return err
	}
	args = append(args, expected)
	result, err := r.db.ExecContext(ctx, `UPDATE message_records SET kind=?,user_id=?,peer_id=?,thread_id=?,seq=?,created_at=?,active=?,pending=?,flag_a=?,flag_b=?,clear_a=?,clear_b=?,targets=?,revision=?,payload=? WHERE id=? AND revision=?`, args...)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return ErrConflict
	}
	return err
}
func messageSQLFilter(f MessageFilter) (string, []any) {
	q := []string{"1=1"}
	args := []any{}
	if len(f.Kinds) > 0 {
		q = append(q, "m.kind IN ("+strings.TrimSuffix(strings.Repeat("?,", len(f.Kinds)), ",")+")")
		for _, k := range f.Kinds {
			args = append(args, k)
		}
	}
	if f.UserID > 0 {
		if f.EitherUser {
			q = append(q, "(m.user_id=? OR m.peer_id=?)")
			args = append(args, f.UserID, f.UserID)
		} else {
			q = append(q, "m.user_id=?")
			args = append(args, f.UserID)
		}
	}
	if f.ThreadID != "" {
		q = append(q, "m.thread_id=?")
		args = append(args, f.ThreadID)
	}
	if len(f.ThreadsAfter) > 0 {
		var parts []string
		for id, seq := range f.ThreadsAfter {
			parts = append(parts, "(m.thread_id=? AND m.seq>?)")
			args = append(args, id, seq)
		}
		q = append(q, "("+strings.Join(parts, " OR ")+")")
	}
	if !f.Before.IsZero() {
		q = append(q, "(m.created_at<? OR (m.created_at=? AND m.id<?))")
		args = append(args, f.Before.UTC(), f.Before.UTC(), f.BeforeID)
	}
	if !f.After.IsZero() {
		q = append(q, "m.created_at>?")
		args = append(args, f.After.UTC())
	}
	if !f.MaxTime.IsZero() {
		q = append(q, "m.created_at<=?")
		args = append(args, f.MaxTime.UTC())
	}
	if f.BeforeSeq > 0 {
		q = append(q, "m.seq<?")
		args = append(args, f.BeforeSeq)
	}
	if f.AfterSeq > 0 {
		q = append(q, "m.seq>?")
		args = append(args, f.AfterSeq)
	}
	if f.ActiveOnly {
		q = append(q, "m.active=TRUE")
	}
	if f.PendingOnly {
		q = append(q, "m.pending=TRUE")
	}
	if f.UnreadOnly {
		q = append(q, "m.flag_a=FALSE")
	}
	if f.HasMessages {
		q = append(q, "m.seq>0")
	}
	if f.BlockedBy > 0 {
		q = append(q, "((m.user_id=? AND m.flag_a=TRUE) OR (m.peer_id=? AND m.flag_b=TRUE))")
		args = append(args, f.BlockedBy, f.BlockedBy)
	}
	if f.VisibleTo > 0 {
		q = append(q, "((m.user_id=? AND m.flag_a=FALSE AND m.seq>m.clear_a) OR (m.peer_id=? AND m.flag_b=FALSE AND m.seq>m.clear_b))")
		args = append(args, f.VisibleTo, f.VisibleTo)
	}
	if f.Audience != "" {
		q = append(q, "(COALESCE(JSON_LENGTH(m.targets),0)=0 OR JSON_CONTAINS(m.targets,?))")
		b, _ := json.Marshal(f.Audience)
		args = append(args, string(b))
	}
	if f.ExcludeNoticeReadUser > 0 {
		q = append(q, "NOT EXISTS(SELECT 1 FROM message_records mr WHERE mr.id=CONCAT(?,m.id))")
		args = append(args, fmtMessageUser("read.notice.", f.ExcludeNoticeReadUser)+".")
	}
	return strings.Join(q, " AND "), args
}
func (r *MySQLMessageRepository) ListMessageRecords(ctx context.Context, f MessageFilter) ([]*models.MessageRecord, error) {
	where, args := messageSQLFilter(f)
	order := "DESC"
	if f.Asc {
		order = "ASC"
	}
	args = append(args, messageLimit(f))
	orderBy := "m.created_at " + order + ",m.id " + order
	if f.ThreadID != "" && len(f.Kinds) == 1 && f.Kinds[0] == "dm" {
		orderBy = "m.seq " + order
	}
	rows, err := r.db.QueryContext(ctx, "SELECT "+messageColumns+" FROM message_records m WHERE "+where+" ORDER BY "+orderBy+" LIMIT ?", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*models.MessageRecord{}
	for rows.Next() {
		v, err := scanMessageRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *MySQLMessageRepository) CountMessageRecords(ctx context.Context, f MessageFilter) (int64, error) {
	where, args := messageSQLFilter(f)
	var n int64
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM message_records m WHERE "+where, args...).Scan(&n)
	return n, err
}
func (r *MySQLMessageRepository) GroupMessageCounts(ctx context.Context, f MessageFilter) (map[string]int64, error) {
	where, args := messageSQLFilter(f)
	key := "m.thread_id"
	if f.GroupByKind {
		key = "m.kind"
	}
	rows, err := r.db.QueryContext(ctx, "SELECT "+key+",COUNT(*) FROM message_records m WHERE "+where+" GROUP BY "+key, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
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

func (r *MySQLMessageRepository) PruneMessageCredentials(ctx context.Context, cutoff time.Time) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM message_records WHERE kind='mqtt_session' AND created_at<=?", cutoff.UTC())
	return err
}
