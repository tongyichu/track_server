package repository

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MySQLAnalyticsRepository implements AnalyticsRepository on top of MySQL.
type MySQLAnalyticsRepository struct{ db *sql.DB }

func NewMySQLAnalyticsRepository(db *sql.DB) *MySQLAnalyticsRepository {
	return &MySQLAnalyticsRepository{db: db}
}

func (r *MySQLAnalyticsRepository) CreateSyncSummary(ctx context.Context, summary *models.AnalyticsSyncSummary) error {
	if summary == nil {
		return nil
	}
	if summary.CreatedAt.IsZero() {
		summary.CreatedAt = time.Now()
	}
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO analytics_sync_summaries (
			job_name, status, started_at, ended_at, duration_ms,
			scanned_files, uploaded_files, failed_files, total_bytes,
			oss_prefix, files_json, error_message, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		summary.JobName, summary.Status, summary.StartedAt, summary.EndedAt, summary.DurationMS,
		summary.ScannedFiles, summary.UploadedFiles, summary.FailedFiles, summary.TotalBytes,
		summary.OSSPrefix, summary.FilesJSON, summary.ErrorMessage, summary.CreatedAt,
	)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	summary.ID = id
	return nil
}

func (r *MySQLAnalyticsRepository) ListSyncSummaries(ctx context.Context, status string, limit, offset int) ([]*models.AnalyticsSyncSummary, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	status = strings.TrimSpace(status)
	query := `
		SELECT id, job_name, status, started_at, ended_at, duration_ms,
			scanned_files, uploaded_files, failed_files, total_bytes,
			oss_prefix, COALESCE(files_json, JSON_ARRAY()), COALESCE(error_message, ''), created_at
		FROM analytics_sync_summaries`
	args := []any{}
	if status != "" {
		query += ` WHERE status=?`
		args = append(args, status)
	}
	query += ` ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAnalyticsSyncSummaries(rows)
}

func (r *MySQLAnalyticsRepository) CountSyncSummaries(ctx context.Context, status string) (int64, error) {
	status = strings.TrimSpace(status)
	if status == "" {
		var n int64
		err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM analytics_sync_summaries`).Scan(&n)
		return n, err
	}
	var n int64
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM analytics_sync_summaries WHERE status=?`, status).Scan(&n)
	return n, err
}

func scanAnalyticsSyncSummaries(rows *sql.Rows) ([]*models.AnalyticsSyncSummary, error) {
	items := make([]*models.AnalyticsSyncSummary, 0)
	for rows.Next() {
		var item models.AnalyticsSyncSummary
		if err := rows.Scan(
			&item.ID, &item.JobName, &item.Status, &item.StartedAt, &item.EndedAt, &item.DurationMS,
			&item.ScannedFiles, &item.UploadedFiles, &item.FailedFiles, &item.TotalBytes,
			&item.OSSPrefix, &item.FilesJSON, &item.ErrorMessage, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, &item)
	}
	return items, rows.Err()
}

// MongoAnalyticsRepository implements AnalyticsRepository on top of MongoDB.
type MongoAnalyticsRepository struct{ collection *mongo.Collection }

func NewMongoAnalyticsRepository(collection *mongo.Collection) *MongoAnalyticsRepository {
	return &MongoAnalyticsRepository{collection: collection}
}

func (r *MongoAnalyticsRepository) CreateSyncSummary(ctx context.Context, summary *models.AnalyticsSyncSummary) error {
	if summary == nil {
		return nil
	}
	if summary.CreatedAt.IsZero() {
		summary.CreatedAt = time.Now()
	}
	_, err := r.collection.InsertOne(ctx, summary)
	return err
}

func (r *MongoAnalyticsRepository) ListSyncSummaries(ctx context.Context, status string, limit, offset int) ([]*models.AnalyticsSyncSummary, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	filter := bson.M{}
	if status = strings.TrimSpace(status); status != "" {
		filter["status"] = status
	}
	cur, err := r.collection.Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "started_at", Value: -1}, {Key: "id", Value: -1}}).
		SetLimit(int64(limit)).
		SetSkip(int64(offset)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	items := make([]*models.AnalyticsSyncSummary, 0)
	for cur.Next(ctx) {
		var item models.AnalyticsSyncSummary
		if err := cur.Decode(&item); err != nil {
			return nil, err
		}
		items = append(items, &item)
	}
	return items, cur.Err()
}

func (r *MongoAnalyticsRepository) CountSyncSummaries(ctx context.Context, status string) (int64, error) {
	filter := bson.M{}
	if status = strings.TrimSpace(status); status != "" {
		filter["status"] = status
	}
	return r.collection.CountDocuments(ctx, filter)
}

// InMemoryAnalyticsRepository implements AnalyticsRepository for tests and fallback mode.
type InMemoryAnalyticsRepository struct {
	mu        sync.Mutex
	nextID    int64
	summaries []*models.AnalyticsSyncSummary
}

func NewInMemoryAnalyticsRepository() *InMemoryAnalyticsRepository {
	return &InMemoryAnalyticsRepository{nextID: 1}
}

func (r *InMemoryAnalyticsRepository) CreateSyncSummary(_ context.Context, summary *models.AnalyticsSyncSummary) error {
	if summary == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *summary
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now()
	}
	cp.ID = r.nextID
	r.nextID++
	r.summaries = append(r.summaries, &cp)
	summary.ID = cp.ID
	return nil
}

func (r *InMemoryAnalyticsRepository) Summaries() []*models.AnalyticsSyncSummary {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*models.AnalyticsSyncSummary, 0, len(r.summaries))
	for _, item := range r.summaries {
		if item == nil {
			continue
		}
		cp := *item
		out = append(out, &cp)
	}
	return out
}

func (r *InMemoryAnalyticsRepository) ListSyncSummaries(_ context.Context, status string, limit, offset int) ([]*models.AnalyticsSyncSummary, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	status = strings.TrimSpace(status)
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]*models.AnalyticsSyncSummary, 0, len(r.summaries))
	for _, item := range r.summaries {
		if item == nil {
			continue
		}
		if status != "" && item.Status != status {
			continue
		}
		cp := *item
		items = append(items, &cp)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].StartedAt.Equal(items[j].StartedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].StartedAt.After(items[j].StartedAt)
	})
	if offset >= len(items) {
		return []*models.AnalyticsSyncSummary{}, nil
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end], nil
}

func (r *InMemoryAnalyticsRepository) CountSyncSummaries(_ context.Context, status string) (int64, error) {
	status = strings.TrimSpace(status)
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for _, item := range r.summaries {
		if item == nil {
			continue
		}
		if status == "" || item.Status == status {
			n++
		}
	}
	return n, nil
}

var _ AnalyticsRepository = (*MySQLAnalyticsRepository)(nil)
var _ AnalyticsRepository = (*MongoAnalyticsRepository)(nil)
var _ AnalyticsRepository = (*InMemoryAnalyticsRepository)(nil)

var errAnalyticsSummaryNotFound = errors.New("analytics sync summary not found")
