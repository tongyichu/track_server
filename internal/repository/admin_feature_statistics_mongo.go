package repository

import (
	"context"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (r *mongoAdminDashboardRepository) GetAdminMapModeStatistics(ctx context.Context) (*models.AdminMapModeStatistics, error) {
	cursor, err := r.db.Collection("track_route_groups").Find(ctx, bson.M{"status": models.TrackRouteGroupStatusActive}, options.Find().SetProjection(bson.M{"_id": 1, "name": 1, "status": 1, "center_lat": 1, "center_lng": 1, "member_count": 1, "city_codes": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	out, cities := newMapModeStatistics(), make(map[string]struct{})
	for cursor.Next(ctx) {
		var group models.TrackRouteGroup
		if err := cursor.Decode(&group); err != nil {
			return nil, err
		}
		addMapModeGroup(out, cities, &group)
	}
	out.CityCount = int64(len(cities))
	return out, cursor.Err()
}

func (r *mongoAdminDashboardRepository) GetAdminCompanionUsageStatistics(ctx context.Context, days int, now time.Time) (*models.AdminCompanionUsageStatistics, error) {
	out, err := newCompanionUsageStatistics(days, now)
	if err != nil {
		return nil, err
	}
	cursor, err := r.db.Collection("companion_sessions").Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"$or": bson.A{bson.M{"started_at": mongoTimeRange(out.Start, out.End)}, bson.M{"status": models.CompanionSessionStatusEnded, "ended_at": mongoTimeRange(out.Start, out.End)}}}}},
		{{Key: "$lookup", Value: bson.M{"from": "companion_session_members", "localField": "session_id", "foreignField": "session_id", "as": "members"}}},
		{{Key: "$project", Value: bson.M{"started_at": 1, "ended_at": 1, "status": 1, "participant_count": bson.M{"$size": bson.M{"$setUnion": bson.A{"$members.user_id", bson.A{}}}}}}},
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var row struct {
			Started      time.Time                     `bson:"started_at"`
			Ended        time.Time                     `bson:"ended_at"`
			Status       models.CompanionSessionStatus `bson:"status"`
			Participants int64                         `bson:"participant_count"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		addCompanionUsage(out, row.Started, row.Ended, row.Status, row.Participants)
	}
	return out, cursor.Err()
}
