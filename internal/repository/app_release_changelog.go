package repository

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

const appReleaseChangelogSchema = `CREATE TABLE IF NOT EXISTS app_release_changelogs (
	id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
	platform VARCHAR(16) NOT NULL COMMENT 'android / ios',
	version_name VARCHAR(32) NOT NULL,
	version_code INT UNSIGNED NOT NULL,
	release_notes TEXT,
	operator_name VARCHAR(64) NOT NULL DEFAULT '',
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
	PRIMARY KEY (id),
	UNIQUE KEY uk_platform_version_code (platform, version_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='App 版本更新日志（独立于发布记录留存）';`

// MySQLAppReleaseChangelogRepository implements AppReleaseChangelogRepository on top of MySQL.
type MySQLAppReleaseChangelogRepository struct{ db *sql.DB }

func NewMySQLAppReleaseChangelogRepository(ctx context.Context, db *sql.DB) (*MySQLAppReleaseChangelogRepository, error) {
	if _, err := db.ExecContext(ctx, appReleaseChangelogSchema); err != nil {
		return nil, err
	}
	return &MySQLAppReleaseChangelogRepository{db: db}, nil
}

func (r *MySQLAppReleaseChangelogRepository) Upsert(ctx context.Context, entry *models.AppReleaseChangelog) error {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO app_release_changelogs (platform, version_name, version_code, release_notes, operator_name)
		VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE version_name=VALUES(version_name), release_notes=VALUES(release_notes), operator_name=VALUES(operator_name)`,
		string(entry.Platform), entry.VersionName, entry.VersionCode, entry.ReleaseNotes, entry.OperatorName,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err == nil && id > 0 {
		entry.ID = id
	}
	return nil
}

func (r *MySQLAppReleaseChangelogRepository) List(ctx context.Context, platform models.AppReleasePlatform) ([]*models.AppReleaseChangelog, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, platform, version_name, version_code, release_notes, operator_name, created_at, updated_at
		FROM app_release_changelogs WHERE platform=? ORDER BY version_code DESC`,
		string(platform),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*models.AppReleaseChangelog, 0)
	for rows.Next() {
		var item models.AppReleaseChangelog
		var notes sql.NullString
		if err := rows.Scan(
			&item.ID, &item.Platform, &item.VersionName, &item.VersionCode,
			&notes, &item.OperatorName, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.ReleaseNotes = notes.String
		items = append(items, &item)
	}
	return items, rows.Err()
}

// InMemoryAppReleaseChangelogRepository implements AppReleaseChangelogRepository in memory.
type InMemoryAppReleaseChangelogRepository struct {
	mu      sync.Mutex
	nextID  int64
	entries map[string]*models.AppReleaseChangelog
}

func NewInMemoryAppReleaseChangelogRepository() *InMemoryAppReleaseChangelogRepository {
	return &InMemoryAppReleaseChangelogRepository{
		nextID:  1,
		entries: make(map[string]*models.AppReleaseChangelog),
	}
}

func appReleaseChangelogKey(platform models.AppReleasePlatform, versionCode int64) string {
	return string(platform) + "#" + strconv.FormatInt(versionCode, 10)
}

func (r *InMemoryAppReleaseChangelogRepository) Upsert(_ context.Context, entry *models.AppReleaseChangelog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := appReleaseChangelogKey(entry.Platform, entry.VersionCode)
	now := time.Now()
	if existing, ok := r.entries[key]; ok {
		existing.VersionName = entry.VersionName
		existing.ReleaseNotes = entry.ReleaseNotes
		existing.OperatorName = entry.OperatorName
		existing.UpdatedAt = now
		entry.ID = existing.ID
		return nil
	}
	stored := *entry
	stored.ID = r.nextID
	stored.CreatedAt = now
	stored.UpdatedAt = now
	r.nextID++
	r.entries[key] = &stored
	entry.ID = stored.ID
	return nil
}

func (r *InMemoryAppReleaseChangelogRepository) List(_ context.Context, platform models.AppReleasePlatform) ([]*models.AppReleaseChangelog, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]*models.AppReleaseChangelog, 0, len(r.entries))
	for _, entry := range r.entries {
		if entry.Platform != platform {
			continue
		}
		copied := *entry
		items = append(items, &copied)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].VersionCode > items[j].VersionCode })
	return items, nil
}

// MongoAppReleaseChangelogRepository 与 MongoAppReleaseRepository 一致，暂为未实现 stub。
type MongoAppReleaseChangelogRepository struct{}

func NewMongoAppReleaseChangelogRepository() *MongoAppReleaseChangelogRepository {
	return &MongoAppReleaseChangelogRepository{}
}

func (r *MongoAppReleaseChangelogRepository) Upsert(context.Context, *models.AppReleaseChangelog) error {
	return errors.New("mongo app release changelog repository not implemented")
}

func (r *MongoAppReleaseChangelogRepository) List(context.Context, models.AppReleasePlatform) ([]*models.AppReleaseChangelog, error) {
	return nil, errors.New("mongo app release changelog repository not implemented")
}
