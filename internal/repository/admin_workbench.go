package repository

import (
	"context"
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
)

type AdminWorkbenchRepository interface {
	WorkbenchCities(context.Context, models.AdminDashboardQuery) ([]models.WorkbenchCity, error)
	WorkbenchCohorts(context.Context, time.Time) ([]models.WorkbenchCohort, error)
	WorkbenchVersions(context.Context, time.Time, bool) (models.WorkbenchVersionCounts, error)
	WorkbenchCompanions(context.Context, models.AdminDashboardQuery) ([]models.WorkbenchCompanion, error)
	WorkbenchBacklogs(context.Context, time.Time, []string) ([]models.WorkbenchBacklog, error)
}

const (
	workbenchRows = 12
	cohortWeeks   = 6
)

type workbenchCities struct {
	query  models.AdminDashboardQuery
	tracks map[string]*workbenchCityCounts
	cities map[string]*workbenchCityCounts
}

type workbenchCityCounts struct {
	models.WorkbenchCity
	collectors map[int64]struct{}
}

func newWorkbenchCities(query models.AdminDashboardQuery) *workbenchCities {
	return &workbenchCities{query: query, tracks: make(map[string]*workbenchCityCounts), cities: make(map[string]*workbenchCityCounts)}
}

func (r *workbenchCities) addTrack(id, code string, created time.Time) {
	code = config.CanonicalCityCode(strings.TrimSpace(code))
	if code == "" {
		return
	}
	city := r.cities[code]
	if city == nil {
		name := config.CityNameByCode(code)
		if name == "" {
			return
		}
		city = &workbenchCityCounts{WorkbenchCity: models.WorkbenchCity{Code: code, Name: name}, collectors: make(map[int64]struct{})}
		r.cities[code] = city
	}
	r.tracks[id] = city
	city.Supply++
	if dashboardBetween(created, r.query.Start, r.query.End) {
		city.Added++
	}
}

func (r *workbenchCities) addCollect(id string, user int64) {
	if city := r.tracks[id]; city != nil && user > 0 {
		city.collectors[user] = struct{}{}
	}
}

func (r *workbenchCities) addNavigation(id string, created time.Time) {
	if city := r.tracks[id]; city != nil {
		if dashboardBetween(created, r.query.Start, r.query.End) {
			city.Navigations++
		}
		if dashboardBetween(created, r.query.PreviousStart, r.query.PreviousEnd) {
			city.PreviousNav++
		}
	}
}

func (r *workbenchCities) rows() []models.WorkbenchCity {
	rows := make([]models.WorkbenchCity, 0, len(r.cities))
	for _, city := range r.cities {
		city.Collectors = len(city.collectors)
		rows = append(rows, city.WorkbenchCity)
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Navigations != b.Navigations {
			return a.Navigations > b.Navigations
		}
		if a.Collectors != b.Collectors {
			return a.Collectors > b.Collectors
		}
		if a.PreviousNav != b.PreviousNav {
			return a.PreviousNav > b.PreviousNav
		}
		if a.Supply != b.Supply {
			return a.Supply > b.Supply
		}
		return a.Code < b.Code
	})
	return rows[:min(len(rows), workbenchRows)]
}

func workbenchMidnight(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
}

type workbenchCohorts struct {
	now   time.Time
	users map[int64]*workbenchCohortUser
	weeks map[string]*models.WorkbenchCohort
}

type workbenchCohortUser struct {
	day, created              time.Time
	week                      *models.WorkbenchCohort
	tracked, dayOne, daySeven bool
}

func workbenchMonday(value time.Time) time.Time {
	return workbenchMidnight(value).AddDate(0, 0, -(int(value.Weekday())+6)%7)
}

func workbenchCohortStart(now time.Time) time.Time {
	return workbenchMonday(now).AddDate(0, 0, -7*(cohortWeeks-1))
}

func newWorkbenchCohorts(now time.Time) *workbenchCohorts {
	r := &workbenchCohorts{now: now, users: make(map[int64]*workbenchCohortUser), weeks: make(map[string]*models.WorkbenchCohort, cohortWeeks)}
	for start := workbenchCohortStart(now); !start.After(now); start = start.AddDate(0, 0, 7) {
		key := start.Format(time.DateOnly)
		week := &models.WorkbenchCohort{Week: key}
		if !now.Before(start.AddDate(0, 0, 8)) {
			week.DayOneLogin = new(int64)
		}
		if !now.Before(start.AddDate(0, 0, 14)) {
			week.DaySevenLogin = new(int64)
			week.FirstTrack = new(int64)
		}
		r.weeks[key] = week
	}
	return r
}

func (r *workbenchCohorts) addUser(id int64, created time.Time) {
	if !dashboardBetween(created, workbenchCohortStart(r.now), r.now) {
		return
	}
	created = created.In(r.now.Location())
	week := r.weeks[workbenchMonday(created).Format(time.DateOnly)]
	week.Registered++
	r.users[id] = &workbenchCohortUser{day: workbenchMidnight(created), created: created, week: week}
}

func (r *workbenchCohorts) addTrack(id int64, created time.Time) {
	user := r.users[id]
	if user != nil && !user.tracked && user.week.FirstTrack != nil && dashboardBetween(created, user.created, user.created.AddDate(0, 0, 7)) {
		user.tracked = true
		*user.week.FirstTrack++
	}
}

func (r *workbenchCohorts) addLogin(id int64, at time.Time) {
	user := r.users[id]
	if user == nil {
		return
	}
	day := workbenchMidnight(at.In(r.now.Location()))
	if !user.dayOne && user.week.DayOneLogin != nil && day.Equal(user.day.AddDate(0, 0, 1)) {
		user.dayOne = true
		*user.week.DayOneLogin++
	}
	if !user.daySeven && user.week.DaySevenLogin != nil && day.Equal(user.day.AddDate(0, 0, 7)) {
		user.daySeven = true
		*user.week.DaySevenLogin++
	}
}

func (r *workbenchCohorts) rows() []models.WorkbenchCohort {
	rows := make([]models.WorkbenchCohort, 0, len(r.weeks))
	for _, week := range r.weeks {
		rows = append(rows, *week)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Week > rows[j].Week })
	return rows
}

type workbenchRoom struct{ ID, Type string }
type workbenchMember struct {
	room string
	user int64
}

type workbenchCompanions struct {
	rooms map[string]*models.WorkbenchCompanion
	types map[string]*models.WorkbenchCompanion
}

func newWorkbenchCompanions() *workbenchCompanions {
	return &workbenchCompanions{rooms: make(map[string]*models.WorkbenchCompanion), types: make(map[string]*models.WorkbenchCompanion)}
}

func (r *workbenchCompanions) addRoom(id, kind string) {
	kind, name := workbenchTrackType(strings.TrimSpace(kind))
	entry := r.types[kind]
	if entry == nil {
		entry = &models.WorkbenchCompanion{Type: kind, Name: name}
		r.types[kind] = entry
	}
	r.rooms[id] = entry
	entry.Rooms++
}

func workbenchManifestTrack(payload string) (*string, bool) {
	var manifest struct {
		TrackID *string `json:"track_id"`
	}
	if json.Unmarshal([]byte(payload), &manifest) != nil || manifest.TrackID == nil {
		return nil, false
	}
	return manifest.TrackID, true
}

func (r *workbenchCompanions) addMember(room, payload string, hasManifest bool, user int64, tracks map[string]*models.Track) {
	row := r.rooms[room]
	if row == nil {
		return
	}
	row.Members++
	if !hasManifest {
		row.Pending++
		return
	}
	id, valid := workbenchManifestTrack(payload)
	if !valid {
		row.Unavailable++
		return
	}
	if *id == "" {
		row.Empty++
		return
	}
	track := tracks[*id]
	if track != nil && track.UserID == user && track.SessionID == room && (track.Status == models.TrackStatusNormal || track.Status == models.TrackStatusPrivate) && track.DeletedAt.IsZero() {
		row.Ready++
	} else {
		row.Unavailable++
	}
}

func (r *workbenchCompanions) rows() []models.WorkbenchCompanion {
	rows := make([]models.WorkbenchCompanion, 0, len(r.types))
	for _, item := range r.types {
		rows = append(rows, *item)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Rooms != rows[j].Rooms {
			return rows[i].Rooms > rows[j].Rooms
		}
		return rows[i].Type < rows[j].Type
	})
	return rows
}

const (
	backlogReview   = "review"
	backlogFeedback = "feedback"
	backlogIndex    = "index"
	backlogDeletion = "deletion"
)

func newWorkbenchBacklog(key string, timed bool) models.WorkbenchBacklog {
	row := models.WorkbenchBacklog{Key: key}
	if timed {
		row.Overdue = new(int64)
	}
	return row
}

func countWorkbenchBacklog(row *models.WorkbenchBacklog, submitted, deadline, now time.Time) {
	row.Open++
	if !submitted.IsZero() && (row.Oldest == nil || submitted.Before(*row.Oldest)) {
		row.Oldest = &submitted
	}
	if row.Overdue != nil && !deadline.IsZero() && !deadline.After(now) {
		*row.Overdue++
	}
}

func hasWorkbenchBacklog(scopes []string, key string) bool { return slices.Contains(scopes, key) }

func workbenchVersionName(name string) string { return strings.TrimSpace(name) }

func workbenchTrackType(kind string) (string, string) {
	for _, item := range config.DefaultTrackTypeConfigs {
		if item.Type == kind || item.Name == kind {
			return item.Type, item.Name
		}
	}
	if kind == "" {
		return kind, "未标注"
	}
	return kind, kind
}

func sortWorkbenchTopTracks(items []models.AdminDashboardTopTrack) {
	sort.Slice(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Navigations != b.Navigations {
			return a.Navigations > b.Navigations
		}
		if a.Collections != b.Collections {
			return a.Collections > b.Collections
		}
		if a.Likes != b.Likes {
			return a.Likes > b.Likes
		}
		if a.Comments != b.Comments {
			return a.Comments > b.Comments
		}
		return a.TrackID < b.TrackID
	})
}
