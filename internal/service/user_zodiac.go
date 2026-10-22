package service

import (
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

// UserProfileResponse is for authenticated self-profile responses, never public user lists.
// ZodiacSign is derived on read and is not part of the persisted User model.
type UserProfileResponse struct {
	models.User
	ZodiacSign string `json:"zodiac_sign"`
}

func NewUserProfileResponse(user *models.User) *UserProfileResponse {
	if user == nil {
		return nil
	}
	return &UserProfileResponse{User: *user, ZodiacSign: zodiacSignFromBirthday(user.Birthday)}
}

// Fixed Gregorian month/day ranges are a display convention, not an astronomical birth chart.
// Keep these boundaries in sync with docs/api/user.md; clients only localize the returned code.
func zodiacSignFromBirthday(birthday string) string {
	date, err := time.Parse(time.DateOnly, birthday)
	if err != nil || date.Format(time.DateOnly) != birthday || birthday < "1900-01-01" || birthday > time.Now().Format(time.DateOnly) {
		return ""
	}
	monthDay := int(date.Month())*100 + date.Day()
	switch {
	case monthDay < 120:
		return "capricorn"
	case monthDay < 219:
		return "aquarius"
	case monthDay < 321:
		return "pisces"
	case monthDay < 420:
		return "aries"
	case monthDay < 521:
		return "taurus"
	case monthDay < 621:
		return "gemini"
	case monthDay < 723:
		return "cancer"
	case monthDay < 823:
		return "leo"
	case monthDay < 923:
		return "virgo"
	case monthDay < 1023:
		return "libra"
	case monthDay < 1122:
		return "scorpio"
	case monthDay < 1222:
		return "sagittarius"
	default:
		return "capricorn"
	}
}
