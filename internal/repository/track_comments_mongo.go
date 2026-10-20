package repository

import (
	"context"
	"errors"
	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"strconv"
	"sync"
)

type MongoCommentRepository struct {
	db      *mongo.Database
	mu      sync.Mutex
	indexed bool
}

func NewMongoCommentRepository(db *mongo.Database) *MongoCommentRepository {
	return &MongoCommentRepository{db: db}
}
func (r *MongoCommentRepository) ensure(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.indexed {
		return nil
	}
	_, err := r.db.Collection("track_comments").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "client_id", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "track_id", Value: 1}, {Key: "root_id", Value: 1}, {Key: "created_at", Value: -1}, {Key: "_id", Value: -1}}},
		{Keys: bson.D{{Key: "root_id", Value: 1}, {Key: "user_id", Value: 1}, {Key: "deleted", Value: 1}, {Key: "created_at", Value: -1}, {Key: "_id", Value: -1}}},
	})
	if err == nil {
		_, err = r.db.Collection("track_comment_reactions").Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "comment_id", Value: 1}, {Key: "reaction", Value: 1}, {Key: "user_id", Value: -1}}, Options: options.Index().SetName("idx_comment_likers")})
	}
	r.indexed = err == nil
	return err
}

type mongoCommentTx struct {
	ctx context.Context
	r   *MongoCommentRepository
}

func (t mongoCommentTx) find(filter interface{}) (*models.TrackComment, error) {
	c := &models.TrackComment{}
	err := t.r.db.Collection("track_comments").FindOne(t.ctx, filter).Decode(c)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	return c, err
}
func (t mongoCommentTx) Find(id string) (*models.TrackComment, error) {
	return t.find(bson.M{"_id": id})
}
func (t mongoCommentTx) FindClient(user int64, key string) (*models.TrackComment, error) {
	return t.find(bson.M{"user_id": user, "client_id": key})
}
func (t mongoCommentTx) Save(c *models.TrackComment) error {
	_, err := t.r.db.Collection("track_comments").ReplaceOne(t.ctx, bson.M{"_id": c.ID}, c, options.Replace().SetUpsert(true))
	return err
}
func (t mongoCommentTx) Image(id string) (*models.CommentImage, error) {
	image := &models.CommentImage{}
	err := t.r.db.Collection("track_comment_images").FindOne(t.ctx, bson.M{"_id": id}).Decode(image)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	return image, err
}
func (t mongoCommentTx) SaveImage(image *models.CommentImage) error {
	_, err := t.r.db.Collection("track_comment_images").ReplaceOne(t.ctx, bson.M{"_id": image.ID}, image, options.Replace().SetUpsert(true))
	return err
}
func (t mongoCommentTx) AddTotal(track string, delta int64) error {
	_, err := t.r.db.Collection("track_comment_stats").UpdateOne(t.ctx, bson.M{"_id": track}, bson.M{"$inc": bson.M{"total_count": delta}})
	return err
}
func commentReactionID(id string, user int64) string { return id + ":" + strconv.FormatInt(user, 10) }
func (t mongoCommentTx) Reaction(id string, user int64) (int, error) {
	var row struct {
		Value int `bson:"reaction"`
	}
	err := t.r.db.Collection("track_comment_reactions").FindOne(t.ctx, bson.M{"_id": commentReactionID(id, user)}).Decode(&row)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return 0, nil
	}
	return row.Value, err
}
func (t mongoCommentTx) SetReaction(id string, user int64, value int) error {
	_, err := t.r.db.Collection("track_comment_reactions").UpdateOne(t.ctx, bson.M{"_id": commentReactionID(id, user)}, bson.M{"$set": bson.M{"comment_id": id, "user_id": user, "reaction": value}}, options.Update().SetUpsert(true))
	return err
}
func (r *MongoCommentRepository) Apply(ctx context.Context, m CommentMutation) (*models.TrackComment, error) {
	if err := r.ensure(ctx); err != nil {
		return nil, err
	}
	var err error
	if m.Action != "reaction" {
		_, err = r.db.Collection("track_comment_stats").UpdateOne(ctx, bson.M{"_id": m.TrackID}, bson.M{"$setOnInsert": bson.M{"total_count": int64(0), "version": int64(0)}}, options.Update().SetUpsert(true))
		if err != nil && !mongo.IsDuplicateKeyError(err) {
			return nil, err
		}
	}
	session, err := r.db.Client().StartSession()
	if err != nil {
		return nil, err
	}
	defer session.EndSession(ctx)
	result, err := session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		if m.Action == "reaction" {
			result, err := r.db.Collection("track_comments").UpdateOne(sc, bson.M{"_id": m.CommentID, "track_id": m.TrackID}, bson.M{"$inc": bson.M{"mutation_version": 1}})
			if err != nil {
				return nil, err
			}
			if result.MatchedCount == 0 {
				return nil, ErrNotFound
			}
		} else {
			if _, err := r.db.Collection("track_comment_stats").UpdateOne(sc, bson.M{"_id": m.TrackID}, bson.M{"$inc": bson.M{"version": 1}}); err != nil {
				return nil, err
			}
		}
		return applyCommentMutation(mongoCommentTx{sc, r}, m)
	})
	if err != nil {
		return nil, err
	}
	return result.(*models.TrackComment), nil
}
func (r *MongoCommentRepository) Find(ctx context.Context, id string) (*models.TrackComment, error) {
	return (mongoCommentTx{ctx, r}).Find(id)
}
func (r *MongoCommentRepository) FindClient(ctx context.Context, user int64, key string) (*models.TrackComment, error) {
	return (mongoCommentTx{ctx, r}).FindClient(user, key)
}
func (r *MongoCommentRepository) FindMany(ctx context.Context, ids []string) (map[string]*models.TrackComment, error) {
	out := map[string]*models.TrackComment{}
	if len(ids) == 0 {
		return out, nil
	}
	cur, err := r.db.Collection("track_comments").Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var items []*models.TrackComment
	if err := cur.All(ctx, &items); err != nil {
		return nil, err
	}
	for _, c := range items {
		out[c.ID] = c
	}
	return out, nil
}
func (r *MongoCommentRepository) List(ctx context.Context, track, root string, cursor *models.CommentCursor, limit int) ([]*models.TrackComment, error) {
	filter := bson.M{"track_id": track, "root_id": root}
	conditions := bson.A{}
	if root == "" {
		conditions = append(conditions, bson.M{"$or": bson.A{bson.M{"deleted": false}, bson.M{"reply_count": bson.M{"$gt": 0}}}})
	} else {
		filter["deleted"] = false
	}
	if cursor != nil {
		conditions = append(conditions, bson.M{"$or": bson.A{bson.M{"created_at": bson.M{"$lt": cursor.CreatedAt}}, bson.M{"created_at": cursor.CreatedAt, "_id": bson.M{"$lt": cursor.ID}}}})
	}
	if len(conditions) > 0 {
		filter["$and"] = conditions
	}
	cur, err := r.db.Collection("track_comments").Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []*models.TrackComment{}
	err = cur.All(ctx, &out)
	return out, err
}
func (r *MongoCommentRepository) Total(ctx context.Context, track string) (int64, error) {
	var row struct {
		Total int64 `bson:"total_count"`
	}
	err := r.db.Collection("track_comment_stats").FindOne(ctx, bson.M{"_id": track}).Decode(&row)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return 0, nil
	}
	return row.Total, err
}
func (r *MongoCommentRepository) Totals(ctx context.Context, tracks []string) (map[string]int64, error) {
	counts := map[string]int64{}
	if len(tracks) == 0 {
		return counts, nil
	}
	cur, err := r.db.Collection("track_comment_stats").Find(ctx, bson.M{"_id": bson.M{"$in": tracks}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var row struct {
			ID    string `bson:"_id"`
			Total int64  `bson:"total_count"`
		}
		if err := cur.Decode(&row); err != nil {
			return nil, err
		}
		counts[row.ID] = row.Total
	}
	return counts, cur.Err()
}
func (r *MongoCommentRepository) AuthorReplies(ctx context.Context, user int64, roots []string) (map[string]*models.TrackComment, error) {
	out := map[string]*models.TrackComment{}
	if len(roots) == 0 {
		return out, nil
	}
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{"_id": bson.M{"$in": roots}}}},
		bson.D{{Key: "$lookup", Value: bson.M{"from": "track_comments", "let": bson.M{"root": "$_id"}, "pipeline": mongo.Pipeline{
			bson.D{{Key: "$match", Value: bson.M{"user_id": user, "deleted": false, "$expr": bson.M{"$eq": bson.A{"$root_id", "$$root"}}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "created_at", Value: -1}, {Key: "_id", Value: -1}}}}, bson.D{{Key: "$limit", Value: 1}}}, "as": "preview"}}},
		bson.D{{Key: "$unwind", Value: "$preview"}}, bson.D{{Key: "$replaceRoot", Value: bson.M{"newRoot": "$preview"}}},
	}
	cur, err := r.db.Collection("track_comments").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var items []*models.TrackComment
	if err := cur.All(ctx, &items); err != nil {
		return nil, err
	}
	for _, c := range items {
		out[c.RootID] = c
	}
	return out, nil
}
func (r *MongoCommentRepository) Reactions(ctx context.Context, user int64, ids []string) (map[string]int, error) {
	out := map[string]int{}
	if len(ids) == 0 {
		return out, nil
	}
	keys := []string{}
	for _, id := range ids {
		keys = append(keys, commentReactionID(id, user))
	}
	cur, err := r.db.Collection("track_comment_reactions").Find(ctx, bson.M{"_id": bson.M{"$in": keys}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var row struct {
			ID    string `bson:"comment_id"`
			Value int    `bson:"reaction"`
		}
		if err := cur.Decode(&row); err != nil {
			return nil, err
		}
		out[row.ID] = row.Value
	}
	return out, cur.Err()
}
func (r *MongoCommentRepository) Likers(ctx context.Context, id string, before int64, limit int) ([]int64, error) {
	if err := r.ensure(ctx); err != nil {
		return nil, err
	}
	filter := bson.M{"comment_id": id, "reaction": 1}
	if before > 0 {
		filter["user_id"] = bson.M{"$lt": before}
	}
	cur, err := r.db.Collection("track_comment_reactions").Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "user_id", Value: -1}}).SetLimit(int64(limit)).SetProjection(bson.M{"user_id": 1, "_id": 0}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	users := []int64{}
	for cur.Next(ctx) {
		var row struct {
			UserID int64 `bson:"user_id"`
		}
		if err := cur.Decode(&row); err != nil {
			return nil, err
		}
		users = append(users, row.UserID)
	}
	return users, cur.Err()
}
func (r *MongoCommentRepository) PutImage(ctx context.Context, image *models.CommentImage) error {
	return (mongoCommentTx{ctx, r}).SaveImage(image)
}
func (r *MongoCommentRepository) Image(ctx context.Context, id string) (*models.CommentImage, error) {
	return (mongoCommentTx{ctx, r}).Image(id)
}
