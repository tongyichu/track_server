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
	if collection != nil {
		_, _ = collection.Indexes().CreateOne(context.Background(), mongo.IndexModel{
			Keys: bson.D{
				{Key: "city_code", Value: 1},
				{Key: "status", Value: 1},
				{Key: "is_running", Value: 1},
				{Key: "start_time", Value: -1},
				{Key: "_id", Value: -1},
			},
			Options: options.Index().SetName("idx_track_recommend_city"),
		})
		_, _ = collection.Indexes().CreateOne(context.Background(), mongo.IndexModel{
			Keys: bson.D{
				{Key: "city_code", Value: 1},
				{Key: "status", Value: 1},
				{Key: "start_time", Value: -1},
				{Key: "_id", Value: -1},
			},
			Options: options.Index().SetName("idx_track_search_city"),
		})
	}
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
			"session_id":                     t.SessionID,
			"city_code":                      t.CityCode,
			"locate_addr":                    t.LocateAddr,
			"track_type":                     t.TrackType,
			"source_tag":                     t.SourceTag,
			"coordinate_system":              t.CoordinateSystem,
			"title":                          t.Title,
			"start_time":                     t.StartTime,
			"end_time":                       t.EndTime,
			"distance":                       t.Distance,
			"duration":                       t.Duration,
			"calories_burned":                t.CaloriesBurned,
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

func (r *MongoTrackRepository) FindByIDs(ctx context.Context, ids []string) (map[string]*models.Track, error) {
	result := make(map[string]*models.Track, len(ids))
	ids = uniqueNonEmptyStrings(ids)
	if len(ids) == 0 {
		return result, nil
	}
	cursor, err := r.collection.Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		track := &models.Track{}
		if err := cursor.Decode(track); err != nil {
			return nil, err
		}
		result[track.ID] = track
	}
	return result, cursor.Err()
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
// 口径与 ListByUserID 保持一致：排除删除与进行中轨迹；其中 trackCount 仅统计 raw_track_url 非空的轨迹。
func (r *MongoTrackRepository) StatsByUserID(ctx context.Context, userID int64) (int64, float64, error) {
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{
			"user_id":    userID,
			"is_running": false,
			"status":     bson.M{"$in": []models.TrackStatus{models.TrackStatusNormal, models.TrackStatusPrivate}},
		}}},
		bson.D{{Key: "$group", Value: bson.M{
			"_id": nil,
			"cnt": bson.M{"$sum": bson.M{"$cond": bson.A{
				bson.M{"$and": bson.A{
					bson.M{"$ne": bson.A{"$raw_track_url", nil}},
					bson.M{"$ne": bson.A{"$raw_track_url", ""}},
				}},
				1,
				0,
			}}},
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

// StatsSummaryByUserID returns user track statistics from tracks collection.
// 口径：排除删除与进行中轨迹，统计正常/私密轨迹的总里程、次数、总耗时和总热量。
func (r *MongoTrackRepository) StatsSummaryByUserID(ctx context.Context, userID int64) (*models.TrackUserStats, error) {
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{
			"user_id":    userID,
			"is_running": false,
			"status":     bson.M{"$in": []models.TrackStatus{models.TrackStatusNormal, models.TrackStatusPrivate}},
		}}},
		bson.D{{Key: "$group", Value: bson.M{
			"_id":            nil,
			"track_count":    bson.M{"$sum": 1},
			"total_distance": bson.M{"$sum": "$distance"},
			"total_duration": bson.M{"$sum": "$duration"},
			"total_calories": bson.M{"$sum": "$calories_burned"},
		}}},
	}
	cur, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	stats := &models.TrackUserStats{}
	if cur.Next(ctx) {
		if err := cur.Decode(stats); err != nil {
			return nil, err
		}
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return stats, nil
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
func (r *MongoTrackRepository) ListRecommend(ctx context.Context, _ int64, cityCode string, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	filter := bson.M{"status": models.TrackStatusNormal, "is_running": false}
	if cityCode != "" {
		filter["city_code"] = cityCode
	}
	if cursor != nil {
		filter["$or"] = []bson.M{
			{"start_time": bson.M{"$lt": cursor.StartTime}},
			{"start_time": cursor.StartTime, "_id": bson.M{"$lt": cursor.ID}},
		}
	}
	return r.listTracks(ctx,
		filter,
		options.Find().SetSort(bson.D{{Key: "start_time", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(int64(limit)),
	)
}

// Search performs case-insensitive title search on normal-status tracks.
func (r *MongoTrackRepository) Search(ctx context.Context, keyword, cityCode string, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	filter := bson.M{"status": models.TrackStatusNormal}
	if cityCode != "" {
		filter["city_code"] = cityCode
	}
	if keyword != "" {
		filter["title"] = primitive.Regex{Pattern: regexp.QuoteMeta(keyword), Options: "i"}
	}
	if cursor != nil {
		filter["$or"] = []bson.M{
			{"start_time": bson.M{"$lt": cursor.StartTime}},
			{"start_time": cursor.StartTime, "_id": bson.M{"$lt": cursor.ID}},
		}
	}
	return r.listTracks(ctx,
		filter,
		options.Find().SetSort(bson.D{{Key: "start_time", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(int64(limit)),
	)
}

// ListAll 返回全量未删除轨迹（按 start_time desc, id desc）。仅供管理后台使用。
func (r *MongoTrackRepository) ListAll(ctx context.Context, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	filter := bson.M{"status": bson.M{"$ne": models.TrackStatusDeleted}}
	if cursor != nil && !cursor.StartTime.IsZero() {
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

// CountAll 返回全量未删除轨迹数量。仅供管理后台使用。
func (r *MongoTrackRepository) CountAll(ctx context.Context) (int64, error) {
	return r.collection.CountDocuments(ctx, bson.M{"status": bson.M{"$ne": models.TrackStatusDeleted}})
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
func (r *MongoUserRepository) FindByID(ctx context.Context, id int64) (*models.User, error) {
	user := &models.User{}
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	return user, err
}

func (r *MongoUserRepository) FindByIDs(ctx context.Context, ids []int64) (map[int64]*models.User, error) {
	result := make(map[int64]*models.User, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := r.collection.Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	defer rows.Close(ctx)
	for rows.Next(ctx) {
		user := &models.User{}
		if err := rows.Decode(user); err != nil {
			return nil, err
		}
		result[user.ID] = user
	}
	return result, rows.Err()
}

func (r *MongoUserRepository) FindByPhone(ctx context.Context, phone string) (*models.User, error) {
	var user models.User
	err := r.collection.FindOne(ctx, bson.M{"phone": phone}).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByNickname is not implemented in this demo and returns an error.
func (r *MongoUserRepository) FindByNickname(context.Context, string) (*models.User, error) {
	return nil, errors.New("MongoUserRepository.FindByNickname not implemented")
}

// Update is not implemented in this demo and returns an error.
func (r *MongoUserRepository) Update(context.Context, *models.User) error {
	return errors.New("MongoUserRepository.Update not implemented")
}

// ListAll is not implemented in this demo and returns an error.
func (r *MongoUserRepository) ListAll(ctx context.Context, cursor *models.UserListCursor, limit int) ([]*models.User, error) {
	if limit <= 0 {
		limit = 20
	}
	filter := bson.M{}
	if cursor != nil {
		filter["$or"] = bson.A{bson.M{"created_at": bson.M{"$lt": cursor.CreatedAt}}, bson.M{"created_at": cursor.CreatedAt, "_id": bson.M{"$lt": cursor.ID}}}
	}
	rows, err := r.collection.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer rows.Close(ctx)
	items := make([]*models.User, 0)
	for rows.Next(ctx) {
		item := &models.User{}
		if err := rows.Decode(item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// CountAll is not implemented in this demo and returns an error.
func (r *MongoUserRepository) CountAll(context.Context) (int64, error) {
	return 0, errors.New("MongoUserRepository.CountAll not implemented")
}

type MongoAccountRestrictionRepository struct {
	collection *mongo.Collection
}

func NewMongoAccountRestrictionRepository(collection *mongo.Collection) *MongoAccountRestrictionRepository {
	return &MongoAccountRestrictionRepository{collection: collection}
}

func (r *MongoAccountRestrictionRepository) CreateAccountRestriction(context.Context, *models.AccountRestriction) error {
	return errors.New("MongoAccountRestrictionRepository.CreateAccountRestriction not implemented")
}

func (r *MongoAccountRestrictionRepository) FindActiveAccountRestriction(context.Context, int64, time.Time) (*models.AccountRestriction, error) {
	return nil, errors.New("MongoAccountRestrictionRepository.FindActiveAccountRestriction not implemented")
}

func (r *MongoAccountRestrictionRepository) ListAccountRestrictionsByUserID(context.Context, int64, int) ([]*models.AccountRestriction, error) {
	return nil, errors.New("MongoAccountRestrictionRepository.ListAccountRestrictionsByUserID not implemented")
}

func (r *MongoAccountRestrictionRepository) RevokeActiveAccountRestrictions(context.Context, int64, string, time.Time) (int64, error) {
	return 0, errors.New("MongoAccountRestrictionRepository.RevokeActiveAccountRestrictions not implemented")
}

// MongoCollectRepository is a stub of CollectRepository backed by MongoDB.
type MongoCollectRepository struct {
	collection *mongo.Collection
}

// NewMongoCollectRepository constructs a Mongo-backed CollectRepository.
func NewMongoCollectRepository(collection *mongo.Collection) *MongoCollectRepository {
	if collection != nil {
		_, _ = collection.Indexes().CreateOne(context.Background(), mongo.IndexModel{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "track_id", Value: 1}}, Options: options.Index().SetName("uk_collect_user_track").SetUnique(true)})
	}
	return &MongoCollectRepository{collection: collection}
}

// IsCollected is not implemented in this demo and returns an error.
func (r *MongoCollectRepository) IsCollected(ctx context.Context, userID int64, trackID string) (bool, error) {
	err := r.collection.FindOne(ctx, bson.M{"user_id": userID, "track_id": trackID}).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	return err == nil, err
}

func (r *MongoCollectRepository) ListCollectedByTrackIDs(ctx context.Context, userID int64, trackIDs []string) (map[string]bool, error) {
	ids := uniqueNonEmptyStrings(trackIDs)
	result := make(map[string]bool, len(ids))
	for _, id := range ids {
		result[id] = false
	}
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := r.collection.Find(ctx, bson.M{"user_id": userID, "track_id": bson.M{"$in": ids}}, options.Find().SetProjection(bson.M{"track_id": 1}))
	if err != nil {
		return nil, err
	}
	defer rows.Close(ctx)
	for rows.Next(ctx) {
		var row struct {
			TrackID string `bson:"track_id"`
		}
		if err := rows.Decode(&row); err != nil {
			return nil, err
		}
		result[row.TrackID] = true
	}
	return result, rows.Err()
}

func (r *MongoCollectRepository) ListByUserID(ctx context.Context, userID int64, cursor *models.TrackCollectCursor, limit int) ([]*models.TrackCollect, error) {
	if limit <= 0 {
		limit = 20
	}
	filter := bson.M{"user_id": userID}
	if cursor != nil {
		filter["$or"] = bson.A{bson.M{"created_at": bson.M{"$lt": cursor.CreatedAt}}, bson.M{"created_at": cursor.CreatedAt, "track_id": bson.M{"$lt": cursor.TrackID}}}
	}
	rows, err := r.collection.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "track_id", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer rows.Close(ctx)
	items := make([]*models.TrackCollect, 0)
	for rows.Next(ctx) {
		item := &models.TrackCollect{}
		if err := rows.Decode(item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MongoCollectRepository) RemoveByTrackID(context.Context, string) error {
	return errors.New("MongoCollectRepository.RemoveByTrackID not implemented")
}

func (r *MongoCollectRepository) CountByTrackIDs(ctx context.Context, trackIDs []string) (map[string]int64, error) {
	ids := uniqueNonEmptyStrings(trackIDs)
	result := make(map[string]int64, len(ids))
	for _, id := range ids {
		result[id] = 0
	}
	if len(ids) == 0 {
		return result, nil
	}
	pipeline := mongo.Pipeline{bson.D{{Key: "$match", Value: bson.M{"track_id": bson.M{"$in": ids}}}}, bson.D{{Key: "$group", Value: bson.M{"_id": "$track_id", "count": bson.M{"$sum": 1}}}}}
	rows, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer rows.Close(ctx)
	for rows.Next(ctx) {
		var row struct {
			TrackID string `bson:"_id"`
			Count   int64  `bson:"count"`
		}
		if err := rows.Decode(&row); err != nil {
			return nil, err
		}
		result[row.TrackID] = row.Count
	}
	return result, rows.Err()
}

// AddCollect is not implemented in this demo and returns an error.
func (r *MongoCollectRepository) AddCollect(context.Context, int64, string) error {
	return errors.New("MongoCollectRepository.AddCollect not implemented")
}

// RemoveCollect is not implemented in this demo and returns an error.
func (r *MongoCollectRepository) RemoveCollect(context.Context, int64, string) error {
	return errors.New("MongoCollectRepository.RemoveCollect not implemented")
}

// MongoFollowRepository is a stub of FollowRepository backed by MongoDB.
type MongoFollowRepository struct {
	collection *mongo.Collection
}

// NewMongoFollowRepository constructs a Mongo-backed FollowRepository.
func NewMongoFollowRepository(collection *mongo.Collection) *MongoFollowRepository {
	return &MongoFollowRepository{collection: collection}
}

func (r *MongoFollowRepository) IsFollowing(context.Context, int64, int64) (bool, error) {
	return false, errors.New("MongoFollowRepository.IsFollowing not implemented")
}

func (r *MongoFollowRepository) AddFollow(context.Context, int64, int64) error {
	return errors.New("MongoFollowRepository.AddFollow not implemented")
}

func (r *MongoFollowRepository) RemoveFollow(context.Context, int64, int64) error {
	return errors.New("MongoFollowRepository.RemoveFollow not implemented")
}

func (r *MongoFollowRepository) ListFollowing(ctx context.Context, userID int64, cursor *models.UserFollowCursor, limit int) ([]*models.UserFollow, error) {
	if limit <= 0 {
		limit = 20
	}
	filter := bson.M{"follower_user_id": userID}
	if cursor != nil {
		filter["$or"] = bson.A{bson.M{"created_at": bson.M{"$lt": cursor.CreatedAt}}, bson.M{"created_at": cursor.CreatedAt, "followee_user_id": bson.M{"$lt": cursor.UserID}}}
	}
	rows, err := r.collection.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "followee_user_id", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer rows.Close(ctx)
	items := make([]*models.UserFollow, 0)
	for rows.Next(ctx) {
		item := &models.UserFollow{}
		if err := rows.Decode(item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MongoFollowRepository) ListFollowers(context.Context, int64, *models.UserFollowCursor, int) ([]*models.UserFollow, error) {
	return nil, errors.New("MongoFollowRepository.ListFollowers not implemented")
}

func (r *MongoFollowRepository) CountFollowing(context.Context, int64) (int64, error) {
	return 0, errors.New("MongoFollowRepository.CountFollowing not implemented")
}

func (r *MongoFollowRepository) CountFollowers(context.Context, int64) (int64, error) {
	return 0, errors.New("MongoFollowRepository.CountFollowers not implemented")
}

// MongoNavigationRepository implements NavigationRepository backed by MongoDB.
type MongoNavigationRepository struct {
	collection *mongo.Collection
}

// NewMongoNavigationRepository constructs a Mongo-backed NavigationRepository.
func NewMongoNavigationRepository(collection *mongo.Collection) *MongoNavigationRepository {
	return &MongoNavigationRepository{collection: collection}
}

func (r *MongoNavigationRepository) AddNavigation(ctx context.Context, userID int64, trackID string) error {
	if userID <= 0 || trackID == "" {
		return nil
	}
	_, err := r.collection.InsertOne(ctx, &models.TrackNavigation{
		ID: time.Now().UnixNano(), TrackID: trackID, NavigatorUserID: userID, CreatedAt: time.Now(),
	})
	return err
}

func (r *MongoNavigationRepository) CountByTrackIDs(ctx context.Context, trackIDs []string) (map[string]int64, error) {
	ids := uniqueNonEmptyStrings(trackIDs)
	result := make(map[string]int64, len(ids))
	for _, id := range ids {
		result[id] = 0
	}
	if len(ids) == 0 {
		return result, nil
	}
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{"track_id": bson.M{"$in": ids}}}},
		bson.D{{Key: "$group", Value: bson.M{"_id": "$track_id", "count": bson.M{"$sum": 1}}}},
	}
	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var row struct {
			TrackID string `bson:"_id"`
			Count   int64  `bson:"count"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		result[row.TrackID] = row.Count
	}
	return result, cursor.Err()
}

func (r *MongoNavigationRepository) ListByUserID(ctx context.Context, userID int64, limit int) ([]*models.TrackNavigation, error) {
	if limit <= 0 {
		limit = 1000
	}
	cursor, err := r.collection.Find(ctx, bson.M{"navigator_user_id": userID}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	items := make([]*models.TrackNavigation, 0)
	for cursor.Next(ctx) {
		item := &models.TrackNavigation{}
		if err := cursor.Decode(item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, cursor.Err()
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

// MongoAppReleaseRepository is a stub of AppReleaseRepository backed by MongoDB.
type MongoAppReleaseRepository struct {
	collection *mongo.Collection
}

type MongoFeedbackRepository struct {
	collection *mongo.Collection
}

// NewMongoAppReleaseRepository constructs a Mongo-backed AppReleaseRepository.
func NewMongoAppReleaseRepository(collection *mongo.Collection) *MongoAppReleaseRepository {
	return &MongoAppReleaseRepository{collection: collection}
}

func NewMongoFeedbackRepository(collection *mongo.Collection) *MongoFeedbackRepository {
	return &MongoFeedbackRepository{collection: collection}
}

func (r *MongoAppReleaseRepository) Upsert(context.Context, *models.AppRelease) error {
	return errors.New("MongoAppReleaseRepository.Upsert not implemented")
}

func (r *MongoAppReleaseRepository) GetByID(context.Context, int64) (*models.AppRelease, error) {
	return nil, errors.New("MongoAppReleaseRepository.GetByID not implemented")
}

func (r *MongoAppReleaseRepository) GetByPlatformVersion(context.Context, models.AppReleasePlatform, int64) (*models.AppRelease, error) {
	return nil, errors.New("MongoAppReleaseRepository.GetByPlatformVersion not implemented")
}

func (r *MongoAppReleaseRepository) List(context.Context, models.AppReleaseListFilter) ([]*models.AppRelease, error) {
	return nil, errors.New("MongoAppReleaseRepository.List not implemented")
}

func (r *MongoAppReleaseRepository) GetLatestPublished(context.Context, models.AppReleasePlatform) (*models.AppRelease, error) {
	return nil, errors.New("MongoAppReleaseRepository.GetLatestPublished not implemented")
}

func (r *MongoAppReleaseRepository) Delete(context.Context, int64) error {
	return errors.New("MongoAppReleaseRepository.Delete not implemented")
}

func (r *MongoFeedbackRepository) Create(ctx context.Context, feedback *models.Feedback) error {
	if feedback == nil || feedback.FeedbackID == "" {
		return errors.New("feedback id is required")
	}
	now := time.Now()
	if feedback.CreatedAt.IsZero() {
		feedback.CreatedAt = now
	}
	if feedback.UpdatedAt.IsZero() {
		feedback.UpdatedAt = feedback.CreatedAt
	}
	if feedback.Status == "" {
		feedback.Status = models.FeedbackStatusPending
	}
	_, err := r.collection.InsertOne(ctx, feedback)
	if mongo.IsDuplicateKeyError(err) {
		return ErrAlreadyExists
	}
	return err
}

func (r *MongoFeedbackRepository) FindByFeedbackID(ctx context.Context, feedbackID string) (*models.Feedback, error) {
	var feedback models.Feedback
	err := r.collection.FindOne(ctx, bson.M{"feedback_id": feedbackID}).Decode(&feedback)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &feedback, nil
}

func (r *MongoFeedbackRepository) List(ctx context.Context, filter models.FeedbackListFilter) ([]*models.Feedback, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	query := bson.M{}
	if filter.UserID > 0 {
		query["user_id"] = filter.UserID
	}
	if filter.Status != "" {
		query["status"] = filter.Status
	}
	if filter.AppVersion != "" {
		query["app_version"] = filter.AppVersion
	}
	if filter.Cursor != nil && !filter.Cursor.CreatedAt.IsZero() && filter.Cursor.FeedbackID != "" {
		query["$or"] = bson.A{
			bson.M{"created_at": bson.M{"$lt": filter.Cursor.CreatedAt}},
			bson.M{"created_at": filter.Cursor.CreatedAt, "feedback_id": bson.M{"$lt": filter.Cursor.FeedbackID}},
		}
	}
	cur, err := r.collection.Find(ctx, query, options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "feedback_id", Value: -1}}).
		SetLimit(int64(limit)),
	)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	res := make([]*models.Feedback, 0, limit)
	for cur.Next(ctx) {
		var item models.Feedback
		if err := cur.Decode(&item); err != nil {
			return nil, err
		}
		res = append(res, &item)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (r *MongoFeedbackRepository) CountByUserAndStatuses(ctx context.Context, userID int64, statuses []models.FeedbackStatus) (int64, error) {
	if userID <= 0 || len(statuses) == 0 {
		return 0, nil
	}
	count, err := r.collection.CountDocuments(ctx, bson.M{
		"user_id": userID,
		"status":  bson.M{"$in": statuses},
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *MongoFeedbackRepository) UpdateStatus(ctx context.Context, feedbackID string, status models.FeedbackStatus, reply string) error {
	res, err := r.collection.UpdateOne(ctx,
		bson.M{"feedback_id": feedbackID},
		bson.M{"$set": bson.M{"status": status, "reply": reply, "updated_at": time.Now()}},
	)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}
