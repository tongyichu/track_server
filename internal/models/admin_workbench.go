package models

import "time"

type WorkbenchCity struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Supply      int64  `json:"supply"`
	Added       int64  `json:"added"`
	Collectors  int    `json:"collectors"`
	Navigations int64  `json:"navigations"`
	PreviousNav int64  `json:"previous_navigations"`
}

type WorkbenchCohort struct {
	Week          string `json:"week"`
	Registered    int64  `json:"registered"`
	FirstTrack    *int64 `json:"first_track"`
	DayOneLogin   *int64 `json:"day_one_login"`
	DaySevenLogin *int64 `json:"day_seven_login"`
}

type WorkbenchVersionCounts struct {
	Devices  map[string]int64
	Feedback map[string]int64
}

type WorkbenchCompanion struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Rooms       int64  `json:"rooms"`
	Members     int64  `json:"members"`
	Ready       int64  `json:"ready"`
	Pending     int64  `json:"pending"`
	Empty       int64  `json:"empty"`
	Unavailable int64  `json:"unavailable"`
}

type WorkbenchBacklog struct {
	Key     string     `json:"key"`
	Open    int64      `json:"open"`
	Overdue *int64     `json:"overdue"`
	Oldest  *time.Time `json:"oldest"`
}
