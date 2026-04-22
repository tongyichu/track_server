package repository

import (
	"context"

	"errors"
	"regexp"
	"sync"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoTrackRepository is a stub of TrackRepository backed by MongoDB.
type MongoTrackRepository struct {
	collection   *mongo.Collection
	mu           sync.Mutex
	nextTrackSeq uint64
}

// MongoTrackWaypointRepository implements TrackWaypointRepository backed by MongoDB.
type MongoTrackWaypointRepository struct {
	mu         sync.Mutex
	nextID     uint64
	collection *mongo.Collection
}

// NewMongoTrackRepository constructs a Mongo-backed TrackRepository.
func NewMongoTrackRepository(collection *mongo.Collection) *MongoTrackRepository {
	return &MongoTrackRepository{
		collection:   collection,
		nextTrackSeq: uint64(time.Now().UnixNano()%int64(trackIDSequenceLimit-1)) + 1,
	}
}

func (r *MongoTrackRepository) NextTrackID(_ context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id, err := encodeTrackID(r.nextTrackSeq)
	if err != nil {
		return "", err
	}
	r.nextTrackSeq++
	return id, nil
}

// NewMongoTrackWaypointRepository constructs a Mongo-backed TrackWaypointRepository.
func NewMongoTrackWaypointRepository(collection *mongo.Collection) *MongoTrackWaypointRepository {
	return &MongoTrackWaypointRepository{collection: collection, nextID: uint64(time.Now().UnixNano())}
}

// Create stores a new track in MongoDB.
func (r *MongoTrackRepository) Create(ctx context.Context, t *models.Track) error {
	if t.ID == "" {
		return errors.New("track id is required")
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	if t.StartTime.IsZero() {
		t.StartTime = t.CreatedAt
	}
	if t.EndTime.IsZero() {
		t.EndTime = t.StartTime
	}
	t.UpdatedAt = t.CreatedAt

	_, err := r.collection.InsertOne(ctx, t)
	if mongo.IsDuplicateKeyError(err) {
		return ErrAlreadyExists
	}
	return err
}

// Update updates an existing track in MongoDB.
func (r *MongoTrackRepository) Update(ctx context.Context, t *models.Track) error {
	t.UpdatedAt = time.Now()
	if t.EndTime.IsZero() {
		t.EndTime = t.StartTime
	}

	res, err := r.collection.UpdateOne(ctx,
		bson.M{"_id": t.ID},
		bson.M{"$set": bson.M{
			"user_id":        t.UserID,
			"title":          t.Title,
			"start_time":     t.StartTime,
			"end_time":       t.EndTime,
			"distance":       t.Distance,
			"duration":       t.Duration,
			"avg_speed_kmh":  t.AvgSpeedKmh,
			"elevation_gain": t.ElevationGain,
			"raw_track_url":  t.RawTrackURL,
			"screenshot_url": t.TrackScreenshotURL,
			"is_running":     t.IsRunning,
			"status":         t.Status,
			"updated_at":     t.UpdatedAt,
		}},
	)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// FindByID finds a track by id.
func (r *MongoTrackRepository) FindByID(ctx context.Context, id string) (*models.Track, error) {
	var track models.Track
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&track)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &track, nil
}

// FindRunningByUserID finds the latest running track of a user.
func (r *MongoTrackRepository) FindRunningByUserID(ctx context.Context, userID int64) (*models.Track, error) {
	var track models.Track
	err := r.collection.FindOne(
		ctx,
		bson.M{"user_id": userID, "is_running": true},
		options.FindOne().SetSort(bson.D{{Key: "start_time", Value: -1}}),
	).Decode(&track)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &track, nil
}

// ListRecommend lists normal-status tracks ordered by start time desc.
func (r *MongoTrackRepository) ListRecommend(ctx context.Context, _ int64, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	return r.listTracks(ctx,
		bson.M{"status": models.TrackStatusNormal, "is_running": false},
		options.Find().SetSort(bson.D{{Key: "start_time", Value: -1}}).SetLimit(int64(limit)),
	)
}

// Search performs case-insensitive title search on normal-status tracks.
func (r *MongoTrackRepository) Search(ctx context.Context, keyword string, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	filter := bson.M{"status": models.TrackStatusNormal}
	if keyword != "" {
		filter["title"] = primitive.Regex{Pattern: regexp.QuoteMeta(keyword), Options: "i"}
	}
	return r.listTracks(ctx,
		filter,
		options.Find().SetSort(bson.D{{Key: "start_time", Value: -1}}).SetLimit(int64(limit)),
	)
}

func (r *MongoTrackRepository) listTracks(ctx context.Context, filter interface{}, opts ...*options.FindOptions) ([]*models.Track, error) {
	cur, err := r.collection.Find(ctx, filter, opts...)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	res := make([]*models.Track, 0)
	for cur.Next(ctx) {
		var track models.Track
		if err := cur.Decode(&track); err != nil {
			return nil, err
		}
		res = append(res, &track)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return res, nil
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
