package models

import "time"

// AdminDailyCount is one calendar-day bucket used by the management dashboard.
// Date is formatted as YYYY-MM-DD in the server's local timezone.
type AdminDailyCount struct {
	Date  string `json:"date" bson:"date"`
	Count int64  `json:"count" bson:"count"`
}

// AdminDashboardQuery describes the time windows used by the management
// dashboard. End values are exclusive, matching the repository conventions.
type AdminDashboardQuery struct {
	Start         time.Time
	End           time.Time
	PreviousStart time.Time
	PreviousEnd   time.Time
	TrendStart    time.Time
	TrendEnd      time.Time
	Now           time.Time
}

type AdminDashboardComparison struct {
	Current  int64 `json:"current"`
	Previous int64 `json:"previous"`
}

type AdminDashboardMetrics struct {
	ActiveUsers        AdminDashboardComparison `json:"active_users"`
	NewUsers           AdminDashboardComparison `json:"new_users"`
	UploadedTracks     AdminDashboardComparison `json:"uploaded_tracks"`
	UploadUsers        AdminDashboardComparison `json:"upload_users"`
	Submissions        AdminDashboardComparison `json:"submissions"`
	PendingSubmissions int64                    `json:"pending_submissions"`
	PendingFeedback    int64                    `json:"pending_feedback"`
	ActiveCompanions   int64                    `json:"active_companions"`
}

// AdminDashboardFunnel is a same-period new-user cohort. Each later stage is a
// subset of Registered, so the UI can safely present it as a conversion funnel.
type AdminDashboardFunnel struct {
	Registered int64 `json:"registered"`
	Uploaded   int64 `json:"uploaded"`
	Submitted  int64 `json:"submitted"`
	Approved   int64 `json:"approved"`
}

type AdminDashboardReviewStats struct {
	Pending        int64   `json:"pending"`
	PendingOver24H int64   `json:"pending_over_24h"`
	PendingOver72H int64   `json:"pending_over_72h"`
	Approved       int64   `json:"approved"`
	Rejected       int64   `json:"rejected"`
	ApprovalRate   float64 `json:"approval_rate"`
	AvgReviewHours float64 `json:"avg_review_hours"`
	OldestPending  string  `json:"oldest_pending,omitempty"`
}

type AdminDashboardDistributionItem struct {
	Key   string `json:"key"`
	Label string `json:"label,omitempty"`
	Count int64  `json:"count"`
}

type AdminDashboardFeedbackStats struct {
	Pending            int64                            `json:"pending"`
	Processing         int64                            `json:"processing"`
	Resolved           int64                            `json:"resolved"`
	Ignored            int64                            `json:"ignored"`
	PendingOver24H     int64                            `json:"pending_over_24h"`
	PendingOver72H     int64                            `json:"pending_over_72h"`
	Created            AdminDashboardComparison         `json:"created"`
	ResolvedInPeriod   int64                            `json:"resolved_in_period"`
	AvgResolutionHours float64                          `json:"avg_resolution_hours"`
	Platforms          []AdminDashboardDistributionItem `json:"platforms"`
	Versions           []AdminDashboardDistributionItem `json:"versions"`
}

type AdminDashboardContentStats struct {
	Statuses          []AdminDashboardDistributionItem `json:"statuses"`
	Types             []AdminDashboardDistributionItem `json:"types"`
	Cities            []AdminDashboardDistributionItem `json:"cities"`
	AvgDistanceM      float64                          `json:"avg_distance_m"`
	AvgDurationS      float64                          `json:"avg_duration_s"`
	StaleRunning      int64                            `json:"stale_running"`
	ZeroDistance      int64                            `json:"zero_distance"`
	FailedMapJobs     int64                            `json:"failed_map_jobs"`
	PendingMapJobs    int64                            `json:"pending_map_jobs"`
	ActiveRouteGroups int64                            `json:"active_route_groups"`
}

type AdminDashboardEngagementTotals struct {
	Collections int64 `json:"collections"`
	Navigations int64 `json:"navigations"`
	Likes       int64 `json:"likes"`
	Comments    int64 `json:"comments"`
}

type AdminDashboardTopTrack struct {
	TrackID     string `json:"track_id"`
	Title       string `json:"title"`
	Collections int64  `json:"collections"`
	Navigations int64  `json:"navigations"`
	Likes       int64  `json:"likes"`
	Comments    int64  `json:"comments"`
	Score       int64  `json:"score"`
}

type AdminDashboardEngagementStats struct {
	Current   AdminDashboardEngagementTotals `json:"current"`
	Previous  AdminDashboardEngagementTotals `json:"previous"`
	TopTracks []AdminDashboardTopTrack       `json:"top_tracks"`
}

type AdminDashboardCompanionStats struct {
	Created         AdminDashboardComparison `json:"created"`
	Active          int64                    `json:"active"`
	Ended           int64                    `json:"ended"`
	AutoEnded       int64                    `json:"auto_ended"`
	AvgParticipants float64                  `json:"avg_participants"`
}

// AdminUserDistributionStats contains read-only profile and track-city aggregates.
type AdminUserDistributionStats struct {
	TotalUsers int64                            `json:"total_users"`
	Gender     []AdminDashboardDistributionItem `json:"gender"`
	Zodiac     []AdminDashboardDistributionItem `json:"zodiac"`
	UserCities []AdminDashboardDistributionItem `json:"user_cities"`
}

type AdminDashboardDaily struct {
	Date              string `json:"date"`
	NewUsers          int64  `json:"new_users"`
	ActiveUsers       int64  `json:"active_users"`
	UploadedTracks    int64  `json:"uploaded_tracks"`
	Collections       int64  `json:"collections"`
	Navigations       int64  `json:"navigations"`
	Likes             int64  `json:"likes"`
	Comments          int64  `json:"comments"`
	CompanionSessions int64  `json:"companion_sessions"`
}

type AdminDashboardStatistics struct {
	Metrics    AdminDashboardMetrics         `json:"metrics"`
	Funnel     AdminDashboardFunnel          `json:"funnel"`
	Review     AdminDashboardReviewStats     `json:"review"`
	Feedback   AdminDashboardFeedbackStats   `json:"feedback"`
	Content    AdminDashboardContentStats    `json:"content"`
	Engagement AdminDashboardEngagementStats `json:"engagement"`
	Companion  AdminDashboardCompanionStats  `json:"companion"`
	Trend      []AdminDashboardDaily         `json:"trend"`
}
