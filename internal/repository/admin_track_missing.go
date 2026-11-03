package repository

import (
	"context"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func ValidAdminTrackMissingFilter(value string) bool {
	switch value {
	case "", "any", "place_candidate", "scenic_spot", "description", "city", "previews":
		return true
	}
	return false
}

func adminTrackMatchesMissing(track *models.Track, missing string, linked, candidate bool) bool {
	if missing == "" {
		return true
	}
	checks := map[string]bool{
		"place_candidate": candidate,
		"scenic_spot":     !linked,
		"description":     strings.TrimSpace(track.Description) == "",
		"city":            strings.TrimSpace(track.CityCode) == "",
		"previews":        strings.TrimSpace(track.PreviewImages.SquareDayURL) == "" || strings.TrimSpace(track.PreviewImages.SquareNightURL) == "" || strings.TrimSpace(track.PreviewImages.LandscapeDayURL) == "" || strings.TrimSpace(track.PreviewImages.LandscapeNightURL) == "",
	}
	if missing == "any" {
		for _, value := range checks {
			if value {
				return true
			}
		}
		return false
	}
	return checks[missing]
}

func (r *InMemoryTrackRepository) SetAdminScenicSpotRepository(spots *MemoryScenicSpotRepository) {
	r.mu.Lock()
	r.adminScenicSpots = spots
	r.mu.Unlock()
}

func (r *InMemoryTrackRepository) adminLinkedScenicTracks(missing string) map[string]bool {
	linked := make(map[string]bool)
	if missing != "any" && missing != "scenic_spot" {
		return linked
	}
	r.mu.RLock()
	spots := r.adminScenicSpots
	r.mu.RUnlock()
	if spots == nil {
		return linked
	}
	// 单独读取景点快照，避免同时持有两个仓储的锁。
	spots.mu.RLock()
	defer spots.mu.RUnlock()
	for id, link := range spots.links {
		if link == nil {
			continue
		}
		if spot := spots.spots[link.SpotID]; spot != nil && spot.Status == models.ScenicSpotActive {
			linked[id] = true
		}
	}
	return linked
}

func adminTrackMissingSQL(filter AdminTrackListFilter) (string, []any) {
	if filter.Missing == "" {
		return "", nil
	}
	conditions := map[string]string{
		"description":     "COALESCE(description,'') REGEXP '^[[:space:]]*$'",
		"city":            "COALESCE(city_code,'') REGEXP '^[[:space:]]*$'",
		"scenic_spot":     `NOT EXISTS (SELECT 1 FROM track_scenic_spot_links l JOIN scenic_spots s ON s.spot_id=l.spot_id WHERE l.track_id=track_records.id AND s.status='active')`,
		"place_candidate": "FALSE",
	}
	previews := make([]string, 0, 4)
	for _, field := range []string{"square_day_url", "square_night_url", "landscape_day_url", "landscape_night_url"} {
		value := "JSON_EXTRACT(preview_images,'$." + field + "')"
		previews = append(previews, "IF(JSON_TYPE("+value+")='NULL','',COALESCE(JSON_UNQUOTE("+value+"),'')) REGEXP '^[[:space:]]*$'")
	}
	conditions["previews"] = "(" + strings.Join(previews, " OR ") + ")"
	var args []any
	if (filter.Missing == "any" || filter.Missing == "place_candidate") && len(filter.PlaceCandidateTrackIDs) > 0 {
		conditions["place_candidate"] = "id IN (" + strings.TrimRight(strings.Repeat("?,", len(filter.PlaceCandidateTrackIDs)), ",") + ")"
		for _, id := range filter.PlaceCandidateTrackIDs {
			args = append(args, id)
		}
	}
	if filter.Missing == "any" {
		parts := []string{conditions["description"], conditions["city"], conditions["previews"], conditions["scenic_spot"], conditions["place_candidate"]}
		return strings.Join(parts, " OR "), args
	}
	condition, ok := conditions[filter.Missing]
	if !ok {
		return "FALSE", nil
	}
	return condition, args
}

func adminMongoEmptyTrackField(field string) bson.M {
	return bson.M{"$or": bson.A{bson.M{field: nil}, bson.M{field: primitive.Regex{Pattern: `^\s*$`}}}}
}

func (r *MongoTrackRepository) listAdminTracksWithMissing(ctx context.Context, base bson.M, filter AdminTrackListFilter, cursor *models.TrackListCursor, limit int) ([]*models.Track, int64, error) {
	pipeline := mongo.Pipeline{{{Key: "$match", Value: base}}}
	if filter.Missing == "any" || filter.Missing == "scenic_spot" {
		pipeline = append(pipeline,
			bson.D{{Key: "$lookup", Value: bson.M{"from": "track_scenic_spot_links", "localField": "_id", "foreignField": "_id", "as": "_admin_links"}}},
			bson.D{{Key: "$lookup", Value: bson.M{"from": "scenic_spots", "localField": "_admin_links.scenic_spot_id", "foreignField": "_id", "as": "_admin_spots"}}},
		)
	}
	previewConditions := bson.A{}
	for _, field := range []string{"square_day_url", "square_night_url", "landscape_day_url", "landscape_night_url"} {
		previewConditions = append(previewConditions, adminMongoEmptyTrackField("preview_images."+field))
	}
	ids := bson.A{}
	for _, id := range filter.PlaceCandidateTrackIDs {
		ids = append(ids, id)
	}
	conditions := map[string]bson.M{
		"description":     adminMongoEmptyTrackField("description"),
		"city":            adminMongoEmptyTrackField("city_code"),
		"previews":        {"$or": previewConditions},
		"place_candidate": {"_id": bson.M{"$in": ids}},
		"scenic_spot":     {"_admin_spots": bson.M{"$not": bson.M{"$elemMatch": bson.M{"status": models.ScenicSpotActive}}}},
	}
	condition := conditions[filter.Missing]
	if filter.Missing == "any" {
		condition = bson.M{"$or": bson.A{conditions["description"], conditions["city"], conditions["previews"], conditions["scenic_spot"], conditions["place_candidate"]}}
	}
	if condition == nil {
		return nil, 0, ErrForbidden
	}
	pipeline = append(pipeline, bson.D{{Key: "$match", Value: condition}}, bson.D{{Key: "$project", Value: bson.M{"_admin_links": 0, "_admin_spots": 0}}})
	items := mongo.Pipeline{}
	if cursor != nil && !cursor.StartTime.IsZero() {
		items = append(items, bson.D{{Key: "$match", Value: bson.M{"$or": bson.A{bson.M{"start_time": bson.M{"$lt": cursor.StartTime}}, bson.M{"start_time": cursor.StartTime, "_id": bson.M{"$lt": cursor.ID}}}}}})
	}
	items = append(items, bson.D{{Key: "$sort", Value: bson.D{{Key: "start_time", Value: -1}, {Key: "_id", Value: -1}}}}, bson.D{{Key: "$limit", Value: limit}})
	pipeline = append(pipeline, bson.D{{Key: "$facet", Value: bson.M{"counts": mongo.Pipeline{{{Key: "$count", Value: "total"}}}, "items": items}}})
	cur, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, err
	}
	defer cur.Close(ctx)
	var result struct {
		Counts []struct {
			Total int64 `bson:"total"`
		} `bson:"counts"`
		Items []*models.Track `bson:"items"`
	}
	if !cur.Next(ctx) {
		return []*models.Track{}, 0, cur.Err()
	}
	if err := cur.Decode(&result); err != nil {
		return nil, 0, err
	}
	var total int64
	if len(result.Counts) > 0 {
		total = result.Counts[0].Total
	}
	for _, track := range result.Items {
		normalizeTrackDescriptionMetadata(track)
	}
	if result.Items == nil {
		result.Items = []*models.Track{}
	}
	return result.Items, total, nil
}
