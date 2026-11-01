package admin

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var errAccountStoreUnavailable = errors.New("后台账号库暂不可用")

// AccountStorage 对少量后台账号使用整体版本 CAS，使迁移、权限更新和最后管理员保护原子生效。
// version=0 表示尚未初始化；已初始化的空数据不能被当成首次启动重新导入。
type AccountStorage interface {
	Load(context.Context) (version int64, data []byte, err error)
	Save(ctx context.Context, version int64, data []byte) (bool, error)
	AppendAudit(context.Context, AuditEntry) error
	ListAudit(context.Context, string, int) ([]AuditEntry, error)
}

type AuditEntry struct {
	ID        string    `json:"id" bson:"_id"`
	ActorID   string    `json:"actor_id" bson:"actor_id"`
	Username  string    `json:"username" bson:"username"`
	Method    string    `json:"method" bson:"method"`
	Path      string    `json:"path" bson:"path"`
	Status    int       `json:"status" bson:"status"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
}

type mysqlAccountStorage struct{ db *sql.DB }

func NewMySQLAccountStorage(ctx context.Context, db *sql.DB) (AccountStorage, error) {
	for _, statement := range []string{adminAccountStateSchema, adminAuditSchema} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return nil, err
		}
	}
	return &mysqlAccountStorage{db: db}, nil
}

const adminAccountStateSchema = `CREATE TABLE IF NOT EXISTS admin_account_state (
 id TINYINT UNSIGNED NOT NULL PRIMARY KEY,
 version BIGINT NOT NULL,
 payload JSON NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`

const adminAuditSchema = `CREATE TABLE IF NOT EXISTS admin_audit_logs (
 id VARCHAR(64) NOT NULL PRIMARY KEY,
 actor_id VARCHAR(64) NOT NULL,
 username VARCHAR(64) NOT NULL,
 method VARCHAR(16) NOT NULL,
 path VARCHAR(1024) NOT NULL,
 status INT NOT NULL,
 created_at DATETIME(6) NOT NULL,
 KEY idx_admin_audit_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`

func (s *mysqlAccountStorage) Load(ctx context.Context) (int64, []byte, error) {
	var version int64
	var data []byte
	err := s.db.QueryRowContext(ctx, "SELECT version, payload FROM admin_account_state WHERE id=1").Scan(&version, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, nil
	}
	return version, data, err
}

func (s *mysqlAccountStorage) Save(ctx context.Context, version int64, data []byte) (bool, error) {
	var result sql.Result
	var err error
	if version == 0 {
		result, err = s.db.ExecContext(ctx, "INSERT IGNORE INTO admin_account_state (id,version,payload) VALUES (1,1,?)", string(data))
	} else {
		result, err = s.db.ExecContext(ctx, "UPDATE admin_account_state SET version=version+1,payload=? WHERE id=1 AND version=?", string(data), version)
	}
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func (s *mysqlAccountStorage) AppendAudit(ctx context.Context, entry AuditEntry) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO admin_audit_logs (id,actor_id,username,method,path,status,created_at) VALUES (?,?,?,?,?,?,?)", entry.ID, entry.ActorID, entry.Username, entry.Method, entry.Path, entry.Status, entry.CreatedAt.UTC())
	return err
}

func (s *mysqlAccountStorage) ListAudit(ctx context.Context, username string, limit int) ([]AuditEntry, error) {
	query := "SELECT id,actor_id,username,method,path,status,created_at FROM admin_audit_logs"
	args := make([]interface{}, 0, 2)
	if username != "" {
		query += " WHERE username=?"
		args = append(args, username)
	}
	query += " ORDER BY created_at DESC,id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]AuditEntry, 0)
	for rows.Next() {
		var entry AuditEntry
		if err := rows.Scan(&entry.ID, &entry.ActorID, &entry.Username, &entry.Method, &entry.Path, &entry.Status, &entry.CreatedAt); err != nil {
			return nil, err
		}
		entry.CreatedAt = entry.CreatedAt.UTC()
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

type mongoAccountStorage struct{ state, audit *mongo.Collection }

func NewMongoAccountStorage(ctx context.Context, db *mongo.Database) (AccountStorage, error) {
	s := &mongoAccountStorage{state: db.Collection("admin_account_state"), audit: db.Collection("admin_audit_logs")}
	if err := db.RunCommand(ctx, bson.M{"ping": 1}).Err(); err != nil {
		return nil, err
	}
	_, err := s.audit.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "created_at", Value: -1}}})
	return s, err
}

func (s *mongoAccountStorage) Load(ctx context.Context) (int64, []byte, error) {
	var doc struct {
		Version int64  `bson:"version"`
		Payload []byte `bson:"payload"`
	}
	err := s.state.FindOne(ctx, bson.M{"_id": "accounts"}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return 0, nil, nil
	}
	return doc.Version, doc.Payload, err
}

func (s *mongoAccountStorage) Save(ctx context.Context, version int64, data []byte) (bool, error) {
	if version == 0 {
		_, err := s.state.InsertOne(ctx, bson.M{"_id": "accounts", "version": int64(1), "payload": data})
		if mongo.IsDuplicateKeyError(err) {
			return false, nil
		}
		return err == nil, err
	}
	result, err := s.state.UpdateOne(ctx, bson.M{"_id": "accounts", "version": version}, bson.M{"$set": bson.M{"payload": data}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return false, err
	}
	return result.ModifiedCount == 1, nil
}

func (s *mongoAccountStorage) AppendAudit(ctx context.Context, entry AuditEntry) error {
	_, err := s.audit.InsertOne(ctx, entry)
	return err
}

func (s *mongoAccountStorage) ListAudit(ctx context.Context, username string, limit int) ([]AuditEntry, error) {
	filter := bson.M{}
	if username != "" {
		filter["username"] = username
	}
	cur, err := s.audit.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	entries := make([]AuditEntry, 0)
	err = cur.All(ctx, &entries)
	return entries, err
}
