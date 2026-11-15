package service

import (
	"context"
	"errors"
	"strings"

	"github.com/tongyichu/track_server/internal/repository"
)

func validNavigationSessionID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, c := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

func (s *TrackService) DeleteNavigationResult(ctx context.Context, user int64, session string) error {
	session = strings.ToLower(session)
	if user <= 0 || !validNavigationSessionID(session) {
		return invalidArg("invalid navigation result")
	}
	linked, ok := s.navigations.(repository.NavigationResultRepository)
	if !ok {
		return errors.New("navigation result storage unavailable")
	}
	return linked.DeleteNavigationResult(ctx, user, session)
}
