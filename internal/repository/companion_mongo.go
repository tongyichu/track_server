package repository

import (
	"context"
	"errors"
	"log"
	"regexp"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoCompanionRepository is a stub of CompanionRepository backed by MongoDB.
type MongoCompanionRepository struct {
	sessionCollection  *mongo.Collection
	memberCollection   *mongo.Collection
	positionCollection *mongo.Collection
}

// NewMongoCompanionRepository constructs a Mongo-backed CompanionRepository.
func NewMongoCompanionRepository(sessionCollection, memberCollection, positionCollection *mongo.Collection) *MongoCompanionRepository {
	if sessionCollection != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := sessionCollection.Database().Collection("companion_track_chunks").Indexes().CreateMany(ctx, []mongo.IndexModel{
			{Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "user_id", Value: 1}, {Key: "index", Value: 1}}},
			{Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "index", Value: 1}}},
		})
		if err != nil {
			log.Printf("companion track indexes unavailable: %v", err)
		}
	}
	return &MongoCompanionRepository{
		sessionCollection:  sessionCollection,
		memberCollection:   memberCollection,
		positionCollection: positionCollection,
	}
}

func (r *MongoCompanionRepository) CreateSession(context.Context, *models.CompanionSession) error {
	return errors.New("MongoCompanionRepository.CreateSession not implemented")
}

func (r *MongoCompanionRepository) UpdateSession(context.Context, *models.CompanionSession) error {
	return errors.New("MongoCompanionRepository.UpdateSession not implemented")
}

func (r *MongoCompanionRepository) FindSessionByID(context.Context, string) (*models.CompanionSession, error) {
	return nil, errors.New("MongoCompanionRepository.FindSessionByID not implemented")
}

func (r *MongoCompanionRepository) FindSessionByJoinToken(context.Context, string) (*models.CompanionSession, error) {
	return nil, errors.New("MongoCompanionRepository.FindSessionByJoinToken not implemented")
}

func (r *MongoCompanionRepository) FindActiveSessionByUserID(context.Context, int64) (*models.CompanionSession, error) {
	return nil, errors.New("MongoCompanionRepository.FindActiveSessionByUserID not implemented")
}

func (r *MongoCompanionRepository) ListSessionsByUserID(context.Context, int64, *models.CompanionSessionListCursor, int) ([]*models.CompanionSession, error) {
	return nil, errors.New("MongoCompanionRepository.ListSessionsByUserID not implemented")
}

func (r *MongoCompanionRepository) CountSessionsByUserID(context.Context, int64) (int64, error) {
	return 0, errors.New("MongoCompanionRepository.CountSessionsByUserID not implemented")
}

func (r *MongoCompanionRepository) ListActiveSessions(context.Context, int) ([]*models.CompanionSession, error) {
	return nil, errors.New("MongoCompanionRepository.ListActiveSessions not implemented")
}

func (r *MongoCompanionRepository) ListAllSessions(context.Context, *models.CompanionSessionListCursor, int) ([]*models.CompanionSession, error) {
	return nil, errors.New("MongoCompanionRepository.ListAllSessions not implemented")
}

func (r *MongoCompanionRepository) CountAllSessions(context.Context) (int64, error) {
	return 0, errors.New("MongoCompanionRepository.CountAllSessions not implemented")
}

// ListAdminCompanions 按组合筛选条件返回同行会话（按 started_at desc, session_id desc）。仅供管理后台使用。
func (r *MongoCompanionRepository) ListAdminCompanions(ctx context.Context, filter AdminCompanionListFilter, cursor *models.CompanionSessionListCursor, limit int) ([]*models.CompanionSession, int64, error) {
	if r.sessionCollection == nil {
		return nil, 0, errors.New("MongoCompanionRepository.ListAdminCompanions not implemented")
	}
	if limit <= 0 {
		limit = 20
	}
	conditions := bson.A{}
	if filter.SessionIDs != nil {
		if len(filter.SessionIDs) == 0 {
			return []*models.CompanionSession{}, 0, nil
		}
		conditions = append(conditions, bson.M{"session_id": bson.M{"$in": filter.SessionIDs}})
	}
	if filter.Title != "" {
		conditions = append(conditions, bson.M{"title": primitive.Regex{Pattern: regexp.QuoteMeta(filter.Title), Options: "i"}})
	}
	if filter.TrackType != "" {
		conditions = append(conditions, bson.M{"track_type": filter.TrackType})
	}
	if filter.OwnerIDs != nil {
		if len(filter.OwnerIDs) == 0 {
			return []*models.CompanionSession{}, 0, nil
		}
		conditions = append(conditions, bson.M{"owner_user_id": bson.M{"$in": filter.OwnerIDs}})
	}
	query := bson.M{}
	if len(conditions) > 0 {
		query["$and"] = conditions
	}
	if cursor != nil && !cursor.StartedAt.IsZero() {
		cursorCond := bson.M{"$or": bson.A{bson.M{"started_at": bson.M{"$lt": cursor.StartedAt}}, bson.M{"started_at": cursor.StartedAt, "session_id": bson.M{"$lt": cursor.SessionID}}}}
		if len(conditions) > 0 {
			query["$and"] = append(conditions, cursorCond)
		} else {
			query = cursorCond
		}
	}
	total, err := r.sessionCollection.CountDocuments(ctx, func() bson.M {
		if len(conditions) > 0 {
			return bson.M{"$and": conditions}
		}
		return bson.M{}
	}())
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.sessionCollection.Find(ctx, query, options.Find().SetSort(bson.D{{Key: "started_at", Value: -1}, {Key: "session_id", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close(ctx)
	items := make([]*models.CompanionSession, 0, limit)
	for rows.Next(ctx) {
		item := &models.CompanionSession{}
		if err := rows.Decode(item); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// CountMembersBySessionIDs 批量统计每个会话的成员记录数。仅供管理后台使用。
func (r *MongoCompanionRepository) CountMembersBySessionIDs(ctx context.Context, sessionIDs []string) (map[string]int, error) {
	counts := make(map[string]int, len(sessionIDs))
	if len(sessionIDs) == 0 {
		return counts, nil
	}
	if r.memberCollection == nil {
		return nil, errors.New("MongoCompanionRepository.CountMembersBySessionIDs not implemented")
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"session_id": bson.M{"$in": sessionIDs}}}},
		{{Key: "$group", Value: bson.M{"_id": "$session_id", "count": bson.M{"$sum": 1}}}},
	}
	rows, err := r.memberCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer rows.Close(ctx)
	for rows.Next(ctx) {
		var result struct {
			SessionID string `bson:"_id"`
			Count     int    `bson:"count"`
		}
		if err := rows.Decode(&result); err != nil {
			return nil, err
		}
		counts[result.SessionID] = result.Count
	}
	return counts, rows.Err()
}

func (r *MongoCompanionRepository) UpsertMember(context.Context, *models.CompanionSessionMember) error {
	return errors.New("MongoCompanionRepository.UpsertMember not implemented")
}

func (r *MongoCompanionRepository) FindMember(context.Context, string, int64) (*models.CompanionSessionMember, error) {
	return nil, errors.New("MongoCompanionRepository.FindMember not implemented")
}

func (r *MongoCompanionRepository) ListMembers(context.Context, string) ([]*models.CompanionSessionMember, error) {
	return nil, errors.New("MongoCompanionRepository.ListMembers not implemented")
}

func (r *MongoCompanionRepository) CountMembersByStatus(context.Context, string, models.CompanionMemberStatus) (int64, error) {
	return 0, errors.New("MongoCompanionRepository.CountMembersByStatus not implemented")
}

func (r *MongoCompanionRepository) UpsertPosition(context.Context, *models.CompanionLivePosition) error {
	return errors.New("MongoCompanionRepository.UpsertPosition not implemented")
}

func (r *MongoCompanionRepository) ListPositions(context.Context, string) ([]*models.CompanionLivePosition, error) {
	return nil, errors.New("MongoCompanionRepository.ListPositions not implemented")
}

func (r *MongoCompanionRepository) DeletePositions(context.Context, string) error {
	return errors.New("MongoCompanionRepository.DeletePositions not implemented")
}

func (r *MongoCompanionRepository) InsertDanmaku(context.Context, *models.CompanionDanmaku) error {
	return errors.New("MongoCompanionRepository.InsertDanmaku not implemented")
}

func (r *MongoCompanionRepository) CountDanmakuByMemberSince(context.Context, string, int64, time.Time) (int64, error) {
	return 0, errors.New("MongoCompanionRepository.CountDanmakuByMemberSince not implemented")
}

func (r *MongoCompanionRepository) CountDanmakuBySessionSince(context.Context, string, time.Time) (int64, error) {
	return 0, errors.New("MongoCompanionRepository.CountDanmakuBySessionSince not implemented")
}

func (r *MongoCompanionRepository) ListDanmakusBySessionID(context.Context, string, int) ([]*models.CompanionDanmaku, error) {
	return nil, errors.New("MongoCompanionRepository.ListDanmakusBySessionID not implemented")
}

func (r *MongoCompanionRepository) DeleteDanmakusBySessionEndedBefore(context.Context, time.Time) (int64, error) {
	return 0, errors.New("MongoCompanionRepository.DeleteDanmakusBySessionEndedBefore not implemented")
}

func (r *MongoCompanionRepository) InsertEvent(context.Context, *models.CompanionEvent) error {
	return errors.New("MongoCompanionRepository.InsertEvent not implemented")
}

func (r *MongoCompanionRepository) FindEventByClientEventID(context.Context, string, string) (*models.CompanionEvent, error) {
	return nil, errors.New("MongoCompanionRepository.FindEventByClientEventID not implemented")
}

func (r *MongoCompanionRepository) CountEventsBySessionID(context.Context, string) (int64, error) {
	return 0, errors.New("MongoCompanionRepository.CountEventsBySessionID not implemented")
}

func (r *MongoCompanionRepository) ListEventsBySessionID(context.Context, string, *models.CompanionEventCursor, int, bool) ([]*models.CompanionEvent, error) {
	return nil, errors.New("MongoCompanionRepository.ListEventsBySessionID not implemented")
}
