package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type mongoAdminDashboardRepository struct {
	db *mongo.Database
}

func (r *mongoAdminDashboardRepository) GetAdminDashboardStatistics(ctx context.Context, query models.AdminDashboardQuery) (*models.AdminDashboardStatistics, error) {
	result := newDashboardStatistics(query)
	if err := r.loadCoreAndFunnel(ctx, query, result); err != nil {
		return nil, err
	}
	if err := r.loadOperations(ctx, query, result); err != nil {
		return nil, err
	}
	if err := r.loadContentAndEngagement(ctx, query, result); err != nil {
		return nil, err
	}
	if err := r.loadMongoDaily(ctx, query, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *mongoAdminDashboardRepository) GetAdminCompanionStatistics(ctx context.Context, query models.AdminDashboardQuery) (*models.AdminDashboardCompanionStats, error) {
	sessions := r.db.Collection("companion_sessions")
	result := &models.AdminDashboardCompanionStats{}
	var err error
	result.Created.Current, err = sessions.CountDocuments(ctx, bson.M{"started_at": mongoTimeRange(query.Start, query.End)})
	if err != nil {
		return nil, err
	}
	result.Created.Previous, err = sessions.CountDocuments(ctx, bson.M{"started_at": mongoTimeRange(query.PreviousStart, query.PreviousEnd)})
	if err != nil {
		return nil, err
	}
	result.Active, err = sessions.CountDocuments(ctx, bson.M{"status": models.CompanionSessionStatusActive})
	if err != nil {
		return nil, err
	}
	endedFilter := bson.M{"status": models.CompanionSessionStatusEnded, "ended_at": mongoTimeRange(query.Start, query.End)}
	result.Ended, err = sessions.CountDocuments(ctx, endedFilter)
	if err != nil {
		return nil, err
	}
	endedFilter["end_source"] = models.CompanionSessionEndSourceAutoClose
	result.AutoEnded, err = sessions.CountDocuments(ctx, endedFilter)
	if err != nil {
		return nil, err
	}
	result.AvgParticipants, err = mongoAverageField(ctx, sessions, bson.M{"started_at": mongoTimeRange(query.Start, query.End)}, "actual_participant_count")
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *mongoAdminDashboardRepository) GetAdminUserDistributionStatistics(ctx context.Context) (*models.AdminUserDistributionStats, error) {
	users := r.db.Collection("users")
	total, err := users.CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	genderExpr := bson.M{"$cond": bson.A{
		bson.M{"$in": bson.A{bson.M{"$ifNull": bson.A{"$gender", ""}}, bson.A{"male", "female"}}},
		bson.M{"$ifNull": bson.A{"$gender", "unknown"}},
		"unknown",
	}}
	genderCounts, err := mongoAdminCountDistribution(ctx, users, mongo.Pipeline{
		bson.D{{Key: "$group", Value: bson.M{"_id": genderExpr, "count": bson.M{"$sum": 1}}}},
	})
	if err != nil {
		return nil, err
	}
	birthdayCounts, err := mongoAdminCountDistribution(ctx, users, mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{"birthday": bson.M{"$type": "string", "$ne": ""}}}},
		bson.D{{Key: "$group", Value: bson.M{"_id": "$birthday", "count": bson.M{"$sum": 1}}}},
	})
	if err != nil {
		return nil, err
	}
	trimmedCity := bson.M{"$trim": bson.M{"input": bson.M{"$ifNull": bson.A{"$city_code", ""}}}}
	cityExpr := bson.M{"$cond": bson.A{
		bson.M{"$eq": bson.A{trimmedCity, ""}}, "unknown", trimmedCity,
	}}
	cityCounts, err := mongoAdminCountDistribution(ctx, users, mongo.Pipeline{
		bson.D{{Key: "$group", Value: bson.M{"_id": cityExpr, "count": bson.M{"$sum": 1}}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}, {Key: "_id", Value: 1}}}},
	})
	if err != nil {
		return nil, err
	}
	outputRows, err := r.db.Collection("tracks").Aggregate(ctx, mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{
			"status": bson.M{"$ne": models.TrackStatusDeleted}, "is_running": false,
			"raw_track_url": bson.M{"$nin": bson.A{nil, ""}},
			"deleted_at":    bson.M{"$in": bson.A{nil, time.Time{}}},
		}}},
		bson.D{{Key: "$group", Value: bson.M{"_id": "$user_id", "track_count": bson.M{"$sum": 1}}}},
		bson.D{{Key: "$lookup", Value: bson.M{"from": "users", "localField": "_id", "foreignField": "_id", "as": "owner"}}},
		bson.D{{Key: "$match", Value: bson.M{"owner.0": bson.M{"$exists": true}}}},
		bson.D{{Key: "$group", Value: bson.M{"_id": "$track_count", "count": bson.M{"$sum": 1}}}},
	}, options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		return nil, fmt.Errorf("admin user track output distribution: %w", err)
	}
	defer outputRows.Close(ctx)
	trackOutputCounts := make(map[string]int64)
	for outputRows.Next(ctx) {
		var row struct {
			TrackCount int64 `bson:"_id"`
			UserCount  int64 `bson:"count"`
		}
		if err := outputRows.Decode(&row); err != nil {
			return nil, err
		}
		trackOutputCounts[adminTrackOutputBucket(row.TrackCount)] += row.UserCount
	}
	if err := outputRows.Err(); err != nil {
		return nil, err
	}
	return buildAdminUserDistributionStats(total, genderCounts, birthdayCounts, cityCounts, trackOutputCounts, time.Now()), nil
}

func (r *mongoAdminDashboardRepository) GetAdminUserLastLoginTimes(ctx context.Context, userIDs []int64) (map[int64]time.Time, error) {
	result := make(map[int64]time.Time, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}
	cursor, err := r.db.Collection("login_log").Aggregate(ctx, mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{"user_id": bson.M{"$in": userIDs}}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "created_at", Value: -1}}}},
		bson.D{{Key: "$group", Value: bson.M{"_id": "$user_id", "last_login_at": bson.M{"$first": "$created_at"}}}},
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var row struct {
			UserID      int64     `bson:"_id"`
			LastLoginAt time.Time `bson:"last_login_at"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		result[row.UserID] = row.LastLoginAt
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func mongoAdminCountDistribution(ctx context.Context, collection *mongo.Collection, pipeline mongo.Pipeline) (map[string]int64, error) {
	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	counts := make(map[string]int64)
	for cursor.Next(ctx) {
		var row struct {
			Key   interface{} `bson:"_id"`
			Count int64       `bson:"count"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		key := fmt.Sprint(row.Key)
		if key == "<nil>" {
			key = "unknown"
		}
		counts[key] = row.Count
	}
	return counts, cursor.Err()
}

func mongoTimeRange(start, end time.Time) bson.M {
	return bson.M{"$gte": start, "$lt": end}
}

func mongoCompletedTrackFilter(start, end time.Time) bson.M {
	return bson.M{
		"created_at":    mongoTimeRange(start, end),
		"status":        bson.M{"$ne": models.TrackStatusDeleted},
		"is_running":    false,
		"raw_track_url": bson.M{"$nin": []interface{}{nil, ""}},
	}
}

func (r *mongoAdminDashboardRepository) loadCoreAndFunnel(ctx context.Context, q models.AdminDashboardQuery, result *models.AdminDashboardStatistics) error {
	var err error
	result.Metrics.NewUsers.Current, err = r.db.Collection("users").CountDocuments(ctx, bson.M{"created_at": mongoTimeRange(q.Start, q.End)})
	if err != nil {
		return fmt.Errorf("dashboard mongo new users: %w", err)
	}
	result.Metrics.NewUsers.Previous, err = r.db.Collection("users").CountDocuments(ctx, bson.M{"created_at": mongoTimeRange(q.PreviousStart, q.PreviousEnd)})
	if err != nil {
		return err
	}
	result.Metrics.ActiveUsers.Current, err = mongoDistinctCount(ctx, r.db.Collection("login_log"), "user_id", bson.M{"created_at": mongoTimeRange(q.Start, q.End)})
	if err != nil {
		return err
	}
	result.Metrics.ActiveUsers.Previous, err = mongoDistinctCount(ctx, r.db.Collection("login_log"), "user_id", bson.M{"created_at": mongoTimeRange(q.PreviousStart, q.PreviousEnd)})
	if err != nil {
		return err
	}
	result.Metrics.UploadedTracks.Current, err = r.db.Collection("tracks").CountDocuments(ctx, mongoCompletedTrackFilter(q.Start, q.End))
	if err != nil {
		return err
	}
	result.Metrics.UploadedTracks.Previous, err = r.db.Collection("tracks").CountDocuments(ctx, mongoCompletedTrackFilter(q.PreviousStart, q.PreviousEnd))
	if err != nil {
		return err
	}
	result.Metrics.UploadUsers.Current, err = mongoDistinctCount(ctx, r.db.Collection("tracks"), "user_id", mongoCompletedTrackFilter(q.Start, q.End))
	if err != nil {
		return err
	}
	result.Metrics.UploadUsers.Previous, err = mongoDistinctCount(ctx, r.db.Collection("tracks"), "user_id", mongoCompletedTrackFilter(q.PreviousStart, q.PreviousEnd))
	if err != nil {
		return err
	}
	result.Metrics.Submissions.Current, err = r.db.Collection("track_submissions").CountDocuments(ctx, bson.M{"submitted_at": mongoTimeRange(q.Start, q.End)})
	if err != nil {
		return err
	}
	result.Metrics.Submissions.Previous, err = r.db.Collection("track_submissions").CountDocuments(ctx, bson.M{"submitted_at": mongoTimeRange(q.PreviousStart, q.PreviousEnd)})
	if err != nil {
		return err
	}

	cohort, err := r.db.Collection("users").Distinct(ctx, "_id", bson.M{"created_at": mongoTimeRange(q.Start, q.End)})
	if err != nil {
		return err
	}
	result.Funnel.Registered = int64(len(cohort))
	if len(cohort) == 0 {
		return nil
	}
	uploadFilter := mongoCompletedTrackFilter(q.Start, q.End)
	uploadFilter["user_id"] = bson.M{"$in": cohort}
	uploaded, err := r.db.Collection("tracks").Distinct(ctx, "user_id", uploadFilter)
	if err != nil {
		return err
	}
	result.Funnel.Uploaded = int64(len(uploaded))
	if len(uploaded) == 0 {
		return nil
	}
	submitted, err := r.db.Collection("track_submissions").Distinct(ctx, "user_id", bson.M{"user_id": bson.M{"$in": uploaded}, "submitted_at": mongoTimeRange(q.Start, q.End)})
	if err != nil {
		return err
	}
	result.Funnel.Submitted = int64(len(submitted))
	if len(submitted) == 0 {
		return nil
	}
	approved, err := r.db.Collection("track_submissions").Distinct(ctx, "user_id", bson.M{
		"user_id": bson.M{"$in": submitted}, "status": models.TrackSubmissionStatusApproved,
		"reviewed_at": mongoTimeRange(q.Start, q.End),
	})
	if err != nil {
		return err
	}
	result.Funnel.Approved = int64(len(approved))
	return nil
}

func (r *mongoAdminDashboardRepository) loadOperations(ctx context.Context, q models.AdminDashboardQuery, result *models.AdminDashboardStatistics) error {
	var err error
	pending := bson.M{"status": models.TrackSubmissionStatusPending}
	result.Review.Pending, err = r.db.Collection("track_submissions").CountDocuments(ctx, pending)
	if err != nil {
		return err
	}
	result.Review.PendingOver24H, err = r.db.Collection("track_submissions").CountDocuments(ctx, bson.M{"status": models.TrackSubmissionStatusPending, "submitted_at": bson.M{"$lt": q.Now.Add(-24 * time.Hour)}})
	if err != nil {
		return err
	}
	result.Review.PendingOver72H, err = r.db.Collection("track_submissions").CountDocuments(ctx, bson.M{"status": models.TrackSubmissionStatusPending, "submitted_at": bson.M{"$lt": q.Now.Add(-72 * time.Hour)}})
	if err != nil {
		return err
	}
	reviewedRange := bson.M{"$gte": q.Start, "$lt": q.End}
	result.Review.Approved, err = r.db.Collection("track_submissions").CountDocuments(ctx, bson.M{"status": models.TrackSubmissionStatusApproved, "reviewed_at": reviewedRange})
	if err != nil {
		return err
	}
	result.Review.Rejected, err = r.db.Collection("track_submissions").CountDocuments(ctx, bson.M{"status": models.TrackSubmissionStatusRejected, "reviewed_at": reviewedRange})
	if err != nil {
		return err
	}
	reviewed := result.Review.Approved + result.Review.Rejected
	if reviewed > 0 {
		result.Review.ApprovalRate = float64(result.Review.Approved) / float64(reviewed)
		average, err := mongoAverageDurationHours(ctx, r.db.Collection("track_submissions"), bson.M{"status": bson.M{"$in": []models.TrackSubmissionStatus{models.TrackSubmissionStatusApproved, models.TrackSubmissionStatusRejected}}, "reviewed_at": reviewedRange}, "submitted_at", "reviewed_at")
		if err != nil {
			return err
		}
		result.Review.AvgReviewHours = average
	}
	var oldest models.TrackSubmission
	err = r.db.Collection("track_submissions").FindOne(ctx, pending, options.FindOne().SetSort(bson.D{{Key: "submitted_at", Value: 1}})).Decode(&oldest)
	if err == nil {
		result.Review.OldestPending = oldest.SubmittedAt.Format(time.RFC3339)
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return err
	}
	result.Metrics.PendingSubmissions = result.Review.Pending

	feedback := r.db.Collection("user_feedbacks")
	for status, target := range map[models.FeedbackStatus]*int64{
		models.FeedbackStatusPending: &result.Feedback.Pending, models.FeedbackStatusProcessing: &result.Feedback.Processing,
		models.FeedbackStatusResolved: &result.Feedback.Resolved, models.FeedbackStatusIgnored: &result.Feedback.Ignored,
	} {
		*target, err = feedback.CountDocuments(ctx, bson.M{"status": status})
		if err != nil {
			return err
		}
	}
	openStatuses := bson.M{"$in": []models.FeedbackStatus{models.FeedbackStatusPending, models.FeedbackStatusProcessing}}
	result.Feedback.PendingOver24H, err = feedback.CountDocuments(ctx, bson.M{"status": openStatuses, "created_at": bson.M{"$lt": q.Now.Add(-24 * time.Hour)}})
	if err != nil {
		return err
	}
	result.Feedback.PendingOver72H, err = feedback.CountDocuments(ctx, bson.M{"status": openStatuses, "created_at": bson.M{"$lt": q.Now.Add(-72 * time.Hour)}})
	if err != nil {
		return err
	}
	result.Feedback.Created.Current, err = feedback.CountDocuments(ctx, bson.M{"created_at": mongoTimeRange(q.Start, q.End)})
	if err != nil {
		return err
	}
	result.Feedback.Created.Previous, err = feedback.CountDocuments(ctx, bson.M{"created_at": mongoTimeRange(q.PreviousStart, q.PreviousEnd)})
	if err != nil {
		return err
	}
	resolvedFilter := bson.M{"status": models.FeedbackStatusResolved, "updated_at": mongoTimeRange(q.Start, q.End)}
	result.Feedback.ResolvedInPeriod, err = feedback.CountDocuments(ctx, resolvedFilter)
	if err != nil {
		return err
	}
	if result.Feedback.ResolvedInPeriod > 0 {
		result.Feedback.AvgResolutionHours, err = mongoAverageDurationHours(ctx, feedback, resolvedFilter, "created_at", "updated_at")
		if err != nil {
			return err
		}
	}
	result.Feedback.Platforms, err = mongoDistribution(ctx, feedback, bson.M{"created_at": mongoTimeRange(q.Start, q.End)}, "platform", 6)
	if err != nil {
		return err
	}
	result.Feedback.Versions, err = mongoDistribution(ctx, feedback, bson.M{"created_at": mongoTimeRange(q.Start, q.End)}, "app_version", 8)
	if err != nil {
		return err
	}
	result.Metrics.PendingFeedback = result.Feedback.Pending + result.Feedback.Processing

	companionStats, err := r.GetAdminCompanionStatistics(ctx, q)
	if err != nil {
		return err
	}
	result.Companion = *companionStats
	result.Metrics.ActiveCompanions = result.Companion.Active
	return nil
}

func (r *mongoAdminDashboardRepository) loadContentAndEngagement(ctx context.Context, q models.AdminDashboardQuery, result *models.AdminDashboardStatistics) error {
	tracks := r.db.Collection("tracks")
	var err error
	periodTracks := mongoCompletedTrackFilter(q.Start, q.End)
	result.Content.Statuses, err = mongoDistribution(ctx, tracks, bson.M{"created_at": mongoTimeRange(q.Start, q.End), "is_running": false, "raw_track_url": bson.M{"$nin": []interface{}{nil, ""}}}, "status", 0)
	if err != nil {
		return err
	}
	result.Content.Types, err = mongoDistribution(ctx, tracks, periodTracks, "track_type", 8)
	if err != nil {
		return err
	}
	result.Content.Cities, err = mongoDistribution(ctx, tracks, periodTracks, "city_code", 10)
	if err != nil {
		return err
	}
	result.Content.AvgDistanceM, err = mongoAverageField(ctx, tracks, periodTracks, "distance")
	if err != nil {
		return err
	}
	result.Content.AvgDurationS, err = mongoAverageField(ctx, tracks, periodTracks, "duration")
	if err != nil {
		return err
	}
	result.Content.StaleRunning, err = tracks.CountDocuments(ctx, bson.M{"status": bson.M{"$ne": models.TrackStatusDeleted}, "is_running": true, "created_at": bson.M{"$lt": q.Now.Add(-24 * time.Hour)}})
	if err != nil {
		return err
	}
	result.Content.ZeroDistance, err = tracks.CountDocuments(ctx, bson.M{"status": bson.M{"$ne": models.TrackStatusDeleted}, "is_running": false, "raw_track_url": bson.M{"$nin": []interface{}{nil, ""}}, "distance": bson.M{"$lte": 0}})
	if err != nil {
		return err
	}
	result.Content.FailedMapJobs, err = r.db.Collection("track_map_index_jobs").CountDocuments(ctx, bson.M{"status": models.TrackMapIndexJobFailed})
	if err != nil {
		return err
	}
	result.Content.PendingMapJobs, err = r.db.Collection("track_map_index_jobs").CountDocuments(ctx, bson.M{"status": bson.M{"$in": []models.TrackMapIndexJobStatus{models.TrackMapIndexJobPending, models.TrackMapIndexJobProcessing}}})
	if err != nil {
		return err
	}
	result.Content.ActiveRouteGroups, err = r.db.Collection("track_route_groups").CountDocuments(ctx, bson.M{"status": models.TrackRouteGroupStatusActive})
	if err != nil {
		return err
	}

	collections := []struct {
		name string
		key  string
	}{
		{"track_collects", "collection"}, {"track_navigations", "navigation"}, {"track_likes", "like"}, {"track_comments", "comment"},
	}
	top := make(map[string]*models.AdminDashboardTopTrack)
	for _, source := range collections {
		collection := r.db.Collection(source.name)
		filter := bson.M{}
		if source.key == "comment" {
			filter["deleted"] = false
		}
		currentFilter := bson.M{"created_at": mongoTimeRange(q.Start, q.End)}
		previousFilter := bson.M{"created_at": mongoTimeRange(q.PreviousStart, q.PreviousEnd)}
		if source.key == "comment" {
			currentFilter["deleted"], previousFilter["deleted"] = false, false
		}
		current, err := collection.CountDocuments(ctx, currentFilter)
		if err != nil {
			return err
		}
		previous, err := collection.CountDocuments(ctx, previousFilter)
		if err != nil {
			return err
		}
		setEngagementTotal(&result.Engagement.Current, source.key, current)
		setEngagementTotal(&result.Engagement.Previous, source.key, previous)
		counts, err := mongoGroupCounts(ctx, collection, filter, "track_id")
		if err != nil {
			return err
		}
		for trackID, count := range counts {
			item := top[trackID]
			if item == nil {
				item = &models.AdminDashboardTopTrack{TrackID: trackID}
				top[trackID] = item
			}
			switch source.key {
			case "collection":
				item.Collections = count
			case "navigation":
				item.Navigations = count
			case "like":
				item.Likes = count
			case "comment":
				item.Comments = count
			}
		}
	}
	ids := make([]string, 0, len(top))
	for trackID := range top {
		ids = append(ids, trackID)
	}
	if len(ids) > 0 {
		cursor, err := tracks.Find(ctx, bson.M{"_id": bson.M{"$in": ids}, "status": models.TrackStatusNormal, "is_running": false}, options.Find().SetProjection(bson.M{"_id": 1, "title": 1}))
		if err != nil {
			return err
		}
		for cursor.Next(ctx) {
			var track struct {
				ID    string `bson:"_id"`
				Title string `bson:"title"`
			}
			if err := cursor.Decode(&track); err != nil {
				cursor.Close(ctx)
				return err
			}
			item := top[track.ID]
			item.Title = track.Title
			item.Score = item.Collections + item.Navigations + item.Likes + item.Comments
			result.Engagement.TopTracks = append(result.Engagement.TopTracks, *item)
		}
		if err := cursor.Close(ctx); err != nil {
			return err
		}
	}
	sort.Slice(result.Engagement.TopTracks, func(i, j int) bool {
		return result.Engagement.TopTracks[i].Score > result.Engagement.TopTracks[j].Score
	})
	if len(result.Engagement.TopTracks) > 10 {
		result.Engagement.TopTracks = result.Engagement.TopTracks[:10]
	}
	return nil
}

func (r *mongoAdminDashboardRepository) loadMongoDaily(ctx context.Context, q models.AdminDashboardQuery, result *models.AdminDashboardStatistics) error {
	index := dashboardDailyIndex(result.Trend)
	sources := []struct {
		collection string
		field      string
		kind       string
		filter     bson.M
		distinct   string
	}{
		{"users", "created_at", "new_user", bson.M{}, ""},
		{"login_log", "created_at", "active_user", bson.M{}, "user_id"},
		{"tracks", "created_at", "track", bson.M{"status": bson.M{"$ne": models.TrackStatusDeleted}, "is_running": false, "raw_track_url": bson.M{"$nin": []interface{}{nil, ""}}}, ""},
		{"track_collects", "created_at", "collection", bson.M{}, ""},
		{"track_navigations", "created_at", "navigation", bson.M{}, ""},
		{"track_likes", "created_at", "like", bson.M{}, ""},
		{"track_comments", "created_at", "comment", bson.M{"deleted": false}, ""},
		{"companion_sessions", "started_at", "companion", bson.M{}, ""},
	}
	for _, source := range sources {
		counts, err := mongoDailyCounts(ctx, r.db.Collection(source.collection), source.field, source.filter, source.distinct, q.TrendStart, q.TrendEnd)
		if err != nil {
			return err
		}
		for day, count := range counts {
			position, ok := index[day]
			if !ok {
				continue
			}
			item := &result.Trend[position]
			switch source.kind {
			case "new_user":
				item.NewUsers = count
			case "active_user":
				item.ActiveUsers = count
			case "track":
				item.UploadedTracks = count
			case "collection":
				item.Collections = count
			case "navigation":
				item.Navigations = count
			case "like":
				item.Likes = count
			case "comment":
				item.Comments = count
			case "companion":
				item.CompanionSessions = count
			}
		}
	}
	return nil
}

func mongoDistinctCount(ctx context.Context, collection *mongo.Collection, field string, filter bson.M) (int64, error) {
	items, err := collection.Distinct(ctx, field, filter)
	return int64(len(items)), err
}

func mongoDistribution(ctx context.Context, collection *mongo.Collection, filter bson.M, field string, limit int64) ([]models.AdminDashboardDistributionItem, error) {
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: filter}},
		bson.D{{Key: "$group", Value: bson.M{"_id": "$" + field, "count": bson.M{"$sum": 1}}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}, {Key: "_id", Value: 1}}}},
	}
	if limit > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$limit", Value: limit}})
	}
	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	items := make([]models.AdminDashboardDistributionItem, 0)
	for cursor.Next(ctx) {
		var row struct {
			Key   interface{} `bson:"_id"`
			Count int64       `bson:"count"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		items = append(items, models.AdminDashboardDistributionItem{Key: fmt.Sprint(row.Key), Count: row.Count})
	}
	return items, cursor.Err()
}

func mongoAverageDurationHours(ctx context.Context, collection *mongo.Collection, filter bson.M, startField, endField string) (float64, error) {
	durationHours := bson.M{"$divide": bson.A{
		bson.M{"$subtract": bson.A{"$" + endField, "$" + startField}},
		3600000,
	}}
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: filter}},
		bson.D{{Key: "$group", Value: bson.M{"_id": nil, "value": bson.M{"$avg": durationHours}}}},
	}
	return mongoSingleAverage(ctx, collection, pipeline)
}

func mongoAverageField(ctx context.Context, collection *mongo.Collection, filter bson.M, field string) (float64, error) {
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: filter}},
		bson.D{{Key: "$group", Value: bson.M{"_id": nil, "value": bson.M{"$avg": "$" + field}}}},
	}
	return mongoSingleAverage(ctx, collection, pipeline)
}

func mongoSingleAverage(ctx context.Context, collection *mongo.Collection, pipeline mongo.Pipeline) (float64, error) {
	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)
	var row struct {
		Value float64 `bson:"value"`
	}
	if cursor.Next(ctx) {
		if err := cursor.Decode(&row); err != nil {
			return 0, err
		}
	}
	return row.Value, cursor.Err()
}

func mongoGroupCounts(ctx context.Context, collection *mongo.Collection, filter bson.M, field string) (map[string]int64, error) {
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: filter}},
		bson.D{{Key: "$group", Value: bson.M{"_id": "$" + field, "count": bson.M{"$sum": 1}}}},
	}
	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	result := make(map[string]int64)
	for cursor.Next(ctx) {
		var row struct {
			ID    string `bson:"_id"`
			Count int64  `bson:"count"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		result[row.ID] = row.Count
	}
	return result, cursor.Err()
}

func mongoDailyCounts(ctx context.Context, collection *mongo.Collection, field string, filter bson.M, distinct string, start, end time.Time) (map[string]int64, error) {
	match := bson.M{}
	for key, value := range filter {
		match[key] = value
	}
	match[field] = mongoTimeRange(start, end)
	groupID := interface{}(bson.M{"day": bson.M{"$dateToString": bson.M{"format": "%Y-%m-%d", "date": "$" + field, "timezone": "Asia/Shanghai"}}})
	pipeline := mongo.Pipeline{bson.D{{Key: "$match", Value: match}}}
	if distinct != "" {
		groupID = bson.M{"day": bson.M{"$dateToString": bson.M{"format": "%Y-%m-%d", "date": "$" + field, "timezone": "Asia/Shanghai"}}, "value": "$" + distinct}
		pipeline = append(pipeline, bson.D{{Key: "$group", Value: bson.M{"_id": groupID}}}, bson.D{{Key: "$group", Value: bson.M{"_id": "$_id.day", "count": bson.M{"$sum": 1}}}})
	} else {
		pipeline = append(pipeline, bson.D{{Key: "$group", Value: bson.M{"_id": "$day", "count": bson.M{"$sum": 1}}}})
		pipeline = mongo.Pipeline{
			bson.D{{Key: "$match", Value: match}},
			bson.D{{Key: "$group", Value: bson.M{"_id": bson.M{"$dateToString": bson.M{"format": "%Y-%m-%d", "date": "$" + field, "timezone": "Asia/Shanghai"}}, "count": bson.M{"$sum": 1}}}},
		}
	}
	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	result := make(map[string]int64)
	for cursor.Next(ctx) {
		var row struct {
			Day   string `bson:"_id"`
			Count int64  `bson:"count"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		result[row.Day] = row.Count
	}
	return result, cursor.Err()
}

func setEngagementTotal(target *models.AdminDashboardEngagementTotals, kind string, value int64) {
	switch kind {
	case "collection":
		target.Collections = value
	case "navigation":
		target.Navigations = value
	case "like":
		target.Likes = value
	case "comment":
		target.Comments = value
	}
}
