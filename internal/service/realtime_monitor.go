package service

import (
	"strconv"
	"strings"
	"sync"
	"time"
)

type RealtimeEvent struct {
	ID        uint64    `json:"id,string"`
	At        time.Time `json:"at"`
	Kind      string    `json:"kind"`
	Module    string    `json:"module"`
	UserID    string    `json:"user_id,omitempty"`
	SessionID string    `json:"session_id,omitempty"`
	ClientID  string    `json:"client_id,omitempty"`
	Reason    string    `json:"reason"`
}

type RealtimeSnapshot struct {
	StartedAt           time.Time  `json:"started_at"`
	PublisherConfigured bool       `json:"publisher_configured"`
	PublisherConnected  bool       `json:"publisher_connected"`
	LastConnectedAt     *time.Time `json:"last_connected_at,omitempty"`
	LastSuccessAt       *time.Time `json:"last_success_at,omitempty"`
	LastHeartbeatAt     *time.Time `json:"last_heartbeat_at,omitempty"`
	LastFailureAt       *time.Time `json:"last_failure_at,omitempty"`
	PublishSuccess      uint64     `json:"publish_success"`
	PublishFailure      uint64     `json:"publish_failure"`
	AuthDenied          uint64     `json:"auth_denied"`
	ACLDenied           uint64     `json:"acl_denied"`
	ConnectionFailure   uint64     `json:"connection_failure"`
	LastLatencyMS       int64      `json:"last_latency_ms"`
}

// RealtimeMonitor retains bounded, process-local diagnostics without credentials or payloads.
type RealtimeMonitor struct {
	mu     sync.Mutex
	state  RealtimeSnapshot
	events []RealtimeEvent
	next   uint64
}

func NewRealtimeMonitor() *RealtimeMonitor {
	return &RealtimeMonitor{state: RealtimeSnapshot{StartedAt: time.Now().UTC()}}
}
func (m *RealtimeMonitor) Snapshot() (RealtimeSnapshot, []RealtimeEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state, append([]RealtimeEvent(nil), m.events...)
}
func realtimeIdentity(username string) (module, user, session string) {
	if len(username) > 1024 {
		return "unknown", "", ""
	}
	parts := strings.Split(username, ":")
	if len(parts) == 4 && parts[0] == "msgv1" {
		if id, err := strconv.ParseInt(parts[1], 10, 64); err == nil && id > 0 {
			return "messages", strconv.FormatInt(id, 10), ""
		}
	}
	if claims, err := parseCompanionMQTTPrincipal(username); err == nil {
		return "companions", strconv.FormatInt(claims.UserID, 10), claims.SessionID
	}
	return "unknown", "", ""
}
func (m *RealtimeMonitor) add(kind, module, user, session, client, reason string) {
	now := time.Now().UTC()
	m.next++
	m.state.LastFailureAt = &now
	clientRunes := []rune(client)
	if len(clientRunes) > 160 {
		client = string(clientRunes[:160])
	}
	if len(session) > 128 {
		session = ""
	}
	e := RealtimeEvent{m.next, now, kind, module, user, session, client, reason}
	if len(m.events) == 200 {
		copy(m.events, m.events[1:])
		m.events[len(m.events)-1] = e
	} else {
		m.events = append(m.events, e)
	}
}
func (m *RealtimeMonitor) Connection(connected bool, failed bool) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.PublisherConfigured = true
	m.state.PublisherConnected = connected
	if connected {
		now := time.Now().UTC()
		m.state.LastConnectedAt = &now
	}
	if failed {
		m.state.ConnectionFailure++
		m.add("connection", "server", "", "", "", "发布连接不可用，等待重连或检查配置")
	}
}
func (m *RealtimeMonitor) Published(topic string, elapsed time.Duration, err error) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.LastLatencyMS = elapsed.Milliseconds()
	if err == nil {
		m.state.PublishSuccess++
		now := time.Now().UTC()
		m.state.LastSuccessAt = &now
		return
	}
	m.state.PublishFailure++
	module := "companions"
	if strings.Contains(topic, "/inbox/") {
		module = "messages"
	}
	m.add("publish", module, "", "", "", "发布未获确认：连接异常、超时或服务不可用")
}
func (m *RealtimeMonitor) Denied(kind, username, client string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	module, user, session := realtimeIdentity(username)
	reason := "凭证无效、过期、账号状态变化或绑定不匹配"
	if kind == "auth" {
		m.state.AuthDenied++
	} else {
		m.state.ACLDenied++
		reason = "订阅或发布未通过权限校验"
	}
	m.add(kind, module, user, session, client, reason)
}
func (m *RealtimeMonitor) Heartbeat() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	m.state.LastHeartbeatAt = &now
}
