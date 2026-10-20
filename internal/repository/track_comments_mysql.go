package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/tongyichu/track_server/internal/models"
	"strings"
	"time"
)

const commentSchema = `CREATE TABLE IF NOT EXISTS track_comments (
 id VARCHAR(64) NOT NULL PRIMARY KEY, track_id VARCHAR(64) NOT NULL, root_id VARCHAR(64) NOT NULL DEFAULT '', reply_to_id VARCHAR(64) NOT NULL DEFAULT '', user_id BIGINT NOT NULL,
 client_id VARCHAR(64) NOT NULL, request_hash VARCHAR(64) NOT NULL, content TEXT NOT NULL, media_json TEXT NOT NULL,
 deleted TINYINT NOT NULL DEFAULT 0, reply_count BIGINT NOT NULL DEFAULT 0, like_count BIGINT NOT NULL DEFAULT 0, dislike_count BIGINT NOT NULL DEFAULT 0,
 created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL,
 UNIQUE KEY uk_comment_client(user_id,client_id),
 KEY idx_comment_page(track_id,root_id,created_at,id),
 KEY idx_comment_author(root_id,user_id,deleted,created_at,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS track_comment_stats (track_id VARCHAR(64) NOT NULL PRIMARY KEY,total_count BIGINT NOT NULL DEFAULT 0) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS track_comment_reactions (comment_id VARCHAR(64) NOT NULL,user_id BIGINT NOT NULL,reaction TINYINT NOT NULL,PRIMARY KEY(comment_id,user_id), KEY idx_comment_likers(comment_id,reaction,user_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS track_comment_images (
 id VARCHAR(64) NOT NULL PRIMARY KEY, track_id VARCHAR(64) NOT NULL,user_id BIGINT NOT NULL,comment_id VARCHAR(64) NOT NULL DEFAULT '',object_key VARCHAR(1024) NOT NULL,oss_url VARCHAR(2048) NOT NULL,
 width INT NOT NULL,height INT NOT NULL,byte_size BIGINT NOT NULL,created_at DATETIME(6) NOT NULL,
 KEY idx_comment_image_owner(user_id,created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`

type MySQLCommentRepository struct{ db *sql.DB }

func NewMySQLCommentRepository(db *sql.DB) *MySQLCommentRepository {
	return &MySQLCommentRepository{db}
}
func ensureMySQLCommentLikerIndex(ctx context.Context, db *sql.DB) error {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='track_comment_reactions' AND INDEX_NAME='idx_comment_likers'`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := db.ExecContext(ctx, `ALTER TABLE track_comment_reactions ADD INDEX idx_comment_likers(comment_id,reaction,user_id)`)
	var databaseError *mysqlDriver.MySQLError
	if errors.As(err, &databaseError) && databaseError.Number == 1061 {
		return nil
	}
	return err
}

type commentSQLQuery interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}
type sqlCommentTx struct {
	ctx context.Context
	q   commentSQLQuery
}

const commentColumns = "id,track_id,root_id,reply_to_id,user_id,client_id,request_hash,content,media_json,deleted,reply_count,like_count,dislike_count,created_at,updated_at"

func scanComment(row scanner) (*models.TrackComment, error) {
	c := &models.TrackComment{}
	var images string
	err := row.Scan(&c.ID, &c.TrackID, &c.RootID, &c.ReplyToID, &c.UserID, &c.ClientID, &c.RequestHash, &c.Content, &images, &c.Deleted, &c.ReplyCount, &c.LikeCount, &c.DislikeCount, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var media struct {
		IDs    []string
		Images []models.CommentMedia
	}
	err = json.Unmarshal([]byte(images), &media)
	c.ImageIDs = media.IDs
	c.Images = media.Images
	return c, err
}
func (t sqlCommentTx) Find(id string) (*models.TrackComment, error) {
	query := "SELECT " + commentColumns + " FROM track_comments WHERE id=?"
	if _, transactional := t.q.(*sql.Tx); transactional {
		query += " FOR UPDATE"
	}
	return scanComment(t.q.QueryRowContext(t.ctx, query, id))
}
func (t sqlCommentTx) FindClient(user int64, key string) (*models.TrackComment, error) {
	return scanComment(t.q.QueryRowContext(t.ctx, "SELECT "+commentColumns+" FROM track_comments WHERE user_id=? AND client_id=?", user, key))
}
func (t sqlCommentTx) Save(c *models.TrackComment) error {
	media, _ := json.Marshal(struct {
		IDs    []string
		Images []models.CommentMedia
	}{c.ImageIDs, c.Images})
	_, found := t.Find(c.ID)
	if found == nil {
		_, err := t.q.ExecContext(t.ctx, `UPDATE track_comments SET content=?,media_json=?,deleted=?,reply_count=?,like_count=?,dislike_count=?,updated_at=? WHERE id=?`, c.Content, string(media), c.Deleted, c.ReplyCount, c.LikeCount, c.DislikeCount, c.UpdatedAt, c.ID)
		return err
	}
	if !errors.Is(found, ErrNotFound) {
		return found
	}
	_, err := t.q.ExecContext(t.ctx, `INSERT INTO track_comments (`+commentColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, c.ID, c.TrackID, c.RootID, c.ReplyToID, c.UserID, c.ClientID, c.RequestHash, c.Content, string(media), c.Deleted, c.ReplyCount, c.LikeCount, c.DislikeCount, c.CreatedAt, c.UpdatedAt)
	return err
}

func (t sqlCommentTx) Image(id string) (*models.CommentImage, error) {
	image := &models.CommentImage{}
	err := t.q.QueryRowContext(t.ctx, `SELECT id,track_id,user_id,comment_id,object_key,oss_url,width,height,byte_size,created_at FROM track_comment_images WHERE id=?`, id).Scan(&image.ID, &image.TrackID, &image.UserID, &image.CommentID, &image.ObjectKey, &image.OSSURL, &image.Width, &image.Height, &image.ByteSize, &image.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return image, err
}
func (t sqlCommentTx) SaveImage(i *models.CommentImage) error {
	_, err := t.q.ExecContext(t.ctx, `INSERT INTO track_comment_images (id,track_id,user_id,comment_id,object_key,oss_url,width,height,byte_size,created_at) VALUES (?,?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE comment_id=VALUES(comment_id)`, i.ID, i.TrackID, i.UserID, i.CommentID, i.ObjectKey, i.OSSURL, i.Width, i.Height, i.ByteSize, i.CreatedAt)
	return err
}
func (t sqlCommentTx) AddTotal(track string, delta int64) error {
	_, err := t.q.ExecContext(t.ctx, `UPDATE track_comment_stats SET total_count=total_count+? WHERE track_id=?`, delta, track)
	return err
}
func (t sqlCommentTx) Reaction(id string, user int64) (int, error) {
	var value int
	err := t.q.QueryRowContext(t.ctx, `SELECT reaction FROM track_comment_reactions WHERE comment_id=? AND user_id=?`, id, user).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return value, err
}
func (t sqlCommentTx) SetReaction(id string, user int64, value int) error {
	_, err := t.q.ExecContext(t.ctx, `INSERT INTO track_comment_reactions(comment_id,user_id,reaction) VALUES(?,?,?) ON DUPLICATE KEY UPDATE reaction=VALUES(reaction)`, id, user, value)
	return err
}
func (r *MySQLCommentRepository) Apply(ctx context.Context, m CommentMutation) (*models.TrackComment, error) {
	for attempt := 0; ; attempt++ {
		result, err := r.applyOnce(ctx, m)
		var databaseError *mysqlDriver.MySQLError
		if err == nil || attempt == 3 || !errors.As(err, &databaseError) || (databaseError.Number != 1213 && databaseError.Number != 1205) {
			return result, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 10 * time.Millisecond):
		}
	}
}
func (r *MySQLCommentRepository) applyOnce(ctx context.Context, m CommentMutation) (*models.TrackComment, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if m.Action != "reaction" {
		if _, err = tx.ExecContext(ctx, `INSERT INTO track_comment_stats(track_id,total_count) VALUES(?,0) ON DUPLICATE KEY UPDATE total_count=total_count`, m.TrackID); err != nil {
			return nil, err
		}
	}
	result, err := applyCommentMutation(sqlCommentTx{ctx, tx}, m)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}
func (r *MySQLCommentRepository) Find(ctx context.Context, id string) (*models.TrackComment, error) {
	return (sqlCommentTx{ctx, r.db}).Find(id)
}
func (r *MySQLCommentRepository) FindClient(ctx context.Context, user int64, key string) (*models.TrackComment, error) {
	return (sqlCommentTx{ctx, r.db}).FindClient(user, key)
}
func readCommentRows(rows *sql.Rows) ([]*models.TrackComment, error) {
	defer rows.Close()
	out := []*models.TrackComment{}
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func sqlCommentIDs(ids []string) (string, []interface{}) {
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return strings.TrimSuffix(strings.Repeat("?,", len(ids)), ","), args
}
func (r *MySQLCommentRepository) FindMany(ctx context.Context, ids []string) (map[string]*models.TrackComment, error) {
	out := map[string]*models.TrackComment{}
	if len(ids) == 0 {
		return out, nil
	}
	where, args := sqlCommentIDs(ids)
	rows, err := r.db.QueryContext(ctx, "SELECT "+commentColumns+" FROM track_comments WHERE id IN ("+where+")", args...)
	if err != nil {
		return nil, err
	}
	items, err := readCommentRows(rows)
	for _, c := range items {
		out[c.ID] = c
	}
	return out, err
}
func (r *MySQLCommentRepository) List(ctx context.Context, track, root string, cursor *models.CommentCursor, limit int) ([]*models.TrackComment, error) {
	query := "SELECT " + commentColumns + " FROM track_comments WHERE track_id=? AND root_id=? AND (deleted=0"
	if root == "" {
		query += " OR reply_count>0"
	}
	query += ")"
	args := []interface{}{track, root}
	if cursor != nil {
		query += " AND (created_at<? OR (created_at=? AND id<?))"
		args = append(args, cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
	}
	query += " ORDER BY created_at DESC,id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return readCommentRows(rows)
}
func (r *MySQLCommentRepository) Total(ctx context.Context, track string) (int64, error) {
	var total int64
	err := r.db.QueryRowContext(ctx, `SELECT total_count FROM track_comment_stats WHERE track_id=?`, track).Scan(&total)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return total, err
}
func (r *MySQLCommentRepository) Totals(ctx context.Context, tracks []string) (map[string]int64, error) {
	counts := map[string]int64{}
	if len(tracks) == 0 {
		return counts, nil
	}
	where, args := sqlCommentIDs(tracks)
	rows, err := r.db.QueryContext(ctx, "SELECT track_id,total_count FROM track_comment_stats WHERE track_id IN ("+where+")", args...)
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
		counts[id] = count
	}
	return counts, rows.Err()
}
func (r *MySQLCommentRepository) AuthorReplies(ctx context.Context, user int64, roots []string) (map[string]*models.TrackComment, error) {
	out := map[string]*models.TrackComment{}
	if len(roots) == 0 {
		return out, nil
	}
	parts := []string{}
	args := []interface{}{}
	for _, root := range roots {
		parts = append(parts, "(SELECT "+commentColumns+" FROM track_comments WHERE root_id=? AND user_id=? AND deleted=0 ORDER BY created_at DESC,id DESC LIMIT 1)")
		args = append(args, root, user)
	}
	query := strings.Join(parts, " UNION ALL ")
	if len(parts) == 1 {
		query = strings.TrimSuffix(strings.TrimPrefix(query, "("), ")")
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	items, err := readCommentRows(rows)
	for _, c := range items {
		out[c.RootID] = c
	}
	return out, err
}
func (r *MySQLCommentRepository) Reactions(ctx context.Context, user int64, ids []string) (map[string]int, error) {
	out := map[string]int{}
	if len(ids) == 0 {
		return out, nil
	}
	where, args := sqlCommentIDs(ids)
	args = append([]interface{}{user}, args...)
	rows, err := r.db.QueryContext(ctx, `SELECT comment_id,reaction FROM track_comment_reactions WHERE user_id=? AND comment_id IN (`+where+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var value int
		if err := rows.Scan(&id, &value); err != nil {
			return nil, err
		}
		out[id] = value
	}
	return out, rows.Err()
}
func (r *MySQLCommentRepository) Likers(ctx context.Context, id string, before int64, limit int) ([]int64, error) {
	query := `SELECT user_id FROM track_comment_reactions WHERE comment_id=? AND reaction=1`
	args := []interface{}{id}
	if before > 0 {
		query += ` AND user_id<?`
		args = append(args, before)
	}
	query += ` ORDER BY user_id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []int64{}
	for rows.Next() {
		var user int64
		if err := rows.Scan(&user); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}
func (r *MySQLCommentRepository) PutImage(ctx context.Context, image *models.CommentImage) error {
	return (sqlCommentTx{ctx, r.db}).SaveImage(image)
}
func (r *MySQLCommentRepository) Image(ctx context.Context, id string) (*models.CommentImage, error) {
	return (sqlCommentTx{ctx, r.db}).Image(id)
}
