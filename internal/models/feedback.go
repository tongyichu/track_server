package models

import "time"

type FeedbackStatus string

const (
	FeedbackStatusPending    FeedbackStatus = "pending"
	FeedbackStatusProcessing FeedbackStatus = "processing"
	FeedbackStatusResolved   FeedbackStatus = "resolved"
	FeedbackStatusIgnored    FeedbackStatus = "ignored"
)

type FeedbackImage struct {
	ImageID     string `json:"image_id" bson:"image_id"`
	StoragePath string `json:"-" bson:"storage_path"`
	URL         string `json:"url,omitempty" bson:"-"`
	MimeType    string `json:"mime_type" bson:"mime_type"`
	Size        int64  `json:"size" bson:"size"`
}

type Feedback struct {
	ID            int64           `json:"-" bson:"id,omitempty"`
	FeedbackID    string          `json:"feedback_id" bson:"feedback_id"`
	UserID        int64           `json:"user_id" bson:"user_id"`
	Content       string          `json:"content" bson:"content"`
	Images        []FeedbackImage `json:"images" bson:"images"`
	Contact       string          `json:"contact,omitempty" bson:"contact"`
	AppVersion    string          `json:"app_version,omitempty" bson:"app_version"`
	Platform      string          `json:"platform,omitempty" bson:"platform"`
	DeviceModel   string          `json:"device_model,omitempty" bson:"device_model"`
	SystemVersion string          `json:"system_version,omitempty" bson:"system_version"`
	Status        FeedbackStatus  `json:"status" bson:"status"`
	Reply         string          `json:"reply,omitempty" bson:"reply"`
	CreatedAt     time.Time       `json:"created_at" bson:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at" bson:"updated_at"`
}

type FeedbackListCursor struct {
	CreatedAt  time.Time `json:"created_at"`
	FeedbackID string    `json:"feedback_id"`
}

type FeedbackListFilter struct {
	UserID     int64
	Status     FeedbackStatus
	AppVersion string
	Cursor     *FeedbackListCursor
	Limit      int
}

type FeedbackPage struct {
	Items      []*Feedback `json:"items"`
	NextCursor string      `json:"next_cursor,omitempty"`
	HasMore    bool        `json:"has_more"`
}
