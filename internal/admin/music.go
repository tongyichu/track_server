package admin

import "github.com/tongyichu/track_server/internal/service"
import "github.com/tongyichu/track_server/internal/musicapi"

func (h *Handler) SetMusicService(s *service.MusicService) {
	h.music = musicapi.Handler{Service: s, Admin: true}
}
