package models

type AdminTrackContributor struct {
	UserID   string `json:"user_id"`
	Nickname string `json:"nickname"`
	Count    int64  `json:"count"`
}

type AdminTrackStatistics struct {
	Types        []AdminDashboardDistributionItem `json:"types"`
	Contributors []AdminTrackContributor          `json:"contributors"`
}
