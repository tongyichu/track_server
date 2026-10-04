package repository

import (
	"context"
	"errors"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoRecommendationRepository struct {
	sessions *mongo.Collection
	profiles *mongo.Collection
	stats    *mongo.Collection
}

func NewMongoRecommendationRepository(sessions, profiles, stats *mongo.Collection) *MongoRecommendationRepository {
	if sessions != nil {
		_, _ = sessions.Indexes().CreateOne(context.Background(), mongo.IndexModel{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetName("idx_recommend_feed_expire").SetExpireAfterSeconds(0)})
	}
	if stats != nil {
		_, _ = stats.Indexes().CreateOne(context.Background(), mongo.IndexModel{Keys: bson.D{{Key: "track_id", Value: 1}, {Key: "stat_date", Value: -1}}, Options: options.Index().SetName("idx_recommend_stats_track_date").SetUnique(true)})
	}
	return &MongoRecommendationRepository{sessions: sessions, profiles: profiles, stats: stats}
}

func (r *MongoRecommendationRepository) SaveFeedSession(ctx context.Context, session *models.RecommendationFeedSession) error {
	if err := validateRecommendationFeedSession(session); err != nil {
		return err
	}
	_, err := r.sessions.ReplaceOne(ctx, bson.M{"_id": session.RequestID}, session, options.Replace().SetUpsert(true))
	return err
}

func (r *MongoRecommendationRepository) GetFeedSession(ctx context.Context, requestID string) (*models.RecommendationFeedSession, error) {
	session := &models.RecommendationFeedSession{}
	err := r.sessions.FindOne(ctx, bson.M{"_id": requestID}).Decode(session)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	return session, err
}

func (r *MongoRecommendationRepository) DeleteExpiredFeedSessions(ctx context.Context, now time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = 1000
	}
	cursor, err := r.sessions.Find(ctx, bson.M{"expires_at": bson.M{"$lte": now}}, options.Find().SetProjection(bson.M{"_id": 1}).SetLimit(int64(limit)))
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)
	ids := make([]string, 0)
	for cursor.Next(ctx) {
		var row struct {
			ID string `bson:"_id"`
		}
		if err := cursor.Decode(&row); err != nil {
			return 0, err
		}
		ids = append(ids, row.ID)
	}
	if len(ids) == 0 {
		return 0, cursor.Err()
	}
	result, err := r.sessions.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return 0, err
	}
	return result.DeletedCount, nil
}

func (r *MongoRecommendationRepository) GetUserProfile(ctx context.Context, userID int64) (*models.RecommendationUserProfile, error) {
	profile := &models.RecommendationUserProfile{}
	err := r.profiles.FindOne(ctx, bson.M{"_id": userID}).Decode(profile)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	return profile, err
}

func (r *MongoRecommendationRepository) UpsertUserProfiles(ctx context.Context, profiles []*models.RecommendationUserProfile) error {
	for _, profile := range profiles {
		if profile != nil {
			if _, err := r.profiles.ReplaceOne(ctx, bson.M{"_id": profile.UserID}, profile, options.Replace().SetUpsert(true)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *MongoRecommendationRepository) ListItemStats(ctx context.Context, trackIDs []string) (map[string]*models.RecommendationItemStats, error) {
	ids := uniqueNonEmptyStrings(trackIDs)
	result := make(map[string]*models.RecommendationItemStats, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	cursor, err := r.stats.Find(ctx, bson.M{"track_id": bson.M{"$in": ids}}, options.Find().SetSort(bson.D{{Key: "track_id", Value: 1}, {Key: "stat_date", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		stat := &models.RecommendationItemStats{}
		if err := cursor.Decode(stat); err != nil {
			return nil, err
		}
		if _, ok := result[stat.TrackID]; !ok {
			result[stat.TrackID] = stat
		}
	}
	return result, cursor.Err()
}

func (r *MongoRecommendationRepository) UpsertItemStats(ctx context.Context, stats []*models.RecommendationItemStats) error {
	for _, stat := range stats {
		if stat != nil {
			filter := bson.M{"track_id": stat.TrackID, "stat_date": stat.StatDate}
			if _, err := r.stats.ReplaceOne(ctx, filter, stat, options.Replace().SetUpsert(true)); err != nil {
				return err
			}
		}
	}
	return nil
}
