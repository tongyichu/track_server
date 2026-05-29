package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
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

func TestCompanionCreateRejectsExistingOwnedActiveSession(t *testing.T) {
	e := newTestEnv()
	ensureTestUser(t, e, 1001, "owner")
	token := e.generateTestToken(1001)

	w := e.perform(http.MethodPost, "/api/v1/companion/session/create", []byte(`{"title":"我的同行"}`), authHeader(token))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected first create status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}

	w = e.perform(http.MethodPost, "/api/v1/companion/session/create", []byte(`{"title":"新的同行"}`), authHeader(token))
	if w.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("expected second create status 400, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var resp struct {
		Error string `json:"error"`
	}
	decodeJSON(t, w.Body.Bytes(), &resp)
	if resp.Error != "you already have an active companion session: 我的同行" {
		t.Fatalf("unexpected error response: %+v", resp)
	}
}

func TestCompanionCreateRejectsWhenAlreadyJoinedAnotherSession(t *testing.T) {
	e := newTestEnv()
	ensureTestUser(t, e, 1001, "owner")
	ensureTestUser(t, e, 1002, "guest")
	ownerToken := e.generateTestToken(1001)
	guestToken := e.generateTestToken(1002)

	w := e.perform(http.MethodPost, "/api/v1/companion/session/create", []byte(`{"title":"别人的同行"}`), authHeader(ownerToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected owner create status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var created handler.StandardResponse[*service.CompanionSessionState]
	decodeJSON(t, w.Body.Bytes(), &created)

	body, _ := json.Marshal(map[string]string{"join_token": created.Data.Join.JoinToken})
	w = e.perform(http.MethodPost, "/api/v1/companion/session/join", body, authHeader(guestToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected join status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}

	w = e.perform(http.MethodPost, "/api/v1/companion/session/create", []byte(`{"title":"我也想开一个"}`), authHeader(guestToken))
	if w.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("expected create status 400, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var resp struct {
		Error string `json:"error"`
	}
	decodeJSON(t, w.Body.Bytes(), &resp)
	if resp.Error != "you already joined an active companion session: 别人的同行" {
		t.Fatalf("unexpected error response: %+v", resp)
	}
}

func TestCompanionJoinRejectsWhenAlreadyJoinedAnotherActiveSession(t *testing.T) {
	e := newTestEnv()
	ensureTestUser(t, e, 1001, "owner")
	ensureTestUser(t, e, 1002, "guest")
	ensureTestUser(t, e, 1003, "third")
	ownerToken := e.generateTestToken(1001)
	guestToken := e.generateTestToken(1002)
	thirdToken := e.generateTestToken(1003)

	w := e.perform(http.MethodPost, "/api/v1/companion/session/create", []byte(`{"title":"第一场同行"}`), authHeader(ownerToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected first create status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var created1 handler.StandardResponse[*service.CompanionSessionState]
	decodeJSON(t, w.Body.Bytes(), &created1)

	joinBody1, _ := json.Marshal(map[string]string{"join_token": created1.Data.Join.JoinToken})
	w = e.perform(http.MethodPost, "/api/v1/companion/session/join", joinBody1, authHeader(guestToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected first join status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}

	w = e.perform(http.MethodPost, "/api/v1/companion/session/create", []byte(`{"title":"第二场同行"}`), authHeader(thirdToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected second create status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var created2 handler.StandardResponse[*service.CompanionSessionState]
	decodeJSON(t, w.Body.Bytes(), &created2)

	joinBody2, _ := json.Marshal(map[string]string{"join_token": created2.Data.Join.JoinToken})
	w = e.perform(http.MethodPost, "/api/v1/companion/session/join", joinBody2, authHeader(guestToken))
	if w.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("expected second join status 400, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var resp struct {
		Error string `json:"error"`
	}
	decodeJSON(t, w.Body.Bytes(), &resp)
	if resp.Error != "you already joined an active companion session: 第一场同行" {
		t.Fatalf("unexpected error response: %+v", resp)
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

func TestCompanionMQTTDanmakuIngest(t *testing.T) {
	e := newTestEnv()
	ensureTestUser(t, e, 1001, "owner")
	ownerToken := e.generateTestToken(1001)

	w := e.perform(http.MethodPost, "/api/v1/companion/session/create", []byte(`{"title":"弹幕测试"}`), authHeader(ownerToken))
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

	// Happy path: 内部 token + 有效 principal + 内容合法 → 200。
	body, _ := json.Marshal(service.CompanionMQTTDanmakuIngestInput{
		SessionID: sessionID,
		UserID:    1001,
		Content:   "hello danmaku",
		ClientID:  credsResp.Data.ClientID,
		Username:  credsResp.Data.Username,
	})
	w = e.perform(http.MethodPost, "/api/v1/internal/companion/mqtt/danmaku-ingest", body, ut.Header{Key: "X-Internal-Token", Value: e.internalToken})
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected danmaku ingest status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}

	// 缺少内部 token → 401。
	w = e.perform(http.MethodPost, "/api/v1/internal/companion/mqtt/danmaku-ingest", body)
	if w.Result().StatusCode() != http.StatusUnauthorized {
		t.Fatalf("expected danmaku ingest missing token status 401, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}

	// principal 不一致 → 403。
	mismatchBody, _ := json.Marshal(service.CompanionMQTTDanmakuIngestInput{
		SessionID: sessionID,
		UserID:    1002,
		Content:   "spoofed",
		ClientID:  credsResp.Data.ClientID,
		Username:  credsResp.Data.Username,
	})
	w = e.perform(http.MethodPost, "/api/v1/internal/companion/mqtt/danmaku-ingest", mismatchBody, ut.Header{Key: "X-Internal-Token", Value: e.internalToken})
	if w.Result().StatusCode() != http.StatusForbidden {
		t.Fatalf("expected danmaku ingest principal mismatch status 403, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}

	// 空内容 → 400。
	emptyBody, _ := json.Marshal(service.CompanionMQTTDanmakuIngestInput{
		SessionID: sessionID,
		UserID:    1001,
		Content:   "   ",
		ClientID:  credsResp.Data.ClientID,
		Username:  credsResp.Data.Username,
	})
	w = e.perform(http.MethodPost, "/api/v1/internal/companion/mqtt/danmaku-ingest", emptyBody, ut.Header{Key: "X-Internal-Token", Value: e.internalToken})
	if w.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("expected danmaku ingest empty content status 400, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
}

func TestCompanionToggleSessionDanmakuHappyPath(t *testing.T) {
	e := newTestEnv()
	ensureTestUser(t, e, 1001, "owner")
	ownerToken := e.generateTestToken(1001)

	w := e.perform(http.MethodPost, "/api/v1/companion/session/create", []byte(`{"title":"toggle"}`), authHeader(ownerToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected create status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var created handler.StandardResponse[*service.CompanionSessionState]
	decodeJSON(t, w.Body.Bytes(), &created)
	sessionID := created.Data.Session.SessionID
	if !created.Data.Session.DanmakuEnabled {
		t.Fatalf("expected newly created session to have danmaku enabled by default")
	}

	body, _ := json.Marshal(map[string]any{"enabled": false})
	w = e.perform(http.MethodPost, "/api/v1/companion/session/"+sessionID+"/danmaku/toggle", body, authHeader(ownerToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected toggle status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var toggled handler.StandardResponse[*service.CompanionSessionState]
	decodeJSON(t, w.Body.Bytes(), &toggled)
	if toggled.Data == nil || toggled.Data.Session == nil {
		t.Fatalf("expected toggle response to carry session state")
	}
	if toggled.Data.Session.DanmakuEnabled {
		t.Fatalf("expected danmaku_enabled=false after toggle, got true")
	}
}

func TestCompanionToggleSessionDanmakuNonOwnerForbidden(t *testing.T) {
	e := newTestEnv()
	ensureTestUser(t, e, 1001, "owner")
	ensureTestUser(t, e, 1002, "guest")
	ownerToken := e.generateTestToken(1001)
	guestToken := e.generateTestToken(1002)

	w := e.perform(http.MethodPost, "/api/v1/companion/session/create", []byte(`{"title":"toggle"}`), authHeader(ownerToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected create status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var created handler.StandardResponse[*service.CompanionSessionState]
	decodeJSON(t, w.Body.Bytes(), &created)
	sessionID := created.Data.Session.SessionID

	joinBody, _ := json.Marshal(map[string]string{"join_token": created.Data.Join.JoinToken})
	w = e.perform(http.MethodPost, "/api/v1/companion/session/join", joinBody, authHeader(guestToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected join status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}

	body, _ := json.Marshal(map[string]any{"enabled": false})
	w = e.perform(http.MethodPost, "/api/v1/companion/session/"+sessionID+"/danmaku/toggle", body, authHeader(guestToken))
	if w.Result().StatusCode() != http.StatusForbidden {
		t.Fatalf("expected toggle status 403 for non-owner, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
}

func TestCompanionToggleSessionDanmakuBadBody(t *testing.T) {
	e := newTestEnv()
	ensureTestUser(t, e, 1001, "owner")
	ownerToken := e.generateTestToken(1001)

	w := e.perform(http.MethodPost, "/api/v1/companion/session/create", []byte(`{"title":"toggle"}`), authHeader(ownerToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected create status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var created handler.StandardResponse[*service.CompanionSessionState]
	decodeJSON(t, w.Body.Bytes(), &created)
	sessionID := created.Data.Session.SessionID

	// 缺 enabled 字段。
	w = e.perform(http.MethodPost, "/api/v1/companion/session/"+sessionID+"/danmaku/toggle", []byte(`{}`), authHeader(ownerToken))
	if w.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("expected toggle missing enabled status 400, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}

	// 空请求体。
	w = e.perform(http.MethodPost, "/api/v1/companion/session/"+sessionID+"/danmaku/toggle", nil, authHeader(ownerToken))
	if w.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("expected toggle empty body status 400, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
}

func TestCompanionListHistory(t *testing.T) {
	e := newTestEnv()
	ensureTestUser(t, e, 1001, "owner")
	ensureTestUser(t, e, 1002, "guest")
	ctx := context.Background()
	guest, err := e.userRepo.FindByID(ctx, 1002)
	if err != nil {
		t.Fatalf("find guest user failed: %v", err)
	}
	guest.AvatarURL = "https://track-avatar.oss-cn-beijing.aliyuncs.com/avatar/1002.png"
	if err := e.userRepo.Update(ctx, guest); err != nil {
		t.Fatalf("update guest avatar failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(e.avatarCacheDir, "1002.png"), []byte("avatar"), 0o644); err != nil {
		t.Fatalf("seed avatar cache failed: %v", err)
	}
	_, err = e.userRepo.CreateIfNotExists(ctx, &models.User{ID: 1003, Nickname: "third", AvatarURL: "https://example.com/third.png"})
	if err != nil {
		t.Fatalf("create third user failed: %v", err)
	}
	ownerToken := e.generateTestToken(1001)
	guestToken := e.generateTestToken(1002)
	thirdToken := e.generateTestToken(1003)

	w := e.perform(http.MethodPost, "/api/v1/companion/session/create", []byte(`{"title":"第一场"}`), authHeader(ownerToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected create1 status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var created1 handler.StandardResponse[*service.CompanionSessionState]
	decodeJSON(t, w.Body.Bytes(), &created1)
	joinBody1, _ := json.Marshal(map[string]string{"join_token": created1.Data.Join.JoinToken})
	w = e.perform(http.MethodPost, "/api/v1/companion/session/join", joinBody1, authHeader(guestToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected join1 status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	w = e.perform(http.MethodPost, "/api/v1/companion/session/"+created1.Data.Session.SessionID+"/end", nil, authHeader(ownerToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected end1 status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}

	w = e.perform(http.MethodPost, "/api/v1/companion/session/create", []byte(`{"title":"第二场"}`), authHeader(guestToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected create2 status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var created2 handler.StandardResponse[*service.CompanionSessionState]
	decodeJSON(t, w.Body.Bytes(), &created2)
	joinBody2, _ := json.Marshal(map[string]string{"join_token": created2.Data.Join.JoinToken})
	w = e.perform(http.MethodPost, "/api/v1/companion/session/join", joinBody2, authHeader(ownerToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected join2 owner status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	w = e.perform(http.MethodPost, "/api/v1/companion/session/join", joinBody2, authHeader(thirdToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected join2 third status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	w = e.perform(http.MethodPost, "/api/v1/companion/session/"+created2.Data.Session.SessionID+"/leave", nil, authHeader(thirdToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected leave2 third status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}

	w = e.perform(http.MethodGet, "/api/v1/companion/session/history?limit=1", nil, authHeader(ownerToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected history page1 status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var page1 handler.StandardResponse[*models.CompanionHistoryPage]
	decodeJSON(t, w.Body.Bytes(), &page1)
	if page1.Data == nil {
		t.Fatalf("expected history page1 data")
	}
	if page1.Data.TotalCount != 2 {
		t.Fatalf("expected total_count=2, got %d", page1.Data.TotalCount)
	}
	if len(page1.Data.Items) != 1 || page1.Data.Items[0].SessionID != created2.Data.Session.SessionID {
		t.Fatalf("unexpected history page1 items: %+v", page1.Data.Items)
	}
	if page1.Data.Items[0].ParticipantCount != 2 {
		t.Fatalf("expected active participant_count=2, got %d", page1.Data.Items[0].ParticipantCount)
	}
	if len(page1.Data.Items[0].Participants) != 2 {
		t.Fatalf("expected 2 active participants, got %d", len(page1.Data.Items[0].Participants))
	}
	guestAvatarRewritten := false
	for _, p := range page1.Data.Items[0].Participants {
		if p.UserID == 1003 {
			t.Fatalf("expected left participant 1003 excluded from active participants: %+v", page1.Data.Items[0].Participants)
		}
		if p.UserID == 1002 {
			guestAvatarRewritten = p.AvatarURL == "/api/v1/static/avatars/1002.png"
		}
	}
	if !guestAvatarRewritten {
		t.Fatalf("expected guest avatar_url rewritten, got %+v", page1.Data.Items[0].Participants)
	}
	if !page1.Data.HasMore || page1.Data.NextCursor == "" {
		t.Fatalf("expected page1 has more, got %+v", page1.Data)
	}

	w = e.perform(http.MethodGet, "/api/v1/companion/session/history?limit=1&cursor="+page1.Data.NextCursor, nil, authHeader(ownerToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected history page2 status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var page2 handler.StandardResponse[*models.CompanionHistoryPage]
	decodeJSON(t, w.Body.Bytes(), &page2)
	if page2.Data == nil || len(page2.Data.Items) != 1 || page2.Data.Items[0].SessionID != created1.Data.Session.SessionID {
		t.Fatalf("unexpected history page2 items: %+v", page2.Data)
	}
	if page2.Data.Items[0].ParticipantCount != 2 || len(page2.Data.Items[0].Participants) != 2 {
		t.Fatalf("expected ended history to keep all participants, got %+v", page2.Data.Items[0])
	}
	if page2.Data.HasMore || page2.Data.NextCursor != "" {
		t.Fatalf("expected page2 end, got %+v", page2.Data)
	}

	w = e.perform(http.MethodGet, "/api/v1/companion/session/history?cursor=bad-cursor", nil, authHeader(ownerToken))
	if w.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("expected invalid cursor status 400, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
}

func TestCompanionListNearby(t *testing.T) {
	e := newTestEnv()
	ensureTestUser(t, e, 1001, "owner")
	ensureTestUser(t, e, 1002, "viewer")
	ownerToken := e.generateTestToken(1001)
	viewerToken := e.generateTestToken(1002)

	w := e.perform(http.MethodPost, "/api/v1/companion/session/create", []byte(`{"title":"near room","visibility":"public"}`), authHeader(ownerToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected create status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var created handler.StandardResponse[*service.CompanionSessionState]
	decodeJSON(t, w.Body.Bytes(), &created)
	sessionID := created.Data.Session.SessionID

	if err := e.companionRepo.UpsertPosition(context.Background(), &models.CompanionLivePosition{
		SessionID:        sessionID,
		UserID:           1001,
		Latitude:         39.9087,
		Longitude:        116.3975,
		CoordinateSystem: "wgs84",
		RecordedAt:       time.Now(),
		Source:           "test",
	}); err != nil {
		t.Fatalf("upsert owner position: %v", err)
	}

	// Missing latitude/longitude -> 400.
	w = e.perform(http.MethodGet, "/api/v1/companion/session/nearby", nil, authHeader(viewerToken))
	if w.Result().StatusCode() != http.StatusBadRequest {
		t.Fatalf("expected missing coord status 400, got %d", w.Result().StatusCode())
	}

	w = e.perform(http.MethodGet, "/api/v1/companion/session/nearby?latitude=39.9090&longitude=116.3980", nil, authHeader(viewerToken))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected nearby status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	var resp handler.StandardResponse[*models.CompanionNearbyPage]
	decodeJSON(t, w.Body.Bytes(), &resp)
	if resp.Data == nil || len(resp.Data.Items) != 1 {
		t.Fatalf("expected 1 nearby item, got %+v", resp.Data)
	}
	item := resp.Data.Items[0]
	if item.SessionID != sessionID {
		t.Fatalf("unexpected session_id %q", item.SessionID)
	}
	if item.JoinToken == "" {
		t.Fatalf("expected join_token in nearby item")
	}
	if item.Anchor == nil || item.Anchor.DistanceM <= 0 {
		t.Fatalf("expected anchor with distance, got %+v", item.Anchor)
	}
}
