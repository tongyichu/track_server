package repository

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func newAdminTrackStatistics() *models.AdminTrackStatistics {
	return &models.AdminTrackStatistics{
		Types:        []models.AdminDashboardDistributionItem{},
		Contributors: []models.AdminTrackContributor{},
	}
}

func (r *mysqlAdminDashboardRepository) GetAdminTrackStatistics(ctx context.Context, start, end time.Time) (*models.AdminTrackStatistics, error) {
	result := newAdminTrackStatistics()
	rows, err := r.db.QueryContext(ctx, `SELECT COALESCE(NULLIF(TRIM(track_type),''),'unknown'),COUNT(*)
		FROM track_records WHERE `+completedTrackSQL+` AND created_at>=? AND created_at<?
		GROUP BY 1 ORDER BY COUNT(*) DESC,1`, start, end)
	if err != nil {
		return nil, fmt.Errorf("admin track type statistics: %w", err)
	}
	for rows.Next() {
		var item models.AdminDashboardDistributionItem
		if err := rows.Scan(&item.Key, &item.Count); err != nil {
			rows.Close()
			return nil, err
		}
		result.Types = append(result.Types, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}

	rows, err = r.db.QueryContext(ctx, `SELECT t.user_id,COALESCE(u.nickname,''),COUNT(*) AS track_count
		FROM track_records t INNER JOIN users u ON u.id=t.user_id
		WHERE `+completedTrackAliasSQL+` AND t.created_at>=? AND t.created_at<?
		GROUP BY t.user_id,u.nickname ORDER BY track_count DESC,t.user_id ASC LIMIT 10`, start, end)
	if err != nil {
		return nil, fmt.Errorf("admin track contributor statistics: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item models.AdminTrackContributor
		var userID int64
		if err := rows.Scan(&userID, &item.Nickname, &item.Count); err != nil {
			return nil, err
		}
		item.UserID = strconv.FormatInt(userID, 10)
		result.Contributors = append(result.Contributors, item)
	}
	return result, rows.Err()
}

func (r *mongoAdminDashboardRepository) GetAdminTrackStatistics(ctx context.Context, start, end time.Time) (*models.AdminTrackStatistics, error) {
	result := newAdminTrackStatistics()
	filter := mongoCompletedTrackFilter(start, end)
	filter["deleted_at"] = bson.M{"$in": bson.A{nil, time.Time{}}}
	types, err := mongoDistribution(ctx, r.db.Collection("tracks"), filter, "track_type", 0)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int64)
	for _, item := range types {
		key := strings.TrimSpace(item.Key)
		if key == "" || key == "<nil>" {
			key = "unknown"
		}
		counts[key] += item.Count
	}
	result.Types = distributionItems(counts, 0)

	// 先关联有效用户再截取前十，避免已删除账号占用排名。
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: filter}},
		bson.D{{Key: "$group", Value: bson.M{"_id": "$user_id", "count": bson.M{"$sum": 1}}}},
		bson.D{{Key: "$lookup", Value: bson.M{"from": "users", "localField": "_id", "foreignField": "_id", "as": "user"}}},
		bson.D{{Key: "$unwind", Value: "$user"}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}, {Key: "_id", Value: 1}}}},
		bson.D{{Key: "$limit", Value: 10}},
		bson.D{{Key: "$project", Value: bson.M{"count": 1, "nickname": bson.M{"$ifNull": bson.A{"$user.nickname", ""}}}}},
	}
	cursor, err := r.db.Collection("tracks").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("admin mongo track contributors: %w", err)
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var row struct {
			UserID   int64  `bson:"_id"`
			Nickname string `bson:"nickname"`
			Count    int64  `bson:"count"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		result.Contributors = append(result.Contributors, models.AdminTrackContributor{
			UserID: strconv.FormatInt(row.UserID, 10), Nickname: row.Nickname, Count: row.Count,
		})
	}
	return result, cursor.Err()
}

func (r *memoryAdminDashboardRepository) GetAdminTrackStatistics(_ context.Context, start, end time.Time) (*models.AdminTrackStatistics, error) {
	result := newAdminTrackStatistics()
	users, usersOK := r.deps.users.(*InMemoryUserRepository)
	tracks, tracksOK := r.deps.tracks.(*InMemoryTrackRepository)
	if !usersOK || !tracksOK {
		return nil, errAdminStatisticsRepositoriesUnavailable
	}
	names := make(map[int64]string)
	users.mu.RLock()
	for id, user := range users.users {
		names[id] = user.Nickname
	}
	users.mu.RUnlock()
	typeCounts := make(map[string]int64)
	userCounts := make(map[int64]int64)
	tracks.mu.RLock()
	for _, track := range tracks.tracks {
		if track.Status == models.TrackStatusDeleted || !track.DeletedAt.IsZero() || track.IsRunning || track.RawTrackURL == "" || !dashboardBetween(track.CreatedAt, start, end) {
			continue
		}
		key := strings.TrimSpace(track.TrackType)
		if key == "" {
			key = "unknown"
		}
		typeCounts[key]++
		if _, exists := names[track.UserID]; exists {
			userCounts[track.UserID]++
		}
	}
	tracks.mu.RUnlock()
	result.Types = distributionItems(typeCounts, 0)
	ids := make([]int64, 0, len(userCounts))
	for id := range userCounts {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if userCounts[ids[i]] == userCounts[ids[j]] {
			return ids[i] < ids[j]
		}
		return userCounts[ids[i]] > userCounts[ids[j]]
	})
	if len(ids) > 10 {
		ids = ids[:10]
	}
	for _, id := range ids {
		result.Contributors = append(result.Contributors, models.AdminTrackContributor{
			UserID: strconv.FormatInt(id, 10), Nickname: names[id], Count: userCounts[id],
		})
	}
	return result, nil
}
