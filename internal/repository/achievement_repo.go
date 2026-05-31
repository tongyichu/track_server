package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MySQLAchievementRepository implements AchievementRepository on top of MySQL.
type MySQLAchievementRepository struct{ db *sql.DB }

func NewMySQLAchievementRepository(db *sql.DB) *MySQLAchievementRepository {
	return &MySQLAchievementRepository{db: db}
}

func (r *MySQLAchievementRepository) UpsertUserReward(ctx context.Context, reward *models.UserAchievementReward) (bool, error) {
	if reward == nil || reward.UserID <= 0 || reward.RewardCode == "" {
		return false, nil
	}
	if reward.EarnedAt.IsZero() {
		reward.EarnedAt = time.Now()
	}
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO user_achievement_rewards (
			user_id, reward_code, source_track_id, source_session_id, progress_snapshot, earned_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE reward_code=reward_code`,
		reward.UserID, reward.RewardCode, reward.SourceTrackID, reward.SourceSessionID, nullableStringValue(reward.ProgressSnapshot), reward.EarnedAt,
	)
	if err != nil {
		if me, ok := err.(*mysql.MySQLError); ok && me.Number == 1062 {
			return false, nil
		}
		return false, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return false, nil
	}
	id, _ := res.LastInsertId()
	if id > 0 {
		reward.ID = id
		return true, nil
	}
	return false, nil
}

func (r *MySQLAchievementRepository) ListUserRewards(ctx context.Context, userID int64) ([]*models.UserAchievementReward, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, reward_code, source_track_id, source_session_id, COALESCE(progress_snapshot, ''), earned_at
		FROM user_achievement_rewards
		WHERE user_id=?
		ORDER BY earned_at DESC, id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAchievementRewards(rows)
}

func (r *MySQLAchievementRepository) ListRecentUserRewards(ctx context.Context, userID int64, limit int) ([]*models.UserAchievementReward, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, reward_code, source_track_id, source_session_id, COALESCE(progress_snapshot, ''), earned_at
		FROM user_achievement_rewards
		WHERE user_id=?
		ORDER BY earned_at DESC, id DESC
		LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAchievementRewards(rows)
}

func scanAchievementRewards(rows *sql.Rows) ([]*models.UserAchievementReward, error) {
	items := make([]*models.UserAchievementReward, 0)
	for rows.Next() {
		var reward models.UserAchievementReward
		if err := rows.Scan(&reward.ID, &reward.UserID, &reward.RewardCode, &reward.SourceTrackID, &reward.SourceSessionID, &reward.ProgressSnapshot, &reward.EarnedAt); err != nil {
			return nil, err
		}
		items = append(items, &reward)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// MongoAchievementRepository implements AchievementRepository on top of MongoDB.
type MongoAchievementRepository struct{ collection *mongo.Collection }

func NewMongoAchievementRepository(collection *mongo.Collection) *MongoAchievementRepository {
	return &MongoAchievementRepository{collection: collection}
}

func (r *MongoAchievementRepository) UpsertUserReward(ctx context.Context, reward *models.UserAchievementReward) (bool, error) {
	if reward == nil || reward.UserID <= 0 || reward.RewardCode == "" {
		return false, nil
	}
	if reward.EarnedAt.IsZero() {
		reward.EarnedAt = time.Now()
	}
	update := bson.M{"$setOnInsert": reward}
	res, err := r.collection.UpdateOne(ctx,
		bson.M{"user_id": reward.UserID, "reward_code": reward.RewardCode},
		update,
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return false, err
	}
	return res.UpsertedCount > 0, nil
}

func (r *MongoAchievementRepository) ListUserRewards(ctx context.Context, userID int64) ([]*models.UserAchievementReward, error) {
	return r.listUserRewards(ctx, userID, 0)
}

func (r *MongoAchievementRepository) ListRecentUserRewards(ctx context.Context, userID int64, limit int) ([]*models.UserAchievementReward, error) {
	if limit <= 0 {
		limit = 10
	}
	return r.listUserRewards(ctx, userID, int64(limit))
}

func (r *MongoAchievementRepository) listUserRewards(ctx context.Context, userID int64, limit int64) ([]*models.UserAchievementReward, error) {
	opts := options.Find().SetSort(bson.D{{Key: "earned_at", Value: -1}, {Key: "reward_code", Value: -1}})
	if limit > 0 {
		opts.SetLimit(limit)
	}
	cur, err := r.collection.Find(ctx, bson.M{"user_id": userID}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	items := make([]*models.UserAchievementReward, 0)
	for cur.Next(ctx) {
		var reward models.UserAchievementReward
		if err := cur.Decode(&reward); err != nil {
			return nil, err
		}
		items = append(items, &reward)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
