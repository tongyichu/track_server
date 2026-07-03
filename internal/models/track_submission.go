package models

import "time"

type TrackSubmissionStatus string

const (
	TrackSubmissionStatusPending     TrackSubmissionStatus = "pending"
	TrackSubmissionStatusApproved    TrackSubmissionStatus = "approved"
	TrackSubmissionStatusRejected    TrackSubmissionStatus = "rejected"
	TrackSubmissionStatusWithdrawn   TrackSubmissionStatus = "withdrawn"
	TrackSubmissionStatusInvalidated TrackSubmissionStatus = "invalidated"
)

type TrackSubmissionImage struct {
	ImageID      string    `json:"image_id" bson:"image_id"`
	SubmissionID string    `json:"submission_id,omitempty" bson:"submission_id"`
	OSSURL       string    `json:"-" bson:"oss_url"`
	URL          string    `json:"url" bson:"-"`
	Caption      string    `json:"caption,omitempty" bson:"caption"`
	SortOrder    int       `json:"sort_order" bson:"sort_order"`
	CreatedAt    time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" bson:"updated_at"`
}

type TrackSubmission struct {
	SubmissionID         string                  `json:"submission_id" bson:"_id"`
	TrackID              string                  `json:"track_id" bson:"track_id"`
	UserID               int64                   `json:"user_id" bson:"user_id"`
	TrackType            string                  `json:"track_type" bson:"track_type"`
	Title                string                  `json:"title" bson:"title"`
	Description          string                  `json:"description" bson:"description"`
	Difficulty           string                  `json:"difficulty" bson:"difficulty"`
	RiskLevel            string                  `json:"risk_level" bson:"risk_level"`
	SuitableMonths       []int                   `json:"suitable_months" bson:"suitable_months"`
	SurfaceTypes         []string                `json:"surface_types" bson:"surface_types"`
	TransportModes       []string                `json:"transport_modes" bson:"transport_modes"`
	TransportDescription string                  `json:"transport_description" bson:"transport_description"`
	Status               TrackSubmissionStatus   `json:"status" bson:"status"`
	Revision             int64                   `json:"revision" bson:"revision"`
	SubmittedAt          time.Time               `json:"submitted_at" bson:"submitted_at"`
	ApprovedAt           *time.Time              `json:"approved_at,omitempty" bson:"approved_at,omitempty"`
	ReviewedAt           *time.Time              `json:"reviewed_at,omitempty" bson:"reviewed_at,omitempty"`
	ReviewedBy           string                  `json:"reviewed_by,omitempty" bson:"reviewed_by"`
	ReviewReason         string                  `json:"review_reason,omitempty" bson:"review_reason"`
	CreatedAt            time.Time               `json:"created_at" bson:"created_at"`
	UpdatedAt            time.Time               `json:"updated_at" bson:"updated_at"`
	Images               []*TrackSubmissionImage `json:"images" bson:"images"`
	Events               []*TrackSubmissionEvent `json:"-" bson:"events,omitempty"`
}

type TrackSubmissionEvent struct {
	ID           int64                 `json:"id" bson:"id"`
	SubmissionID string                `json:"submission_id" bson:"submission_id"`
	Revision     int64                 `json:"revision" bson:"revision"`
	EventType    string                `json:"event_type" bson:"event_type"`
	FromStatus   TrackSubmissionStatus `json:"from_status,omitempty" bson:"from_status"`
	ToStatus     TrackSubmissionStatus `json:"to_status" bson:"to_status"`
	OperatorType string                `json:"operator_type" bson:"operator_type"`
	Operator     string                `json:"operator" bson:"operator"`
	Reason       string                `json:"reason,omitempty" bson:"reason"`
	SnapshotJSON string                `json:"-" bson:"snapshot_json"`
	CreatedAt    time.Time             `json:"created_at" bson:"created_at"`
}

type TrackSubmissionSummary struct {
	Status       TrackSubmissionStatus `json:"status"`
	Revision     int64                 `json:"revision"`
	ReviewReason string                `json:"review_reason,omitempty"`
}

type TrackSubmissionListFilter struct {
	Status     TrackSubmissionStatus
	Difficulty string
	RiskLevel  string
	TrackType  string
	UserID     int64
	Limit      int
}
