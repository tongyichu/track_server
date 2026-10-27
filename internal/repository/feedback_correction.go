package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/go-sql-driver/mysql"
	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const feedbackCorrectionConfigTable = `CREATE TABLE IF NOT EXISTS feedback_correction_config (
	id TINYINT NOT NULL PRIMARY KEY,
	version BIGINT NOT NULL,
	options_json JSON NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

func ensureMySQLFeedbackCorrectionSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, feedbackCorrectionConfigTable); err != nil {
		return err
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='user_feedbacks' AND COLUMN_NAME='correction_json'`).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return nil
	}
	_, err := db.ExecContext(ctx, `ALTER TABLE user_feedbacks ADD COLUMN correction_json JSON NULL`)
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1060 {
		return nil
	}
	return err
}

func (r *MySQLFeedbackRepository) GetCorrectionConfig(ctx context.Context) (*models.FeedbackCorrectionConfig, error) {
	var config models.FeedbackCorrectionConfig
	var data []byte
	err := r.db.QueryRowContext(ctx, `SELECT version, options_json FROM feedback_correction_config WHERE id=1`).Scan(&config.Version, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &config.Options); err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *MySQLFeedbackRepository) SaveCorrectionConfig(ctx context.Context, config *models.FeedbackCorrectionConfig, expectedVersion int64) error {
	data, err := json.Marshal(config.Options)
	if err != nil {
		return err
	}
	if expectedVersion == 0 {
		_, err = r.db.ExecContext(ctx, `INSERT INTO feedback_correction_config (id, version, options_json) VALUES (1,?,?)`, config.Version, string(data))
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return ErrAlreadyExists
		}
		return err
	}
	result, err := r.db.ExecContext(ctx, `UPDATE feedback_correction_config SET version=?, options_json=? WHERE id=1 AND version=?`, config.Version, string(data), expectedVersion)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return ErrAlreadyExists
	}
	return err
}

func (r *MongoFeedbackRepository) GetCorrectionConfig(ctx context.Context) (*models.FeedbackCorrectionConfig, error) {
	var config models.FeedbackCorrectionConfig
	err := r.collection.Database().Collection("feedback_correction_config").FindOne(ctx, bson.M{"_id": "track"}).Decode(&config)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	return &config, err
}

func (r *MongoFeedbackRepository) SaveCorrectionConfig(ctx context.Context, config *models.FeedbackCorrectionConfig, expectedVersion int64) error {
	collection := r.collection.Database().Collection("feedback_correction_config")
	if expectedVersion == 0 {
		_, err := collection.InsertOne(ctx, bson.M{"_id": "track", "version": config.Version, "options": config.Options})
		if mongo.IsDuplicateKeyError(err) {
			return ErrAlreadyExists
		}
		return err
	}
	result, err := collection.UpdateOne(ctx, bson.M{"_id": "track", "version": expectedVersion}, bson.M{"$set": bson.M{"version": config.Version, "options": config.Options}})
	if err == nil && result.MatchedCount == 0 {
		return ErrAlreadyExists
	}
	return err
}

func (r *InMemoryFeedbackRepository) GetCorrectionConfig(_ context.Context) (*models.FeedbackCorrectionConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.correctionConfig == nil {
		return nil, ErrNotFound
	}
	return cloneCorrectionConfig(r.correctionConfig), nil
}

func (r *InMemoryFeedbackRepository) SaveCorrectionConfig(_ context.Context, config *models.FeedbackCorrectionConfig, expectedVersion int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	version := int64(0)
	if r.correctionConfig != nil {
		version = r.correctionConfig.Version
	}
	if version != expectedVersion {
		return ErrAlreadyExists
	}
	r.correctionConfig = cloneCorrectionConfig(config)
	return nil
}

func cloneCorrectionConfig(config *models.FeedbackCorrectionConfig) *models.FeedbackCorrectionConfig {
	clone := *config
	clone.Options = append([]models.FeedbackCorrectionOption{}, config.Options...)
	return &clone
}
