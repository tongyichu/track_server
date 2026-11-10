package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

type mysqlAdminDailySource struct {
	kind, table, column, count, filter string
}

// Daily bounds remain time.Time parameters so the driver uses the same storage
// timezone as writes; grouping a raw DATETIME string would discard that zone.
// SQL fragments in sources are internal constants, never request values.
func mysqlAdminDailyQuery(start, end time.Time, sources []mysqlAdminDailySource) (string, []interface{}) {
	var queries []string
	var args []interface{}
	day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	for ; day.Before(end) && start.Before(end); day = day.AddDate(0, 0, 1) {
		lower := day
		if lower.Before(start) {
			lower = start
		}
		next := day.AddDate(0, 0, 1)
		if next.After(end) {
			next = end
		}
		for _, source := range sources {
			query := "SELECT ?,?," + source.count + " FROM " + source.table + " WHERE " + source.column + ">=? AND " + source.column + "<?"
			if source.filter != "" {
				query += " AND " + source.filter
			}
			queries = append(queries, query)
			args = append(args, source.kind, day.Format(time.DateOnly), lower, next)
		}
	}
	return strings.Join(queries, " UNION ALL "), args
}

func listMySQLAdminDailyCounts(ctx context.Context, db *sql.DB, start, end time.Time, source mysqlAdminDailySource) ([]models.AdminDailyCount, error) {
	items := make([]models.AdminDailyCount, 0)
	query, args := mysqlAdminDailyQuery(start, end, []mysqlAdminDailySource{source})
	if query == "" {
		return items, nil
	}
	rows, err := db.QueryContext(ctx, query+" ORDER BY 2", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var item models.AdminDailyCount
		if err := rows.Scan(&kind, &item.Date, &item.Count); err != nil {
			return nil, err
		}
		if item.Count > 0 {
			items = append(items, item)
		}
	}
	return items, rows.Err()
}
