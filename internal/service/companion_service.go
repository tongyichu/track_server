package service

import (
	"context"
	"crypto/hmac"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

const (
	defaultCompanionTitle          = "与友同行"
	defaultCompanionMaxMembers     = 8
	maxCompanionMaxMembers         = 32
	defaultCompanionPageSize       = 20
	maxCompanionPageSize           = 50
	defaultCompanionJoinTokenTTL   = 2 * time.Hour
	defaultCompanionMQTTTopicRoot  = "companion"
	defaultCompanionMQTTTTL        = time.Hour
	defaultCompanionMQTTPublishTTL = 5 * time.Second
	companionMQTTPrincipalV1       = "cmpv1"
)

// CompanionService 实现“同行”控制面的业务逻辑。
type CompanionService struct {
	repo      repository.CompanionRepository
	users     repository.UserRepository
	mqtt      CompanionMQTTOptions
	publisher CompanionControlPublisher
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
	Event          string    `json:"event"`
	SessionID      string    `json:"session_id"`
	MemberUserID   int64     `json:"member_user_id,omitempty"`
	OperatorUserID int64     `json:"operator_user_id,omitempty"`
	Reason         string    `json:"reason,omitempty"`
	At             time.Time `json:"at"`
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

// CreateCompanionSessionInput describes the payload to create a companion session.
type CreateCompanionSessionInput struct {
	Title      string `json:"title"`
	MaxMembers int    `json:"max_members"`
}

// JoinCompanionSessionInput describes the payload to join a companion session.
type JoinCompanionSessionInput struct {
	JoinToken string `json:"join_token"`
}

// CompanionJoinInfo is the owner-only invitation info returned by control plane APIs.
type CompanionJoinInfo struct {
	JoinToken         string    `json:"join_token"`
	JoinTokenExpireAt time.Time `json:"join_token_expire_at"`
}

// CompanionSessionState is the standard control-plane response envelope.
type CompanionSessionState struct {
	Session  *models.CompanionSession  `json:"session"`
	Join     *CompanionJoinInfo        `json:"join,omitempty"`
	Snapshot *models.CompanionSnapshot `json:"snapshot"`
}

// ListCompanionHistoryInput describes paging input for companion history list.
type ListCompanionHistoryInput struct {
	Cursor string
	Limit  int
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

type companionMQTTPrincipalClaims struct {
	SessionID string
	UserID    int64
	ExpiresAt time.Time
	Nonce     string
}

const (
	CompanionControlEventMemberLeft   = "member_left"
	CompanionControlEventSessionEnded = "session_ended"
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
		if topic == s.memberLocationTopic(claims.SessionID, claims.UserID) || topic == s.memberPresenceTopic(claims.SessionID, claims.UserID) {
			return CompanionMQTTACLResult{Result: "allow"}
		}
	case "subscribe", "sub":
		if topic == s.sessionLocationWildcard(claims.SessionID) || topic == s.sessionPresenceWildcard(claims.SessionID) || topic == s.controlTopic(claims.SessionID) || topic == s.memberLocationTopic(claims.SessionID, claims.UserID) || topic == s.memberPresenceTopic(claims.SessionID, claims.UserID) {
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
	joinToken, err := randomToken("join_", 18)
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

	session := &models.CompanionSession{
		SessionID:         sessionID,
		OwnerUserID:       ownerUserID,
		Status:            models.CompanionSessionStatusActive,
		JoinToken:         joinToken,
		JoinTokenExpireAt: now.Add(defaultCompanionJoinTokenTTL),
		Title:             title,
		MaxMembers:        maxMembers,
		StartedAt:         now,
		CreatedAt:         now,
		UpdatedAt:         now,
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

// JoinSession joins an existing active companion session using join token.
func (s *CompanionService) JoinSession(ctx context.Context, userID int64, in JoinCompanionSessionInput) (*CompanionSessionState, error) {
	if s == nil || s.repo == nil || s.users == nil {
		return nil, errors.New("companion service not configured")
	}
	if userID <= 0 {
		return nil, invalidArg("user_id is required")
	}
	if strings.TrimSpace(in.JoinToken) == "" {
		return nil, invalidArg("join_token is required")
	}
	if _, err := s.users.FindByID(ctx, userID); err != nil {
		return nil, err
	}
	session, err := s.repo.FindSessionByJoinToken(ctx, strings.TrimSpace(in.JoinToken))
	if err != nil {
		return nil, err
	}
	if session.Status != models.CompanionSessionStatusActive {
		return nil, invalidArg("companion session already ended")
	}
	if !session.JoinTokenExpireAt.IsZero() && time.Now().After(session.JoinTokenExpireAt) {
		return nil, invalidArg("join_token expired")
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
		return s.endSessionInternal(ctx, session, 0)
	}
	return nil
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
	state := &CompanionSessionState{Session: session, Snapshot: snapshot}
	if includeJoin {
		state.Join = &CompanionJoinInfo{JoinToken: session.JoinToken, JoinTokenExpireAt: session.JoinTokenExpireAt}
	}
	_ = userID
	return state, nil
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
			UserID:    member.UserID,
			Nickname:  user.Nickname,
			AvatarURL: fallbackAvatarURL(user.ID, user.AvatarURL),
		})
	}
	clone := *session
	return &models.CompanionHistoryItem{
		SessionID:        clone.SessionID,
		Title:            clone.Title,
		ParticipantCount: participantCount,
		StartedAt:        clone.StartedAt,
		Status:           clone.Status,
		Participants:     participants,
	}, nil
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
	}
}

func (s *CompanionService) verifyMQTTBinding(ctx context.Context, principal, clientID, password string, verifyPassword bool) (*companionMQTTPrincipalClaims, *models.CompanionSessionMember, error) {
	if err := s.requireMQTTConfigured(); err != nil {
		return nil, nil, err
	}
	claims, err := parseCompanionMQTTPrincipal(strings.TrimSpace(principal))
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
	if member.MQTTPrincipal != strings.TrimSpace(principal) || member.MQTTClientID != strings.TrimSpace(clientID) {
		return nil, nil, repository.ErrForbidden
	}
	if verifyPassword {
		expected := s.signMQTTCredentials(strings.TrimSpace(principal), strings.TrimSpace(clientID))
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
			AvatarURL:      fallbackAvatarURL(user.ID, user.AvatarURL),
			JoinedAt:       member.JoinedAt,
			LastSeenAt:     member.LastSeenAt,
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
	return &models.CompanionSnapshot{
		SnapshotAt: time.Now(),
		Members:    memberSnapshots,
		Positions:  visiblePositions,
	}, nil
}

func (s *CompanionService) endSessionInternal(ctx context.Context, session *models.CompanionSession, operatorUserID int64) error {
	now := time.Now()
	session.Status = models.CompanionSessionStatusEnded
	session.EndedAt = now
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
	reason := "all_members_left"
	if operatorUserID > 0 {
		reason = "owner_ended"
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
