package models

type FeedbackCorrectionOption struct {
	ID        string `json:"id" bson:"id"`
	Name      string `json:"name" bson:"name"`
	NameEN    string `json:"name_en" bson:"name_en"`
	Enabled   bool   `json:"enabled" bson:"enabled"`
	SortOrder int    `json:"sort_order" bson:"sort_order"`
}

type FeedbackCorrectionConfig struct {
	Version int64                      `json:"version" bson:"version"`
	Options []FeedbackCorrectionOption `json:"options" bson:"options"`
}

// FeedbackCorrection is a submission-time snapshot, independent of later option edits.
type FeedbackCorrection struct {
	TrackID      string `json:"track_id" bson:"track_id"`
	TrackTitle   string `json:"track_title" bson:"track_title"`
	OptionID     string `json:"option_id" bson:"option_id"`
	OptionName   string `json:"option_name" bson:"option_name"`
	OptionNameEN string `json:"option_name_en" bson:"option_name_en"`
	Notes        string `json:"notes" bson:"notes"`
}
