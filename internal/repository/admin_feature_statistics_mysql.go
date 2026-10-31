package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

func (r *mysqlAdminDashboardRepository) GetAdminMapModeStatistics(ctx context.Context) (*models.AdminMapModeStatistics, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT group_id,name,center_lat,center_lng,member_count,COALESCE(NULLIF(city_codes_json,''),'[]') FROM track_route_groups WHERE status='active'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out, cities := newMapModeStatistics(), make(map[string]struct{})
	for rows.Next() {
		group := &models.TrackRouteGroup{Status: models.TrackRouteGroupStatusActive}
		var codes string
		if err := rows.Scan(&group.GroupID, &group.Name, &group.CenterLat, &group.CenterLng, &group.MemberCount, &codes); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(codes), &group.CityCodes); err != nil {
			return nil, err
		}
		addMapModeGroup(out, cities, group)
	}
	out.CityCount = int64(len(cities))
	return out, rows.Err()
}

func (r *mysqlAdminDashboardRepository) GetAdminCompanionUsageStatistics(ctx context.Context, days int, now time.Time) (*models.AdminCompanionUsageStatistics, error) {
	out, err := newCompanionUsageStatistics(days, now)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT s.session_id,s.started_at,s.ended_at,s.status,COUNT(DISTINCT m.user_id)
    FROM companion_sessions s LEFT JOIN companion_session_members m ON m.session_id=s.session_id
    WHERE (s.started_at>=? AND s.started_at<?) OR (s.status='ended' AND s.ended_at>=? AND s.ended_at<?)
    GROUP BY s.session_id,s.started_at,s.ended_at,s.status`, out.Start, out.End, out.Start, out.End)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var started time.Time
		var ended sql.NullTime
		var status models.CompanionSessionStatus
		var participants int64
		if err := rows.Scan(&id, &started, &ended, &status, &participants); err != nil {
			return nil, err
		}
		addCompanionUsage(out, started, ended.Time, status, participants)
	}
	return out, rows.Err()
}
