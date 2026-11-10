package service

import (
	"context"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

func (s *MessageService) SetAvatarCache(cache *AssetCacheService) {
	s.avatarCache = cache
}

func (s *MessageService) messageUserView(ctx context.Context, id int64, user *models.User) models.MessageUser {
	view := models.MessageUser{ID: id, Nickname: "用户已注销"}
	if user == nil {
		return view
	}
	view.Nickname = user.Nickname
	avatar := strings.TrimSpace(user.AvatarURL)
	if shouldRewriteAvatarURL(avatar) {
		// OSS 原图可能为私有对象，只下发已同步的本站头像，不能回退到原图地址。
		local := ""
		if s.avatarCache != nil {
			cacheCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			local = s.avatarCache.EnsureCached(cacheCtx, id, formatUserAvatarCacheKey(id), avatar)
			cancel()
		}
		avatar = local
	}
	view.AvatarURL = fallbackAvatarURL(id, avatar)
	return view
}
