package admin

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (h *Handler) auditTargetNames(ctx context.Context, items []AuditEntry) map[string]map[string]string {
	var userIDs []int64
	var trackIDs, spotIDs []string
	accounts := make(map[string]struct{})
	for _, item := range items {
		route, ok := strings.CutPrefix(item.Path, "/admin/api/")
		if !ok {
			continue
		}
		parts := strings.Split(route, "/")
		id := ""
		if len(parts) > 1 {
			id, _ = url.PathUnescape(parts[1])
		}
		switch parts[0] {
		case "users":
			if userID, err := strconv.ParseInt(id, 10, 64); err == nil && userID > 0 {
				userIDs = append(userIDs, userID)
			}
		case "tracks", "place-candidates":
			if strings.HasPrefix(id, "NO.") {
				trackIDs = append(trackIDs, id)
			}
		case "scenic-spots":
			switch id {
			case "", "backfill", "description", "provider-content":
			default:
				spotIDs = append(spotIDs, id)
			}
			if len(parts) == 4 && parts[2] == "tracks" {
				trackIDs = append(trackIDs, parts[3])
			}
		case "route-groups":
			if len(parts) == 4 && parts[2] == "members" {
				trackIDs = append(trackIDs, parts[3])
			}
		case "accounts":
			if id != "" {
				accounts[id] = struct{}{}
			}
		case "password", "security", "password-recovery":
			if item.Username != "" {
				accounts[item.Username] = struct{}{}
			}
		}
	}

	// Names are optional. If a lookup times out or the object is gone, its ID still identifies the audit entry.
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	names := make(map[string]map[string]string)
	if len(userIDs) > 0 && h.userRepo != nil {
		if users, err := h.userRepo.FindByIDs(ctx, userIDs); err == nil {
			names["user"] = make(map[string]string, len(users))
			for id, user := range users {
				if user != nil {
					names["user"][strconv.FormatInt(id, 10)] = user.Nickname
				}
			}
		}
	}
	if len(trackIDs) > 0 && h.trackRepo != nil {
		if tracks, err := h.trackRepo.FindByIDs(ctx, trackIDs); err == nil {
			names["track"] = make(map[string]string, len(tracks))
			for id, track := range tracks {
				if track != nil {
					names["track"][id] = track.Title
				}
			}
		}
	}
	if len(spotIDs) > 0 && h.scenicSpotSvc.Enabled() {
		if spots, err := h.scenicSpotSvc.NamesByID(ctx, spotIDs); err == nil {
			names["spot"] = spots
		}
	}
	if len(accounts) > 0 {
		if list, err := h.auth.manager.list(ctx); err == nil {
			names["account"] = make(map[string]string)
			for _, account := range list {
				if _, ok := accounts[account.Username]; ok {
					names["account"][account.Username] = account.DisplayName
				}
			}
		}
	}
	return names
}
