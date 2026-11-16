package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type CompanionHistoryRepository interface {
	RemoveHistory(context.Context, int64, string, string) error
}

var (
	_ CompanionHistoryRepository = (*MySQLCompanionRepository)(nil)
	_ CompanionHistoryRepository = (*MongoCompanionRepository)(nil)
	_ CompanionHistoryRepository = (*InMemoryCompanionRepository)(nil)
)

func companionHistoryRemovalTarget(session *models.CompanionSession, user int64, scope string) (int64, error) {
	if user <= 0 {
		return 0, ErrForbidden
	}
	if session.Status != models.CompanionSessionStatusEnded {
		return 0, ErrConflict
	}
	if session.OwnerUserID == user {
		if scope != "all" {
			return 0, ErrConflict
		}
		return 0, nil
	}
	if scope != "self" {
		return 0, ErrForbidden
	}
	return user, nil
}

func (r *MySQLCompanionRepository) RemoveHistory(ctx context.Context, user int64, id, scope string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var session models.CompanionSession
	err = tx.QueryRowContext(ctx, `SELECT owner_user_id,status FROM companion_sessions WHERE session_id=? FOR UPDATE`, id).Scan(&session.OwnerUserID, &session.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	var member int64
	err = tx.QueryRowContext(ctx, `SELECT user_id FROM companion_session_members WHERE session_id=? AND user_id=?`, id, user).Scan(&member)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrForbidden
	}
	if err != nil {
		return err
	}
	target, err := companionHistoryRemovalTarget(&session, user, scope)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO companion_history_removals(session_id,user_id,actor_user_id,removed_at) VALUES(?,?,?,?) ON DUPLICATE KEY UPDATE session_id=VALUES(session_id)`, id, target, user, time.Now())
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *InMemoryCompanionRepository) RemoveHistory(_ context.Context, user int64, id, scope string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	session := r.sessions[id]
	if session == nil {
		return ErrNotFound
	}
	if r.members[id][user] == nil {
		return ErrForbidden
	}
	target, err := companionHistoryRemovalTarget(session, user, scope)
	if err != nil {
		return err
	}
	if r.historyRemoved == nil {
		r.historyRemoved = make(map[string]map[int64]time.Time)
	}
	if r.historyRemoved[id] == nil {
		r.historyRemoved[id] = make(map[int64]time.Time)
	}
	if _, exists := r.historyRemoved[id][target]; !exists {
		r.historyRemoved[id][target] = time.Now()
	}
	return nil
}

func (r *InMemoryCompanionRepository) historyVisible(id string, user int64) bool {
	_, all := r.historyRemoved[id][0]
	_, self := r.historyRemoved[id][user]
	return !all && !self
}

func (r *MongoCompanionRepository) RemoveHistory(ctx context.Context, user int64, id, scope string) error {
	var session models.CompanionSession
	err := r.sessionCollection.FindOne(ctx, bson.M{"session_id": id}).Decode(&session)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	err = r.memberCollection.FindOne(ctx, bson.M{"session_id": id, "user_id": user}).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrForbidden
	}
	if err != nil {
		return err
	}
	// Ended sessions cannot transfer ownership or become active again.
	target, err := companionHistoryRemovalTarget(&session, user, scope)
	if err != nil {
		return err
	}
	_, err = r.sessionCollection.Database().Collection("companion_history_removals").UpdateOne(ctx,
		bson.M{"_id": fmt.Sprintf("%s:%d", id, target)},
		bson.M{"$setOnInsert": bson.M{"session_id": id, "user_id": target, "actor_user_id": user, "removed_at": time.Now()}}, options.Update().SetUpsert(true))
	if mongo.IsDuplicateKeyError(err) {
		return nil
	}
	return err
}

func (r *MongoCompanionRepository) historyPipeline(user int64, cursor *models.CompanionSessionListCursor) mongo.Pipeline {
	pipeline := mongo.Pipeline{}
	if cursor != nil {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.M{"$or": bson.A{
			bson.M{"started_at": bson.M{"$lt": cursor.StartedAt}},
			bson.M{"started_at": cursor.StartedAt, "session_id": bson.M{"$lt": cursor.SessionID}},
		}}}})
	}
	lookup := func(collection, name string, userCondition bson.M) bson.D {
		return bson.D{{Key: "$lookup", Value: bson.M{"from": collection, "let": bson.M{"room": "$session_id"}, "pipeline": bson.A{
			bson.M{"$match": bson.M{"$expr": bson.M{"$and": bson.A{bson.M{"$eq": bson.A{"$session_id", "$$room"}}, userCondition}}}},
			bson.M{"$limit": 1},
		}, "as": name}}}
	}
	pipeline = append(pipeline,
		lookup(r.memberCollection.Name(), "history_member", bson.M{"$eq": bson.A{"$user_id", user}}),
		bson.D{{Key: "$match", Value: bson.M{"history_member.0": bson.M{"$exists": true}}}},
		lookup("companion_history_removals", "history_removal", bson.M{"$in": bson.A{"$user_id", bson.A{int64(0), user}}}),
		bson.D{{Key: "$match", Value: bson.M{"history_removal.0": bson.M{"$exists": false}}}},
	)
	return pipeline
}
