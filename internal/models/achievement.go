package models

import "time"

// AchievementRewardType describes the top-level reward kind.
type AchievementRewardType string

const (
	AchievementRewardTypeBadge     AchievementRewardType = "badge"
	AchievementRewardTypeMilestone AchievementRewardType = "milestone"
)

// AchievementRarity describes display rarity of an achievement reward.
type AchievementRarity string

const (
	AchievementRarityCommon    AchievementRarity = "common"
	AchievementRarityRare      AchievementRarity = "rare"
	AchievementRarityEpic      AchievementRarity = "epic"
	AchievementRarityLegendary AchievementRarity = "legendary"
)

// AchievementDefinition is a server-side static reward definition.
type AchievementDefinition struct {
	Code        string                `json:"code" bson:"code"`
	Type        AchievementRewardType `json:"type" bson:"type"`
	Category    string                `json:"category" bson:"category"`
	Name        string                `json:"name" bson:"name"`
	Description string                `json:"description" bson:"description"`
	Rarity      AchievementRarity     `json:"rarity" bson:"rarity"`
	IconURL     string                `json:"icon_url" bson:"icon_url"`
	TargetValue float64               `json:"target_value" bson:"target_value"`
}

// UserAchievementReward is an earned reward record.
type UserAchievementReward struct {
	ID               int64     `json:"id" bson:"id"`
	UserID           int64     `json:"user_id" bson:"user_id"`
	RewardCode       string    `json:"reward_code" bson:"reward_code"`
	SourceTrackID    string    `json:"source_track_id,omitempty" bson:"source_track_id"`
	SourceSessionID  string    `json:"source_session_id,omitempty" bson:"source_session_id"`
	EarnedAt         time.Time `json:"earned_at" bson:"earned_at"`
	ProgressSnapshot string    `json:"progress_snapshot,omitempty" bson:"progress_snapshot"`
}

// AchievementLevel describes one level threshold.
type AchievementLevel struct {
	Level int    `json:"level"`
	Name  string `json:"name"`
	XP    int64  `json:"xp"`
}

// UserAchievementStats is the aggregated achievement stats of a user.
type UserAchievementStats struct {
	TotalXP             int64                                `json:"total_xp"`
	CurrentLevel        AchievementLevel                     `json:"current_level"`
	NextLevel           *AchievementLevel                    `json:"next_level,omitempty"`
	LevelProgress       float64                              `json:"level_progress"`
	QualifiedTrackCount int64                                `json:"qualified_track_count"`
	TotalDistance       float64                              `json:"total_distance"`
	TotalDuration       int64                                `json:"total_duration"`
	TotalElevationGain  int64                                `json:"total_elevation_gain"`
	CompanionCount      int64                                `json:"companion_count"`
	TypeStats           map[string]TrackTypeAchievementStats `json:"type_stats"`
}

// TrackTypeAchievementStats is aggregated stats for one track type.
type TrackTypeAchievementStats struct {
	Distance         float64 `json:"distance"`
	Duration         int64   `json:"duration"`
	ElevationGain    int64   `json:"elevation_gain"`
	TrackCount       int64   `json:"track_count"`
	XP               int64   `json:"xp"`
	MaxDistance      float64 `json:"max_distance"`
	MaxElevationGain int64   `json:"max_elevation_gain"`
}

// AchievementRewardView combines definition, progress and earned state.
type AchievementRewardView struct {
	AchievementDefinition
	Earned       bool       `json:"earned"`
	EarnedAt     *time.Time `json:"earned_at,omitempty"`
	CurrentValue float64    `json:"current_value"`
	Progress     float64    `json:"progress"`
}

// AchievementSummary is the achievement center home response.
type AchievementSummary struct {
	Stats         UserAchievementStats     `json:"stats"`
	RecentRewards []*AchievementRewardView `json:"recent_rewards"`
}

// AchievementRewardList is the achievement center badge/milestone response.
type AchievementRewardList struct {
	Stats   UserAchievementStats     `json:"stats"`
	Rewards []*AchievementRewardView `json:"rewards"`
}
