package models

import "time"

type AdminFeatureMapPoint struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type AdminMapModeStatistics struct {
	GroupCount   int64                            `json:"group_count"`
	CityCount    int64                            `json:"city_count"`
	Unlocated    int64                            `json:"unlocated"`
	Points       []AdminFeatureMapPoint           `json:"points"`
	Distribution []AdminDashboardDistributionItem `json:"distribution"`
}

type AdminCompanionUsagePoint struct {
	At      time.Time `json:"at"`
	Label   string    `json:"label"`
	Created int64     `json:"created"`
	Ended   int64     `json:"ended"`
}

type AdminCompanionUsageStatistics struct {
	Days         int                              `json:"days"`
	Start        time.Time                        `json:"start"`
	End          time.Time                        `json:"end"`
	Bucket       string                           `json:"bucket"`
	Trend        []AdminCompanionUsagePoint       `json:"trend"`
	Participants []AdminDashboardDistributionItem `json:"participants"`
}
