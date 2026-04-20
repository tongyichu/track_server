package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/tongyichu/track_server/internal/models"
)

// NewMySQLRepositories wires MySQL-backed repositories and ensures schema exists.
func NewMySQLRepositories(ctx context.Context, db *sql.DB) (TrackRepository, UserRepository, CollectRepository, LoginLogRepository, error) {
	if err := ensureMySQLSchema(ctx, db); err != nil {
		return nil, nil, nil, nil, err
	}
	return NewMySQLTrackRepository(db), NewMySQLUserRepository(db), NewMySQLCollectRepository(db), NewMySQLLoginLogRepository(db), nil
}

func ensureMySQLSchema(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id BIGINT PRIMARY KEY,
			nickname VARCHAR(255) NOT NULL DEFAULT '',
			avatar_url TEXT NOT NULL,
			signature TEXT NOT NULL,
			phone VARCHAR(32) NOT NULL DEFAULT '',
			client_language VARCHAR(64) NOT NULL DEFAULT '',
			created_at DATETIME(6) NOT NULL,
			updated_at DATETIME(6) NOT NULL,
			INDEX idx_users_updated_at (updated_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		`CREATE TABLE IF NOT EXISTS track_records (
			id VARCHAR(64) NOT NULL,
			user_id BIGINT UNSIGNED NOT NULL,
			title VARCHAR(128) NOT NULL DEFAULT '',
			start_time DATETIME NOT NULL,
			end_time DATETIME NOT NULL,
			distance DECIMAL(10,2) NOT NULL DEFAULT 0.00,
			duration INT UNSIGNED NOT NULL DEFAULT 0,
			elevation_gain INT NOT NULL DEFAULT 0,
			raw_track_url VARCHAR(255) NOT NULL,
			screenshot_url VARCHAR(255) NOT NULL DEFAULT '',
			status TINYINT NOT NULL DEFAULT 1,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY (id),
			KEY idx_user_time (user_id, start_time)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		`CREATE TABLE IF NOT EXISTS track_waypoints (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			track_id VARCHAR(128) NOT NULL,
			user_id BIGINT UNSIGNED NOT NULL,
			lat DECIMAL(10,7) NOT NULL,
			lng DECIMAL(10,7) NOT NULL,
			elevation INT NOT NULL DEFAULT 0,
			node_time DATETIME NOT NULL,
			media_type TINYINT NOT NULL,
			content TEXT,
			media_urls JSON,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (id),
			KEY idx_track_id (track_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		`CREATE TABLE IF NOT EXISTS track_collects (
			user_id BIGINT NOT NULL,
			track_id VARCHAR(64) NOT NULL,
			created_at DATETIME(6) NOT NULL,
			PRIMARY KEY (user_id, track_id),
			INDEX idx_collects_track (track_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		`CREATE TABLE IF NOT EXISTS login_log (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			user_id BIGINT NOT NULL,
			login_type VARCHAR(20) NOT NULL COMMENT 'sms/wechat/apple',
			ip VARCHAR(45),
			device_id VARCHAR(128) COMMENT '设备唯一标识（客户端上报），用于判断是否新设备登录、设备维度限流和风控',
			platform VARCHAR(10) COMMENT '客户端平台：ios / android，用于按平台统计和排查问题',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
	}

	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if err := ensureMySQLUsersPhoneColumn(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLTrackScreenshotURLColumn(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLUserIDColumnsBigint(ctx, db); err != nil {
		return err
	}
	return nil
}

func ensureMySQLUsersPhoneColumn(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'phone'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check users.phone column: %w", err)
	}
	if count > 0 {
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE users ADD COLUMN phone VARCHAR(32) NOT NULL DEFAULT '' AFTER signature`)
	if err != nil {
		return fmt.Errorf("add users.phone column: %w", err)
	}
	return nil
}

func ensureMySQLTrackScreenshotURLColumn(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'track_records' AND COLUMN_NAME = 'screenshot_url'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check track_records.screenshot_url column: %w", err)
	}
	if count > 0 {
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE track_records ADD COLUMN screenshot_url VARCHAR(255) NOT NULL DEFAULT '' AFTER raw_track_url`)
	if err != nil {
		return fmt.Errorf("add track_records.screenshot_url column: %w", err)
	}
	return nil
}

func ensureMySQLUserIDColumnsBigint(ctx context.Context, db *sql.DB) error {
	columns := []struct {
		table  string
		column string
	}{
		{table: "users", column: "id"},
		{table: "track_records", column: "user_id"},
		{table: "track_waypoints", column: "user_id"},
		{table: "track_collects", column: "user_id"},
	}
	for _, item := range columns {
		if err := ensureMySQLBigintColumn(ctx, db, item.table, item.column); err != nil {
			return err
		}
	}
	return nil
}

func ensureMySQLBigintColumn(ctx context.Context, db *sql.DB, tableName, columnName string) error {
	var dataType string
	err := db.QueryRowContext(ctx, `
		SELECT DATA_TYPE
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`,
		tableName, columnName,
	).Scan(&dataType)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("check %s.%s type: %w", tableName, columnName, err)
	}
	if dataType == "bigint" {
		return nil
	}
	if err := ensureMySQLColumnConvertibleToBigint(ctx, db, tableName, columnName, dataType); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s BIGINT NOT NULL", tableName, columnName))
	if err != nil {
		return fmt.Errorf("alter %s.%s to BIGINT: %w", tableName, columnName, err)
	}
	return nil
}

func ensureMySQLColumnConvertibleToBigint(ctx context.Context, db *sql.DB, tableName, columnName, dataType string) error {
	switch dataType {
	case "varchar", "char", "text", "tinytext", "mediumtext", "longtext":
		var invalidCount int
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = '' OR %s NOT REGEXP '^[0-9]+$'", tableName, columnName, columnName)
		if err := db.QueryRowContext(ctx, query).Scan(&invalidCount); err != nil {
			return fmt.Errorf("check %s.%s convertible to BIGINT: %w", tableName, columnName, err)
		}
		if invalidCount > 0 {
			return fmt.Errorf("%s.%s contains non-numeric values, cannot migrate to BIGINT", tableName, columnName)
		}
	}
	return nil
}

// MySQLTrackRepository implements TrackRepository on top of MySQL.
type MySQLTrackRepository struct{ db *sql.DB }

// MySQLTrackWaypointRepository implements TrackWaypointRepository on top of MySQL.
type MySQLTrackWaypointRepository struct{ db *sql.DB }

func NewMySQLTrackRepository(db *sql.DB) *MySQLTrackRepository { return &MySQLTrackRepository{db: db} }

func NewMySQLTrackWaypointRepository(db *sql.DB) *MySQLTrackWaypointRepository {
	return &MySQLTrackWaypointRepository{db: db}
}

func (r *MySQLTrackRepository) Create(ctx context.Context, t *models.Track) error {
	if t.ID == "" {
		return errors.New("track id is required")
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	if t.StartTime.IsZero() {
		t.StartTime = t.CreatedAt
	}
	if t.EndTime.IsZero() {
		t.EndTime = t.StartTime
	}
	t.UpdatedAt = t.CreatedAt

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO track_records (
			id, user_id, title, start_time, end_time,
			distance, duration, elevation_gain, raw_track_url, screenshot_url, status,
			created_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.UserID, t.Title, t.StartTime, t.EndTime,
		t.Distance, t.Duration, t.ElevationGain, t.RawTrackURL, t.TrackScreenshotURL, t.Status,
		t.CreatedAt, t.UpdatedAt,
	)
	if err != nil {
		return err
	}
	return nil
}

func (r *MySQLTrackRepository) Update(ctx context.Context, t *models.Track) error {
	t.UpdatedAt = time.Now()
	if t.EndTime.IsZero() {
		t.EndTime = t.StartTime
	}

	res, err := r.db.ExecContext(ctx,
		`UPDATE track_records SET
			user_id=?, title=?, start_time=?, end_time=?,
			distance=?, duration=?, elevation_gain=?, raw_track_url=?, screenshot_url=?, status=?, updated_at=?
		WHERE id=?`,
		t.UserID, t.Title, t.StartTime, t.EndTime,
		t.Distance, t.Duration, t.ElevationGain, t.RawTrackURL, t.TrackScreenshotURL, t.Status, t.UpdatedAt,
		t.ID,
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

func (r *MySQLTrackRepository) FindByID(ctx context.Context, id string) (*models.Track, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, title, start_time, end_time,
			distance, duration, elevation_gain, raw_track_url, screenshot_url, status,
			created_at, updated_at
		FROM track_records WHERE id=?`, id)

	var t models.Track
	if err := row.Scan(
		&t.ID, &t.UserID, &t.Title, &t.StartTime, &t.EndTime,
		&t.Distance, &t.Duration, &t.ElevationGain, &t.RawTrackURL, &t.TrackScreenshotURL, &t.Status,
		&t.CreatedAt, &t.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &t, nil
}

func (r *MySQLTrackRepository) FindRunningByUserID(ctx context.Context, userID int64) (*models.Track, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id FROM track_records WHERE user_id=? AND status=? ORDER BY start_time DESC LIMIT 1`,
		userID, models.TrackStatusNormal,
	)
	var id string
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return r.FindByID(ctx, id)
}

func (r *MySQLTrackRepository) ListRecommend(ctx context.Context, _ int64, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id FROM track_records WHERE status=? ORDER BY start_time DESC LIMIT ?`,
		models.TrackStatusNormal, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	res := make([]*models.Track, 0, len(ids))
	for _, id := range ids {
		t, err := r.FindByID(ctx, id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		res = append(res, t)
	}
	return res, nil
}

func (r *MySQLTrackRepository) Search(ctx context.Context, keyword string, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	like := "%" + keyword + "%"
	rows, err := r.db.QueryContext(ctx,
		`SELECT id FROM track_records
		 WHERE status=? AND (? = '' OR title LIKE ?)
		 ORDER BY start_time DESC LIMIT ?`,
		models.TrackStatusNormal, keyword, like, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	res := make([]*models.Track, 0, len(ids))
	for _, id := range ids {
		t, err := r.FindByID(ctx, id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		res = append(res, t)
	}
	return res, nil
}

func (r *MySQLTrackWaypointRepository) Create(ctx context.Context, waypoint *models.TrackWaypoint) error {
	if waypoint.TrackID == "" {
		return errors.New("track waypoint track_id is required")
	}
	if waypoint.CreatedAt.IsZero() {
		waypoint.CreatedAt = time.Now()
	}
	var mediaURLsJSON interface{}
	if waypoint.MediaURLs != nil {
		buf, err := json.Marshal(waypoint.MediaURLs)
		if err != nil {
			return err
		}
		mediaURLsJSON = string(buf)
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO track_waypoints (
			track_id, user_id, lat, lng, elevation,
			node_time, media_type, content, media_urls, created_at
		) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		waypoint.TrackID, waypoint.UserID, waypoint.Lat, waypoint.Lng, waypoint.Elevation,
		waypoint.NodeTime, waypoint.MediaType, waypoint.Content, mediaURLsJSON, waypoint.CreatedAt,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	waypoint.ID = uint64(id)
	return nil
}

func (r *MySQLTrackWaypointRepository) ListByTrackID(ctx context.Context, trackID string) ([]*models.TrackWaypoint, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, track_id, user_id, lat, lng, elevation,
			node_time, media_type, content, media_urls, created_at
		FROM track_waypoints
		WHERE track_id=?
		ORDER BY node_time ASC, id ASC`,
		trackID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := make([]*models.TrackWaypoint, 0)
	for rows.Next() {
		var (
			waypoint     models.TrackWaypoint
			content      sql.NullString
			mediaURLsRaw sql.NullString
		)
		if err := rows.Scan(
			&waypoint.ID, &waypoint.TrackID, &waypoint.UserID, &waypoint.Lat, &waypoint.Lng, &waypoint.Elevation,
			&waypoint.NodeTime, &waypoint.MediaType, &content, &mediaURLsRaw, &waypoint.CreatedAt,
		); err != nil {
			return nil, err
		}
		if content.Valid {
			waypoint.Content = content.String
		}
		if mediaURLsRaw.Valid && mediaURLsRaw.String != "" {
			if err := json.Unmarshal([]byte(mediaURLsRaw.String), &waypoint.MediaURLs); err != nil {
				return nil, err
			}
		}
		res = append(res, &waypoint)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

// MySQLUserRepository implements UserRepository on top of MySQL.
type MySQLUserRepository struct{ db *sql.DB }

func NewMySQLUserRepository(db *sql.DB) *MySQLUserRepository { return &MySQLUserRepository{db: db} }

func (r *MySQLUserRepository) CreateIfNotExists(ctx context.Context, u *models.User) (*models.User, error) {
	// Fast path: if already exists, return it.
	if existing, err := r.FindByID(ctx, u.ID); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	now := time.Now()
	if u.CreatedAt.IsZero() {
		u.CreatedAt = now
	}
	u.UpdatedAt = u.CreatedAt

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO users (id, nickname, avatar_url, signature, phone, client_language, created_at, updated_at)
			 VALUES (?,?,?,?,?,?,?,?)
			 ON DUPLICATE KEY UPDATE id=id`,
		u.ID, u.Nickname, u.AvatarURL, u.Signature, u.Phone, u.ClientLanguage, u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return r.FindByID(ctx, u.ID)
}

func (r *MySQLUserRepository) FindByID(ctx context.Context, id int64) (*models.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, nickname, avatar_url, signature, phone, client_language, created_at, updated_at FROM users WHERE id=?`,
		id,
	)
	var u models.User
	if err := row.Scan(&u.ID, &u.Nickname, &u.AvatarURL, &u.Signature, &u.Phone, &u.ClientLanguage, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *MySQLUserRepository) Update(ctx context.Context, u *models.User) error {
	u.UpdatedAt = time.Now()
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET nickname=?, avatar_url=?, signature=?, phone=?, client_language=?, updated_at=? WHERE id=?`,
		u.Nickname, u.AvatarURL, u.Signature, u.Phone, u.ClientLanguage, u.UpdatedAt, u.ID,
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

// MySQLCollectRepository implements CollectRepository on top of MySQL.
type MySQLCollectRepository struct{ db *sql.DB }

func NewMySQLCollectRepository(db *sql.DB) *MySQLCollectRepository {
	return &MySQLCollectRepository{db: db}
}

func (r *MySQLCollectRepository) IsCollected(ctx context.Context, userID int64, trackID string) (bool, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT 1 FROM track_collects WHERE user_id=? AND track_id=? LIMIT 1`,
		userID, trackID,
	)
	var one int
	if err := row.Scan(&one); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *MySQLCollectRepository) AddCollect(ctx context.Context, userID int64, trackID string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO track_collects (user_id, track_id, created_at)
		 VALUES (?,?,?)
		 ON DUPLICATE KEY UPDATE created_at=created_at`,
		userID, trackID, time.Now(),
	)
	if err != nil {
		return err
	}
	return nil
}

func (r *MySQLCollectRepository) RemoveCollect(ctx context.Context, userID int64, trackID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM track_collects WHERE user_id=? AND track_id=?`, userID, trackID)
	if err != nil {
		return err
	}
	return nil
}

// MySQLLoginLogRepository implements LoginLogRepository on top of MySQL.
type MySQLLoginLogRepository struct{ db *sql.DB }

func NewMySQLLoginLogRepository(db *sql.DB) *MySQLLoginLogRepository {
	return &MySQLLoginLogRepository{db: db}
}

func (r *MySQLLoginLogRepository) Create(ctx context.Context, log *models.LoginLog) error {
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO login_log (user_id, login_type, ip, device_id, platform, created_at)
		 VALUES (?,?,?,?,?,?)`,
		log.UserID, log.LoginType, log.IP, log.DeviceID, log.Platform, log.CreatedAt,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	log.ID = id
	return nil
}

func (r *MySQLLoginLogRepository) ListByUserID(ctx context.Context, userID int64, limit int) ([]*models.LoginLog, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, login_type, ip, device_id, platform, created_at
		 FROM login_log WHERE user_id=? ORDER BY created_at DESC, id DESC LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := make([]*models.LoginLog, 0, limit)
	for rows.Next() {
		var item models.LoginLog
		if err := rows.Scan(&item.ID, &item.UserID, &item.LoginType, &item.IP, &item.DeviceID, &item.Platform, &item.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

// OpenMySQL opens a MySQL connection pool with some safe defaults.
func OpenMySQL(dsn string) (*sql.DB, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	// Ensure time parsing works across the codebase.
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	if _, ok := cfg.Params["parseTime"]; !ok {
		cfg.Params["parseTime"] = "true"
	}
	if _, ok := cfg.Params["charset"]; !ok {
		cfg.Params["charset"] = "utf8mb4"
	}
	if _, ok := cfg.Params["collation"]; !ok {
		cfg.Params["collation"] = "utf8mb4_unicode_ci"
	}

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}
	// Conservative defaults; caller can tune if needed.
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	return db, nil
}
