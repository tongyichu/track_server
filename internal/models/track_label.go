package models

// TrackLabel is a display projection of existing track metadata, never a second persisted source.
type TrackLabel struct {
	Type           string `json:"type"`
	Value          string `json:"value"`
	Name           string `json:"name"`
	NameEN         string `json:"name_en,omitempty"`
	ThemeColor     string `json:"theme_color,omitempty"`
	ThemeColorDark string `json:"theme_color_dark,omitempty"`
}
