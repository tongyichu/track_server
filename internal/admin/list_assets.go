package admin

import (
	"context"
	"sync"

	"github.com/tongyichu/track_server/internal/models"
)

// Keep cache misses from serially delaying every row, with bounded downloads.
// Each result is ready before the response; never publish a not-yet-cached URL.
func parallelAdminAssets(count int, visit func(int)) {
	workers := 4
	if count < workers {
		workers = count
	}
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func(offset int) {
			defer wait.Done()
			for index := offset; index < count; index += workers {
				visit(index)
			}
		}(worker)
	}
	wait.Wait()
}

func (h *Handler) decorateAdminUserAvatars(ctx context.Context, users []*models.User) {
	if h == nil || h.userSvc == nil {
		return
	}
	unique := make([]*models.User, 0, len(users))
	seen := make(map[*models.User]bool, len(users))
	for _, user := range users {
		if user != nil && !seen[user] {
			seen[user] = true
			unique = append(unique, user)
		}
	}
	parallelAdminAssets(len(unique), func(index int) {
		user := unique[index]
		h.userSvc.DecorateAvatar(ctx, user)
		user.AvatarURL = rewriteAdminStaticURL(user.AvatarURL)
	})
}
