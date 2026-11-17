package service

import (
	"context"
	"fmt"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type MusicPlacement struct {
	ID        string `json:"id"`
	Revision  int64  `json:"revision"`
	Published bool   `json:"published"`
}

// Arrange changes only placement metadata. Existing repository CAS applies to every
// changed item; the caller must reload after any failure, including partial saves.
func (s *MusicService) Arrange(ctx context.Context, layout []MusicPlacement) ([]models.MusicTrack, int, error) {
	items, err := s.List(ctx, true)
	if err != nil {
		return nil, 0, err
	}
	if len(layout) > 500 || len(layout) != len(items) {
		return nil, 0, repository.ErrMusicConflict
	}
	byID := make(map[string]models.MusicTrack, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	next := make([]models.MusicTrack, 0, len(layout))
	changed := make([]bool, len(layout))
	newlyPublished := make([]bool, len(layout))
	for i, placement := range layout {
		old, ok := byID[placement.ID]
		if !ok || old.Revision != placement.Revision {
			return nil, 0, repository.ErrMusicConflict
		}
		delete(byID, placement.ID)
		if placement.Published && old.AudioLink == "" && !musicHash.MatchString(old.AudioSHA) {
			return nil, 0, fmt.Errorf("%w: 音乐缺少有效音频", ErrMusicInput)
		}
		changed[i] = old.Published != placement.Published || old.SortOrder != i
		newlyPublished[i] = !old.Published && placement.Published
		old.Published, old.SortOrder = placement.Published, i
		next = append(next, old)
	}
	// Probe only newly published links; rearranging or taking down an existing
	// item must not depend on its external audio host.
	for i, item := range next {
		if newlyPublished[i] && item.AudioLink != "" {
			info, e := probeMusicLink(ctx, item.AudioLink)
			if e != nil {
				return nil, 0, e
			}
			next[i].AudioLink, next[i].AudioBytes, next[i].AudioVersion = info.URL, info.Bytes, info.Version
		}
	}
	saved := 0
	for i := range next {
		if !changed[i] {
			continue
		}
		previous := next[i].Revision
		next[i].Revision++
		next[i].UpdatedAt = time.Now().UTC()
		if err := s.repo.SaveMusic(ctx, next[i], previous); err != nil {
			return nil, saved, err
		}
		saved++
	}
	return next, saved, nil
}
