package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/common/ut"

	"github.com/tongyichu/track_server/internal/handler"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/service"
)

func ensureTestUser(t *testing.T, e *testEnv, userID int64, nickname string) {
	t.Helper()
	ctx := context.Background()
	_, err := e.userRepo.CreateIfNotExists(ctx, &models.User{ID: userID, Nickname: nickname})
	if err != nil {
		t.Fatalf("create test user failed: %v", err)
	}
}

func TestCompanionCreateAndGetCurrent(t *testing.T) {
	e := newTestEnv()
	ensureTestUser(t, e, 1001, "owner")
	token := e.generateTestToken(1001)

	body, _ := json.Marshal(map[string]any{"title": "周末同行", "max_members": 6})
	w := e.perform(http.MethodPost, "/api/v1/companion/session/create", body, authHeader(token))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected create status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var created handler.StandardResponse[*service.CompanionSessionState]
	decodeJSON(t, w.Body.Bytes(), &created)
	if created.Data == nil || created.Data.Session == nil || created.Data.Join == nil {
		t.Fatalf("expected session and join info in create response")
	}
	if created.Data.Session.Status != models.CompanionSessionStatusActive {
		t.Fatalf("expected active session, got %q", created.Data.Session.Status)
	}
	if created.Data.Session.OwnerUserID != 1001 {
		t.Fatalf("expected owner 1001, got %d", created.Data.Session.OwnerUserID)
	}
	if len(created.Data.Snapshot.Members) != 1 {
		t.Fatalf("expected 1 member in snapshot, got %d", len(created.Data.Snapshot.Members))
	}

	w = e.perform(http.MethodGet, "/api/v1/companion/session/current", nil, authHeader(token))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected current status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var current handler.StandardResponse[*service.CompanionSessionState]
	decodeJSON(t, w.Body.Bytes(), &current)
	if current.Data == nil || current.Data.Session == nil {
		t.Fatalf("expected current session data")
	}
	if current.Data.Session.SessionID != created.Data.Session.SessionID {
		t.Fatalf("expected current session_id %q, got %q", created.Data.Session.SessionID, current.Data.Session.SessionID)
	}
}

func TestCompanionJoinIncludesSnapshotPositions(t *testing.T) {
	e := newTestEnv()
	ensureTestUser(t, e, 1001, "owner")
	ensureTestUser(t, e, 1002, "guest")
	ownerToken := e.generateTestToken(1001)
	guestToken := e.generateTestToken(1002)

	w := e.perform(http.MethodPost, "/api/v1/companion/session/create", []byte(`{"title":"一起走"}`), authHeader(ownerToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected create status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var created handler.StandardResponse[*service.CompanionSessionState]
	decodeJSON(t, w.Body.Bytes(), &created)
	sessionID := created.Data.Session.SessionID
	joinToken := created.Data.Join.JoinToken

	err := e.companionRepo.UpsertPosition(context.Background(), &models.CompanionLivePosition{
		SessionID:        sessionID,
		UserID:           1001,
		TrackID:          "NO.00000001",
		Latitude:         30.123,
		Longitude:        120.456,
		CoordinateSystem: "GCJ02",
		RecordedAt:       time.Now(),
		Seq:              3,
		Source:           "test",
	})
	if err != nil {
		t.Fatalf("seed position failed: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"join_token": joinToken})
	w = e.perform(http.MethodPost, "/api/v1/companion/session/join", body, authHeader(guestToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected join status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var joined handler.StandardResponse[*service.CompanionSessionState]
	decodeJSON(t, w.Body.Bytes(), &joined)
	if joined.Data == nil || joined.Data.Snapshot == nil {
		t.Fatalf("expected snapshot in join response")
	}
	if len(joined.Data.Snapshot.Members) != 2 {
		t.Fatalf("expected 2 members in snapshot, got %d", len(joined.Data.Snapshot.Members))
	}
	if len(joined.Data.Snapshot.Positions) != 1 {
		t.Fatalf("expected 1 position in snapshot, got %d", len(joined.Data.Snapshot.Positions))
	}
	if joined.Data.Snapshot.Positions[0].UserID != 1001 {
		t.Fatalf("expected owner position in snapshot, got user_id=%d", joined.Data.Snapshot.Positions[0].UserID)
	}

	w = e.perform(http.MethodGet, "/api/v1/companion/session/"+sessionID+"/snapshot", nil, authHeader(guestToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected snapshot status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var snapshot handler.StandardResponse[*models.CompanionSnapshot]
	decodeJSON(t, w.Body.Bytes(), &snapshot)
	if snapshot.Data == nil || len(snapshot.Data.Positions) != 1 {
		t.Fatalf("expected 1 position in snapshot endpoint")
	}
}

func TestCompanionOwnerCannotLeaveWhileOthersJoined(t *testing.T) {
	e := newTestEnv()
	ensureTestUser(t, e, 1001, "owner")
	ensureTestUser(t, e, 1002, "guest")
	ownerToken := e.generateTestToken(1001)
	guestToken := e.generateTestToken(1002)

	w := e.perform(http.MethodPost, "/api/v1/companion/session/create", []byte(`{}`), authHeader(ownerToken))
	var created handler.StandardResponse[*service.CompanionSessionState]
	decodeJSON(t, w.Body.Bytes(), &created)
	body, _ := json.Marshal(map[string]string{"join_token": created.Data.Join.JoinToken})
	w = e.perform(http.MethodPost, "/api/v1/companion/session/join", body, authHeader(guestToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected join status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}

	w = e.perform(http.MethodPost, "/api/v1/companion/session/"+created.Data.Session.SessionID+"/leave", nil, authHeader(ownerToken))
	if w.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("expected owner leave status 400, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}

	w = e.perform(http.MethodPost, "/api/v1/companion/session/"+created.Data.Session.SessionID+"/end", nil, authHeader(ownerToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected end status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}

	w = e.perform(http.MethodGet, "/api/v1/companion/session/current", nil, authHeader(ownerToken))
	if w.Result().StatusCode() != http.StatusNotFound {
		t.Fatalf("expected ended current status 404, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
}

func TestCompanionIssueMQTTCredentialsAndAuthACL(t *testing.T) {
	e := newTestEnv()
	ensureTestUser(t, e, 1001, "owner")
	ownerToken := e.generateTestToken(1001)

	w := e.perform(http.MethodPost, "/api/v1/companion/session/create", []byte(`{"title":"一起走"}`), authHeader(ownerToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected create status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var created handler.StandardResponse[*service.CompanionSessionState]
	decodeJSON(t, w.Body.Bytes(), &created)

	w = e.perform(http.MethodPost, "/api/v1/companion/session/"+created.Data.Session.SessionID+"/mqtt/credentials", nil, authHeader(ownerToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected mqtt credentials status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var credsResp handler.StandardResponse[*service.CompanionMQTTCredentials]
	decodeJSON(t, w.Body.Bytes(), &credsResp)
	if credsResp.Data == nil || credsResp.Data.ClientID == "" || credsResp.Data.Username == "" || credsResp.Data.Password == "" {
		t.Fatalf("expected non-empty mqtt credentials")
	}
	if credsResp.Data.Topics.LocationPublish == "" || credsResp.Data.Topics.ControlSubscribe == "" {
		t.Fatalf("expected mqtt topic bindings in response")
	}

	authBody, _ := json.Marshal(service.CompanionMQTTAuthInput{
		ClientID: credsResp.Data.ClientID,
		Username: credsResp.Data.Username,
		Password: credsResp.Data.Password,
	})
	w = e.perform(http.MethodPost, "/api/v1/internal/mqtt/auth", authBody, ut.Header{Key: "X-Internal-Token", Value: e.internalToken})
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected mqtt auth status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var authResp service.CompanionMQTTAuthResult
	decodeJSON(t, w.Body.Bytes(), &authResp)
	if authResp.Result != "allow" {
		t.Fatalf("expected mqtt auth allow, got %+v", authResp)
	}

	aclBody, _ := json.Marshal(service.CompanionMQTTACLInput{
		ClientID: credsResp.Data.ClientID,
		Username: credsResp.Data.Username,
		Action:   "publish",
		Topic:    credsResp.Data.Topics.LocationPublish,
	})
	w = e.perform(http.MethodPost, "/api/v1/internal/mqtt/acl", aclBody, ut.Header{Key: "X-Internal-Token", Value: e.internalToken})
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected mqtt acl status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var aclResp service.CompanionMQTTACLResult
	decodeJSON(t, w.Body.Bytes(), &aclResp)
	if aclResp.Result != "allow" {
		t.Fatalf("expected mqtt acl allow, got %+v", aclResp)
	}

	denyACLBody, _ := json.Marshal(service.CompanionMQTTACLInput{
		ClientID: credsResp.Data.ClientID,
		Username: credsResp.Data.Username,
		Action:   "publish",
		Topic:    "companion/other/member/999/location",
	})
	w = e.perform(http.MethodPost, "/api/v1/internal/mqtt/acl", denyACLBody, ut.Header{Key: "X-Internal-Token", Value: e.internalToken})
	decodeJSON(t, w.Body.Bytes(), &aclResp)
	if aclResp.Result != "deny" {
		t.Fatalf("expected mqtt acl deny for foreign topic, got %+v", aclResp)
	}
}

func TestCompanionMQTTIngestUpdatesPresenceAndSnapshot(t *testing.T) {
	e := newTestEnv()
	ensureTestUser(t, e, 1001, "owner")
	ownerToken := e.generateTestToken(1001)

	w := e.perform(http.MethodPost, "/api/v1/companion/session/create", []byte(`{"title":"一起走"}`), authHeader(ownerToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected create status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var created handler.StandardResponse[*service.CompanionSessionState]
	decodeJSON(t, w.Body.Bytes(), &created)
	sessionID := created.Data.Session.SessionID

	w = e.perform(http.MethodPost, "/api/v1/companion/session/"+sessionID+"/mqtt/credentials", nil, authHeader(ownerToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected mqtt credentials status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var credsResp handler.StandardResponse[*service.CompanionMQTTCredentials]
	decodeJSON(t, w.Body.Bytes(), &credsResp)

	presenceBody, _ := json.Marshal(service.CompanionMQTTPresenceIngestInput{
		SessionID:  sessionID,
		UserID:     1001,
		Status:     models.CompanionPresenceStatusOnline,
		LastSeenAt: time.Now(),
		ClientID:   credsResp.Data.ClientID,
		Username:   credsResp.Data.Username,
	})
	w = e.perform(http.MethodPost, "/api/v1/internal/companion/mqtt/presence-ingest", presenceBody, ut.Header{Key: "X-Internal-Token", Value: e.internalToken})
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected presence ingest status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}

	recordedAt := time.Now().UTC().Truncate(time.Second)
	locationBody, _ := json.Marshal(service.CompanionMQTTLocationIngestInput{
		SessionID:        sessionID,
		UserID:           1001,
		TrackID:          "NO.00000001",
		Latitude:         30.123,
		Longitude:        120.456,
		CoordinateSystem: "GCJ02",
		SpeedKmh:         5.2,
		Heading:          90,
		AccuracyM:        8,
		Altitude:         100,
		RecordedAt:       recordedAt,
		Seq:              10,
		ClientID:         credsResp.Data.ClientID,
		Username:         credsResp.Data.Username,
	})
	w = e.perform(http.MethodPost, "/api/v1/internal/companion/mqtt/location-ingest", locationBody, ut.Header{Key: "X-Internal-Token", Value: e.internalToken})
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected location ingest status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}

	w = e.perform(http.MethodGet, "/api/v1/companion/session/"+sessionID+"/snapshot", nil, authHeader(ownerToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected snapshot status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var snapshotResp handler.StandardResponse[*models.CompanionSnapshot]
	decodeJSON(t, w.Body.Bytes(), &snapshotResp)
	if snapshotResp.Data == nil || len(snapshotResp.Data.Members) != 1 || len(snapshotResp.Data.Positions) != 1 {
		t.Fatalf("expected one member and one position in snapshot")
	}
	if snapshotResp.Data.Members[0].PresenceStatus != models.CompanionPresenceStatusOnline {
		t.Fatalf("expected online presence, got %q", snapshotResp.Data.Members[0].PresenceStatus)
	}
	if snapshotResp.Data.Positions[0].Seq != 10 || snapshotResp.Data.Positions[0].RecordedAt.IsZero() {
		t.Fatalf("expected latest ingested position in snapshot")
	}

	olderLocationBody, _ := json.Marshal(service.CompanionMQTTLocationIngestInput{
		SessionID:        sessionID,
		UserID:           1001,
		TrackID:          "NO.00000001",
		Latitude:         31.0,
		Longitude:        121.0,
		CoordinateSystem: "GCJ02",
		RecordedAt:       recordedAt.Add(-time.Minute),
		Seq:              9,
		ClientID:         credsResp.Data.ClientID,
		Username:         credsResp.Data.Username,
	})
	w = e.perform(http.MethodPost, "/api/v1/internal/companion/mqtt/location-ingest", olderLocationBody, ut.Header{Key: "X-Internal-Token", Value: e.internalToken})
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected older location ingest status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	w = e.perform(http.MethodGet, "/api/v1/companion/session/"+sessionID+"/snapshot", nil, authHeader(ownerToken))
	decodeJSON(t, w.Body.Bytes(), &snapshotResp)
	if snapshotResp.Data.Positions[0].Seq != 10 {
		t.Fatalf("expected stale location to be ignored, got seq=%d", snapshotResp.Data.Positions[0].Seq)
	}
}

func TestCompanionInternalMQTTRoutesRequireToken(t *testing.T) {
	e := newTestEnv()
	w := e.perform(http.MethodPost, "/api/v1/internal/mqtt/auth", []byte(`{}`))
	if w.Result().StatusCode() != http.StatusUnauthorized {
		t.Fatalf("expected missing internal token status 401, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
}
