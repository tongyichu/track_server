package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// TransferOwner commits the session owner and both member roles as one change.
func (r *MySQLCompanionRepository) TransferOwner(ctx context.Context, id string, owner, target int64) (*models.CompanionSession, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	session, err := scanCompanionSessionRow(tx.QueryRowContext(ctx, companionSessionSelectSQL()+" WHERE session_id=? FOR UPDATE", id))
	if err != nil {
		return nil, err
	}
	if session.OwnerUserID != owner || session.Status != models.CompanionSessionStatusActive || owner == target {
		return nil, ErrForbidden
	}
	for _, user := range []int64{owner, target} {
		var status models.CompanionMemberStatus
		err = tx.QueryRowContext(ctx, "SELECT member_status FROM companion_session_members WHERE session_id=? AND user_id=? FOR UPDATE", id, user).Scan(&status)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		if status != models.CompanionMemberStatusJoined {
			return nil, ErrForbidden
		}
	}
	session.OwnerUserID = target
	session.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
	if _, err = tx.ExecContext(ctx, "UPDATE companion_sessions SET owner_user_id=?, updated_at=? WHERE session_id=?", target, session.UpdatedAt, id); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE companion_session_members SET role=CASE WHEN user_id=? THEN 'owner' ELSE 'member' END WHERE session_id=?", target, id); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return session, nil
}

func (r *InMemoryCompanionRepository) TransferOwner(_ context.Context, id string, owner, target int64) (*models.CompanionSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	session := r.sessions[id]
	if session == nil {
		return nil, ErrNotFound
	}
	if session.OwnerUserID != owner || session.Status != models.CompanionSessionStatusActive || owner == target {
		return nil, ErrForbidden
	}
	for _, user := range []int64{owner, target} {
		member := r.members[id][user]
		if member == nil {
			return nil, ErrNotFound
		}
		if member.MemberStatus != models.CompanionMemberStatusJoined {
			return nil, ErrForbidden
		}
	}
	session.OwnerUserID = target
	session.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
	for user, member := range r.members[id] {
		member.Role = models.CompanionMemberRoleMember
		if user == target {
			member.Role = models.CompanionMemberRoleOwner
		}
	}
	clone := *session
	return &clone, nil
}

func (r *MongoCompanionRepository) TransferOwner(ctx context.Context, id string, owner, target int64) (*models.CompanionSession, error) {
	if r.sessionCollection == nil || r.memberCollection == nil {
		return nil, errors.New("companion collections unavailable")
	}
	mongoSession, err := r.sessionCollection.Database().Client().StartSession()
	if err != nil {
		return nil, err
	}
	defer mongoSession.EndSession(ctx)
	var result *models.CompanionSession
	_, err = mongoSession.WithTransaction(ctx, func(tx mongo.SessionContext) (interface{}, error) {
		var session models.CompanionSession
		if e := r.sessionCollection.FindOne(tx, bson.M{"session_id": id}).Decode(&session); e != nil {
			if errors.Is(e, mongo.ErrNoDocuments) {
				return nil, ErrNotFound
			}
			return nil, e
		}
		if session.OwnerUserID != owner || session.Status != models.CompanionSessionStatusActive || owner == target {
			return nil, ErrForbidden
		}
		for _, user := range []int64{owner, target} {
			var member models.CompanionSessionMember
			if e := r.memberCollection.FindOne(tx, bson.M{"session_id": id, "user_id": user}).Decode(&member); e != nil {
				if errors.Is(e, mongo.ErrNoDocuments) {
					return nil, ErrNotFound
				}
				return nil, e
			}
			if member.MemberStatus != models.CompanionMemberStatusJoined {
				return nil, ErrForbidden
			}
		}
		session.OwnerUserID = target
		session.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
		updated, e := r.sessionCollection.UpdateOne(tx, bson.M{"session_id": id, "owner_user_id": owner, "status": models.CompanionSessionStatusActive}, bson.M{"$set": bson.M{"owner_user_id": target, "updated_at": session.UpdatedAt}})
		if e != nil {
			return nil, e
		}
		if updated.MatchedCount != 1 {
			return nil, ErrForbidden
		}
		if _, e = r.memberCollection.UpdateMany(tx, bson.M{"session_id": id}, bson.M{"$set": bson.M{"role": models.CompanionMemberRoleMember}}); e != nil {
			return nil, e
		}
		if _, e = r.memberCollection.UpdateOne(tx, bson.M{"session_id": id, "user_id": target}, bson.M{"$set": bson.M{"role": models.CompanionMemberRoleOwner}}); e != nil {
			return nil, e
		}
		result = &session
		return nil, nil
	})
	return result, err
}
