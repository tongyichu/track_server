package repository

import (
	"context"
	"errors"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/mongo"
)

// MongoCompanionRepository is a stub of CompanionRepository backed by MongoDB.
type MongoCompanionRepository struct {
	sessionCollection  *mongo.Collection
	memberCollection   *mongo.Collection
	positionCollection *mongo.Collection
}

// NewMongoCompanionRepository constructs a Mongo-backed CompanionRepository.
func NewMongoCompanionRepository(sessionCollection, memberCollection, positionCollection *mongo.Collection) *MongoCompanionRepository {
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

