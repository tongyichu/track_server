package repository

import (
	"context"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
)

// AdminUserProfilePatch only contains fields writable by the admin form.
type AdminUserProfilePatch struct {
	Nickname   *string `json:"nickname"`
	Signature  *string `json:"signature"`
	Gender     *string `json:"gender"`
	Birthday   *string `json:"birthday"`
	IsInternal *bool   `json:"is_internal"`
}

func (p AdminUserProfilePatch) Empty() bool {
	return p.Nickname == nil && p.Signature == nil && p.Gender == nil && p.Birthday == nil && p.IsInternal == nil
}

func (p AdminUserProfilePatch) Apply(user *models.User, now time.Time) {
	if p.Nickname != nil {
		user.Nickname = *p.Nickname
	}
	if p.Signature != nil {
		user.Signature = *p.Signature
	}
	if p.Gender != nil {
		user.Gender = *p.Gender
	}
	if p.Birthday != nil {
		user.Birthday = *p.Birthday
	}
	if p.IsInternal != nil {
		user.IsInternal = *p.IsInternal
	}
	user.UpdatedAt = now
}

type AdminUserProfileRepository interface {
	PatchAdminUserProfile(ctx context.Context, userID int64, patch AdminUserProfilePatch, now time.Time) error
}

func (p AdminUserProfilePatch) fields(now time.Time) bson.D {
	fields := bson.D{{Key: "updated_at", Value: now}}
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"nickname", p.Nickname}, {"signature", p.Signature}, {"gender", p.Gender}, {"birthday", p.Birthday},
	} {
		if field.value != nil {
			fields = append(fields, bson.E{Key: field.name, Value: *field.value})
		}
	}
	if p.IsInternal != nil {
		fields = append(fields, bson.E{Key: "is_internal", Value: *p.IsInternal})
	}
	return fields
}

func (r *MySQLUserRepository) PatchAdminUserProfile(ctx context.Context, userID int64, patch AdminUserProfilePatch, now time.Time) error {
	fields := patch.fields(now)
	assignments := make([]string, 0, len(fields))
	args := make([]any, 0, len(fields)+1)
	for _, field := range fields {
		assignments = append(assignments, field.Key+"=?")
		value := field.Value
		if field.Key == "signature" {
			value = nullableStringValue(*patch.Signature)
		}
		args = append(args, value)
	}
	args = append(args, userID)
	_, err := r.db.ExecContext(ctx, "UPDATE users SET "+strings.Join(assignments, ",")+" WHERE id=?", args...)
	return err
}

func (r *MongoUserRepository) PatchAdminUserProfile(ctx context.Context, userID int64, patch AdminUserProfilePatch, now time.Time) error {
	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": userID}, bson.M{"$set": patch.fields(now)})
	if err == nil && result.MatchedCount == 0 {
		return ErrNotFound
	}
	return err
}

func (r *InMemoryUserRepository) PatchAdminUserProfile(_ context.Context, userID int64, patch AdminUserProfilePatch, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	user, exists := r.users[userID]
	if !exists {
		return ErrNotFound
	}
	patch.Apply(user, now)
	return nil
}
