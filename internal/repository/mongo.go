package repository

import (
	"context"

	"errors"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/mongo"
)

// MongoTrackRepository is a stub of TrackRepository backed by MongoDB.
type MongoTrackRepository struct {
	collection *mongo.Collection
}

// NewMongoTrackRepository constructs a Mongo-backed TrackRepository.
func NewMongoTrackRepository(collection *mongo.Collection) *MongoTrackRepository {
	return &MongoTrackRepository{collection: collection}
}

// Create is not implemented in this demo and returns an error.
func (r *MongoTrackRepository) Create(context.Context, *models.Track) error {
	return errors.New("MongoTrackRepository.Create not implemented")
}

// Update is not implemented in this demo and returns an error.
func (r *MongoTrackRepository) Update(context.Context, *models.Track) error {
	return errors.New("MongoTrackRepository.Update not implemented")
}

// FindByID is not implemented in this demo and returns an error.
func (r *MongoTrackRepository) FindByID(context.Context, string) (*models.Track, error) {
	return nil, errors.New("MongoTrackRepository.FindByID not implemented")
}

// FindRunningByUserID is not implemented in this demo and returns an error.
func (r *MongoTrackRepository) FindRunningByUserID(context.Context, int64) (*models.Track, error) {
	return nil, errors.New("MongoTrackRepository.FindRunningByUserID not implemented")
}

// ListRecommend is not implemented in this demo and returns an error.
func (r *MongoTrackRepository) ListRecommend(context.Context, int64, int) ([]*models.Track, error) {
	return nil, errors.New("MongoTrackRepository.ListRecommend not implemented")
}

// Search is not implemented in this demo and returns an error.
func (r *MongoTrackRepository) Search(context.Context, string, int) ([]*models.Track, error) {
	return nil, errors.New("MongoTrackRepository.Search not implemented")
}

// MongoUserRepository is a stub of UserRepository backed by MongoDB.
type MongoUserRepository struct {
	collection *mongo.Collection
}

// NewMongoUserRepository constructs a Mongo-backed UserRepository.
func NewMongoUserRepository(collection *mongo.Collection) *MongoUserRepository {
	return &MongoUserRepository{collection: collection}
}

// CreateIfNotExists is not implemented in this demo and returns an error.
func (r *MongoUserRepository) CreateIfNotExists(context.Context, *models.User) (*models.User, error) {
	return nil, errors.New("MongoUserRepository.CreateIfNotExists not implemented")
}

// FindByID is not implemented in this demo and returns an error.
func (r *MongoUserRepository) FindByID(context.Context, int64) (*models.User, error) {
	return nil, errors.New("MongoUserRepository.FindByID not implemented")
}

// Update is not implemented in this demo and returns an error.
func (r *MongoUserRepository) Update(context.Context, *models.User) error {
	return errors.New("MongoUserRepository.Update not implemented")
}

// MongoCollectRepository is a stub of CollectRepository backed by MongoDB.
type MongoCollectRepository struct {
	collection *mongo.Collection
}

// NewMongoCollectRepository constructs a Mongo-backed CollectRepository.
func NewMongoCollectRepository(collection *mongo.Collection) *MongoCollectRepository {
	return &MongoCollectRepository{collection: collection}
}

// IsCollected is not implemented in this demo and returns an error.
func (r *MongoCollectRepository) IsCollected(context.Context, int64, string) (bool, error) {
	return false, errors.New("MongoCollectRepository.IsCollected not implemented")
}

// AddCollect is not implemented in this demo and returns an error.
func (r *MongoCollectRepository) AddCollect(context.Context, int64, string) error {
	return errors.New("MongoCollectRepository.AddCollect not implemented")
}

// RemoveCollect is not implemented in this demo and returns an error.
func (r *MongoCollectRepository) RemoveCollect(context.Context, int64, string) error {
	return errors.New("MongoCollectRepository.RemoveCollect not implemented")
}

// MongoLoginLogRepository is a stub of LoginLogRepository backed by MongoDB.
type MongoLoginLogRepository struct {
	collection *mongo.Collection
}

// NewMongoLoginLogRepository constructs a Mongo-backed LoginLogRepository.
func NewMongoLoginLogRepository(collection *mongo.Collection) *MongoLoginLogRepository {
	return &MongoLoginLogRepository{collection: collection}
}

// Create is not implemented in this demo and returns an error.
func (r *MongoLoginLogRepository) Create(context.Context, *models.LoginLog) error {
	return errors.New("MongoLoginLogRepository.Create not implemented")
}

// ListByUserID is not implemented in this demo and returns an error.
func (r *MongoLoginLogRepository) ListByUserID(context.Context, int64, int) ([]*models.LoginLog, error) {
	return nil, errors.New("MongoLoginLogRepository.ListByUserID not implemented")
}
