package middleware

import (
	"sync"
	"time"
)

type TokenBlacklist struct {
	mu      sync.RWMutex
	tokens  map[string]time.Time
	closeCh chan struct{}
}

func NewTokenBlacklist() *TokenBlacklist {
	bl := &TokenBlacklist{
		tokens:  make(map[string]time.Time),
		closeCh: make(chan struct{}),
	}
	go bl.cleanupLoop()
	return bl
}

func (bl *TokenBlacklist) Add(token string, expiresAt time.Time) {
	bl.mu.Lock()
	bl.tokens[token] = expiresAt
	bl.mu.Unlock()
}

func (bl *TokenBlacklist) IsBlacklisted(token string) bool {
	bl.mu.RLock()
	defer bl.mu.RUnlock()
	_, ok := bl.tokens[token]
	return ok
}

func (bl *TokenBlacklist) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			bl.cleanup()
		case <-bl.closeCh:
			return
		}
	}
}

func (bl *TokenBlacklist) cleanup() {
	now := time.Now()
	bl.mu.Lock()
	for token, exp := range bl.tokens {
		if now.After(exp) {
			delete(bl.tokens, token)
		}
	}
	bl.mu.Unlock()
}

func (bl *TokenBlacklist) Close() {
	close(bl.closeCh)
}
