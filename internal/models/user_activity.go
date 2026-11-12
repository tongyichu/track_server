package models

import "time"

// UserActivityStats is private operational data, not an App user profile field.
type UserActivityStats struct {
	LastActiveAt         *time.Time
	LastActiveAppVersion string
	ActiveDays7D         int
}
