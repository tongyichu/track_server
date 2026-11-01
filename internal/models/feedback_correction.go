package models

import "time"

type FeedbackCorrectionOption struct {
	ID            string `json:"id" bson:"id"`
	Name          string `json:"name" bson:"name"`
	NameEN        string `json:"name_en" bson:"name_en"`
	Enabled       bool   `json:"enabled" bson:"enabled"`
	SortOrder     int    `json:"sort_order" bson:"sort_order"`
	Description   string `json:"description,omitempty" bson:"description,omitempty"`
	DescriptionEN string `json:"description_en,omitempty" bson:"description_en,omitempty"`
	NotesHint     string `json:"notes_hint,omitempty" bson:"notes_hint,omitempty"`
	NotesHintEN   string `json:"notes_hint_en,omitempty" bson:"notes_hint_en,omitempty"`
}

type FeedbackCorrectionConfig struct {
	Version int64                      `json:"version" bson:"version"`
	Options []FeedbackCorrectionOption `json:"options" bson:"options"`
}

// FeedbackCorrection is a submission-time snapshot, independent of later option edits.
type FeedbackCorrection struct {
	TrackID      string               `json:"track_id" bson:"track_id"`
	TrackTitle   string               `json:"track_title" bson:"track_title"`
	OptionID     string               `json:"option_id" bson:"option_id"`
	OptionName   string               `json:"option_name" bson:"option_name"`
	OptionNameEN string               `json:"option_name_en" bson:"option_name_en"`
	Notes        string               `json:"notes" bson:"notes"`
	Revision     int64                `json:"revision" bson:"revision"`
	Outcome      string               `json:"outcome,omitempty" bson:"outcome,omitempty"`
	HandledAt    *time.Time           `json:"handled_at,omitempty" bson:"handled_at,omitempty"`
	HandledBy    string               `json:"-" bson:"handled_by,omitempty"`
	TrackChange  *FeedbackTrackChange `json:"-" bson:"track_change,omitempty"`
}

// FeedbackTrackChange records an actual saved track edit, visible only to administrators.
type FeedbackTrackChange struct {
	Operator string    `json:"operator" bson:"operator"`
	SavedAt  time.Time `json:"saved_at" bson:"saved_at"`
	Fields   []string  `json:"fields" bson:"fields"`
}
