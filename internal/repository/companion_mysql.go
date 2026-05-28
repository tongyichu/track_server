package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

// MySQLCompanionRepository implements CompanionRepository on top of MySQL.
type MySQLCompanionRepository struct{ db *sql.DB }

func NewMySQLCompanionRepository(db *sql.DB) *MySQLCompanionRepository {
	return &MySQLCompanionRepository{db: db}
}

func (r *MySQLCompanionRepository) CreateSession(ctx context.Context, session *models.CompanionSession) error {
	if session == nil || session.SessionID == "" {
		return errors.New("session_id is required")
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}
	if session.StartedAt.IsZero() {
		session.StartedAt = session.CreatedAt
	}
	session.UpdatedAt = session.CreatedAt
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO companion_sessions (
			session_id, owner_user_id, status, join_token, join_token_expire_at,
			title, track_type, locate_addr, max_members, started_at, ended_at, created_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		session.SessionID,
		session.OwnerUserID,
		session.Status,
		session.JoinToken,
		session.JoinTokenExpireAt,
		session.Title,
		session.TrackType,
		session.LocateAddr,
		session.MaxMembers,
		session.StartedAt,
		nullableTimeValue(session.EndedAt),
		session.CreatedAt,
		session.UpdatedAt,
	)
	if err != nil {
		return err
	}
	return nil
}

func (r *MySQLCompanionRepository) UpdateSession(ctx context.Context, session *models.CompanionSession) error {
	if session == nil || session.SessionID == "" {
		return errors.New("session_id is required")
	}
	session.UpdatedAt = time.Now()
	res, err := r.db.ExecContext(ctx,
		`UPDATE companion_sessions
		 SET owner_user_id=?, status=?, join_token=?, join_token_expire_at=?, title=?, track_type=?, locate_addr=?, max_members=?, started_at=?, ended_at=?, updated_at=?
		 WHERE session_id=?`,
		session.OwnerUserID,
		session.Status,
		session.JoinToken,
		session.JoinTokenExpireAt,
		session.Title,
		session.TrackType,
		session.LocateAddr,
		session.MaxMembers,
		session.StartedAt,
		nullableTimeValue(session.EndedAt),
		session.UpdatedAt,
		session.SessionID,
	)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MySQLCompanionRepository) FindSessionByID(ctx context.Context, sessionID string) (*models.CompanionSession, error) {
	row := r.db.QueryRowContext(ctx, companionSessionSelectSQL()+` WHERE session_id=?`, sessionID)
	return scanCompanionSessionRow(row)
}

func (r *MySQLCompanionRepository) FindSessionByJoinToken(ctx context.Context, joinToken string) (*models.CompanionSession, error) {
	row := r.db.QueryRowContext(ctx, companionSessionSelectSQL()+` WHERE join_token=?`, joinToken)
	return scanCompanionSessionRow(row)
}

func (r *MySQLCompanionRepository) FindActiveSessionByUserID(ctx context.Context, userID int64) (*models.CompanionSession, error) {
	row := r.db.QueryRowContext(ctx,
		companionSessionSelectSQL()+` INNER JOIN companion_session_members m ON m.session_id = companion_sessions.session_id
		 WHERE m.user_id=? AND m.member_status=? AND companion_sessions.status=?
		 ORDER BY companion_sessions.created_at DESC LIMIT 1`,
		userID, models.CompanionMemberStatusJoined, models.CompanionSessionStatusActive,
	)
	return scanCompanionSessionRow(row)
}

func (r *MySQLCompanionRepository) ListSessionsByUserID(ctx context.Context, userID int64, cursor *models.CompanionSessionListCursor, limit int) ([]*models.CompanionSession, error) {
	query := companionSessionSelectSQL() + ` WHERE EXISTS (
			SELECT 1 FROM companion_session_members m
			 WHERE m.session_id = companion_sessions.session_id AND m.user_id=?
		)`
	args := []any{userID}
	if cursor != nil {
		query += ` AND (companion_sessions.started_at < ? OR (companion_sessions.started_at = ? AND companion_sessions.session_id < ?))`
		args = append(args, cursor.StartedAt, cursor.StartedAt, cursor.SessionID)
	}
	query += ` ORDER BY companion_sessions.started_at DESC, companion_sessions.session_id DESC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*models.CompanionSession, 0)
	for rows.Next() {
		item, err := scanCompanionSession(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *MySQLCompanionRepository) CountSessionsByUserID(ctx context.Context, userID int64) (int64, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT session_id) FROM companion_session_members WHERE user_id=?`,
		userID,
	)
	var count int64
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *MySQLCompanionRepository) UpsertMember(ctx context.Context, member *models.CompanionSessionMember) error {
	if member == nil || member.SessionID == "" || member.UserID <= 0 {
		return errors.New("invalid companion member")
	}
	if member.JoinedAt.IsZero() {
		member.JoinedAt = time.Now()
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO companion_session_members (
			session_id, user_id, role, member_status, presence_status,
			joined_at, left_at, last_seen_at, mqtt_client_id, mqtt_principal
		) VALUES (?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
			role=VALUES(role),
			member_status=VALUES(member_status),
			presence_status=VALUES(presence_status),
			joined_at=VALUES(joined_at),
			left_at=VALUES(left_at),
			last_seen_at=VALUES(last_seen_at),
			mqtt_client_id=VALUES(mqtt_client_id),
			mqtt_principal=VALUES(mqtt_principal)`,
		member.SessionID,
		member.UserID,
		member.Role,
		member.MemberStatus,
		member.PresenceStatus,
		member.JoinedAt,
		nullableTimeValue(member.LeftAt),
		nullableTimeValue(member.LastSeenAt),
		member.MQTTClientID,
		member.MQTTPrincipal,
	)
	return err
}

func (r *MySQLCompanionRepository) FindMember(ctx context.Context, sessionID string, userID int64) (*models.CompanionSessionMember, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT session_id, user_id, role, member_status, presence_status, joined_at, left_at, last_seen_at, mqtt_client_id, mqtt_principal
		 FROM companion_session_members WHERE session_id=? AND user_id=?`,
		sessionID, userID,
	)
	return scanCompanionMemberRow(row)
}

func (r *MySQLCompanionRepository) ListMembers(ctx context.Context, sessionID string) ([]*models.CompanionSessionMember, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT session_id, user_id, role, member_status, presence_status, joined_at, left_at, last_seen_at, mqtt_client_id, mqtt_principal
		 FROM companion_session_members WHERE session_id=?
		 ORDER BY CASE role WHEN 'owner' THEN 0 ELSE 1 END, joined_at ASC, user_id ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*models.CompanionSessionMember, 0)
	for rows.Next() {
		item, err := scanCompanionMember(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *MySQLCompanionRepository) CountMembersByStatus(ctx context.Context, sessionID string, status models.CompanionMemberStatus) (int64, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM companion_session_members WHERE session_id=? AND member_status=?`,
		sessionID, status,
	)
	var count int64
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *MySQLCompanionRepository) UpsertPosition(ctx context.Context, position *models.CompanionLivePosition) error {
	if position == nil || position.SessionID == "" || position.UserID <= 0 {
		return errors.New("invalid companion position")
	}
	now := time.Now()
	if position.CreatedAt.IsZero() {
		position.CreatedAt = now
	}
	position.UpdatedAt = now
	if position.Source == "" {
		position.Source = "http"
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO companion_live_positions (
			session_id, user_id, track_id, latitude, longitude, coordinate_system,
			speed_kmh, heading, accuracy_m, altitude, recorded_at, seq, source, created_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
			track_id=VALUES(track_id),
			latitude=VALUES(latitude),
			longitude=VALUES(longitude),
			coordinate_system=VALUES(coordinate_system),
			speed_kmh=VALUES(speed_kmh),
			heading=VALUES(heading),
			accuracy_m=VALUES(accuracy_m),
			altitude=VALUES(altitude),
			recorded_at=VALUES(recorded_at),
			seq=VALUES(seq),
			source=VALUES(source),
			updated_at=VALUES(updated_at)`,
		position.SessionID,
		position.UserID,
		nullableStringValue(position.TrackID),
		position.Latitude,
		position.Longitude,
		position.CoordinateSystem,
		position.SpeedKmh,
		position.Heading,
		position.AccuracyM,
		position.Altitude,
		position.RecordedAt,
		position.Seq,
		position.Source,
		position.CreatedAt,
		position.UpdatedAt,
	)
	return err
}

func (r *MySQLCompanionRepository) ListPositions(ctx context.Context, sessionID string) ([]*models.CompanionLivePosition, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT session_id, user_id, track_id, latitude, longitude, coordinate_system, speed_kmh, heading, accuracy_m, altitude, recorded_at, seq, source, created_at, updated_at
		 FROM companion_live_positions WHERE session_id=? ORDER BY recorded_at DESC, user_id ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*models.CompanionLivePosition, 0)
	for rows.Next() {
		item, err := scanCompanionPosition(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *MySQLCompanionRepository) DeletePositions(ctx context.Context, sessionID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM companion_live_positions WHERE session_id=?`, sessionID)
	return err
}

func (r *MySQLCompanionRepository) InsertDanmaku(ctx context.Context, d *models.CompanionDanmaku) error {
	if d == nil || d.SessionID == "" || d.UserID <= 0 {
		return errors.New("invalid companion danmaku")
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now()
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO companion_danmakus (session_id, user_id, content, created_at) VALUES (?,?,?,?)`,
		d.SessionID, d.UserID, d.Content, d.CreatedAt,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	d.ID = id
	return nil
}

func (r *MySQLCompanionRepository) CountDanmakuByMemberSince(ctx context.Context, sessionID string, userID int64, since time.Time) (int64, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM companion_danmakus WHERE session_id=? AND user_id=? AND created_at>=?`,
		sessionID, userID, since,
	)
	var count int64
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func companionSessionSelectSQL() string {
	return `SELECT companion_sessions.session_id, companion_sessions.owner_user_id, companion_sessions.status, companion_sessions.join_token, companion_sessions.join_token_expire_at, companion_sessions.title, companion_sessions.track_type, companion_sessions.locate_addr, companion_sessions.max_members, companion_sessions.started_at, companion_sessions.ended_at, companion_sessions.created_at, companion_sessions.updated_at FROM companion_sessions`
}

type companionSessionRowScanner interface { Scan(dest ...any) error }

func scanCompanionSession(row companionSessionRowScanner) (*models.CompanionSession, error) {
	var (
		item    models.CompanionSession
		endedAt sql.NullTime
	)
	if err := row.Scan(&item.SessionID, &item.OwnerUserID, &item.Status, &item.JoinToken, &item.JoinTokenExpireAt, &item.Title, &item.TrackType, &item.LocateAddr, &item.MaxMembers, &item.StartedAt, &endedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	if endedAt.Valid {
		item.EndedAt = endedAt.Time
	}
	return &item, nil
}

func scanCompanionSessionRow(row *sql.Row) (*models.CompanionSession, error) {
	item, err := scanCompanionSession(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return item, nil
}

type companionMemberRowScanner interface { Scan(dest ...any) error }

func scanCompanionMember(row companionMemberRowScanner) (*models.CompanionSessionMember, error) {
	var (
		item          models.CompanionSessionMember
		leftAt        sql.NullTime
		lastSeenAt    sql.NullTime
		mqttClientID  sql.NullString
		mqttPrincipal sql.NullString
	)
	if err := row.Scan(&item.SessionID, &item.UserID, &item.Role, &item.MemberStatus, &item.PresenceStatus, &item.JoinedAt, &leftAt, &lastSeenAt, &mqttClientID, &mqttPrincipal); err != nil {
		return nil, err
	}
	if leftAt.Valid { item.LeftAt = leftAt.Time }
	if lastSeenAt.Valid { item.LastSeenAt = lastSeenAt.Time }
	if mqttClientID.Valid { item.MQTTClientID = mqttClientID.String }
	if mqttPrincipal.Valid { item.MQTTPrincipal = mqttPrincipal.String }
	return &item, nil
}

func scanCompanionMemberRow(row *sql.Row) (*models.CompanionSessionMember, error) {
	item, err := scanCompanionMember(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return item, nil
}

type companionPositionRowScanner interface { Scan(dest ...any) error }

func scanCompanionPosition(row companionPositionRowScanner) (*models.CompanionLivePosition, error) {
	var (
		item    models.CompanionLivePosition
		trackID sql.NullString
	)
	if err := row.Scan(&item.SessionID, &item.UserID, &trackID, &item.Latitude, &item.Longitude, &item.CoordinateSystem, &item.SpeedKmh, &item.Heading, &item.AccuracyM, &item.Altitude, &item.RecordedAt, &item.Seq, &item.Source, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	if trackID.Valid {
		item.TrackID = trackID.String
	}
	return &item, nil
}
