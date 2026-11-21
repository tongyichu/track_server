package repository

import (
	"context"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func workbenchMongo[T any](ctx context.Context, collection *mongo.Collection, filter, projection bson.M, visit func(T)) error {
	rows, err := collection.Find(ctx, filter, options.Find().SetProjection(projection))
	if err != nil {
		return err
	}
	defer rows.Close(ctx)
	for rows.Next(ctx) {
		var row T
		if err := rows.Decode(&row); err != nil {
			return err
		}
		visit(row)
	}
	return rows.Err()
}

func mongoWorkbenchTrackFilter(start, end time.Time) bson.M {
	filter := mongoCompletedTrackFilter(start, end)
	filter["raw_track_url"] = bson.M{"$exists": true, "$nin": bson.A{nil, ""}}
	filter["deleted_at"] = bson.M{"$in": bson.A{nil, time.Time{}}}
	return filter
}

func (r *mongoAdminDashboardRepository) WorkbenchCities(ctx context.Context, q models.AdminDashboardQuery) ([]models.WorkbenchCity, error) {
	result := newWorkbenchCities(q)
	filter := mongoWorkbenchTrackFilter(q.Start, q.End)
	delete(filter, "created_at")
	filter["status"] = models.TrackStatusNormal
	err := workbenchMongo(ctx, r.db.Collection("tracks"), filter, bson.M{"city_code": 1, "created_at": 1}, func(t models.Track) {
		result.addTrack(t.ID, t.CityCode, t.CreatedAt)
	})
	if err != nil {
		return nil, err
	}
	err = workbenchMongo(ctx, r.db.Collection("track_collects"), bson.M{"created_at": mongoTimeRange(q.Start, q.End)}, bson.M{"_id": 0, "track_id": 1, "user_id": 1}, func(item models.TrackCollect) {
		result.addCollect(item.TrackID, item.UserID)
	})
	if err != nil {
		return nil, err
	}
	err = workbenchMongo(ctx, r.db.Collection("track_navigations"), bson.M{"$or": bson.A{bson.M{"created_at": mongoTimeRange(q.Start, q.End)}, bson.M{"created_at": mongoTimeRange(q.PreviousStart, q.PreviousEnd)}}}, bson.M{"_id": 0, "track_id": 1, "created_at": 1}, func(item models.TrackNavigation) {
		result.addNavigation(item.TrackID, item.CreatedAt)
	})
	if err != nil {
		return nil, err
	}
	return result.rows(), nil
}

func (r *mongoAdminDashboardRepository) WorkbenchCohorts(ctx context.Context, now time.Time) ([]models.WorkbenchCohort, error) {
	result := newWorkbenchCohorts(now)
	start := workbenchCohortStart(now)
	err := workbenchMongo(ctx, r.db.Collection("users"), bson.M{"created_at": mongoTimeRange(start, now)}, bson.M{"created_at": 1}, func(user models.User) {
		result.addUser(user.ID, user.CreatedAt)
	})
	if err != nil {
		return nil, err
	}
	if len(result.users) == 0 {
		return result.rows(), nil
	}
	err = workbenchMongo(ctx, r.db.Collection("tracks"), mongoWorkbenchTrackFilter(start, now), bson.M{"_id": 0, "user_id": 1, "created_at": 1}, func(track models.Track) {
		result.addTrack(track.UserID, track.CreatedAt)
	})
	if err != nil {
		return nil, err
	}
	err = workbenchMongo(ctx, r.db.Collection("login_log"), bson.M{"created_at": mongoTimeRange(start, now)}, bson.M{"_id": 0, "user_id": 1, "created_at": 1}, func(entry models.LoginLog) {
		result.addLogin(entry.UserID, entry.CreatedAt)
	})
	if err != nil {
		return nil, err
	}
	return result.rows(), nil
}

func (r *mongoAdminDashboardRepository) WorkbenchVersions(ctx context.Context, since time.Time, includeFeedback bool) (models.WorkbenchVersionCounts, error) {
	result := models.WorkbenchVersionCounts{}
	// This backend has no device stats storage. Keep device counts unavailable.
	if !includeFeedback {
		return result, nil
	}
	result.Feedback = make(map[string]int64)
	err := workbenchMongo(ctx, r.db.Collection("user_feedbacks"), bson.M{"platform": "android", "created_at": bson.M{"$gte": since}}, bson.M{"app_version": 1}, func(item models.Feedback) {
		result.Feedback[workbenchVersionName(item.AppVersion)]++
	})
	return result, err
}

func (r *mongoAdminDashboardRepository) WorkbenchCompanions(ctx context.Context, q models.AdminDashboardQuery) ([]models.WorkbenchCompanion, error) {
	result := newWorkbenchCompanions()
	var ids []string
	err := workbenchMongo(ctx, r.db.Collection("companion_sessions"), bson.M{"status": "ended", "ended_at": mongoTimeRange(q.Start, q.End)}, bson.M{"_id": 0, "session_id": 1, "track_type": 1}, func(room models.CompanionSession) {
		result.addRoom(room.SessionID, room.TrackType)
		ids = append(ids, room.SessionID)
	})
	if err != nil {
		return nil, err
	}
	for start := 0; start < len(ids); start += 500 {
		scope := bson.M{"$in": ids[start:min(start+500, len(ids))]}
		tracks := make(map[string]*models.Track)
		err = workbenchMongo(ctx, r.db.Collection("tracks"), bson.M{"session_id": scope}, bson.M{"user_id": 1, "session_id": 1, "status": 1, "deleted_at": 1}, func(track models.Track) {
			tracks[track.ID] = &track
		})
		if err != nil {
			return nil, err
		}
		manifests := make(map[workbenchMember]string)
		err = workbenchMongo(ctx, r.db.Collection("companion_track_chunks"), bson.M{"session_id": scope, "index": 0}, bson.M{"_id": 0, "session_id": 1, "user_id": 1, "data": 1}, func(item models.CompanionTrackChunk) {
			manifests[workbenchMember{item.SessionID, item.UserID}] = item.Data
		})
		if err != nil {
			return nil, err
		}
		err = workbenchMongo(ctx, r.db.Collection("companion_session_members"), bson.M{"session_id": scope}, bson.M{"_id": 0, "session_id": 1, "user_id": 1}, func(item models.CompanionSessionMember) {
			payload, found := manifests[workbenchMember{item.SessionID, item.UserID}]
			result.addMember(item.SessionID, payload, found, item.UserID, tracks)
		})
		if err != nil {
			return nil, err
		}
	}
	return result.rows(), nil
}

func (r *mongoAdminDashboardRepository) WorkbenchBacklogs(ctx context.Context, now time.Time, scopes []string) ([]models.WorkbenchBacklog, error) {
	specs := []struct {
		scope, key, collection, opened, due string
		status                              bson.M
	}{
		{backlogReview, "review", "track_submissions", "submitted_at", "submitted_at", bson.M{"status": "pending"}},
		{backlogFeedback, "feedback", "user_feedbacks", "created_at", "created_at", bson.M{"status": bson.M{"$in": bson.A{"pending", "processing"}}}},
		{backlogIndex, "index", "track_map_index_jobs", "created_at", "next_run_at", bson.M{"status": bson.M{"$in": bson.A{"pending", "processing"}}}},
		{backlogIndex, "index_failed", "track_map_index_jobs", "created_at", "", bson.M{"status": "failed"}},
	}
	rows := make([]models.WorkbenchBacklog, 0, len(specs))
	for _, spec := range specs {
		if !hasWorkbenchBacklog(scopes, spec.scope) {
			continue
		}
		row := newWorkbenchBacklog(spec.key, spec.due != "")
		cursor, err := r.db.Collection(spec.collection).Aggregate(ctx, mongo.Pipeline{
			{{Key: "$match", Value: spec.status}},
			{{Key: "$group", Value: bson.M{"_id": nil, "count": bson.M{"$sum": 1}, "oldest": bson.M{"$min": "$" + spec.opened}}}},
		})
		if err != nil {
			return nil, err
		}
		var group []struct {
			Count  int64
			Oldest *time.Time
		}
		if err := cursor.All(ctx, &group); err != nil {
			return nil, err
		}
		if len(group) > 0 {
			row.Open, row.Oldest = group[0].Count, group[0].Oldest
		}
		if row.Overdue != nil {
			spec.status[spec.due] = bson.M{"$lte": now.Add(-24 * time.Hour)}
			count, err := r.db.Collection(spec.collection).CountDocuments(ctx, spec.status)
			if err != nil {
				return nil, err
			}
			*row.Overdue = count
		}
		rows = append(rows, row)
	}
	return rows, nil
}
