package service

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/tongyichu/track_server/internal/repository"
)

const userActivityInterval = time.Minute
const userActivityCacheLimit = 10000

type userActivityWindow struct {
	mu       sync.Mutex
	attempts map[int64]time.Time
}

// RecordActivity is best effort; activity writes never change profile updated_at.
func (s *UserService) RecordActivity(ctx context.Context, userID int64) {
	if s == nil || userID <= 0 || ctx.Err() != nil {
		return
	}
	repo, ok := s.users.(repository.UserActivityRepository)
	if !ok {
		return
	}
	now := time.Now()
	w := &s.activity
	w.mu.Lock()
	if now.Sub(w.attempts[userID]) < userActivityInterval {
		w.mu.Unlock()
		return
	}
	if w.attempts == nil {
		w.attempts = make(map[int64]time.Time)
	}
	if len(w.attempts) >= userActivityCacheLimit {
		for id, at := range w.attempts {
			if now.Sub(at) >= userActivityInterval {
				delete(w.attempts, id)
			}
		}
		if len(w.attempts) >= userActivityCacheLimit {
			for id := range w.attempts {
				delete(w.attempts, id)
				break
			}
		}
	}
	w.attempts[userID] = now
	w.mu.Unlock()
	writeCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	if err := repo.TouchUserActivity(writeCtx, userID, now); err != nil {
		log.Printf("record user activity failed: %v", err)
	}
}
