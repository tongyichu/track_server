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
			"user_id":                        t.UserID,
			"city_code":                      t.CityCode,
			"track_type":                     t.TrackType,
			"title":                          t.Title,
			"start_time":                     t.StartTime,
			"end_time":                       t.EndTime,
			"distance":                       t.Distance,
			"duration":                       t.Duration,
			"avg_speed_kmh":                  t.AvgSpeedKmh,
			"elevation_gain":                 t.ElevationGain,
			"raw_track_url":                  t.RawTrackURL,
			"screenshot_url":                 t.TrackScreenshotURL,
			"track_no_map_bg_screenshot_url": t.TrackNoMapBgScreenshotURL,
			"is_running":                     t.IsRunning,
			"status":                         t.Status,
			"updated_at":                     t.UpdatedAt,
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

// StatsByUserID returns (trackCount, totalDistance) for tracks owned by user.
// 口径与 ListByUserID 保持一致：排除删除与进行中轨迹。
func (r *MongoTrackRepository) StatsByUserID(ctx context.Context, userID int64) (int64, float64, error) {
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{
			"user_id":    userID,
			"is_running": false,
			"status": bson.M{"$in": []models.TrackStatus{models.TrackStatusNormal, models.TrackStatusPrivate}},
		}}},
		bson.D{{Key: "$group", Value: bson.M{
			"_id":  nil,
			"cnt":  bson.M{"$sum": 1},
			"dist": bson.M{"$sum": "$distance"},
		}}},
	}
	cur, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, 0, err
	}
	defer cur.Close(ctx)

	var out struct {
		Cnt  int64   `bson:"cnt"`
		Dist float64 `bson:"dist"`
	}
	if cur.Next(ctx) {
		if err := cur.Decode(&out); err != nil {
			return 0, 0, err
		}
		return out.Cnt, out.Dist, nil
	}
	if err := cur.Err(); err != nil {
		return 0, 0, err
	}
	return 0, 0, nil
}

// ListByUserID lists tracks of a user ordered by start time desc.
// It excludes deleted tracks and running tracks by default.
func (r *MongoTrackRepository) ListByUserID(ctx context.Context, userID int64, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	filter := bson.M{
		"user_id":    userID,
		"is_running": false,
		"status":     bson.M{"$in": []models.TrackStatus{models.TrackStatusNormal, models.TrackStatusPrivate}},
	}
	if cursor != nil {
		filter["$or"] = []bson.M{
			{"start_time": bson.M{"$lt": cursor.StartTime}},
			{"start_time": cursor.StartTime, "id": bson.M{"$lt": cursor.ID}},
		}
	}
	return r.listTracks(ctx,
		filter,
		options.Find().SetSort(bson.D{{Key: "start_time", Value: -1}, {Key: "id", Value: -1}}).SetLimit(int64(limit)),
	)
}

// ListRecommend lists normal-status tracks ordered by start_time desc, id desc.
func (r *MongoTrackRepository) ListRecommend(ctx context.Context, _ int64, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	filter := bson.M{"status": models.TrackStatusNormal, "is_running": false}
	if cursor != nil {
		filter["$or"] = []bson.M{
			{"start_time": bson.M{"$lt": cursor.StartTime}},
			{"start_time": cursor.StartTime, "id": bson.M{"$lt": cursor.ID}},
		}
	}
	return r.listTracks(ctx,
		filter,
		options.Find().SetSort(bson.D{{Key: "start_time", Value: -1}, {Key: "id", Value: -1}}).SetLimit(int64(limit)),
	)
}

// Search performs case-insensitive title search on normal-status tracks.
func (r *MongoTrackRepository) Search(ctx context.Context, keyword string, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	filter := bson.M{"status": models.TrackStatusNormal}
	if keyword != "" {
		filter["title"] = primitive.Regex{Pattern: regexp.QuoteMeta(keyword), Options: "i"}
	}
	if cursor != nil {
		filter["$or"] = []bson.M{
			{"start_time": bson.M{"$lt": cursor.StartTime}},
			{"start_time": cursor.StartTime, "id": bson.M{"$lt": cursor.ID}},
		}
	}
	return r.listTracks(ctx,
		filter,
		options.Find().SetSort(bson.D{{Key: "start_time", Value: -1}, {Key: "id", Value: -1}}).SetLimit(int64(limit)),
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

// FindByNickname is not implemented in this demo and returns an error.
func (r *MongoUserRepository) FindByNickname(context.Context, string) (*models.User, error) {
	return nil, errors.New("MongoUserRepository.FindByNickname not implemented")
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

func (r *MongoCollectRepository) ListByUserID(context.Context, int64, *models.TrackCollectCursor, int) ([]*models.TrackCollect, error) {
	return nil, errors.New("MongoCollectRepository.ListByUserID not implemented")
}

func (r *MongoCollectRepository) RemoveByTrackID(context.Context, string) error {
	return errors.New("MongoCollectRepository.RemoveByTrackID not implemented")
}

func (r *MongoCollectRepository) CountByTrackIDs(context.Context, []string) (map[string]int64, error) {
	return nil, errors.New("MongoCollectRepository.CountByTrackIDs not implemented")
}

// AddCollect is not implemented in this demo and returns an error.
func (r *MongoCollectRepository) AddCollect(context.Context, int64, string) error {
	return errors.New("MongoCollectRepository.AddCollect not implemented")
}

// RemoveCollect is not implemented in this demo and returns an error.
func (r *MongoCollectRepository) RemoveCollect(context.Context, int64, string) error {
	return errors.New("MongoCollectRepository.RemoveCollect not implemented")
}

// MongoNavigationRepository is a stub of NavigationRepository backed by MongoDB.
type MongoNavigationRepository struct {
	collection *mongo.Collection
}

// NewMongoNavigationRepository constructs a Mongo-backed NavigationRepository.
func NewMongoNavigationRepository(collection *mongo.Collection) *MongoNavigationRepository {
	return &MongoNavigationRepository{collection: collection}
}

func (r *MongoNavigationRepository) AddNavigation(context.Context, int64, string) error {
	return errors.New("MongoNavigationRepository.AddNavigation not implemented")
}

func (r *MongoNavigationRepository) CountByTrackIDs(context.Context, []string) (map[string]int64, error) {
	return nil, errors.New("MongoNavigationRepository.CountByTrackIDs not implemented")
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
