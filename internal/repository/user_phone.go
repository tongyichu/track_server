package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ErrPhoneAmbiguous             = fmt.Errorf("手机号关联多个账号，请联系管理员处理: %w", ErrConflict)
	ErrPhoneConstraintUnavailable = errors.New("手机号唯一约束尚未就绪，请稍后再试")
)

func phoneWriteError(err error) error {
	var duplicate *mysql.MySQLError
	if errors.As(err, &duplicate) && duplicate.Number == 1062 || mongo.IsDuplicateKeyError(err) {
		return ErrAlreadyExists
	}
	return err
}

// Existing installations must resolve duplicates and apply the documented DDL
// explicitly. Never make a failed migration trigger the startup memory fallback.
func (r *MySQLUserRepository) requirePhoneConstraint(ctx context.Context) error {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='users' AND INDEX_NAME='uk_users_phone_key'
		AND NON_UNIQUE=0 AND COLUMN_NAME='phone_key' AND SEQ_IN_INDEX=1
		AND (SELECT COUNT(*) FROM information_schema.STATISTICS s WHERE s.TABLE_SCHEMA=DATABASE()
		AND s.TABLE_NAME='users' AND s.INDEX_NAME='uk_users_phone_key')=1`).Scan(&count)
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrPhoneConstraintUnavailable
	}
	return nil
}

func (r *MySQLUserRepository) BindPhone(ctx context.Context, id int64, phone string) error {
	phone, err := models.NormalizePhone(phone)
	if err != nil {
		return err
	}
	if err := r.requirePhoneConstraint(ctx); err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `UPDATE users SET phone=?, updated_at=? WHERE id=?`, phone, time.Now(), id)
	if err != nil {
		return phoneWriteError(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		_, err = r.FindByID(ctx, id)
	}
	return err
}

func (r *InMemoryUserRepository) BindPhone(_ context.Context, id int64, phone string) error {
	phone, err := models.NormalizePhone(phone)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return ErrNotFound
	}
	for otherID, other := range r.users {
		if otherID != id && other.Phone == phone {
			return ErrAlreadyExists
		}
	}
	u.Phone, u.UpdatedAt = phone, time.Now()
	return nil
}

func (r *MongoUserRepository) requirePhoneConstraint(ctx context.Context) error {
	_, err := r.collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "phone", Value: 1}},
		Options: options.Index().SetName("uk_users_phone").SetUnique(true).
			SetPartialFilterExpression(bson.M{"phone": bson.M{"$type": "string", "$gt": ""}}),
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPhoneConstraintUnavailable, err)
	}
	return nil
}

func (r *MongoUserRepository) BindPhone(ctx context.Context, id int64, phone string) error {
	phone, err := models.NormalizePhone(phone)
	if err != nil {
		return err
	}
	if err := r.requirePhoneConstraint(ctx); err != nil {
		return err
	}
	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"phone": phone, "updated_at": time.Now()}})
	if err != nil {
		return phoneWriteError(err)
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func cloneUser(u *models.User) *models.User {
	copy := *u
	if copy.TokenVersion <= 0 {
		copy.TokenVersion = 1
	}
	return &copy
}
