package repository

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

func (r *InMemoryTrackRepository) ListAdminTracks(_ context.Context, filter AdminTrackListFilter, cursor *models.TrackListCursor, limit int) ([]*models.Track, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	linked := r.adminLinkedScenicTracks(filter.Missing)
	candidates := make(map[string]bool, len(filter.PlaceCandidateTrackIDs))
	for _, id := range filter.PlaceCandidateTrackIDs {
		candidates[id] = true
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	userIDs := make(map[int64]struct{}, len(filter.UserIDs))
	for _, id := range filter.UserIDs {
		userIDs[id] = struct{}{}
	}
	cityCodes := make(map[string]struct{}, len(filter.CityCodes))
	for _, code := range filter.CityCodes {
		cityCodes[code] = struct{}{}
	}
	wantDeleted := filter.Status != nil && *filter.Status == models.TrackStatusDeleted
	items := make([]*models.Track, 0)
	var total int64
	for _, track := range r.tracks {
		if track == nil {
			continue
		}
		if wantDeleted != (track.Status == models.TrackStatusDeleted) {
			continue
		}
		if filter.Title != "" && !strings.Contains(strings.ToLower(track.Title), strings.ToLower(filter.Title)) {
			continue
		}
		if filter.CityCode != "" && track.CityCode != filter.CityCode {
			continue
		}
		if filter.CityCode == "" && len(cityCodes) > 0 {
			if _, ok := cityCodes[track.CityCode]; !ok {
				continue
			}
		}
		if filter.Status != nil && track.Status != *filter.Status {
			continue
		}
		if filter.UserIDs != nil {
			if _, ok := userIDs[track.UserID]; !ok {
				continue
			}
		}
		if !adminTrackMatchesMissing(track, filter.Missing, linked[track.ID], candidates[track.ID]) {
			continue
		}
		total++
		if cursor != nil && !cursor.StartTime.IsZero() && (track.StartTime.After(cursor.StartTime) || (track.StartTime.Equal(cursor.StartTime) && track.ID >= cursor.ID)) {
			continue
		}
		clone := *track
		items = append(items, &clone)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].StartTime.Equal(items[j].StartTime) {
			return items[i].ID > items[j].ID
		}
		return items[i].StartTime.After(items[j].StartTime)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, total, nil
}

// AdminHardDeleteTrack 内存实现的物理删除：仅允许删除已下架轨迹，
// 内存仓储只持有轨迹主表与（注入的）景点关联，删除主表记录并清理景点关联。
func (r *InMemoryTrackRepository) AdminHardDeleteTrack(_ context.Context, trackID string) error {
	trackID = strings.TrimSpace(trackID)
	if trackID == "" {
		return ErrNotFound
	}
	r.mu.Lock()
	track, ok := r.tracks[trackID]
	if !ok {
		r.mu.Unlock()
		return ErrNotFound
	}
	if track.Status != models.TrackStatusDeleted {
		r.mu.Unlock()
		return ErrConflict
	}
	delete(r.tracks, trackID)
	r.mu.Unlock()
	if r.adminScenicSpots != nil {
		_ = r.adminScenicSpots.DeleteTrackScenicSpotLink(context.Background(), trackID, "")
	}
	return nil
}

func escapeAdminLike(value string) string {
	value = strings.ReplaceAll(value, "!", "!!")
	value = strings.ReplaceAll(value, "%", "!%")
	return strings.ReplaceAll(value, "_", "!_")
}

func (r *InMemoryTrackRepository) ListAdminTracksByTitle(_ context.Context, title string, cursor *models.TrackListCursor, limit int) ([]*models.Track, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	needle := strings.ToLower(title)
	items := make([]*models.Track, 0)
	var total int64
	for _, track := range r.tracks {
		if track == nil || track.Status == models.TrackStatusDeleted || !strings.Contains(strings.ToLower(track.Title), needle) {
			continue
		}
		total++
		if cursor != nil && !cursor.StartTime.IsZero() && (track.StartTime.After(cursor.StartTime) || (track.StartTime.Equal(cursor.StartTime) && track.ID >= cursor.ID)) {
			continue
		}
		clone := *track
		items = append(items, &clone)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].StartTime.Equal(items[j].StartTime) {
			return items[i].ID > items[j].ID
		}
		return items[i].StartTime.After(items[j].StartTime)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, total, nil
}

func adminZodiacMonthDayRanges(sign string) [][2]string {
	switch sign {
	case "capricorn":
		return [][2]string{{"12-22", "12-31"}, {"01-01", "01-19"}}
	case "aquarius":
		return [][2]string{{"01-20", "02-18"}}
	case "pisces":
		return [][2]string{{"02-19", "03-20"}}
	case "aries":
		return [][2]string{{"03-21", "04-19"}}
	case "taurus":
		return [][2]string{{"04-20", "05-20"}}
	case "gemini":
		return [][2]string{{"05-21", "06-20"}}
	case "cancer":
		return [][2]string{{"06-21", "07-22"}}
	case "leo":
		return [][2]string{{"07-23", "08-22"}}
	case "virgo":
		return [][2]string{{"08-23", "09-22"}}
	case "libra":
		return [][2]string{{"09-23", "10-22"}}
	case "scorpio":
		return [][2]string{{"10-23", "11-21"}}
	case "sagittarius":
		return [][2]string{{"11-22", "12-21"}}
	default:
		return nil
	}
}

func adminUserMatchesZodiac(birthday, sign string) bool {
	if sign == "" {
		return true
	}
	date, err := time.Parse(time.DateOnly, birthday)
	if err != nil || date.Format(time.DateOnly) != birthday || birthday < "1900-01-01" || birthday > time.Now().Format(time.DateOnly) {
		return false
	}
	monthDay := date.Format("01-02")
	for _, bounds := range adminZodiacMonthDayRanges(sign) {
		if monthDay >= bounds[0] && monthDay <= bounds[1] {
			return true
		}
	}
	return false
}

func (r *InMemoryAccountRestrictionRepository) hasActive(userID int64, now time.Time) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, item := range r.restrictions[userID] {
		if item != nil && item.Status == models.AccountRestrictionStatusActive && (item.ExpiresAt == nil || item.ExpiresAt.After(now)) {
			return true
		}
	}
	return false
}

func (r *InMemoryUserRepository) ListAdminUsers(_ context.Context, filter AdminUserListFilter, cursor *models.UserListCursor, limit int) ([]*models.User, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	needle := strings.ToLower(filter.Title)
	now := time.Now()
	items := make([]*models.User, 0)
	var total int64
	for _, user := range r.users {
		if user == nil || !strings.Contains(strings.ToLower(user.Nickname), needle) {
			continue
		}
		if filter.IsInternal != nil && user.IsInternal != *filter.IsInternal {
			continue
		}
		if filter.CityCode != "" && user.CityCode != filter.CityCode {
			continue
		}
		if filter.ProvinceCode != "" && user.ProvinceCode != filter.ProvinceCode {
			continue
		}
		if (filter.Gender == "male" || filter.Gender == "female") && user.Gender != filter.Gender {
			continue
		}
		if filter.Gender == "unknown" && (user.Gender == "male" || user.Gender == "female") {
			continue
		}
		if !adminUserMatchesZodiac(user.Birthday, filter.ZodiacSign) {
			continue
		}
		if filter.Restricted != nil {
			restricted := r.accountRestrictions != nil && r.accountRestrictions.hasActive(user.ID, now)
			if restricted != *filter.Restricted {
				continue
			}
		}
		total++
		if cursor != nil && !cursor.CreatedAt.IsZero() && (user.CreatedAt.After(cursor.CreatedAt) || (user.CreatedAt.Equal(cursor.CreatedAt) && user.ID >= cursor.ID)) {
			continue
		}
		clone := *user
		if clone.TokenVersion <= 0 {
			clone.TokenVersion = 1
		}
		items = append(items, &clone)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, total, nil
}

func (r *InMemoryCompanionRepository) ListAdminCompanions(_ context.Context, filter AdminCompanionListFilter, cursor *models.CompanionSessionListCursor, limit int) ([]*models.CompanionSession, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if filter.OwnerIDs != nil && len(filter.OwnerIDs) == 0 {
		return []*models.CompanionSession{}, 0, nil
	}
	var ownerSet map[int64]struct{}
	if filter.OwnerIDs != nil {
		ownerSet = make(map[int64]struct{}, len(filter.OwnerIDs))
		for _, id := range filter.OwnerIDs {
			ownerSet[id] = struct{}{}
		}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	needle := strings.ToLower(filter.Title)
	items := make([]*models.CompanionSession, 0)
	var total int64
	for _, session := range r.sessions {
		if session == nil {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(session.Title), needle) {
			continue
		}
		if filter.TrackType != "" && session.TrackType != filter.TrackType {
			continue
		}
		if ownerSet != nil {
			if _, ok := ownerSet[session.OwnerUserID]; !ok {
				continue
			}
		}
		total++
		if cursor != nil && !cursor.StartedAt.IsZero() && (session.StartedAt.After(cursor.StartedAt) || (session.StartedAt.Equal(cursor.StartedAt) && session.SessionID >= cursor.SessionID)) {
			continue
		}
		clone := *session
		items = append(items, &clone)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].StartedAt.Equal(items[j].StartedAt) {
			return items[i].SessionID > items[j].SessionID
		}
		return items[i].StartedAt.After(items[j].StartedAt)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, total, nil
}

// CountMembersBySessionIDs 批量统计每个会话的成员记录数。仅供管理后台使用。
func (r *InMemoryCompanionRepository) CountMembersBySessionIDs(_ context.Context, sessionIDs []string) (map[string]int, error) {
	counts := make(map[string]int, len(sessionIDs))
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, sessionID := range sessionIDs {
		counts[sessionID] = len(r.members[sessionID])
	}
	return counts, nil
}
