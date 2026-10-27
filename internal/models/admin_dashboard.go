package models

// AdminDailyCount is one calendar-day bucket used by the management dashboard.
// Date is formatted as YYYY-MM-DD in the server's local timezone.
type AdminDailyCount struct {
	Date  string `json:"date" bson:"date"`
	Count int64  `json:"count" bson:"count"`
}
