package repository

import (
	"context"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	_ CompanionTrackMetadataWriter = (*MySQLTrackRepository)(nil)
	_ CompanionTrackMetadataWriter = (*MongoTrackRepository)(nil)
	_ CompanionTrackMetadataWriter = (*InMemoryTrackRepository)(nil)
)

type CompanionTrackMetadataWriter interface {
	ApplyCompanionDescription(context.Context, *models.Track, string) (bool, error)
	FillMissingCompanionLocation(context.Context, *models.Track, string, string) (bool, error)
}

func companionDescriptionWritable(t *models.Track) bool {
	return t.DescriptionSource == "" || t.DescriptionSource == "auto" || t.DescriptionSource == "ai" || t.DescriptionSource == "companion"
}
func (r *MySQLTrackRepository) ApplyCompanionDescription(ctx context.Context, expected *models.Track, text string) (bool, error) {
	if !companionDescriptionWritable(expected) {
		return false, nil
	}
	result, err := r.db.ExecContext(ctx, `UPDATE track_records SET route_description=?,description_source='companion',updated_at=? WHERE id=? AND user_id=? AND updated_at=? AND raw_track_url=? AND status IN (1,2) AND is_running=0 AND deleted_at IS NULL AND COALESCE(route_description,'')=? AND COALESCE(description_source,'auto') IN ('','auto','ai','companion')`, text, time.Now(), expected.ID, expected.UserID, expected.UpdatedAt, expected.RawTrackURL, expected.Description)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n > 0, err
}
func (r *MongoTrackRepository) ApplyCompanionDescription(ctx context.Context, expected *models.Track, text string) (bool, error) {
	if !companionDescriptionWritable(expected) {
		return false, nil
	}
	filter := bson.M{"_id": expected.ID, "user_id": expected.UserID, "updated_at": expected.UpdatedAt, "raw_track_url": expected.RawTrackURL, "status": bson.M{"$in": bson.A{models.TrackStatusNormal, models.TrackStatusPrivate}}, "is_running": false, "description_source": bson.M{"$in": bson.A{"", nil, "auto", "ai", "companion"}}}
	result, err := r.collection.UpdateOne(ctx, filter, bson.M{"$set": bson.M{"description": text, "description_source": "companion", "updated_at": time.Now()}})
	if err != nil {
		return false, err
	}
	return result.MatchedCount > 0, nil
}
func (r *InMemoryTrackRepository) ApplyCompanionDescription(_ context.Context, expected *models.Track, text string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.tracks[expected.ID]
	if t == nil || t.UserID != expected.UserID || !t.UpdatedAt.Equal(expected.UpdatedAt) || t.RawTrackURL != expected.RawTrackURL || t.IsRunning || !t.DeletedAt.IsZero() || (t.Status != models.TrackStatusNormal && t.Status != models.TrackStatusPrivate) || !companionDescriptionWritable(t) {
		return false, nil
	}
	t.Description = text
	t.DescriptionSource = "companion"
	t.UpdatedAt = time.Now()
	return true, nil
}

func missingCompanionLocation(t *models.Track, city, addr string) (bool, bool) {
	return t.CityCode == "" && city != "", t.LocateAddr == "" && addr != ""
}
func (r *MySQLTrackRepository) FillMissingCompanionLocation(ctx context.Context, t *models.Track, city, addr string) (bool, error) {
	setCity, setAddr := missingCompanionLocation(t, city, addr)
	if !setCity && !setAddr {
		return false, nil
	}
	query := "UPDATE track_records SET updated_at=?"
	args := []any{time.Now()}
	if setCity {
		query += ",city_code=?"
		args = append(args, city)
	}
	if setAddr {
		query += ",locate_addr=?"
		args = append(args, addr)
	}
	query += " WHERE id=? AND user_id=? AND updated_at=? AND raw_track_url=? AND status IN (1,2) AND is_running=0 AND deleted_at IS NULL"
	args = append(args, t.ID, t.UserID, t.UpdatedAt, t.RawTrackURL)
	if setCity {
		query += " AND COALESCE(city_code,'')=''"
	}
	if setAddr {
		query += " AND COALESCE(locate_addr,'')=''"
	}
	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
func (r *MongoTrackRepository) FillMissingCompanionLocation(ctx context.Context, t *models.Track, city, addr string) (bool, error) {
	setCity, setAddr := missingCompanionLocation(t, city, addr)
	if !setCity && !setAddr {
		return false, nil
	}
	set := bson.M{"updated_at": time.Now()}
	if setCity {
		set["city_code"] = city
	}
	if setAddr {
		set["locate_addr"] = addr
	}
	filter := bson.M{"_id": t.ID, "user_id": t.UserID, "updated_at": t.UpdatedAt, "raw_track_url": t.RawTrackURL, "status": bson.M{"$in": bson.A{models.TrackStatusNormal, models.TrackStatusPrivate}}, "is_running": false}
	if setCity {
		filter["city_code"] = bson.M{"$in": bson.A{"", nil}}
	}
	if setAddr {
		filter["locate_addr"] = bson.M{"$in": bson.A{"", nil}}
	}
	result, err := r.collection.UpdateOne(ctx, filter, bson.M{"$set": set})
	if err != nil {
		return false, err
	}
	return result.MatchedCount > 0, nil
}
func (r *InMemoryTrackRepository) FillMissingCompanionLocation(_ context.Context, expected *models.Track, city, addr string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.tracks[expected.ID]
	if t == nil || t.UserID != expected.UserID || !t.UpdatedAt.Equal(expected.UpdatedAt) || t.RawTrackURL != expected.RawTrackURL || t.IsRunning || !t.DeletedAt.IsZero() || (t.Status != models.TrackStatusNormal && t.Status != models.TrackStatusPrivate) {
		return false, nil
	}
	setCity, setAddr := missingCompanionLocation(t, city, addr)
	if !setCity && !setAddr {
		return false, nil
	}
	if setCity {
		t.CityCode = city
	}
	if setAddr {
		t.LocateAddr = addr
	}
	t.UpdatedAt = time.Now()
	return true, nil
}
