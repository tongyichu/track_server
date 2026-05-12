package admin

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// Session 表示一个管理员登录会话。
type Session struct {
	Token     string
	Username  string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// SessionStore 是管理后台的内存会话存储，不依赖外部存储。
//
// 设计取舍：
// - 管理员数量很少、会话数量极其有限，内存存储足够用；
// - 进程重启后需要重新登录，这也是后台系统的合理行为；
// - 过期会话懒清理 + 定时清理（GC 协程）。
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	ttl      time.Duration

	stop chan struct{}
}

// NewSessionStore 创建一个 SessionStore，ttl<=0 时默认为 12 小时。
func NewSessionStore(ttl time.Duration) *SessionStore {
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	s := &SessionStore{
		sessions: make(map[string]*Session),
		ttl:      ttl,
		stop:     make(chan struct{}),
	}
	go s.gcLoop()
	return s
}

// Close 停止后台 GC 协程。
func (s *SessionStore) Close() {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
}

// Create 为指定用户名创建一个新会话并返回。
func (s *SessionStore) Create(username string) (*Session, error) {
	token, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	sess := &Session{
		Token:     token,
		Username:  username,
		CreatedAt: now,
		ExpiresAt: now.Add(s.ttl),
	}
	s.mu.Lock()
	s.sessions[token] = sess
	s.mu.Unlock()
	return sess, nil
}

// Get 返回 token 对应的未过期会话；若不存在或已过期返回 nil。
func (s *SessionStore) Get(token string) *Session {
	if token == "" {
		return nil
	}
	s.mu.RLock()
	sess, ok := s.sessions[token]
	s.mu.RUnlock()
	if !ok {
		return nil
	}
	if time.Now().After(sess.ExpiresAt) {
		s.Delete(token)
		return nil
	}
	return sess
}

// Delete 删除 token 对应会话。
func (s *SessionStore) Delete(token string) {
	if token == "" {
		return
	}
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

// TTL 返回会话有效期。
func (s *SessionStore) TTL() time.Duration { return s.ttl }

func (s *SessionStore) gcLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case now := <-ticker.C:
			s.gc(now)
		}
	}
}

func (s *SessionStore) gc(now time.Time) {
	s.mu.Lock()
	for token, sess := range s.sessions {
		if now.After(sess.ExpiresAt) {
			delete(s.sessions, token)
		}
	}
	s.mu.Unlock()
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
