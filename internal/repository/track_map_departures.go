package repository

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func (r *MySQLTrackMapRepository) VisitRouteDepartures(ctx context.Context, filter models.TrackMapQueryFilter, visit func(models.TrackMapDeparture)) error {
	conds := []string{"g.status = ?", "t.status = ?", "t.is_running = 0", "t.deleted_at IS NULL"}
	args := []any{models.TrackRouteGroupStatusActive, models.TrackStatusNormal}
	if len(filter.TrackIDs) > 0 {
		conds = append(conds, "i.track_id IN ("+strings.TrimSuffix(strings.Repeat("?,", len(filter.TrackIDs)), ",")+")")
		for _, id := range filter.TrackIDs {
			args = append(args, id)
		}
	}
	if filter.TrackType != "" {
		conds = append(conds, "g.track_type = ?")
		args = append(args, filter.TrackType)
	}
	if codes := regionCodeCandidates([]string{filter.CityCode}); len(codes) > 0 {
		conds = append(conds, "i.city_code IN ("+strings.TrimSuffix(strings.Repeat("?,", len(codes)), ",")+")")
		for _, code := range codes {
			args = append(args, code)
		}
	}
	if bbox := filter.BBox; bbox != nil {
		conds = append(conds, "i.start_lat BETWEEN ? AND ? AND i.start_lng BETWEEN ? AND ?")
		args = append(args, bbox.MinLatitude, bbox.MaxLatitude, bbox.MinLongitude, bbox.MaxLongitude)
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT g.group_id, g.name, g.area_id, g.representative_track_id, g.member_count, g.track_type,
		       i.track_id, i.city_code, i.start_lat, i.start_lng, i.end_lat, i.end_lng, i.distance
		FROM track_geo_indexes i
		JOIN track_route_group_members m ON m.track_id = i.track_id
		JOIN track_route_groups g ON g.group_id = m.group_id
		JOIN track_records t ON t.id = i.track_id
		WHERE `+strings.Join(conds, " AND "), args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row models.TrackMapDeparture
		if err := rows.Scan(&row.GroupID, &row.GroupName, &row.AreaID, &row.RepresentativeTrackID, &row.MemberCount,
			&row.TrackType, &row.TrackID, &row.CityCode, &row.Start.Latitude, &row.Start.Longitude,
			&row.End.Latitude, &row.End.Longitude, &row.Distance); err != nil {
			return err
		}
		visit(row)
	}
	return rows.Err()
}

func (r *MongoTrackMapRepository) VisitRouteDepartures(ctx context.Context, filter models.TrackMapQueryFilter, visit func(models.TrackMapDeparture)) error {
	indexMatch := bson.M{}
	if len(filter.TrackIDs) > 0 {
		indexMatch["track_id"] = bson.M{"$in": filter.TrackIDs}
	}
	if codes := regionCodeCandidates([]string{filter.CityCode}); len(codes) > 0 {
		indexMatch["city_code"] = bson.M{"$in": codes}
	}
	if bbox := filter.BBox; bbox != nil {
		indexMatch["start_lat"] = bson.M{"$gte": bbox.MinLatitude, "$lte": bbox.MaxLatitude}
		indexMatch["start_lng"] = bson.M{"$gte": bbox.MinLongitude, "$lte": bbox.MaxLongitude}
	}
	groupMatch := bson.M{"group.status": models.TrackRouteGroupStatusActive}
	if filter.TrackType != "" {
		groupMatch["group.track_type"] = filter.TrackType
	}
	cursor, err := r.index.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: indexMatch}},
		{{Key: "$lookup", Value: bson.M{"from": r.members.Name(), "localField": "track_id", "foreignField": "track_id", "as": "member"}}},
		{{Key: "$unwind", Value: "$member"}},
		{{Key: "$lookup", Value: bson.M{"from": r.groups.Name(), "localField": "member.group_id", "foreignField": "_id", "as": "group"}}},
		{{Key: "$unwind", Value: "$group"}},
		{{Key: "$match", Value: groupMatch}},
		{{Key: "$lookup", Value: bson.M{"from": r.tracks.Name(), "localField": "track_id", "foreignField": "_id", "as": "track"}}},
		{{Key: "$unwind", Value: "$track"}},
		{{Key: "$match", Value: bson.M{"track.status": models.TrackStatusNormal, "track.is_running": bson.M{"$ne": true},
			"$or": bson.A{bson.M{"track.deleted_at": nil}, bson.M{"track.deleted_at": time.Time{}}}}}},
		{{Key: "$project", Value: bson.M{
			"group_id": "$group._id", "group_name": "$group.name", "area_id": "$group.area_id",
			"representative_track_id": "$group.representative_track_id", "member_count": "$group.member_count",
			"track_type": "$group.track_type", "track_id": 1, "city_code": 1, "distance": 1,
			"start": bson.M{"latitude": "$start_lat", "longitude": "$start_lng"},
			"end":   bson.M{"latitude": "$end_lat", "longitude": "$end_lng"},
		}}},
	})
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var row models.TrackMapDeparture
		if err := cursor.Decode(&row); err != nil {
			return err
		}
		visit(row)
	}
	return cursor.Err()
}

func (r *InMemoryTrackMapRepository) VisitRouteDepartures(ctx context.Context, filter models.TrackMapQueryFilter, visit func(models.TrackMapDeparture)) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	r.tracks.mu.RLock()
	defer r.tracks.mu.RUnlock()
	codes := regionCodeCandidates([]string{filter.CityCode})
	trackIDs := make(map[string]struct{}, len(filter.TrackIDs))
	for _, id := range filter.TrackIDs {
		trackIDs[id] = struct{}{}
	}
	for groupID, members := range r.routeMembers {
		group := r.routeGroups[groupID]
		if group == nil || group.Status != models.TrackRouteGroupStatusActive || filter.TrackType != "" && group.TrackType != filter.TrackType {
			continue
		}
		for trackID := range members {
			if _, wanted := trackIDs[trackID]; len(trackIDs) > 0 && !wanted {
				continue
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			index, track := r.indexes[trackID], r.tracks.tracks[trackID]
			if index == nil || track == nil || track.Status != models.TrackStatusNormal || track.IsRunning || !track.DeletedAt.IsZero() {
				continue
			}
			if len(codes) > 0 && !slices.Contains(codes, index.CityCode) {
				continue
			}
			if bbox := filter.BBox; bbox != nil &&
				(index.StartLat < bbox.MinLatitude || index.StartLat > bbox.MaxLatitude ||
					index.StartLng < bbox.MinLongitude || index.StartLng > bbox.MaxLongitude) {
				continue
			}
			visit(models.TrackMapDeparture{
				GroupID: groupID, GroupName: group.Name, AreaID: group.AreaID,
				RepresentativeTrackID: group.RepresentativeTrackID, MemberCount: group.MemberCount, TrackType: group.TrackType,
				TrackID: trackID, CityCode: index.CityCode, Distance: index.Distance,
				Start: models.TrackMapPoint{Latitude: index.StartLat, Longitude: index.StartLng},
				End:   models.TrackMapPoint{Latitude: index.EndLat, Longitude: index.EndLng},
			})
		}
	}
	return nil
}
