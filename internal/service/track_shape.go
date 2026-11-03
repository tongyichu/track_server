package service

import (
	"unicode"
	"unicode/utf8"
)

func validateTrackShapeLabel(label *string) error {
	if label == nil {
		return nil
	}
	if utf8.RuneCountInString(*label) > 32 {
		return invalidArg("shape_label must be at most 32 characters")
	}
	for _, r := range *label {
		if unicode.IsControl(r) {
			return invalidArg("shape_label must not contain control characters")
		}
	}
	return nil
}
