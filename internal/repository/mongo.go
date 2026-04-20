package repository

import (
	"context"

	"errors"
	"sync"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoTrackRepository is a stub of TrackRepository backed by MongoDB.
type MongoTrackRepository struct {
	collection *mongo.Collection
}

// MongoTrackWaypointRepository implements TrackWaypointRepository backed by MongoDB.
type MongoTrackWaypointRepository struct {
	mu         sync.Mutex
	nextID     uint64
	collection *mongo.Collection
}

// NewMongoTrackRepository constructs a Mongo-backed TrackRepository.
func NewMongoTrackRepository(collection *mongo.Collection) *MongoTrackRepository {
	return &MongoTrackRepository{collection: collection}
}

// NewMongoTrackWaypointRepository constructs a Mongo-backed TrackWaypointRepository.
func NewMongoTrackWaypointRepository(collection *mongo.Collection) *MongoTrackWaypointRepository {
	return &MongoTrackWaypointRepository{collection: collection, nextID: uint64(time.Now().UnixNano())}
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

func (r *MongoTrackWaypointRepository) Create(ctx context.Context, waypoint *models.TrackWaypoint) error {
	if waypoint.TrackID == "" {
		return errors.New("track waypoint track_id is required")
	}
	if waypoint.CreatedAt.IsZero() {
		waypoint.CreatedAt = time.Now()
	}
	if waypoint.ID == 0 {
		r.mu.Lock()
		r.nextID++
		waypoint.ID = r.nextID
		r.mu.Unlock()
	}
	_, err := r.collection.InsertOne(ctx, waypoint)
	return err
}

func (r *MongoTrackWaypointRepository) ListByTrackID(ctx context.Context, trackID string) ([]*models.TrackWaypoint, error) {
	cur, err := r.collection.Find(ctx,
		bson.M{"track_id": trackID},
		options.Find().SetSort(bson.D{{Key: "node_time", Value: 1}, {Key: "id", Value: 1}}),
	)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	res := make([]*models.TrackWaypoint, 0)
	for cur.Next(ctx) {
		var waypoint models.TrackWaypoint
		if err := cur.Decode(&waypoint); err != nil {
			return nil, err
		}
		res = append(res, &waypoint)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return res, nil
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
