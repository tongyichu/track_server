package models

import "time"

// RecommendationStrategy identifies the server-side strategy used to build a feed.
type RecommendationStrategy string

const (
	RecommendationStrategyPersonalized RecommendationStrategy = "personalized"
	RecommendationStrategyHybrid       RecommendationStrategy = "hybrid"
	RecommendationStrategyLegacy       RecommendationStrategy = "legacy"
)

// RecommendationCandidateSource identifies the primary reason a candidate entered the feed.
type RecommendationCandidateSource string

const (
	RecommendationSourceContentAffinity RecommendationCandidateSource = "content_affinity"
	RecommendationSourceCityHot         RecommendationCandidateSource = "city_hot"
	RecommendationSourceFollowedAuthor  RecommendationCandidateSource = "followed_author"
	RecommendationSourceQuality         RecommendationCandidateSource = "quality"
	RecommendationSourceExploration     RecommendationCandidateSource = "exploration"
	RecommendationSourceLegacyFill      RecommendationCandidateSource = "legacy_fill"
	RecommendationSourceLegacy          RecommendationCandidateSource = "legacy"
)

// RecommendationMetadata is returned only when the new recommendation protocol is active.
type RecommendationMetadata struct {
	RequestID string                 `json:"request_id"`
	Strategy  RecommendationStrategy `json:"strategy"`
}

// RecommendationFeedItem is an immutable item stored in a feed session.
type RecommendationFeedItem struct {
	TrackID         string                        `json:"track_id" bson:"track_id"`
	CandidateSource RecommendationCandidateSource `json:"candidate_source" bson:"candidate_source"`
	RecommendReason string                        `json:"recommend_reason" bson:"recommend_reason"`
}

// RecommendationFeedSession freezes one ranked feed for stable cursor pagination.
type RecommendationFeedSession struct {
	RequestID string                   `json:"request_id" bson:"_id"`
	UserID    int64                    `json:"user_id" bson:"user_id"`
	CityCode  string                   `json:"city_code" bson:"city_code"`
	Strategy  RecommendationStrategy   `json:"strategy" bson:"strategy"`
	Items     []RecommendationFeedItem `json:"items" bson:"items"`
	CreatedAt time.Time                `json:"created_at" bson:"created_at"`
	ExpiresAt time.Time                `json:"expires_at" bson:"expires_at"`
}

// RecommendationUserProfile is the latest offline aggregate used by online ranking.
// Map keys are persisted as JSON/BSON keys so the structure remains storage neutral.
type RecommendationUserProfile struct {
	UserID             int64              `json:"user_id" bson:"_id"`
	TrackTypeWeights   map[string]float64 `json:"track_type_weights" bson:"track_type_weights"`
	CityWeights        map[string]float64 `json:"city_weights" bson:"city_weights"`
	AuthorWeights      map[string]float64 `json:"author_weights" bson:"author_weights"`
	CollectedTrackIDs  []string           `json:"collected_track_ids" bson:"collected_track_ids"`
	NavigatedTrackIDs  []string           `json:"navigated_track_ids" bson:"navigated_track_ids"`
	PositiveEventCount int64              `json:"positive_event_count" bson:"positive_event_count"`
	DataThrough        time.Time          `json:"data_through" bson:"data_through"`
	GeneratedAt        time.Time          `json:"generated_at" bson:"generated_at"`
}

// RecommendationItemStats is a daily material aggregate used by online ranking.
type RecommendationItemStats struct {
	TrackID         string    `json:"track_id" bson:"track_id"`
	StatDate        time.Time `json:"stat_date" bson:"stat_date"`
	CollectCount    int64     `json:"collect_count" bson:"collect_count"`
	NavigateCount   int64     `json:"navigate_count" bson:"navigate_count"`
	ImpressionCount int64     `json:"impression_count" bson:"impression_count"`
	ClickCount      int64     `json:"click_count" bson:"click_count"`
	DetailViewCount int64     `json:"detail_view_count" bson:"detail_view_count"`
	HotScore        float64   `json:"hot_score" bson:"hot_score"`
	DataThrough     time.Time `json:"data_through" bson:"data_through"`
	GeneratedAt     time.Time `json:"generated_at" bson:"generated_at"`
	UpdatedAt       time.Time `json:"updated_at" bson:"updated_at"`
}

// TrackNavigation is one confirmed navigation usage row from the business database.
type TrackNavigation struct {
	ID              int64     `json:"id" bson:"_id,omitempty"`
	TrackID         string    `json:"track_id" bson:"track_id"`
	NavigatorUserID int64     `json:"navigator_user_id" bson:"navigator_user_id"`
	CreatedAt       time.Time `json:"created_at" bson:"created_at"`
}
