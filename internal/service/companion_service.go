package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

const (
	defaultCompanionTitle          = "与友同行"
	defaultCompanionMaxMembers     = 8
	maxCompanionMaxMembers         = 32
	defaultCompanionPageSize       = 20
	maxCompanionPageSize           = 50
	defaultCompanionMQTTTopicRoot  = "companion"
	defaultCompanionMQTTTTL        = time.Hour
	defaultCompanionMQTTPublishTTL = 5 * time.Second
	companionMQTTPrincipalV1       = "cmpv1"

	// 弹幕：内容长度上限（按 UTF-8 字符数计算）。
	companionDanmakuMaxContentLength = 200
	// 弹幕：单成员限速窗口长度。
	companionDanmakuRateLimitWindow = 10 * time.Second
	// 弹幕：单成员在 companionDanmakuRateLimitWindow 内允许发送的最大条数。
	companionDanmakuRateLimitMax = 5
	// 弹幕：session 级总量限速窗口长度（与单成员窗口一致即可）。
	companionDanmakuSessionRateLimitWindow = 10 * time.Second
	// 弹幕：session 级在窗口内允许发送的最大条数（所有成员合计）。
	companionDanmakuSessionRateLimitMax = 50

	// 附近房间：默认 / 最大搜索半径（米）。
	defaultCompanionNearbyRadiusMeters = 5000.0
	maxCompanionNearbyRadiusMeters     = 20000.0
	// 附近房间：单次返回的最大房间数；轮播卡片场景已绰绰有余。
	maxCompanionNearbyItems = 50
	// 地球平均半径（米），Haversine 距离计算用。
	earthRadiusMeters = 6371000.0

	// 自动收尾：单次扫描 active session 的上限。
	companionAutoCloseScanLimit = 1000

	// 关键事件：单场同行最多保留 owner 上报的事件数。
	companionEventMaxPerSession    = 100
	companionEventMaxTitleRunes    = 64
	companionEventMaxContentRunes  = 500
	companionEventMaxMetadataBytes = 2048
)

type companionAutoCloseRule struct {
	InactiveTimeout time.Duration
	MaxDuration     time.Duration
}

var defaultCompanionAutoCloseRule = companionAutoCloseRule{
	InactiveTimeout: 45 * time.Minute,
	MaxDuration:     24 * time.Hour,
}

var companionAutoCloseRules = map[string]companionAutoCloseRule{
	"running": {
		InactiveTimeout: 30 * time.Minute,
		MaxDuration:     8 * time.Hour,
	},
	"hiking": {
		InactiveTimeout: 30 * time.Minute,
		MaxDuration:     16 * time.Hour,
	},
	"climbing": {
		InactiveTimeout: 45 * time.Minute,
		MaxDuration:     24 * time.Hour,
	},
	"riding": {
		InactiveTimeout: 30 * time.Minute,
		MaxDuration:     24 * time.Hour,
	},
	"driving": {
		InactiveTimeout: 60 * time.Minute,
		MaxDuration:     72 * time.Hour,
	},
}

var companionEventTypes = map[string]struct{}{
	"member_left":         {},
	"member_disconnected": {},
	"member_reconnected":  {},
	"notice_sent":         {},
	"checkpoint_reached":  {},
	"risk_reported":       {},
	"custom":              {},
}

// CompanionService 实现“同行”控制面的业务逻辑。
type CompanionService struct {
	repo             repository.CompanionRepository
	users            repository.UserRepository
	tracks           repository.TrackRepository
	mqtt             CompanionMQTTOptions
	avatarCache      *AssetCacheService
	screenshotCache  *AssetCacheService
	publisher        CompanionControlPublisher
	danmakuPublisher CompanionControlPublisher
}

type CompanionAutoCloseResult struct {
	Scanned int `json:"scanned"`
	Closed  int `json:"closed"`
}

// CompanionMQTTOptions defines MQTT / EMQX integration options.
type CompanionMQTTOptions struct {
	BrokerURL        string
	WebsocketURL     string
	TopicPrefix      string
	CredentialTTL    time.Duration
	CredentialSecret string
}

// CompanionControlPublisher publishes companion control events to EMQX.
type CompanionControlPublisher interface {
	Publish(ctx context.Context, topic string, payload []byte) error
	Close() error
}

// CompanionControlEvent represents one control message delivered on companion/{session_id}/control.
type CompanionControlEvent struct {
	Event          string `json:"event"`
	SessionID      string `json:"session_id"`
	MemberUserID   int64  `json:"member_user_id,omitempty"`
	OperatorUserID int64  `json:"operator_user_id,omitempty"`
	Reason         string `json:"reason,omitempty"`
	// Enabled 仅用于 danmaku_toggled 事件：true=已开启，false=已关闭。
	// 用指针以便在不携带该字段的事件中通过 omitempty 隐藏。
	Enabled *bool     `json:"enabled,omitempty"`
	At      time.Time `json:"at"`
}

// CompanionMQTTControlPublisherOptions defines server-side MQTT publishing options.
type CompanionMQTTControlPublisherOptions struct {
	BrokerURL      string
	ClientID       string
	Username       string
	Password       string
	PublishTimeout time.Duration
}

type mqttCompanionControlPublisher struct {
	client         mqtt.Client
	publishTimeout time.Duration
}

// NewCompanionService constructs a new CompanionService.
func NewCompanionService(repo repository.CompanionRepository, users repository.UserRepository) *CompanionService {
	return &CompanionService{
		repo:  repo,
		users: users,
		mqtt: CompanionMQTTOptions{
			TopicPrefix:   defaultCompanionMQTTTopicRoot,
			CredentialTTL: defaultCompanionMQTTTTL,
		},
	}
}

// SetTrackRepository injects track repository for enforcing running-track/companion exclusivity.
func (s *CompanionService) SetTrackRepository(repo repository.TrackRepository) {
	if s == nil {
		return
	}
	s.tracks = repo
}

// SetMQTTOptions updates MQTT / EMQX integration options.
func (s *CompanionService) SetMQTTOptions(opts CompanionMQTTOptions) {
	if s == nil {
		return
	}
	if strings.TrimSpace(opts.TopicPrefix) == "" {
		opts.TopicPrefix = defaultCompanionMQTTTopicRoot
	} else {
		opts.TopicPrefix = strings.Trim(strings.TrimSpace(opts.TopicPrefix), "/")
	}
	if opts.CredentialTTL <= 0 {
		opts.CredentialTTL = defaultCompanionMQTTTTL
	}
	opts.BrokerURL = strings.TrimSpace(opts.BrokerURL)
	opts.WebsocketURL = strings.TrimSpace(opts.WebsocketURL)
	opts.CredentialSecret = strings.TrimSpace(opts.CredentialSecret)
	s.mqtt = opts
}

// SetControlPublisher injects the server-side control message publisher.
func (s *CompanionService) SetControlPublisher(publisher CompanionControlPublisher) {
	if s == nil {
		return
	}
	s.publisher = publisher
}

// SetDanmakuPublisher injects the server-side danmaku broadcast publisher.
//
// 弹幕广播 topic 与控制 topic 的语义不同（前者高频、面向所有成员；后者低频、用于会话生命周期事件），
// 因此独立持有一个 publisher，便于上层在需要时为其分配独立的 MQTT 客户端 / 限速策略。
// 若不单独设置，IngestDanmakuFromMQTT 会回退使用 controlPublisher。
func (s *CompanionService) SetDanmakuPublisher(publisher CompanionControlPublisher) {
	if s == nil {
		return
	}
	s.danmakuPublisher = publisher
}

// SetAvatarCache injects avatar cache for rewriting participant avatar URLs.
func (s *CompanionService) SetAvatarCache(cache *AssetCacheService) {
	if s == nil {
		return
	}
	s.avatarCache = cache
}

// SetScreenshotCache injects screenshot cache for rewriting companion session screenshot URLs.
func (s *CompanionService) SetScreenshotCache(cache *AssetCacheService) {
	if s == nil {
		return
	}
	s.screenshotCache = cache
}

// CreateCompanionSessionInput describes the payload to create a companion session.
type CreateCompanionSessionInput struct {
	Title      string `json:"title"`
	TrackType  string `json:"track_type"`
	LocateAddr string `json:"locate_addr"`
	MaxMembers int    `json:"max_members"`
	// Visibility 可选；默认 private（向后兼容）。
	// public 房间凭 session_id 即可加入，并会出现在附近房间列表中。
	Visibility string `json:"visibility"`
}

// JoinCompanionSessionInput describes the payload to join a companion session.
//
// 私密房间必须填写 join_token；公开房间可填写 session_id。两者二选一。
type JoinCompanionSessionInput struct {
	JoinToken string `json:"join_token"`
	SessionID string `json:"session_id"`
}

// UpdateCompanionSessionStatsInput describes owner-updatable summary data after a session ends.
type UpdateCompanionSessionStatsInput struct {
	LocateAddr             *string  `json:"locate_addr"`
	TotalDistance          *float64 `json:"total_distance"`
	TotalDuration          *int64   `json:"total_duration"`
	TrackScreenshotURL     *string  `json:"track_screenshot_url"`
	ActualParticipantCount *int64   `json:"actual_participant_count"`
}

// CreateCompanionEventInput describes one owner-reported key event in a companion session.
type CreateCompanionEventInput struct {
	EventType     string          `json:"event_type"`
	TargetUserID  int64           `json:"target_user_id"`
	Title         string          `json:"title"`
	Content       string          `json:"content"`
	EventTime     time.Time       `json:"event_time"`
	ClientEventID string          `json:"client_event_id"`
	Metadata      json.RawMessage `json:"metadata"`
}

// ListCompanionEventsInput describes paging input for owner-visible companion events.
type ListCompanionEventsInput struct {
	Cursor string
	Limit  int
	Order  string
}

// CompanionJoinInfo is the owner-only invitation info returned by control plane APIs.
type CompanionJoinInfo struct {
	JoinToken string `json:"join_token"`
}

// CompanionSessionState is the standard control-plane response envelope.
type CompanionSessionState struct {
	Session  *models.CompanionSession  `json:"session"`
	Join     *CompanionJoinInfo        `json:"join,omitempty"`
	Snapshot *models.CompanionSnapshot `json:"snapshot"`
}

// CompanionSessionPreview is returned when a user previews a room by join token without joining it.
type CompanionSessionPreview struct {
	Session  *models.CompanionSession  `json:"session"`
	Snapshot *models.CompanionSnapshot `json:"snapshot"`
}

// ListCompanionHistoryInput describes paging input for companion history list.
type ListCompanionHistoryInput struct {
	Cursor string
	Limit  int
}

// ListCompanionNearbyInput 描述查询附近 active 同行房间的入参。
//
//   - Latitude / Longitude：客户端定位（WGS84）。
//   - RadiusMeters：可选，默认 defaultCompanionNearbyRadiusMeters，最大 maxCompanionNearbyRadiusMeters。
//   - Limit：可选，默认 / 最大 maxCompanionNearbyItems。
type ListCompanionNearbyInput struct {
	Latitude     float64
	Longitude    float64
	RadiusMeters float64
	Limit        int
}

// CompanionPositionUpsertInput is reserved for future EMQX / HTTP ingest integration.
type CompanionPositionUpsertInput struct {
	TrackID          string
	Latitude         float64
	Longitude        float64
	CoordinateSystem string
	SpeedKmh         float64
	Heading          float64
	AccuracyM        float64
	Altitude         float64
	RecordedAt       time.Time
	Seq              int64
	Source           string
}

// CompanionMQTTTopicBindings describes MQTT topic permissions for one member.
type CompanionMQTTTopicBindings struct {
	LocationPublish   string `json:"location_publish"`
	LocationSubscribe string `json:"location_subscribe"`
	PresencePublish   string `json:"presence_publish"`
	PresenceSubscribe string `json:"presence_subscribe"`
	ControlSubscribe  string `json:"control_subscribe"`
	DanmakuPublish    string `json:"danmaku_publish"`
	DanmakuSubscribe  string `json:"danmaku_subscribe"`
}

// CompanionMQTTCredentials is returned to app clients so they can connect to EMQX.
type CompanionMQTTCredentials struct {
	SessionID    string                     `json:"session_id"`
	BrokerURL    string                     `json:"broker_url,omitempty"`
	WebsocketURL string                     `json:"websocket_url,omitempty"`
	ClientID     string                     `json:"client_id"`
	Username     string                     `json:"username"`
	Password     string                     `json:"password"`
	ExpiresAt    time.Time                  `json:"expires_at"`
	Topics       CompanionMQTTTopicBindings `json:"topics"`
}

// CompanionMQTTAuthInput is the EMQX HTTP AuthN payload.
type CompanionMQTTAuthInput struct {
	ClientID string `json:"clientid"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// CompanionMQTTAuthResult is the EMQX HTTP AuthN response body.
type CompanionMQTTAuthResult struct {
	Result      string `json:"result"`
	IsSuperuser bool   `json:"is_superuser"`
}

// CompanionMQTTACLInput is the EMQX HTTP AuthZ payload.
type CompanionMQTTACLInput struct {
	ClientID string `json:"clientid"`
	Username string `json:"username"`
	Action   string `json:"action"`
	Topic    string `json:"topic"`
}

// CompanionMQTTACLResult is the EMQX HTTP AuthZ response body.
type CompanionMQTTACLResult struct {
	Result string `json:"result"`
}

// CompanionMQTTLocationIngestInput is the HTTP callback payload from EMQX rule engine.
type CompanionMQTTLocationIngestInput struct {
	SessionID        string    `json:"session_id"`
	UserID           int64     `json:"user_id"`
	TrackID          string    `json:"track_id"`
	Latitude         float64   `json:"latitude"`
	Longitude        float64   `json:"longitude"`
	CoordinateSystem string    `json:"coordinate_system"`
	SpeedKmh         float64   `json:"speed_kmh"`
	Heading          float64   `json:"heading"`
	AccuracyM        float64   `json:"accuracy_m"`
	Altitude         float64   `json:"altitude"`
	RecordedAt       time.Time `json:"recorded_at"`
	Seq              int64     `json:"seq"`
	Source           string    `json:"source"`
	ClientID         string    `json:"client_id"`
	Username         string    `json:"username"`
}

// CompanionMQTTPresenceIngestInput is the HTTP callback payload from EMQX rule engine.
type CompanionMQTTPresenceIngestInput struct {
	SessionID  string                         `json:"session_id"`
	UserID     int64                          `json:"user_id"`
	Status     models.CompanionPresenceStatus `json:"presence_status"`
	LastSeenAt time.Time                      `json:"last_seen_at"`
	ClientID   string                         `json:"client_id"`
	Username   string                         `json:"username"`
}

// CompanionMQTTDanmakuIngestInput is the HTTP callback payload from EMQX rule engine when a member publishes a danmaku.
//
// 字段来源：
//   - SessionID / UserID 由 EMQX rule SQL 从 topic 中提取；
//   - Content 来自客户端 publish payload；
//   - ClientID / Username 用于服务端复核 MQTT principal。
type CompanionMQTTDanmakuIngestInput struct {
	SessionID string `json:"session_id"`
	UserID    int64  `json:"user_id"`
	Content   string `json:"content"`
	ClientID  string `json:"client_id"`
	Username  string `json:"username"`
}

// CompanionDanmakuBroadcast 是服务端向 companion/{session_id}/danmaku 广播的消息体。
//
// 客户端约定：
//   - 收到 message_id 等于本次客户端发送的对应记录时（通常通过 user_id+content 自匹配，或服务端在客户端连接元信息中预占 message_id），视为发送成功；
//   - 客户端启动 publish 后开启超时（建议 3s），超时未收到自身广播则展示发送失败。
type CompanionDanmakuBroadcast struct {
	MessageID int64     `json:"message_id"`
	SessionID string    `json:"session_id"`
	UserID    int64     `json:"user_id"`
	Nickname  string    `json:"nickname"`
	AvatarURL string    `json:"avatar_url"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type companionMQTTPrincipalClaims struct {
	SessionID string
	UserID    int64
	ExpiresAt time.Time
	Nonce     string
}

const (
	CompanionControlEventMemberLeft     = "member_left"
	CompanionControlEventSessionEnded   = "session_ended"
	CompanionControlEventMemberKicked   = "member_kicked"
	CompanionControlEventDanmakuToggled = "danmaku_toggled"
)

// NewMQTTCompanionControlPublisher creates a best-effort MQTT publisher for companion control events.
func NewMQTTCompanionControlPublisher(opts CompanionMQTTControlPublisherOptions) (CompanionControlPublisher, error) {
	brokerURL := normalizeCompanionMQTTBrokerURL(opts.BrokerURL)
	if brokerURL == "" {
		return nil, errors.New("companion mqtt broker_url is required")
	}
	clientID := strings.TrimSpace(opts.ClientID)
	if clientID == "" {
		randomSuffix, err := randomToken("", 6)
		if err != nil {
			return nil, err
		}
		clientID = "track-server-companion-" + randomSuffix
	}
	publishTimeout := opts.PublishTimeout
	if publishTimeout <= 0 {
		publishTimeout = defaultCompanionMQTTPublishTTL
	}
	mqttOpts := mqtt.NewClientOptions()
	mqttOpts.AddBroker(brokerURL)
	mqttOpts.SetClientID(clientID)
	mqttOpts.SetCleanSession(true)
	mqttOpts.SetAutoReconnect(true)
	mqttOpts.SetConnectRetry(true)
	mqttOpts.SetOrderMatters(false)
	mqttOpts.SetWriteTimeout(publishTimeout)
	mqttOpts.SetConnectTimeout(publishTimeout)
	if username := strings.TrimSpace(opts.Username); username != "" {
		mqttOpts.SetUsername(username)
		mqttOpts.SetPassword(opts.Password)
	}
	client := mqtt.NewClient(mqttOpts)
	token := client.Connect()
	if !token.WaitTimeout(publishTimeout) {
		return nil, errors.New("mqtt control publisher connect timeout")
	}
	if err := token.Error(); err != nil {
		return nil, err
	}
	return &mqttCompanionControlPublisher{client: client, publishTimeout: publishTimeout}, nil
}

func (p *mqttCompanionControlPublisher) Publish(ctx context.Context, topic string, payload []byte) error {
	if p == nil || p.client == nil {
		return errors.New("mqtt control publisher not configured")
	}
	timeout := p.publishTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if remain := time.Until(deadline); remain > 0 && remain < timeout {
			timeout = remain
		}
	}
	if timeout <= 0 {
		timeout = defaultCompanionMQTTPublishTTL
	}
	token := p.client.Publish(topic, 1, false, payload)
	if !token.WaitTimeout(timeout) {
		return errors.New("mqtt control publish timeout")
	}
	return token.Error()
}

func (p *mqttCompanionControlPublisher) Close() error {
	if p == nil || p.client == nil {
		return nil
	}
	p.client.Disconnect(250)
	return nil
}

// IssueMQTTCredentials issues short-lived MQTT credentials to one joined member.
func (s *CompanionService) IssueMQTTCredentials(ctx context.Context, userID int64, sessionID string) (*CompanionMQTTCredentials, error) {
	if err := s.requireMQTTConfigured(); err != nil {
		return nil, err
	}
	session, err := s.requireJoinedActiveSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	member, err := s.repo.FindMember(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	nonce, err := randomToken("", 6)
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(s.mqtt.CredentialTTL).UTC().Truncate(time.Second)
	principal := fmt.Sprintf("%s:%s:%d:%d:%s", companionMQTTPrincipalV1, session.SessionID, userID, expiresAt.Unix(), nonce)
	clientID := fmt.Sprintf("cmp-%s-%d-%s", session.SessionID, userID, nonce)
	password := s.signMQTTCredentials(principal, clientID)
	member.MQTTPrincipal = principal
	member.MQTTClientID = clientID
	if err := s.repo.UpsertMember(ctx, member); err != nil {
		return nil, err
	}
	return &CompanionMQTTCredentials{
		SessionID:    session.SessionID,
		BrokerURL:    s.mqtt.BrokerURL,
		WebsocketURL: s.mqtt.WebsocketURL,
		ClientID:     clientID,
		Username:     principal,
		Password:     password,
		ExpiresAt:    expiresAt,
		Topics:       s.buildMQTTTopicBindings(session.SessionID, userID),
	}, nil
}

// AuthenticateMQTTConnection validates EMQX HTTP AuthN callbacks.
func (s *CompanionService) AuthenticateMQTTConnection(ctx context.Context, in CompanionMQTTAuthInput) CompanionMQTTAuthResult {
	if err := s.requireMQTTConfigured(); err != nil {
		return CompanionMQTTAuthResult{Result: "deny"}
	}
	if strings.TrimSpace(in.ClientID) == "" || strings.TrimSpace(in.Username) == "" || in.Password == "" {
		return CompanionMQTTAuthResult{Result: "deny"}
	}
	if _, _, err := s.verifyMQTTBinding(ctx, in.Username, in.ClientID, in.Password, true); err != nil {
		return CompanionMQTTAuthResult{Result: "deny"}
	}
	return CompanionMQTTAuthResult{Result: "allow", IsSuperuser: false}
}

// AuthorizeMQTTOperation validates EMQX HTTP AuthZ callbacks.
func (s *CompanionService) AuthorizeMQTTOperation(ctx context.Context, in CompanionMQTTACLInput) CompanionMQTTACLResult {
	if err := s.requireMQTTConfigured(); err != nil {
		return CompanionMQTTACLResult{Result: "deny"}
	}
	claims, _, err := s.verifyMQTTBinding(ctx, in.Username, in.ClientID, "", false)
	if err != nil {
		return CompanionMQTTACLResult{Result: "deny"}
	}
	action := strings.ToLower(strings.TrimSpace(in.Action))
	topic := strings.TrimSpace(in.Topic)
	if topic == "" {
		return CompanionMQTTACLResult{Result: "deny"}
	}
	switch action {
	case "publish", "pub":
		if topic == s.memberLocationTopic(claims.SessionID, claims.UserID) || topic == s.memberPresenceTopic(claims.SessionID, claims.UserID) || topic == s.memberDanmakuUplinkTopic(claims.SessionID, claims.UserID) {
			return CompanionMQTTACLResult{Result: "allow"}
		}
	case "subscribe", "sub":
		if topic == s.sessionLocationWildcard(claims.SessionID) || topic == s.sessionPresenceWildcard(claims.SessionID) || topic == s.controlTopic(claims.SessionID) || topic == s.memberLocationTopic(claims.SessionID, claims.UserID) || topic == s.memberPresenceTopic(claims.SessionID, claims.UserID) || topic == s.sessionDanmakuBroadcastTopic(claims.SessionID) {
			return CompanionMQTTACLResult{Result: "allow"}
		}
	}
	return CompanionMQTTACLResult{Result: "deny"}
}

// IngestLocationFromMQTT ingests one latest location snapshot from EMQX rule engine.
func (s *CompanionService) IngestLocationFromMQTT(ctx context.Context, in CompanionMQTTLocationIngestInput) error {
	if strings.TrimSpace(in.Username) != "" || strings.TrimSpace(in.ClientID) != "" {
		claims, _, err := s.verifyMQTTBinding(ctx, in.Username, in.ClientID, "", false)
		if err != nil {
			return err
		}
		if claims.SessionID != strings.TrimSpace(in.SessionID) || claims.UserID != in.UserID {
			return repository.ErrForbidden
		}
	}
	if s.shouldIgnoreIncomingPosition(ctx, strings.TrimSpace(in.SessionID), in.UserID, in.Seq, in.RecordedAt) {
		return nil
	}
	source := strings.TrimSpace(in.Source)
	if source == "" {
		source = "mqtt_rule_engine"
	}
	return s.UpsertPositionSnapshot(ctx, strings.TrimSpace(in.SessionID), in.UserID, CompanionPositionUpsertInput{
		TrackID:          in.TrackID,
		Latitude:         in.Latitude,
		Longitude:        in.Longitude,
		CoordinateSystem: in.CoordinateSystem,
		SpeedKmh:         in.SpeedKmh,
		Heading:          in.Heading,
		AccuracyM:        in.AccuracyM,
		Altitude:         in.Altitude,
		RecordedAt:       in.RecordedAt,
		Seq:              in.Seq,
		Source:           source,
	})
}

// IngestPresenceFromMQTT ingests one online/offline event from EMQX rule engine.
func (s *CompanionService) IngestPresenceFromMQTT(ctx context.Context, in CompanionMQTTPresenceIngestInput) error {
	if strings.TrimSpace(in.Username) != "" || strings.TrimSpace(in.ClientID) != "" {
		claims, _, err := s.verifyMQTTBinding(ctx, in.Username, in.ClientID, "", false)
		if err != nil {
			return err
		}
		if claims.SessionID != strings.TrimSpace(in.SessionID) || claims.UserID != in.UserID {
			return repository.ErrForbidden
		}
	}
	return s.UpdatePresence(ctx, strings.TrimSpace(in.SessionID), in.UserID, in.Status, in.LastSeenAt)
}

// IngestDanmakuFromMQTT ingests one danmaku message from EMQX rule engine.
//
// 流程（方案 A）：
//  1. 通过 client_id / username 复核 MQTT principal，确保上行身份未被伪造；
//  2. 校验 session 仍为 active 且当前用户为 joined 成员；
//  3. 内容做 UTF-8 长度限制 (≤ companionDanmakuMaxContentLength)，去掉两端空白；
//  4. 滚动窗口限速：单成员在 companionDanmakuRateLimitWindow 内最多
//     companionDanmakuRateLimitMax 条；
//  5. 落库（持久化用于审计 / 回溯，不在 snapshot 返回）；
//  6. 服务端向 sessionDanmakuBroadcastTopic 广播给所有成员（包括发送者，
//     发送者据此判定“发送成功”，超时未收到则展示失败）。
func (s *CompanionService) IngestDanmakuFromMQTT(ctx context.Context, in CompanionMQTTDanmakuIngestInput) error {
	if s == nil || s.repo == nil {
		return errors.New("companion service not configured")
	}
	sessionID := strings.TrimSpace(in.SessionID)
	if sessionID == "" {
		return invalidArg("session_id is required")
	}
	if in.UserID <= 0 {
		return invalidArg("user_id is required")
	}
	// 1. principal 复核：上行 ingest 必须由 EMQX rule engine 携带，
	// 否则任何外部调用都可以伪造 session_id / user_id 直接广播。
	if strings.TrimSpace(in.Username) == "" && strings.TrimSpace(in.ClientID) == "" {
		return repository.ErrForbidden
	}
	claims, _, err := s.verifyMQTTBinding(ctx, in.Username, in.ClientID, "", false)
	if err != nil {
		return err
	}
	if claims.SessionID != sessionID || claims.UserID != in.UserID {
		return repository.ErrForbidden
	}
	// 2. session active + member joined 由 verifyMQTTBinding 内部保证（断言一次）。
	session, err := s.requireJoinedActiveSession(ctx, in.UserID, sessionID)
	if err != nil {
		return err
	}
	// 2.5 会话级弹幕开关：owner 关闭后所有成员均不可发送。
	if !session.DanmakuEnabled {
		return invalidArg("danmaku disabled")
	}
	// 3. 内容校验。
	content := strings.TrimSpace(in.Content)
	if content == "" {
		return invalidArg("content is required")
	}
	if utf8.RuneCountInString(content) > companionDanmakuMaxContentLength {
		return invalidArg(fmt.Sprintf("content exceeds %d characters", companionDanmakuMaxContentLength))
	}
	// 3.5 本地敏感词审核：命中即整条拒绝；不向客户端暴露具体词，仅服务端日志记录。
	if matched, hit := config.MatchSensitive(content); hit {
		log.Printf("companion danmaku rejected by sensitive word: session=%s user=%d matched=%q", sessionID, in.UserID, matched)
		return invalidArg("content contains sensitive content")
	}
	// 4. 单成员限速。
	since := time.Now().Add(-companionDanmakuRateLimitWindow)
	count, err := s.repo.CountDanmakuByMemberSince(ctx, sessionID, in.UserID, since)
	if err != nil {
		return err
	}
	if count >= companionDanmakuRateLimitMax {
		return invalidArg("danmaku rate limit exceeded")
	}
	// 4.5 session 级总量限速。
	sessionSince := time.Now().Add(-companionDanmakuSessionRateLimitWindow)
	sessionCount, err := s.repo.CountDanmakuBySessionSince(ctx, sessionID, sessionSince)
	if err != nil {
		return err
	}
	if sessionCount >= companionDanmakuSessionRateLimitMax {
		return invalidArg("session danmaku rate limit exceeded")
	}
	// 5. 持久化。
	now := time.Now()
	record := &models.CompanionDanmaku{
		SessionID: sessionID,
		UserID:    in.UserID,
		Content:   content,
		CreatedAt: now,
	}
	if err := s.repo.InsertDanmaku(ctx, record); err != nil {
		return err
	}
	// 6. 广播（best-effort）。
	user, err := s.users.FindByID(ctx, in.UserID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return err
	}
	if user == nil {
		user = &models.User{ID: in.UserID}
	}
	avatar := fallbackAvatarURL(user.ID, user.AvatarURL)
	if s.avatarCache != nil && shouldRewriteAvatarURL(avatar) {
		cacheCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if local := s.avatarCache.EnsureCached(cacheCtx, user.ID, formatUserAvatarCacheKey(user.ID), avatar); local != "" {
			avatar = local
		}
		cancel()
	}
	broadcast := CompanionDanmakuBroadcast{
		MessageID: record.ID,
		SessionID: session.SessionID,
		UserID:    in.UserID,
		Nickname:  user.Nickname,
		AvatarURL: avatar,
		Content:   content,
		CreatedAt: record.CreatedAt,
	}
	s.publishDanmakuBroadcast(ctx, session.SessionID, broadcast)
	return nil
}

// SetSessionDanmakuEnabled toggles danmaku on/off for a session.
//
//   - 仅会话 owner 可调用；
//   - 会话必须处于 active 状态；
//   - 状态等于目标值时幂等返回当前 state，不广播事件；
//   - 状态变更后通过 control topic 广播 `danmaku_toggled` 事件，所有订阅成员实时感知。
func (s *CompanionService) SetSessionDanmakuEnabled(ctx context.Context, operatorUserID int64, sessionID string, enabled bool) (*CompanionSessionState, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("companion service not configured")
	}
	if operatorUserID <= 0 {
		return nil, invalidArg("user_id is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, invalidArg("session_id is required")
	}
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.Status != models.CompanionSessionStatusActive {
		return nil, invalidArg("companion session already ended")
	}
	if session.OwnerUserID != operatorUserID {
		return nil, repository.ErrForbidden
	}
	if session.DanmakuEnabled == enabled {
		// 幂等：状态未变化，不更新、不广播。
		return s.buildSessionState(ctx, session, operatorUserID, true)
	}
	session.DanmakuEnabled = enabled
	if err := s.repo.UpdateSession(ctx, session); err != nil {
		return nil, err
	}
	now := time.Now()
	reason := "danmaku_disabled"
	if enabled {
		reason = "danmaku_enabled"
	}
	enabledCopy := enabled
	s.publishControlEvent(ctx, sessionID, CompanionControlEvent{
		Event:          CompanionControlEventDanmakuToggled,
		SessionID:      sessionID,
		OperatorUserID: operatorUserID,
		Reason:         reason,
		Enabled:        &enabledCopy,
		At:             now,
	})
	return s.buildSessionState(ctx, session, operatorUserID, true)
}

// CreateSession creates a new active companion session owned by the user.
func (s *CompanionService) CreateSession(ctx context.Context, ownerUserID int64, in CreateCompanionSessionInput) (*CompanionSessionState, error) {
	if s == nil || s.repo == nil || s.users == nil {
		return nil, errors.New("companion service not configured")
	}
	if ownerUserID <= 0 {
		return nil, invalidArg("user_id is required")
	}
	if _, err := s.users.FindByID(ctx, ownerUserID); err != nil {
		return nil, err
	}
	if err := s.ensureNoRunningTrack(ctx, ownerUserID); err != nil {
		return nil, err
	}
	if existing, err := s.repo.FindActiveSessionByUserID(ctx, ownerUserID); err == nil {
		title := strings.TrimSpace(existing.Title)
		if title == "" {
			title = defaultCompanionTitle
		}
		if existing.OwnerUserID == ownerUserID {
			return nil, invalidArg(fmt.Sprintf("you already have an active companion session: %s", title))
		}
		return nil, invalidArg(fmt.Sprintf("you already joined an active companion session: %s", title))
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}

	now := time.Now()
	sessionID, err := randomToken("sess_", 18)
	if err != nil {
		return nil, err
	}
	joinToken, err := randomToken("", 12)
	if err != nil {
		return nil, err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = defaultCompanionTitle
	}
	maxMembers := in.MaxMembers
	if maxMembers <= 0 {
		maxMembers = defaultCompanionMaxMembers
	}
	if maxMembers < 2 {
		return nil, invalidArg("max_members must be >= 2")
	}
	if maxMembers > maxCompanionMaxMembers {
		maxMembers = maxCompanionMaxMembers
	}

	visibility := models.CompanionSessionVisibility(strings.TrimSpace(in.Visibility))
	switch visibility {
	case "":
		visibility = models.CompanionSessionVisibilityPrivate
	case models.CompanionSessionVisibilityPrivate, models.CompanionSessionVisibilityPublic:
		// ok
	default:
		return nil, invalidArg("visibility must be private or public")
	}

	session := &models.CompanionSession{
		SessionID:      sessionID,
		OwnerUserID:    ownerUserID,
		Status:         models.CompanionSessionStatusActive,
		Visibility:     visibility,
		JoinToken:      joinToken,
		Title:          title,
		TrackType:      normalizeTrackTypeCode(in.TrackType),
		LocateAddr:     strings.TrimSpace(in.LocateAddr),
		MaxMembers:     maxMembers,
		DanmakuEnabled: true,
		StartedAt:      now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, err
	}
	if err := s.repo.UpsertMember(ctx, &models.CompanionSessionMember{
		SessionID:      session.SessionID,
		UserID:         ownerUserID,
		Role:           models.CompanionMemberRoleOwner,
		MemberStatus:   models.CompanionMemberStatusJoined,
		PresenceStatus: models.CompanionPresenceStatusOffline,
		JoinedAt:       now,
	}); err != nil {
		return nil, err
	}
	return s.buildSessionState(ctx, session, ownerUserID, true)
}

// JoinSession joins an existing active companion session.
//
// 入参 join_token / session_id 二选一：
//   - join_token：私密 / 公开房间均可使用；
//   - session_id：仅公开（visibility=public）房间可凭此加入；私密房间将返回 forbidden。
func (s *CompanionService) JoinSession(ctx context.Context, userID int64, in JoinCompanionSessionInput) (*CompanionSessionState, error) {
	if s == nil || s.repo == nil || s.users == nil {
		return nil, errors.New("companion service not configured")
	}
	if userID <= 0 {
		return nil, invalidArg("user_id is required")
	}
	joinToken := strings.TrimSpace(in.JoinToken)
	sessionIDInput := strings.TrimSpace(in.SessionID)
	if joinToken == "" && sessionIDInput == "" {
		return nil, invalidArg("join_token or session_id is required")
	}
	if _, err := s.users.FindByID(ctx, userID); err != nil {
		return nil, err
	}
	if err := s.ensureNoRunningTrack(ctx, userID); err != nil {
		return nil, err
	}
	var (
		session *models.CompanionSession
		err     error
	)
	if joinToken != "" {
		session, err = s.repo.FindSessionByJoinToken(ctx, joinToken)
	} else {
		session, err = s.repo.FindSessionByID(ctx, sessionIDInput)
	}
	if err != nil {
		return nil, err
	}
	if session.Status != models.CompanionSessionStatusActive {
		return nil, invalidArg("companion session already ended")
	}
	// 仅 session_id 入口需要校验可见性：私密房间不允许凭 session_id 加入。
	if joinToken == "" && session.Visibility != models.CompanionSessionVisibilityPublic {
		return nil, repository.ErrForbidden
	}
	if active, err := s.repo.FindActiveSessionByUserID(ctx, userID); err == nil {
		if active.SessionID == session.SessionID {
			return s.buildSessionState(ctx, session, userID, userID == session.OwnerUserID)
		}
		title := strings.TrimSpace(active.Title)
		if title == "" {
			title = defaultCompanionTitle
		}
		return nil, invalidArg(fmt.Sprintf("you already joined an active companion session: %s", title))
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	if member, err := s.repo.FindMember(ctx, session.SessionID, userID); err == nil {
		if member.MemberStatus == models.CompanionMemberStatusJoined {
			return s.buildSessionState(ctx, session, userID, userID == session.OwnerUserID)
		}
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	joinedCount, err := s.repo.CountMembersByStatus(ctx, session.SessionID, models.CompanionMemberStatusJoined)
	if err != nil {
		return nil, err
	}
	if session.MaxMembers > 0 && joinedCount >= int64(session.MaxMembers) {
		return nil, invalidArg("companion session is full")
	}
	now := time.Now()
	if err := s.repo.UpsertMember(ctx, &models.CompanionSessionMember{
		SessionID:      session.SessionID,
		UserID:         userID,
		Role:           models.CompanionMemberRoleMember,
		MemberStatus:   models.CompanionMemberStatusJoined,
		PresenceStatus: models.CompanionPresenceStatusOffline,
		JoinedAt:       now,
	}); err != nil {
		return nil, err
	}
	return s.buildSessionState(ctx, session, userID, false)
}

// PreviewSessionByJoinToken returns active room information for a join token without changing membership.
func (s *CompanionService) PreviewSessionByJoinToken(ctx context.Context, userID int64, joinToken string) (*CompanionSessionPreview, error) {
	if s == nil || s.repo == nil || s.users == nil {
		return nil, errors.New("companion service not configured")
	}
	if userID <= 0 {
		return nil, invalidArg("user_id is required")
	}
	joinToken = strings.TrimSpace(joinToken)
	if joinToken == "" {
		return nil, invalidArg("join_token is required")
	}
	if _, err := s.users.FindByID(ctx, userID); err != nil {
		return nil, err
	}
	session, err := s.repo.FindSessionByJoinToken(ctx, joinToken)
	if err != nil {
		return nil, err
	}
	if session.Status != models.CompanionSessionStatusActive {
		return nil, invalidArg("companion session already ended")
	}
	snapshot, err := s.buildSnapshot(ctx, session)
	if err != nil {
		return nil, err
	}
	// Preview is intentionally read-only and omits invitation and live position details.
	snapshot.JoinToken = ""
	snapshot.Positions = nil
	return &CompanionSessionPreview{
		Session:  companionSessionForResponse(session),
		Snapshot: snapshot,
	}, nil
}

func (s *CompanionService) ensureNoRunningTrack(ctx context.Context, userID int64) error {
	if s == nil || s.tracks == nil {
		return nil
	}
	track, err := s.tracks.FindRunningByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return err
	}
	return invalidArg(fmt.Sprintf("you already have a running track: %s", track.ID))
}

// GetCurrentSession returns the active companion session current user joined, if any.
func (s *CompanionService) GetCurrentSession(ctx context.Context, userID int64) (*CompanionSessionState, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("companion service not configured")
	}
	if userID <= 0 {
		return nil, invalidArg("user_id is required")
	}
	session, err := s.repo.FindActiveSessionByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.buildSessionState(ctx, session, userID, userID == session.OwnerUserID)
}

// ListHistory returns paged companion records that current user has participated in.
func (s *CompanionService) ListHistory(ctx context.Context, userID int64, input ListCompanionHistoryInput) (*models.CompanionHistoryPage, error) {
	if s == nil || s.repo == nil || s.users == nil {
		return nil, errors.New("companion service not configured")
	}
	if userID <= 0 {
		return nil, invalidArg("user_id is required")
	}
	limit := normalizeCompanionPageLimit(input.Limit)
	cursor, err := decodeCompanionSessionListCursor(input.Cursor)
	if err != nil {
		return nil, err
	}
	totalCount, err := s.repo.CountSessionsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	sessions, err := s.repo.ListSessionsByUserID(ctx, userID, cursor, limit+1)
	if err != nil {
		return nil, err
	}
	hasMore := len(sessions) > limit
	if hasMore {
		sessions = sessions[:limit]
	}
	items := make([]*models.CompanionHistoryItem, 0, len(sessions))
	for _, session := range sessions {
		item, err := s.buildHistoryItem(ctx, session)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	page := &models.CompanionHistoryPage{Items: items, TotalCount: totalCount, HasMore: hasMore}
	if hasMore && len(sessions) > 0 {
		nextCursor, err := encodeCompanionSessionListCursor(sessions[len(sessions)-1].StartedAt, sessions[len(sessions)-1].SessionID)
		if err != nil {
			return nil, err
		}
		page.NextCursor = nextCursor
	}
	return page, nil
}

// ListNearbySessions 返回客户端附近的 active 同行房间列表。
//
// 设计要点：
//   - 用户提供经纬度（WGS84）；
//   - 服务端取每个 active session 中 owner 的最新位置作为定位锚点；
//   - 用 Haversine 公式估算锚点与请求位置的球面距离，过滤超过半径的房间；
//   - 返回信息只包含距离米数 + 采样时间，不暴露房间锚点经纬度，避免反向定位；
//   - 不过滤已满 / 已加入的房间，由前端展示状态（已满灰态、已加入跳过）；
//   - 没有定位数据的 active 房间无法估算距离，跳过。
func (s *CompanionService) ListNearbySessions(ctx context.Context, userID int64, in ListCompanionNearbyInput) (*models.CompanionNearbyPage, error) {
	if s == nil || s.repo == nil || s.users == nil {
		return nil, errors.New("companion service not configured")
	}
	if userID <= 0 {
		return nil, invalidArg("user_id is required")
	}
	if in.Latitude < -90 || in.Latitude > 90 {
		return nil, invalidArg("latitude must be in [-90, 90]")
	}
	if in.Longitude < -180 || in.Longitude > 180 {
		return nil, invalidArg("longitude must be in [-180, 180]")
	}
	radius := in.RadiusMeters
	if radius <= 0 {
		radius = defaultCompanionNearbyRadiusMeters
	}
	if radius > maxCompanionNearbyRadiusMeters {
		radius = maxCompanionNearbyRadiusMeters
	}
	limit := in.Limit
	if limit <= 0 || limit > maxCompanionNearbyItems {
		limit = maxCompanionNearbyItems
	}
	// 拉取所有 active session（数量受 maxCompanionNearbyItems*4 限制，足以覆盖即使大半被半径过滤掉的场景）。
	sessions, err := s.repo.ListActiveSessions(ctx, maxCompanionNearbyItems*4)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		item     *models.CompanionNearbyItem
		distance float64
	}
	candidates := make([]candidate, 0, len(sessions))
	for _, session := range sessions {
		if session == nil {
			continue
		}
		// 只返回公开房间，避免向陌生用户暴露私密房间的 session_id / join_token。
		if session.Visibility != models.CompanionSessionVisibilityPublic {
			continue
		}
		positions, err := s.repo.ListPositions(ctx, session.SessionID)
		if err != nil {
			return nil, err
		}
		var ownerPos *models.CompanionLivePosition
		for _, position := range positions {
			if position == nil || position.UserID != session.OwnerUserID {
				continue
			}
			clone := *position
			ownerPos = &clone
			break
		}
		if ownerPos == nil {
			// owner 尚未上传过位置，无法估算距离，跳过。
			continue
		}
		distance := haversineMeters(in.Latitude, in.Longitude, ownerPos.Latitude, ownerPos.Longitude)
		if distance > radius {
			continue
		}
		members, err := s.repo.ListMembers(ctx, session.SessionID)
		if err != nil {
			return nil, err
		}
		nearbyMembers := make([]models.CompanionNearbyMember, 0, len(members))
		var memberCount int
		for _, member := range members {
			if member == nil || member.MemberStatus != models.CompanionMemberStatusJoined {
				continue
			}
			memberCount++
			user, err := s.users.FindByID(ctx, member.UserID)
			if err != nil {
				if !errors.Is(err, repository.ErrNotFound) {
					return nil, err
				}
				user = &models.User{ID: member.UserID}
			}
			avatar := fallbackAvatarURL(user.ID, user.AvatarURL)
			if s.avatarCache != nil && shouldRewriteAvatarURL(avatar) {
				cacheCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				if local := s.avatarCache.EnsureCached(cacheCtx, member.UserID, formatUserAvatarCacheKey(member.UserID), avatar); local != "" {
					avatar = local
				}
				cancel()
			}
			nearbyMembers = append(nearbyMembers, models.CompanionNearbyMember{
				UserID:    member.UserID,
				Role:      member.Role,
				Nickname:  user.Nickname,
				AvatarURL: avatar,
			})
		}
		// owner 优先排在前。
		sort.SliceStable(nearbyMembers, func(i, j int) bool {
			if nearbyMembers[i].Role != nearbyMembers[j].Role {
				return nearbyMembers[i].Role == models.CompanionMemberRoleOwner
			}
			return nearbyMembers[i].UserID < nearbyMembers[j].UserID
		})
		candidates = append(candidates, candidate{
			item: &models.CompanionNearbyItem{
				SessionID:              session.SessionID,
				Title:                  session.Title,
				TrackType:              session.TrackType,
				LocateAddr:             session.LocateAddr,
				JoinToken:              session.JoinToken,
				MaxMembers:             session.MaxMembers,
				MemberCount:            memberCount,
				TotalDistance:          session.TotalDistance,
				TotalDuration:          session.TotalDuration,
				TrackScreenshotURL:     s.rewriteCompanionScreenshotURL(ctx, userID, session),
				ActualParticipantCount: session.ActualParticipantCount,
				StartedAt:              session.StartedAt,
				ExpiresAt:              companionSessionExpiresAt(session),
				Anchor: &models.CompanionNearbyAnchor{
					DistanceM:  distance,
					RecordedAt: ownerPos.RecordedAt,
				},
				Members: nearbyMembers,
			},
			distance: distance,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].distance < candidates[j].distance
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	items := make([]*models.CompanionNearbyItem, 0, len(candidates))
	for _, c := range candidates {
		items = append(items, c.item)
	}
	return &models.CompanionNearbyPage{
		Items:    items,
		RadiusM:  radius,
		CenterAt: time.Now(),
	}, nil
}

// haversineMeters 计算两个经纬度（度）之间的球面距离（米）。
func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	rlat1 := lat1 * math.Pi / 180
	rlat2 := lat2 * math.Pi / 180
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rlat1)*math.Cos(rlat2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusMeters * c
}

// GetSnapshot returns the latest session snapshot for a joined member.
func (s *CompanionService) GetSnapshot(ctx context.Context, userID int64, sessionID string) (*models.CompanionSnapshot, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("companion service not configured")
	}
	session, err := s.requireJoinedActiveSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	return s.buildSnapshot(ctx, session)
}

// LeaveSession marks the current member as left. Owner can only leave when they are the last joined member.
func (s *CompanionService) LeaveSession(ctx context.Context, userID int64, sessionID string) error {
	if s == nil || s.repo == nil {
		return errors.New("companion service not configured")
	}
	session, err := s.requireJoinedActiveSession(ctx, userID, sessionID)
	if err != nil {
		return err
	}
	member, err := s.repo.FindMember(ctx, sessionID, userID)
	if err != nil {
		return err
	}
	joinedCount, err := s.repo.CountMembersByStatus(ctx, sessionID, models.CompanionMemberStatusJoined)
	if err != nil {
		return err
	}
	if member.Role == models.CompanionMemberRoleOwner && joinedCount > 1 {
		return invalidArg("owner cannot leave active session while other members are joined; end session instead")
	}
	now := time.Now()
	member.MemberStatus = models.CompanionMemberStatusLeft
	member.PresenceStatus = models.CompanionPresenceStatusOffline
	member.LeftAt = now
	member.LastSeenAt = now
	if err := s.repo.UpsertMember(ctx, member); err != nil {
		return err
	}
	s.publishControlEvent(ctx, sessionID, CompanionControlEvent{
		Event:          CompanionControlEventMemberLeft,
		SessionID:      sessionID,
		MemberUserID:   userID,
		OperatorUserID: userID,
		Reason:         "member_left",
		At:             now,
	})
	joinedCount, err = s.repo.CountMembersByStatus(ctx, sessionID, models.CompanionMemberStatusJoined)
	if err != nil {
		return err
	}
	if joinedCount == 0 {
		return s.endSessionInternalWithReason(ctx, session, 0, "all_members_left", models.CompanionSessionEndSourceMemberFlow, time.Now())
	}
	return nil
}

// KickSessionMember removes a joined member from an active session. Only the owner can kick members.
func (s *CompanionService) KickSessionMember(ctx context.Context, ownerUserID int64, sessionID string, targetUserID int64) (*CompanionSessionState, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("companion service not configured")
	}
	if ownerUserID <= 0 {
		return nil, invalidArg("user_id is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, invalidArg("session_id is required")
	}
	if targetUserID <= 0 {
		return nil, invalidArg("user_id is required")
	}
	if targetUserID == ownerUserID {
		return nil, invalidArg("owner cannot kick self")
	}
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.Status != models.CompanionSessionStatusActive {
		return nil, invalidArg("companion session already ended")
	}
	if session.OwnerUserID != ownerUserID {
		return nil, repository.ErrForbidden
	}
	if _, err := s.requireJoinedActiveSession(ctx, ownerUserID, sessionID); err != nil {
		return nil, err
	}
	member, err := s.repo.FindMember(ctx, sessionID, targetUserID)
	if err != nil {
		return nil, err
	}
	if member.Role == models.CompanionMemberRoleOwner {
		return nil, invalidArg("owner cannot kick self")
	}
	if member.MemberStatus != models.CompanionMemberStatusJoined {
		return nil, invalidArg("companion member is not joined")
	}
	now := time.Now()
	member.MemberStatus = models.CompanionMemberStatusKicked
	member.PresenceStatus = models.CompanionPresenceStatusOffline
	member.LeftAt = now
	member.LastSeenAt = now
	if err := s.repo.UpsertMember(ctx, member); err != nil {
		return nil, err
	}
	s.publishControlEvent(ctx, sessionID, CompanionControlEvent{
		Event:          CompanionControlEventMemberKicked,
		SessionID:      sessionID,
		MemberUserID:   targetUserID,
		OperatorUserID: ownerUserID,
		Reason:         "member_kicked",
		At:             now,
	})
	return s.buildSessionState(ctx, session, ownerUserID, true)
}

// EndSession ends an active session. Only owner can perform this operation.
func (s *CompanionService) EndSession(ctx context.Context, operatorUserID int64, sessionID string) error {
	if s == nil || s.repo == nil {
		return errors.New("companion service not configured")
	}
	if operatorUserID <= 0 {
		return invalidArg("user_id is required")
	}
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if session.Status != models.CompanionSessionStatusActive {
		return invalidArg("companion session already ended")
	}
	if session.OwnerUserID != operatorUserID {
		return repository.ErrForbidden
	}
	_, err = s.requireJoinedActiveSession(ctx, operatorUserID, sessionID)
	if err != nil {
		return err
	}
	return s.endSessionInternal(ctx, session, operatorUserID)
}

// UpdateSessionStats updates owner-provided summary data after a companion session ends.
func (s *CompanionService) UpdateSessionStats(ctx context.Context, ownerUserID int64, sessionID string, in UpdateCompanionSessionStatsInput) (*models.CompanionSession, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("companion service not configured")
	}
	if ownerUserID <= 0 {
		return nil, invalidArg("user_id is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, invalidArg("session_id is required")
	}
	if in.LocateAddr == nil && in.TotalDistance == nil && in.TotalDuration == nil && in.TrackScreenshotURL == nil && in.ActualParticipantCount == nil {
		return nil, invalidArg("nothing to update")
	}
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.OwnerUserID != ownerUserID {
		return nil, repository.ErrForbidden
	}
	if session.Status != models.CompanionSessionStatusEnded {
		return nil, invalidArg("companion session is not ended")
	}
	if in.LocateAddr != nil {
		session.LocateAddr = strings.TrimSpace(*in.LocateAddr)
	}
	if in.TotalDistance != nil {
		if *in.TotalDistance < 0 {
			return nil, invalidArg("total_distance must be >= 0")
		}
		session.TotalDistance = *in.TotalDistance
	}
	if in.TotalDuration != nil {
		if *in.TotalDuration < 0 {
			return nil, invalidArg("total_duration must be >= 0")
		}
		session.TotalDuration = *in.TotalDuration
	}
	if in.TrackScreenshotURL != nil {
		src := strings.TrimSpace(*in.TrackScreenshotURL)
		session.TrackScreenshotURL = src
		if s.screenshotCache != nil && src != "" {
			key := companionScreenshotCacheKey(session.SessionID)
			if err := s.screenshotCache.RemoveTempCached(key); err != nil {
				log.Printf("remove temp companion screenshot cache failed: session=%s err=%v", session.SessionID, err)
			}
			s.screenshotCache.PrefetchAsync(ownerUserID, key, src)
			session.TrackScreenshotURL = s.screenshotCache.GuessLocalURL(key, src)
		}
	}
	if in.ActualParticipantCount != nil {
		if *in.ActualParticipantCount < 0 {
			return nil, invalidArg("actual_participant_count must be >= 0")
		}
		session.ActualParticipantCount = *in.ActualParticipantCount
	}
	if err := s.repo.UpdateSession(ctx, session); err != nil {
		return nil, err
	}
	return companionSessionForResponse(session), nil
}

// CreateEvent records one owner-reported key event in a companion session.
func (s *CompanionService) CreateEvent(ctx context.Context, ownerUserID int64, sessionID string, in CreateCompanionEventInput) (*models.CompanionEvent, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("companion service not configured")
	}
	if ownerUserID <= 0 {
		return nil, invalidArg("user_id is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, invalidArg("session_id is required")
	}
	clientEventID := strings.TrimSpace(in.ClientEventID)
	if clientEventID == "" {
		return nil, invalidArg("client_event_id is required")
	}
	if len(clientEventID) > 128 {
		return nil, invalidArg("client_event_id is too long")
	}
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.OwnerUserID != ownerUserID {
		return nil, repository.ErrForbidden
	}
	if existing, err := s.repo.FindEventByClientEventID(ctx, sessionID, clientEventID); err == nil {
		return companionEventForResponse(existing), nil
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	eventType := strings.TrimSpace(in.EventType)
	if _, ok := companionEventTypes[eventType]; !ok {
		return nil, invalidArg("invalid event_type")
	}
	title := strings.TrimSpace(in.Title)
	if utf8.RuneCountInString(title) > companionEventMaxTitleRunes {
		return nil, invalidArg(fmt.Sprintf("title exceeds %d characters", companionEventMaxTitleRunes))
	}
	content := strings.TrimSpace(in.Content)
	if utf8.RuneCountInString(content) > companionEventMaxContentRunes {
		return nil, invalidArg(fmt.Sprintf("content exceeds %d characters", companionEventMaxContentRunes))
	}
	if in.TargetUserID < 0 {
		return nil, invalidArg("target_user_id must be >= 0")
	}
	if in.TargetUserID > 0 {
		if _, err := s.repo.FindMember(ctx, sessionID, in.TargetUserID); err != nil {
			return nil, err
		}
	}
	now := time.Now()
	eventTime := in.EventTime
	if eventTime.IsZero() {
		eventTime = now
	}
	if eventTime.Before(session.StartedAt.Add(-5 * time.Minute)) {
		return nil, invalidArg("event_time is too early")
	}
	if eventTime.After(now.Add(time.Minute)) {
		return nil, invalidArg("event_time is too late")
	}
	if session.Status == models.CompanionSessionStatusEnded && !session.EndedAt.IsZero() && eventTime.After(session.EndedAt.Add(5*time.Minute)) {
		return nil, invalidArg("event_time is too late")
	}
	metadataJSON, metadata, err := normalizeCompanionEventMetadata(in.Metadata)
	if err != nil {
		return nil, err
	}
	count, err := s.repo.CountEventsBySessionID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if count >= companionEventMaxPerSession {
		return nil, invalidArg("companion event limit exceeded")
	}
	event := &models.CompanionEvent{
		SessionID:     sessionID,
		OwnerUserID:   ownerUserID,
		EventType:     eventType,
		TargetUserID:  in.TargetUserID,
		Title:         title,
		Content:       content,
		EventTime:     eventTime,
		ClientEventID: clientEventID,
		Metadata:      metadata,
		MetadataJSON:  metadataJSON,
		CreatedAt:     now,
	}
	if err := s.repo.InsertEvent(ctx, event); err != nil {
		if errors.Is(err, repository.ErrAlreadyExists) {
			return s.repo.FindEventByClientEventID(ctx, sessionID, clientEventID)
		}
		return nil, err
	}
	return companionEventForResponse(event), nil
}

// ListEvents returns owner-visible key events in a companion session.
func (s *CompanionService) ListEvents(ctx context.Context, ownerUserID int64, sessionID string, in ListCompanionEventsInput) (*models.CompanionEventPage, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("companion service not configured")
	}
	if ownerUserID <= 0 {
		return nil, invalidArg("user_id is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, invalidArg("session_id is required")
	}
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.OwnerUserID != ownerUserID {
		return nil, repository.ErrForbidden
	}
	order := strings.ToLower(strings.TrimSpace(in.Order))
	if order == "" {
		order = "asc"
	}
	if order != "asc" && order != "desc" {
		return nil, invalidArg("invalid order")
	}
	ascending := order == "asc"
	limit := normalizeCompanionPageLimit(in.Limit)
	cursor, err := decodeCompanionEventCursor(in.Cursor)
	if err != nil {
		return nil, err
	}
	events, err := s.repo.ListEventsBySessionID(ctx, sessionID, cursor, limit+1, ascending)
	if err != nil {
		return nil, err
	}
	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}
	for i := range events {
		events[i] = companionEventForResponse(events[i])
	}
	page := &models.CompanionEventPage{Items: events, HasMore: hasMore}
	if hasMore && len(events) > 0 {
		last := events[len(events)-1]
		nextCursor, err := encodeCompanionEventCursor(last.EventTime, last.ID)
		if err != nil {
			return nil, err
		}
		page.NextCursor = nextCursor
	}
	return page, nil
}

// AutoCloseInactiveSessions ends active sessions that have no recent member activity
// or exceed the hard maximum duration for their track type.
func (s *CompanionService) AutoCloseInactiveSessions(ctx context.Context, now time.Time) (*CompanionAutoCloseResult, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("companion service not configured")
	}
	if now.IsZero() {
		now = time.Now()
	}
	sessions, err := s.repo.ListActiveSessions(ctx, companionAutoCloseScanLimit)
	if err != nil {
		return nil, err
	}
	result := &CompanionAutoCloseResult{Scanned: len(sessions)}
	for _, session := range sessions {
		if session == nil || session.Status != models.CompanionSessionStatusActive {
			continue
		}
		reason, shouldClose, err := s.shouldAutoCloseSession(ctx, session, now)
		if err != nil {
			return result, err
		}
		if !shouldClose {
			continue
		}
		if err := s.endSessionInternalWithReason(ctx, session, 0, reason, models.CompanionSessionEndSourceAutoClose, now); err != nil {
			var iae *InvalidArgumentError
			if errors.As(err, &iae) && strings.Contains(iae.Error(), "already ended") {
				continue
			}
			return result, err
		}
		result.Closed++
		log.Printf("companion auto close: session=%s reason=%s track_type=%s", session.SessionID, reason, session.TrackType)
	}
	return result, nil
}

func (s *CompanionService) shouldAutoCloseSession(ctx context.Context, session *models.CompanionSession, now time.Time) (string, bool, error) {
	rule := companionAutoCloseRuleForTrackType(session.TrackType)
	if !session.StartedAt.IsZero() && now.Sub(session.StartedAt) >= rule.MaxDuration {
		return "max_duration_exceeded", true, nil
	}
	members, err := s.repo.ListMembers(ctx, session.SessionID)
	if err != nil {
		return "", false, err
	}
	joinedMembers := make([]*models.CompanionSessionMember, 0, len(members))
	for _, member := range members {
		if member != nil && member.MemberStatus == models.CompanionMemberStatusJoined {
			joinedMembers = append(joinedMembers, member)
		}
	}
	if len(joinedMembers) == 0 {
		return "all_members_left", true, nil
	}
	positions, err := s.repo.ListPositions(ctx, session.SessionID)
	if err != nil {
		return "", false, err
	}
	positionByUser := make(map[int64]*models.CompanionLivePosition, len(positions))
	for _, position := range positions {
		if position == nil {
			continue
		}
		prev := positionByUser[position.UserID]
		if prev == nil || position.RecordedAt.After(prev.RecordedAt) {
			positionByUser[position.UserID] = position
		}
	}
	for _, member := range joinedMembers {
		lastActivity := companionMemberLastActivity(member, positionByUser[member.UserID])
		if lastActivity.IsZero() || now.Sub(lastActivity) < rule.InactiveTimeout {
			return "", false, nil
		}
	}
	return "inactive_timeout", true, nil
}

func companionMemberLastActivity(member *models.CompanionSessionMember, position *models.CompanionLivePosition) time.Time {
	if member == nil {
		return time.Time{}
	}
	last := member.JoinedAt
	if member.LastSeenAt.After(last) {
		last = member.LastSeenAt
	}
	if position != nil && position.RecordedAt.After(last) {
		last = position.RecordedAt
	}
	return last
}

func companionAutoCloseRuleForTrackType(trackType string) companionAutoCloseRule {
	if rule, ok := companionAutoCloseRules[normalizeTrackTypeCode(trackType)]; ok {
		return rule
	}
	return defaultCompanionAutoCloseRule
}

// UpdatePresence updates member online/offline status, reserved for future EMQX integration.
func (s *CompanionService) UpdatePresence(ctx context.Context, sessionID string, userID int64, status models.CompanionPresenceStatus, lastSeenAt time.Time) error {
	if status != models.CompanionPresenceStatusOnline && status != models.CompanionPresenceStatusOffline {
		return invalidArg("invalid presence_status")
	}
	_, err := s.requireJoinedActiveSession(ctx, userID, sessionID)
	if err != nil {
		return err
	}
	member, err := s.repo.FindMember(ctx, sessionID, userID)
	if err != nil {
		return err
	}
	member.PresenceStatus = status
	if !lastSeenAt.IsZero() {
		member.LastSeenAt = lastSeenAt
	} else {
		member.LastSeenAt = time.Now()
	}
	return s.repo.UpsertMember(ctx, member)
}

// UpsertPositionSnapshot upserts member latest live position, reserved for future EMQX integration.
func (s *CompanionService) UpsertPositionSnapshot(ctx context.Context, sessionID string, userID int64, in CompanionPositionUpsertInput) error {
	_, err := s.requireJoinedActiveSession(ctx, userID, sessionID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(in.CoordinateSystem) == "" {
		return invalidArg("coordinate_system is required")
	}
	if in.RecordedAt.IsZero() {
		in.RecordedAt = time.Now()
	}
	if in.Source == "" {
		in.Source = "http"
	}
	if in.Seq < 0 {
		return invalidArg("seq must be >= 0")
	}
	return s.repo.UpsertPosition(ctx, &models.CompanionLivePosition{
		SessionID:        sessionID,
		UserID:           userID,
		TrackID:          strings.TrimSpace(in.TrackID),
		Latitude:         in.Latitude,
		Longitude:        in.Longitude,
		CoordinateSystem: strings.TrimSpace(in.CoordinateSystem),
		SpeedKmh:         in.SpeedKmh,
		Heading:          in.Heading,
		AccuracyM:        in.AccuracyM,
		Altitude:         in.Altitude,
		RecordedAt:       in.RecordedAt,
		Seq:              in.Seq,
		Source:           in.Source,
	})
}

func (s *CompanionService) requireJoinedActiveSession(ctx context.Context, userID int64, sessionID string) (*models.CompanionSession, error) {
	if userID <= 0 {
		return nil, invalidArg("user_id is required")
	}
	if strings.TrimSpace(sessionID) == "" {
		return nil, invalidArg("session_id is required")
	}
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.Status != models.CompanionSessionStatusActive {
		return nil, invalidArg("companion session already ended")
	}
	member, err := s.repo.FindMember(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	if member.MemberStatus != models.CompanionMemberStatusJoined {
		return nil, repository.ErrForbidden
	}
	return session, nil
}

func (s *CompanionService) buildSessionState(ctx context.Context, session *models.CompanionSession, userID int64, includeJoin bool) (*CompanionSessionState, error) {
	if session == nil {
		return nil, repository.ErrNotFound
	}
	snapshot, err := s.buildSnapshot(ctx, session)
	if err != nil {
		return nil, err
	}
	state := &CompanionSessionState{Session: companionSessionForResponse(session), Snapshot: snapshot}
	if includeJoin {
		state.Join = &CompanionJoinInfo{JoinToken: session.JoinToken}
	}
	_ = userID
	return state, nil
}

func companionSessionForResponse(session *models.CompanionSession) *models.CompanionSession {
	if session == nil {
		return nil
	}
	item := *session
	item.ExpiresAt = companionSessionExpiresAt(session)
	return &item
}

func companionSessionExpiresAt(session *models.CompanionSession) time.Time {
	if session == nil || session.StartedAt.IsZero() {
		return time.Time{}
	}
	rule := companionAutoCloseRuleForTrackType(session.TrackType)
	return session.StartedAt.Add(rule.MaxDuration)
}

func companionScreenshotCacheKey(sessionID string) string {
	return "companion_" + strings.TrimSpace(sessionID)
}

func (s *CompanionService) rewriteCompanionScreenshotURL(ctx context.Context, userID int64, session *models.CompanionSession) string {
	if session == nil || strings.TrimSpace(session.TrackScreenshotURL) == "" {
		return ""
	}
	src := strings.TrimSpace(session.TrackScreenshotURL)
	if strings.HasPrefix(src, "/api/v1/static/") || s.screenshotCache == nil {
		return src
	}
	key := companionScreenshotCacheKey(session.SessionID)
	cacheCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if local := s.screenshotCache.EnsureCached(cacheCtx, userID, key, src); local != "" {
		return local
	}
	return s.screenshotCache.GuessLocalURL(key, src)
}

func (s *CompanionService) buildHistoryItem(ctx context.Context, session *models.CompanionSession) (*models.CompanionHistoryItem, error) {
	if session == nil {
		return nil, repository.ErrNotFound
	}
	members, err := s.repo.ListMembers(ctx, session.SessionID)
	if err != nil {
		return nil, err
	}
	participants := make([]models.CompanionHistoryParticipant, 0, len(members))
	var participantCount int64
	for _, member := range members {
		if member == nil {
			continue
		}
		include := session.Status == models.CompanionSessionStatusEnded || member.MemberStatus == models.CompanionMemberStatusJoined
		if !include {
			continue
		}
		participantCount++
		user, err := s.users.FindByID(ctx, member.UserID)
		if err != nil {
			if !errors.Is(err, repository.ErrNotFound) {
				return nil, err
			}
			user = &models.User{ID: member.UserID}
		}
		participants = append(participants, models.CompanionHistoryParticipant{
			UserID:   member.UserID,
			Nickname: user.Nickname,
			AvatarURL: func() string {
				avatar := fallbackAvatarURL(user.ID, user.AvatarURL)
				if s.avatarCache != nil && shouldRewriteAvatarURL(avatar) {
					cacheCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
					defer cancel()
					if local := s.avatarCache.EnsureCached(cacheCtx, member.UserID, formatUserAvatarCacheKey(member.UserID), avatar); local != "" {
						avatar = local
					}
				}
				return avatar
			}(),
		})
	}
	clone := *session
	endAt := clone.EndedAt
	if endAt.IsZero() {
		endAt = time.Now()
	}
	var durationSeconds int64
	if !clone.StartedAt.IsZero() && endAt.After(clone.StartedAt) {
		durationSeconds = int64(endAt.Sub(clone.StartedAt) / time.Second)
	}
	item := &models.CompanionHistoryItem{
		SessionID:              clone.SessionID,
		Title:                  clone.Title,
		TrackType:              clone.TrackType,
		LocateAddr:             clone.LocateAddr,
		ParticipantCount:       participantCount,
		StartedAt:              clone.StartedAt,
		DurationSeconds:        durationSeconds,
		TotalDistance:          clone.TotalDistance,
		TotalDuration:          clone.TotalDuration,
		TrackScreenshotURL:     s.rewriteCompanionScreenshotURL(ctx, clone.OwnerUserID, &clone),
		ActualParticipantCount: clone.ActualParticipantCount,
		Status:                 clone.Status,
		Participants:           participants,
	}
	if clone.Status == models.CompanionSessionStatusActive {
		item.JoinToken = clone.JoinToken
	}
	return item, nil
}

func (s *CompanionService) requireMQTTConfigured() error {
	if s == nil || s.repo == nil || s.users == nil {
		return errors.New("companion service not configured")
	}
	if strings.TrimSpace(s.mqtt.CredentialSecret) == "" {
		return errors.New("companion mqtt not configured")
	}
	return nil
}

func (s *CompanionService) buildMQTTTopicBindings(sessionID string, userID int64) CompanionMQTTTopicBindings {
	return CompanionMQTTTopicBindings{
		LocationPublish:   s.memberLocationTopic(sessionID, userID),
		LocationSubscribe: s.sessionLocationWildcard(sessionID),
		PresencePublish:   s.memberPresenceTopic(sessionID, userID),
		PresenceSubscribe: s.sessionPresenceWildcard(sessionID),
		ControlSubscribe:  s.controlTopic(sessionID),
		DanmakuPublish:    s.memberDanmakuUplinkTopic(sessionID, userID),
		DanmakuSubscribe:  s.sessionDanmakuBroadcastTopic(sessionID),
	}
}

func (s *CompanionService) verifyMQTTBinding(ctx context.Context, principal, clientID, password string, verifyPassword bool) (*companionMQTTPrincipalClaims, *models.CompanionSessionMember, error) {
	if err := s.requireMQTTConfigured(); err != nil {
		return nil, nil, err
	}
	trimmedPrincipal := strings.TrimSpace(principal)
	trimmedClientID := strings.TrimSpace(clientID)
	claims, err := parseCompanionMQTTPrincipal(trimmedPrincipal)
	if err != nil {
		return nil, nil, err
	}
	if time.Now().After(claims.ExpiresAt) {
		return nil, nil, errors.New("mqtt credential expired")
	}
	session, err := s.repo.FindSessionByID(ctx, claims.SessionID)
	if err != nil {
		return nil, nil, err
	}
	if session.Status != models.CompanionSessionStatusActive {
		return nil, nil, invalidArg("companion session already ended")
	}
	member, err := s.repo.FindMember(ctx, claims.SessionID, claims.UserID)
	if err != nil {
		return nil, nil, err
	}
	if member.MemberStatus != models.CompanionMemberStatusJoined {
		return nil, nil, repository.ErrForbidden
	}
	if member.MQTTPrincipal != trimmedPrincipal || member.MQTTClientID != trimmedClientID {
		return nil, nil, repository.ErrForbidden
	}
	if verifyPassword {
		expected := s.signMQTTCredentials(trimmedPrincipal, trimmedClientID)
		if !hmac.Equal([]byte(expected), []byte(password)) {
			return nil, nil, repository.ErrForbidden
		}
	}
	return claims, member, nil
}

func (s *CompanionService) signMQTTCredentials(principal, clientID string) string {
	mac := hmac.New(sha256.New, []byte(s.mqtt.CredentialSecret))
	_, _ = mac.Write([]byte(principal))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write([]byte(clientID))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func normalizeCompanionPageLimit(limit int) int {
	if limit <= 0 {
		return defaultCompanionPageSize
	}
	if limit > maxCompanionPageSize {
		return maxCompanionPageSize
	}
	return limit
}

func decodeCompanionSessionListCursor(raw string) (*models.CompanionSessionListCursor, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	buf, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, invalidArg("invalid cursor")
	}
	var cursor models.CompanionSessionListCursor
	if err := json.Unmarshal(buf, &cursor); err != nil {
		return nil, invalidArg("invalid cursor")
	}
	if cursor.SessionID == "" || cursor.StartedAt.IsZero() {
		return nil, invalidArg("invalid cursor")
	}
	return &cursor, nil
}

func encodeCompanionSessionListCursor(startedAt time.Time, sessionID string) (string, error) {
	buf, err := json.Marshal(models.CompanionSessionListCursor{StartedAt: startedAt, SessionID: sessionID})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func decodeCompanionEventCursor(raw string) (*models.CompanionEventCursor, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	buf, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, invalidArg("invalid cursor")
	}
	var cursor models.CompanionEventCursor
	if err := json.Unmarshal(buf, &cursor); err != nil {
		return nil, invalidArg("invalid cursor")
	}
	if cursor.ID <= 0 || cursor.EventTime.IsZero() {
		return nil, invalidArg("invalid cursor")
	}
	return &cursor, nil
}

func encodeCompanionEventCursor(eventTime time.Time, id int64) (string, error) {
	buf, err := json.Marshal(models.CompanionEventCursor{EventTime: eventTime, ID: id})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func normalizeCompanionEventMetadata(raw json.RawMessage) (string, json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "", nil, nil
	}
	if len(trimmed) > companionEventMaxMetadataBytes {
		return "", nil, invalidArg(fmt.Sprintf("metadata exceeds %d bytes", companionEventMaxMetadataBytes))
	}
	if !json.Valid(trimmed) {
		return "", nil, invalidArg("invalid metadata")
	}
	if !bytes.Equal(trimmed, []byte("null")) && trimmed[0] != '{' {
		return "", nil, invalidArg("metadata must be an object")
	}
	if bytes.Equal(trimmed, []byte("null")) {
		return "", nil, nil
	}
	clone := append([]byte(nil), trimmed...)
	return string(clone), clone, nil
}

func companionEventForResponse(event *models.CompanionEvent) *models.CompanionEvent {
	if event == nil {
		return nil
	}
	clone := *event
	if len(clone.Metadata) == 0 && clone.MetadataJSON != "" {
		clone.Metadata = []byte(clone.MetadataJSON)
	}
	if clone.Metadata != nil {
		clone.Metadata = append([]byte(nil), clone.Metadata...)
	}
	return &clone
}

func parseCompanionMQTTPrincipal(principal string) (*companionMQTTPrincipalClaims, error) {
	parts := strings.Split(strings.TrimSpace(principal), ":")
	if len(parts) != 5 || parts[0] != companionMQTTPrincipalV1 {
		return nil, errors.New("invalid mqtt principal")
	}
	userID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || userID <= 0 {
		return nil, errors.New("invalid mqtt principal user_id")
	}
	expUnix, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || expUnix <= 0 {
		return nil, errors.New("invalid mqtt principal expires_at")
	}
	if strings.TrimSpace(parts[1]) == "" || strings.TrimSpace(parts[4]) == "" {
		return nil, errors.New("invalid mqtt principal")
	}
	return &companionMQTTPrincipalClaims{
		SessionID: parts[1],
		UserID:    userID,
		ExpiresAt: time.Unix(expUnix, 0).UTC(),
		Nonce:     parts[4],
	}, nil
}

func (s *CompanionService) mqttTopicPrefix() string {
	prefix := strings.Trim(strings.TrimSpace(s.mqtt.TopicPrefix), "/")
	if prefix == "" {
		return defaultCompanionMQTTTopicRoot
	}
	return prefix
}

func (s *CompanionService) memberLocationTopic(sessionID string, userID int64) string {
	return fmt.Sprintf("%s/%s/member/%d/location", s.mqttTopicPrefix(), sessionID, userID)
}

func (s *CompanionService) memberPresenceTopic(sessionID string, userID int64) string {
	return fmt.Sprintf("%s/%s/member/%d/presence", s.mqttTopicPrefix(), sessionID, userID)
}

func (s *CompanionService) sessionLocationWildcard(sessionID string) string {
	return fmt.Sprintf("%s/%s/member/+/location", s.mqttTopicPrefix(), sessionID)
}

func (s *CompanionService) sessionPresenceWildcard(sessionID string) string {
	return fmt.Sprintf("%s/%s/member/+/presence", s.mqttTopicPrefix(), sessionID)
}

func (s *CompanionService) controlTopic(sessionID string) string {
	return fmt.Sprintf("%s/%s/control", s.mqttTopicPrefix(), sessionID)
}

// memberDanmakuUplinkTopic 是单个成员上行弹幕的 topic（client publish only）。
//
// 客户端流程：发出弹幕时 publish 到该 topic，由 EMQX Rule Engine 触发服务端 ingest。
func (s *CompanionService) memberDanmakuUplinkTopic(sessionID string, userID int64) string {
	return fmt.Sprintf("%s/%s/member/%d/danmaku", s.mqttTopicPrefix(), sessionID, userID)
}

// sessionDanmakuBroadcastTopic 是会话级弹幕广播 topic（client subscribe only，服务端 publish）。
//
// 客户端流程：subscribe 该 topic 接收所有人的弹幕（包括自己——用于发送成功确认）。
// 客户端禁止 publish 到该 topic。
func (s *CompanionService) sessionDanmakuBroadcastTopic(sessionID string) string {
	return fmt.Sprintf("%s/%s/danmaku", s.mqttTopicPrefix(), sessionID)
}

func (s *CompanionService) shouldIgnoreIncomingPosition(ctx context.Context, sessionID string, userID int64, seq int64, recordedAt time.Time) bool {
	if strings.TrimSpace(sessionID) == "" || userID <= 0 {
		return false
	}
	positions, err := s.repo.ListPositions(ctx, sessionID)
	if err != nil {
		return false
	}
	for _, existing := range positions {
		if existing == nil || existing.UserID != userID {
			continue
		}
		if seq > 0 || existing.Seq > 0 {
			if seq < existing.Seq {
				return true
			}
			if seq == existing.Seq && (!recordedAt.After(existing.RecordedAt) || recordedAt.IsZero()) {
				return true
			}
			return false
		}
		if recordedAt.IsZero() {
			return !existing.RecordedAt.IsZero()
		}
		return !recordedAt.After(existing.RecordedAt)
	}
	return false
}

func (s *CompanionService) buildSnapshot(ctx context.Context, session *models.CompanionSession) (*models.CompanionSnapshot, error) {
	members, err := s.repo.ListMembers(ctx, session.SessionID)
	if err != nil {
		return nil, err
	}
	positions, err := s.repo.ListPositions(ctx, session.SessionID)
	if err != nil {
		return nil, err
	}
	joinedUserSet := make(map[int64]struct{}, len(members))
	memberSnapshots := make([]models.CompanionMemberSnapshot, 0, len(members))
	for _, member := range members {
		if member == nil || member.MemberStatus != models.CompanionMemberStatusJoined {
			continue
		}
		joinedUserSet[member.UserID] = struct{}{}
		user, err := s.users.FindByID(ctx, member.UserID)
		if err != nil {
			if !errors.Is(err, repository.ErrNotFound) {
				return nil, err
			}
			user = &models.User{ID: member.UserID}
		}
		memberSnapshots = append(memberSnapshots, models.CompanionMemberSnapshot{
			UserID:         member.UserID,
			Role:           member.Role,
			MemberStatus:   member.MemberStatus,
			PresenceStatus: member.PresenceStatus,
			Nickname:       user.Nickname,
			AvatarURL: func() string {
				avatar := fallbackAvatarURL(user.ID, user.AvatarURL)
				if s.avatarCache != nil && shouldRewriteAvatarURL(avatar) {
					cacheCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
					defer cancel()
					if local := s.avatarCache.EnsureCached(cacheCtx, member.UserID, formatUserAvatarCacheKey(member.UserID), avatar); local != "" {
						avatar = local
					}
				}
				return avatar
			}(),
			JoinedAt:   member.JoinedAt,
			LastSeenAt: member.LastSeenAt,
		})
	}
	visiblePositions := make([]*models.CompanionLivePosition, 0, len(positions))
	for _, position := range positions {
		if position == nil {
			continue
		}
		if _, ok := joinedUserSet[position.UserID]; !ok {
			continue
		}
		clone := *position
		visiblePositions = append(visiblePositions, &clone)
	}
	snapshot := &models.CompanionSnapshot{
		SnapshotAt: time.Now(),
		Members:    memberSnapshots,
		Positions:  visiblePositions,
	}
	if session.Status == models.CompanionSessionStatusActive {
		snapshot.JoinToken = session.JoinToken
	}
	return snapshot, nil
}

func (s *CompanionService) endSessionInternal(ctx context.Context, session *models.CompanionSession, operatorUserID int64) error {
	reason := "all_members_left"
	source := models.CompanionSessionEndSourceMemberFlow
	if operatorUserID > 0 {
		reason = "owner_ended"
		source = models.CompanionSessionEndSourceOwner
	}
	return s.endSessionInternalWithReason(ctx, session, operatorUserID, reason, source, time.Now())
}

func (s *CompanionService) endSessionInternalWithReason(ctx context.Context, session *models.CompanionSession, operatorUserID int64, reason string, source models.CompanionSessionEndSource, now time.Time) error {
	if now.IsZero() {
		now = time.Now()
	}
	if strings.TrimSpace(reason) == "" {
		reason = "all_members_left"
	}
	if strings.TrimSpace(string(source)) == "" {
		source = models.CompanionSessionEndSourceMemberFlow
	}
	session.Status = models.CompanionSessionStatusEnded
	session.EndedAt = now
	session.EndReason = reason
	session.EndSource = source
	session.EndOperatorUserID = operatorUserID
	session.UpdatedAt = now
	if err := s.repo.UpdateSession(ctx, session); err != nil {
		return err
	}
	members, err := s.repo.ListMembers(ctx, session.SessionID)
	if err != nil {
		return err
	}
	for _, member := range members {
		if member == nil {
			continue
		}
		if member.MemberStatus == models.CompanionMemberStatusEnded {
			continue
		}
		member.MemberStatus = models.CompanionMemberStatusEnded
		member.PresenceStatus = models.CompanionPresenceStatusOffline
		if member.LeftAt.IsZero() {
			member.LeftAt = now
		}
		member.LastSeenAt = now
		if err := s.repo.UpsertMember(ctx, member); err != nil {
			return err
		}
	}
	s.publishControlEvent(ctx, session.SessionID, CompanionControlEvent{
		Event:          CompanionControlEventSessionEnded,
		SessionID:      session.SessionID,
		OperatorUserID: operatorUserID,
		Reason:         reason,
		At:             now,
	})
	return nil
}

func (s *CompanionService) publishControlEvent(ctx context.Context, sessionID string, event CompanionControlEvent) {
	if s == nil || s.publisher == nil {
		return
	}
	payload, err := json.Marshal(event)
	if err != nil {
		log.Printf("companion control event marshal failed for session %s: %v", sessionID, err)
		return
	}
	publishCtx, cancel := context.WithTimeout(ctx, defaultCompanionMQTTPublishTTL)
	defer cancel()
	if err := s.publisher.Publish(publishCtx, s.controlTopic(sessionID), payload); err != nil {
		log.Printf("companion control event publish failed for session %s topic %s: %v", sessionID, s.controlTopic(sessionID), err)
	}
}

// publishDanmakuBroadcast 通过 best-effort 将弹幕广播到 sessionDanmakuBroadcastTopic。
//
// publisher 选择优先级：
//  1. SetDanmakuPublisher 注入的独立发布器；
//  2. 回退到 SetControlPublisher 注入的发布器；
//  3. 都未配置则只记日志，不阻塞 ingest 主流程（客户端会在超时后展示失败）。
func (s *CompanionService) publishDanmakuBroadcast(ctx context.Context, sessionID string, event CompanionDanmakuBroadcast) {
	if s == nil {
		return
	}
	publisher := s.danmakuPublisher
	if publisher == nil {
		publisher = s.publisher
	}
	if publisher == nil {
		log.Printf("companion danmaku publisher not configured, drop broadcast for session %s", sessionID)
		return
	}
	payload, err := json.Marshal(event)
	if err != nil {
		log.Printf("companion danmaku marshal failed for session %s: %v", sessionID, err)
		return
	}
	topic := s.sessionDanmakuBroadcastTopic(sessionID)
	publishCtx, cancel := context.WithTimeout(ctx, defaultCompanionMQTTPublishTTL)
	defer cancel()
	if err := publisher.Publish(publishCtx, topic, payload); err != nil {
		log.Printf("companion danmaku publish failed for session %s topic %s: %v", sessionID, topic, err)
	}
}

func normalizeCompanionMQTTBrokerURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(raw, "mqtt://"):
		return "tcp://" + strings.TrimPrefix(raw, "mqtt://")
	case strings.HasPrefix(raw, "mqtts://"):
		return "ssl://" + strings.TrimPrefix(raw, "mqtts://")
	default:
		return raw
	}
}

func randomToken(prefix string, rawBytes int) (string, error) {
	buf := make([]byte, rawBytes)
	if _, err := crand.Read(buf); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf), nil
}
