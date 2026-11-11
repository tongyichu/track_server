package models

import (
	"errors"
	"strings"
)

var ErrInvalidPhone = errors.New("手机号须为11位数字")

// NormalizePhone is shared by SMS login and all account binding paths.
// The current SMS contract accepts mainland 11-digit numbers only.
func NormalizePhone(phone string) (string, error) {
	phone = strings.TrimSpace(phone)
	if len(phone) != 11 || phone[0] != '1' {
		return "", ErrInvalidPhone
	}
	for _, digit := range phone {
		if digit < '0' || digit > '9' {
			return "", ErrInvalidPhone
		}
	}
	return phone, nil
}
