package service

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
	"log"
	"strconv"
	"strings"
	"time"
)

const messageMQTTPrincipal = "msgv1:"

type messageMQTTSession struct {
	Username     string    `json:"username"`
	ClientID     string    `json:"client_id"`
	TokenVersion int64     `json:"token_version"`
	ExpiresAt    time.Time `json:"expires_at"`
}
type MessageMQTTCredentials struct {
	BrokerURL    string    `json:"broker_url"`
	WebsocketURL string    `json:"websocket_url"`
	ClientID     string    `json:"client_id"`
	Username     string    `json:"username"`
	Password     string    `json:"password"`
	ExpiresAt    time.Time `json:"expires_at"`
	Topics       []string  `json:"topics"`
}

func (s *MessageService) SetRealtime(companion *CompanionService, durable bool) {
	s.companion = companion
	s.durable = durable
	if companion != nil {
		companion.messages = s
	}
}
func (s *MessageService) messageTopic(user int64) string {
	return strings.Trim(s.companion.mqtt.TopicPrefix, "/") + "/inbox/users/" + strconv.FormatInt(user, 10)
}
func (s *MessageService) announcementTopic() string {
	return strings.Trim(s.companion.mqtt.TopicPrefix, "/") + "/inbox/announcements"
}
func (s *MessageService) IssueMessageCredentials(ctx context.Context, user int64) (*MessageMQTTCredentials, error) {
	if !s.Enabled() || !s.durable || s.companion == nil || s.companion.publisher == nil || s.companion.mqtt.CredentialSecret == "" || s.companion.mqtt.BrokerURL == "" && s.companion.mqtt.WebsocketURL == "" {
		return nil, errors.New("message realtime unavailable")
	}
	profile, err := s.users.FindByID(ctx, user)
	if err != nil {
		return nil, err
	}
	nonce, err := randomToken("", 12)
	if err != nil {
		return nil, err
	}
	expiry := messageNow().Add(15 * time.Minute).Truncate(time.Second)
	username := fmt.Sprintf("%s%d:%d:%s", messageMQTTPrincipal, user, expiry.Unix(), nonce)
	clientID := fmt.Sprintf("msg-%d-%s", user, nonce)
	payload := messageMQTTSession{Username: username, ClientID: clientID, TokenVersion: profile.TokenVersion, ExpiresAt: expiry}
	row := &models.MessageRecord{ID: "mqtt." + messageHash(username), Kind: "mqtt_session", UserID: user, CreatedAt: expiry, Active: true, Revision: 1, Payload: messageJSON(payload)}
	if err = s.repo.PutMessageRecord(ctx, row, 0); err != nil {
		return nil, err
	}
	return &MessageMQTTCredentials{BrokerURL: s.companion.mqtt.BrokerURL, WebsocketURL: s.companion.mqtt.WebsocketURL, ClientID: clientID, Username: username, Password: s.companion.signMQTTCredentials(username, clientID), ExpiresAt: expiry, Topics: []string{s.messageTopic(user), s.announcementTopic()}}, nil
}
func (s *MessageService) verifyMessageCredentials(ctx context.Context, username, clientID, password string, checkPassword bool) (int64, error) {
	if !s.Enabled() || !s.durable || s.companion == nil || !strings.HasPrefix(username, messageMQTTPrincipal) {
		return 0, repository.ErrForbidden
	}
	row, err := s.repo.GetMessageRecord(ctx, "mqtt."+messageHash(username))
	if err != nil {
		return 0, err
	}
	var v messageMQTTSession
	if json.Unmarshal(row.Payload, &v) != nil || v.Username != username || v.ClientID != clientID || !time.Now().Before(v.ExpiresAt) {
		return 0, repository.ErrForbidden
	}
	profile, err := s.users.FindByID(ctx, row.UserID)
	if err != nil || profile.TokenVersion != v.TokenVersion {
		return 0, repository.ErrForbidden
	}
	if checkPassword && !hmac.Equal([]byte(s.companion.signMQTTCredentials(username, clientID)), []byte(password)) {
		return 0, repository.ErrForbidden
	}
	return row.UserID, nil
}
func (s *MessageService) authenticateMessageMQTT(ctx context.Context, in CompanionMQTTAuthInput) CompanionMQTTAuthResult {
	if _, err := s.verifyMessageCredentials(ctx, in.Username, in.ClientID, in.Password, true); err != nil {
		return CompanionMQTTAuthResult{Result: "deny"}
	}
	return CompanionMQTTAuthResult{Result: "allow", IsSuperuser: false}
}
func (s *MessageService) authorizeMessageMQTT(ctx context.Context, in CompanionMQTTACLInput) CompanionMQTTACLResult {
	user, err := s.verifyMessageCredentials(ctx, in.Username, in.ClientID, "", false)
	if err != nil {
		return CompanionMQTTACLResult{Result: "deny"}
	}
	if (in.Action == "subscribe" || in.Action == "sub") && (in.Topic == s.messageTopic(user) || in.Topic == s.announcementTopic()) {
		return CompanionMQTTACLResult{Result: "allow"}
	}
	return CompanionMQTTACLResult{Result: "deny"}
}
func (s *MessageService) RunMessageDelivery(ctx context.Context) {
	if !s.Enabled() || !s.durable {
		return
	}
	var lastPrune, lastHeartbeat time.Time
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			work, cancel := context.WithTimeout(ctx, 15*time.Second)
			if time.Since(lastPrune) > time.Hour {
				if err := s.repo.PruneMessageCredentials(work, messageNow()); err != nil {
					log.Printf("message credential cleanup failed: %v", err)
				}
				lastPrune = messageNow()
			}
			s.activateScheduledAnnouncements(work)
			s.repairPending(work)
			s.deliverPending(work)
			if s.companion != nil && s.companion.publisher != nil && time.Since(lastHeartbeat) >= 20*time.Second && work.Err() == nil {
				heartbeat, stop := context.WithTimeout(work, 3*time.Second)
				if s.companion.publisher.Publish(heartbeat, s.announcementTopic(), messageJSON(map[string]string{"event": "inbox_heartbeat"})) == nil {
					lastHeartbeat = messageNow()
					s.companion.realtime.Heartbeat()
				}
				stop()
			}
			cancel()
		}
	}
}
func (s *MessageService) deliverPending(ctx context.Context) {
	if s.companion == nil || s.companion.publisher == nil {
		return
	}
	kinds := append([]string{"dm", "notice", "user_state", "notice_read"}, interactionKinds...)
	rows, err := s.repo.ListMessageRecords(ctx, repository.MessageFilter{Kinds: kinds, PendingOnly: true, MaxTime: messageNow(), Limit: 100, Asc: true})
	if err != nil {
		log.Printf("message delivery scan failed: %v", err)
		return
	}
	for _, row := range rows {
		if ctx.Err() != nil {
			return
		}
		topic := s.messageTopic(row.UserID)
		if row.Kind == "notice" {
			topic = s.announcementTopic()
		}
		payload := messageJSON(map[string]any{"event": "inbox_changed", "id": row.ID, "kind": row.Kind, "revision": row.Revision})
		send, cancel := context.WithTimeout(ctx, 3*time.Second)
		err := s.companion.publisher.Publish(send, topic, payload)
		cancel()
		if err == nil && row.Kind == "dm" && row.PeerID != row.UserID {
			send, cancel = context.WithTimeout(ctx, 3*time.Second)
			err = s.companion.publisher.Publish(send, s.messageTopic(row.PeerID), payload)
			cancel()
		}
		if err != nil {
			log.Printf("message delivery deferred: %v", err)
			return
		}
		if err = s.repo.MarkMessageDelivered(ctx, row.ID, row.Revision); err != nil && !errors.Is(err, repository.ErrConflict) {
			log.Printf("message delivery receipt failed: %v", err)
		}
	}
}
