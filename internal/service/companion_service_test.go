package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type publishedControlMessage struct {
	topic string
	event CompanionControlEvent
}

type mockCompanionControlPublisher struct {
	messages []publishedControlMessage
	err      error
}

type blockingAssetDownloader struct {
	called  atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (d *blockingAssetDownloader) DownloadObject(_ int64, _ string, localPath string) error {
	if d.called.Add(1) == 1 {
		close(d.started)
	}
	<-d.release
	return os.WriteFile(localPath, []byte("avatar"), 0o644)
}

type panicAssetDownloader struct {
	called atomic.Int32
}

func (d *panicAssetDownloader) DownloadObject(_ int64, _ string, _ string) error {
	d.called.Add(1)
	panic("boom")
}

func (m *mockCompanionControlPublisher) Publish(_ context.Context, topic string, payload []byte) error {
	if m.err != nil {
		return m.err
	}
	var event CompanionControlEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return err
	}
	m.messages = append(m.messages, publishedControlMessage{topic: topic, event: event})
	return nil
}

func (m *mockCompanionControlPublisher) Close() error { return nil }

func newCompanionServiceForTest(t *testing.T) (*CompanionService, repository.CompanionRepository, repository.UserRepository) {
	t.Helper()
	_, userRepo, _, _, _, _, companionRepo := repository.NewInMemoryRepositories()
	ctx := context.Background()
	for _, user := range []*models.User{{ID: 1001, Nickname: "owner"}, {ID: 1002, Nickname: "guest"}} {
		if _, err := userRepo.CreateIfNotExists(ctx, user); err != nil {
			t.Fatalf("CreateIfNotExists failed: %v", err)
		}
	}
	svc := NewCompanionService(companionRepo, userRepo)
	return svc, companionRepo, userRepo
}

func TestCompanionServiceLeavePublishesMemberLeft(t *testing.T) {
	svc, _, _ := newCompanionServiceForTest(t)
	pub := &mockCompanionControlPublisher{}
	svc.SetControlPublisher(pub)

	created, err := svc.CreateSession(context.Background(), 1001, CreateCompanionSessionInput{Title: "一起走"})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if _, err := svc.JoinSession(context.Background(), 1002, JoinCompanionSessionInput{JoinToken: created.Join.JoinToken}); err != nil {
		t.Fatalf("JoinSession returned error: %v", err)
	}
	if err := svc.LeaveSession(context.Background(), 1002, created.Session.SessionID); err != nil {
		t.Fatalf("LeaveSession returned error: %v", err)
	}
	if len(pub.messages) != 1 {
		t.Fatalf("expected 1 published control message, got %d", len(pub.messages))
	}
	msg := pub.messages[0]
	if msg.topic != "companion/"+created.Session.SessionID+"/control" {
		t.Fatalf("unexpected topic %q", msg.topic)
	}
	if msg.event.Event != CompanionControlEventMemberLeft || msg.event.MemberUserID != 1002 || msg.event.OperatorUserID != 1002 {
		t.Fatalf("unexpected member_left event: %+v", msg.event)
	}
}

func TestCompanionServiceKickMemberPublishesMemberKicked(t *testing.T) {
	svc, repo, _ := newCompanionServiceForTest(t)
	pub := &mockCompanionControlPublisher{}
	svc.SetControlPublisher(pub)
	ctx := context.Background()

	created, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "一起走"})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if _, err := svc.JoinSession(ctx, 1002, JoinCompanionSessionInput{JoinToken: created.Join.JoinToken}); err != nil {
		t.Fatalf("JoinSession returned error: %v", err)
	}
	if err := repo.UpsertPosition(ctx, &models.CompanionLivePosition{
		SessionID:        created.Session.SessionID,
		UserID:           1002,
		Latitude:         39.9,
		Longitude:        116.3,
		CoordinateSystem: "wgs84",
		RecordedAt:       time.Now(),
		Seq:              1,
		Source:           "test",
	}); err != nil {
		t.Fatalf("UpsertPosition returned error: %v", err)
	}

	state, err := svc.KickSessionMember(ctx, 1001, created.Session.SessionID, 1002)
	if err != nil {
		t.Fatalf("KickSessionMember returned error: %v", err)
	}
	if state == nil || state.Snapshot == nil {
		t.Fatalf("expected kick response to carry session state")
	}
	if len(state.Snapshot.Members) != 1 || state.Snapshot.Members[0].UserID != 1001 {
		t.Fatalf("expected only owner in snapshot after kick, got %+v", state.Snapshot.Members)
	}
	if len(state.Snapshot.Positions) != 0 {
		t.Fatalf("expected kicked member position to be hidden, got %d", len(state.Snapshot.Positions))
	}
	member, err := repo.FindMember(ctx, created.Session.SessionID, 1002)
	if err != nil {
		t.Fatalf("FindMember returned error: %v", err)
	}
	if member.MemberStatus != models.CompanionMemberStatusKicked || member.PresenceStatus != models.CompanionPresenceStatusOffline {
		t.Fatalf("expected kicked/offline member, got %+v", member)
	}
	if len(pub.messages) != 1 {
		t.Fatalf("expected 1 published control message, got %d", len(pub.messages))
	}
	msg := pub.messages[0]
	if msg.topic != "companion/"+created.Session.SessionID+"/control" {
		t.Fatalf("unexpected topic %q", msg.topic)
	}
	if msg.event.Event != CompanionControlEventMemberKicked || msg.event.MemberUserID != 1002 || msg.event.OperatorUserID != 1001 || msg.event.Reason != "member_kicked" {
		t.Fatalf("unexpected member_kicked event: %+v", msg.event)
	}
	if _, err := svc.GetCurrentSession(ctx, 1002); err == nil {
		t.Fatalf("expected kicked member to have no current session")
	}
}

func TestCompanionServiceKickMemberRejectsNonOwnerAndSelf(t *testing.T) {
	svc, _, _ := newCompanionServiceForTest(t)
	ctx := context.Background()

	created, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "一起走"})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if _, err := svc.JoinSession(ctx, 1002, JoinCompanionSessionInput{JoinToken: created.Join.JoinToken}); err != nil {
		t.Fatalf("JoinSession returned error: %v", err)
	}

	if _, err := svc.KickSessionMember(ctx, 1002, created.Session.SessionID, 1001); err == nil || err != repository.ErrForbidden {
		t.Fatalf("expected non-owner kick to be forbidden, got %v", err)
	}
	if _, err := svc.KickSessionMember(ctx, 1001, created.Session.SessionID, 1001); err == nil || err.Error() != "owner cannot kick self" {
		t.Fatalf("expected owner cannot kick self, got %v", err)
	}
}

func TestCompanionServiceCreateAndJoinRejectRunningTrack(t *testing.T) {
	trackRepo, userRepo, _, _, _, _, companionRepo := repository.NewInMemoryRepositories()
	ctx := context.Background()
	for _, user := range []*models.User{{ID: 1001, Nickname: "owner"}, {ID: 1002, Nickname: "guest"}, {ID: 1003, Nickname: "runner"}} {
		if _, err := userRepo.CreateIfNotExists(ctx, user); err != nil {
			t.Fatalf("CreateIfNotExists returned error: %v", err)
		}
	}
	svc := NewCompanionService(companionRepo, userRepo)
	svc.SetTrackRepository(trackRepo)

	if err := trackRepo.Create(ctx, &models.Track{ID: "trk-running", UserID: 1001, Title: "running", IsRunning: true, Status: models.TrackStatusNormal}); err != nil {
		t.Fatalf("Create running track returned error: %v", err)
	}
	if _, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "同行"}); err == nil || err.Error() != "you already have a running track: trk-running" {
		t.Fatalf("expected running track create rejection, got %v", err)
	}

	created, err := svc.CreateSession(ctx, 1002, CreateCompanionSessionInput{Title: "可加入同行"})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if err := trackRepo.Create(ctx, &models.Track{ID: "trk-runner", UserID: 1003, Title: "running", IsRunning: true, Status: models.TrackStatusNormal}); err != nil {
		t.Fatalf("Create runner track returned error: %v", err)
	}
	if _, err := svc.JoinSession(ctx, 1003, JoinCompanionSessionInput{JoinToken: created.Join.JoinToken}); err == nil || err.Error() != "you already have a running track: trk-runner" {
		t.Fatalf("expected running track join rejection, got %v", err)
	}
}

func TestCompanionServiceJoinAllowsCompletedTrack(t *testing.T) {
	trackRepo, userRepo, _, _, _, _, companionRepo := repository.NewInMemoryRepositories()
	ctx := context.Background()
	for _, user := range []*models.User{{ID: 1001, Nickname: "owner"}, {ID: 1002, Nickname: "guest"}} {
		if _, err := userRepo.CreateIfNotExists(ctx, user); err != nil {
			t.Fatalf("CreateIfNotExists returned error: %v", err)
		}
	}
	svc := NewCompanionService(companionRepo, userRepo)
	svc.SetTrackRepository(trackRepo)
	created, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "同行"})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if err := trackRepo.Create(ctx, &models.Track{ID: "trk-done", UserID: 1002, Title: "done", IsRunning: false, Status: models.TrackStatusNormal}); err != nil {
		t.Fatalf("Create completed track returned error: %v", err)
	}
	if _, err := svc.JoinSession(ctx, 1002, JoinCompanionSessionInput{JoinToken: created.Join.JoinToken}); err != nil {
		t.Fatalf("expected completed track to allow join, got %v", err)
	}
}

func TestCompanionServiceAutoCloseInactiveSession(t *testing.T) {
	svc, repo, _ := newCompanionServiceForTest(t)
	pub := &mockCompanionControlPublisher{}
	svc.SetControlPublisher(pub)
	ctx := context.Background()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	created, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "忘记关闭", TrackType: "跑步"})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	sessionID := created.Session.SessionID
	session, err := repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		t.Fatalf("FindSessionByID returned error: %v", err)
	}
	session.StartedAt = now.Add(-1 * time.Hour)
	if err := repo.UpdateSession(ctx, session); err != nil {
		t.Fatalf("UpdateSession returned error: %v", err)
	}
	_, err = svc.JoinSession(ctx, 1002, JoinCompanionSessionInput{JoinToken: created.Join.JoinToken})
	if err != nil {
		t.Fatalf("JoinSession returned error: %v", err)
	}
	for _, userID := range []int64{1001, 1002} {
		member, err := repo.FindMember(ctx, sessionID, userID)
		if err != nil {
			t.Fatalf("FindMember(%d): %v", userID, err)
		}
		member.JoinedAt = now.Add(-1 * time.Hour)
		member.LastSeenAt = now.Add(-31 * time.Minute)
		member.PresenceStatus = models.CompanionPresenceStatusOffline
		if err := repo.UpsertMember(ctx, member); err != nil {
			t.Fatalf("UpsertMember(%d): %v", userID, err)
		}
	}

	result, err := svc.AutoCloseInactiveSessions(ctx, now)
	if err != nil {
		t.Fatalf("AutoCloseInactiveSessions returned error: %v", err)
	}
	if result.Closed != 1 {
		t.Fatalf("expected one closed session, got %+v", result)
	}
	session, err = repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		t.Fatalf("FindSessionByID returned error: %v", err)
	}
	if session.Status != models.CompanionSessionStatusEnded {
		t.Fatalf("expected session ended, got %s", session.Status)
	}
	if session.EndReason != "inactive_timeout" || session.EndSource != models.CompanionSessionEndSourceAutoClose || session.EndOperatorUserID != 0 {
		t.Fatalf("unexpected end audit fields: reason=%q source=%q operator=%d", session.EndReason, session.EndSource, session.EndOperatorUserID)
	}
	if len(pub.messages) == 0 || pub.messages[len(pub.messages)-1].event.Reason != "inactive_timeout" {
		t.Fatalf("expected inactive_timeout event, got %+v", pub.messages)
	}
}

func TestCompanionServiceAutoCloseUsesLatestPositionAsActivity(t *testing.T) {
	svc, repo, _ := newCompanionServiceForTest(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	created, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "仍在活动", TrackType: "徒步"})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	sessionID := created.Session.SessionID
	session, err := repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		t.Fatalf("FindSessionByID returned error: %v", err)
	}
	session.StartedAt = now.Add(-1 * time.Hour)
	if err := repo.UpdateSession(ctx, session); err != nil {
		t.Fatalf("UpdateSession returned error: %v", err)
	}
	member, err := repo.FindMember(ctx, sessionID, 1001)
	if err != nil {
		t.Fatalf("FindMember returned error: %v", err)
	}
	member.JoinedAt = now.Add(-2 * time.Hour)
	member.LastSeenAt = now.Add(-2 * time.Hour)
	member.PresenceStatus = models.CompanionPresenceStatusOffline
	if err := repo.UpsertMember(ctx, member); err != nil {
		t.Fatalf("UpsertMember returned error: %v", err)
	}
	if err := repo.UpsertPosition(ctx, &models.CompanionLivePosition{
		SessionID:        sessionID,
		UserID:           1001,
		Latitude:         30,
		Longitude:        120,
		CoordinateSystem: "GCJ02",
		RecordedAt:       now.Add(-10 * time.Minute),
		CreatedAt:        now.Add(-10 * time.Minute),
		UpdatedAt:        now.Add(-10 * time.Minute),
	}); err != nil {
		t.Fatalf("UpsertPosition returned error: %v", err)
	}

	result, err := svc.AutoCloseInactiveSessions(ctx, now)
	if err != nil {
		t.Fatalf("AutoCloseInactiveSessions returned error: %v", err)
	}
	if result.Closed != 0 {
		t.Fatalf("expected no closed sessions, got %+v", result)
	}
}

func TestCompanionServiceAutoCloseMaxDuration(t *testing.T) {
	svc, repo, _ := newCompanionServiceForTest(t)
	pub := &mockCompanionControlPublisher{}
	svc.SetControlPublisher(pub)
	ctx := context.Background()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	created, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "超长同行", TrackType: "跑步"})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	session, err := repo.FindSessionByID(ctx, created.Session.SessionID)
	if err != nil {
		t.Fatalf("FindSessionByID returned error: %v", err)
	}
	session.StartedAt = now.Add(-9 * time.Hour)
	if err := repo.UpdateSession(ctx, session); err != nil {
		t.Fatalf("UpdateSession returned error: %v", err)
	}
	member, err := repo.FindMember(ctx, session.SessionID, 1001)
	if err != nil {
		t.Fatalf("FindMember returned error: %v", err)
	}
	member.LastSeenAt = now
	if err := repo.UpsertMember(ctx, member); err != nil {
		t.Fatalf("UpsertMember returned error: %v", err)
	}

	result, err := svc.AutoCloseInactiveSessions(ctx, now)
	if err != nil {
		t.Fatalf("AutoCloseInactiveSessions returned error: %v", err)
	}
	if result.Closed != 1 {
		t.Fatalf("expected one closed session, got %+v", result)
	}
	if len(pub.messages) == 0 || pub.messages[len(pub.messages)-1].event.Reason != "max_duration_exceeded" {
		t.Fatalf("expected max_duration_exceeded event, got %+v", pub.messages)
	}
	session, err = repo.FindSessionByID(ctx, created.Session.SessionID)
	if err != nil {
		t.Fatalf("FindSessionByID returned error: %v", err)
	}
	if session.EndReason != "max_duration_exceeded" || session.EndSource != models.CompanionSessionEndSourceAutoClose || session.EndOperatorUserID != 0 {
		t.Fatalf("unexpected end audit fields: reason=%q source=%q operator=%d", session.EndReason, session.EndSource, session.EndOperatorUserID)
	}
}

func TestCompanionServiceEndPublishesSessionEnded(t *testing.T) {
	svc, repo, _ := newCompanionServiceForTest(t)
	pub := &mockCompanionControlPublisher{}
	svc.SetControlPublisher(pub)

	created, err := svc.CreateSession(context.Background(), 1001, CreateCompanionSessionInput{Title: "一起走"})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if err := svc.EndSession(context.Background(), 1001, created.Session.SessionID); err != nil {
		t.Fatalf("EndSession returned error: %v", err)
	}
	if len(pub.messages) != 1 {
		t.Fatalf("expected 1 published control message, got %d", len(pub.messages))
	}
	msg := pub.messages[0]
	if msg.event.Event != CompanionControlEventSessionEnded || msg.event.OperatorUserID != 1001 || msg.event.Reason != "owner_ended" {
		t.Fatalf("unexpected session_ended event: %+v", msg.event)
	}
	session, err := repo.FindSessionByID(context.Background(), created.Session.SessionID)
	if err != nil {
		t.Fatalf("FindSessionByID returned error: %v", err)
	}
	if session.EndReason != "owner_ended" || session.EndSource != models.CompanionSessionEndSourceOwner || session.EndOperatorUserID != 1001 {
		t.Fatalf("unexpected end audit fields: reason=%q source=%q operator=%d", session.EndReason, session.EndSource, session.EndOperatorUserID)
	}
}

func TestCompanionServiceAutoEndPublishesSessionEndedAfterLastLeave(t *testing.T) {
	svc, repo, _ := newCompanionServiceForTest(t)
	pub := &mockCompanionControlPublisher{}
	svc.SetControlPublisher(pub)

	created, err := svc.CreateSession(context.Background(), 1001, CreateCompanionSessionInput{Title: "一起走"})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if err := svc.LeaveSession(context.Background(), 1001, created.Session.SessionID); err != nil {
		t.Fatalf("LeaveSession returned error: %v", err)
	}
	if len(pub.messages) != 2 {
		t.Fatalf("expected 2 published control messages, got %d", len(pub.messages))
	}
	if pub.messages[0].event.Event != CompanionControlEventMemberLeft {
		t.Fatalf("expected first event member_left, got %+v", pub.messages[0].event)
	}
	if pub.messages[1].event.Event != CompanionControlEventSessionEnded || pub.messages[1].event.Reason != "all_members_left" {
		t.Fatalf("expected second event session_ended/all_members_left, got %+v", pub.messages[1].event)
	}
	if pub.messages[1].event.At.Before(pub.messages[0].event.At.Add(-time.Second)) {
		t.Fatalf("expected session_ended timestamp to be close to member_left timestamp")
	}
	session, err := repo.FindSessionByID(context.Background(), created.Session.SessionID)
	if err != nil {
		t.Fatalf("FindSessionByID returned error: %v", err)
	}
	if session.EndReason != "all_members_left" || session.EndSource != models.CompanionSessionEndSourceMemberFlow || session.EndOperatorUserID != 0 {
		t.Fatalf("unexpected end audit fields: reason=%q source=%q operator=%d", session.EndReason, session.EndSource, session.EndOperatorUserID)
	}
}

func TestCompanionServiceCreateSessionRejectsExistingOwnedActiveSession(t *testing.T) {
	svc, _, _ := newCompanionServiceForTest(t)
	ctx := context.Background()

	if _, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "我的同行"}); err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	_, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "新的同行"})
	if err == nil {
		t.Fatalf("expected create session to be rejected")
	}
	if err.Error() != "you already have an active companion session: 我的同行" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompanionServiceCreateSessionRejectsWhenAlreadyJoinedAnotherSession(t *testing.T) {
	svc, _, _ := newCompanionServiceForTest(t)
	ctx := context.Background()

	created, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "别人的同行"})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if _, err := svc.JoinSession(ctx, 1002, JoinCompanionSessionInput{JoinToken: created.Join.JoinToken}); err != nil {
		t.Fatalf("JoinSession returned error: %v", err)
	}

	_, err = svc.CreateSession(ctx, 1002, CreateCompanionSessionInput{Title: "我也想开一个"})
	if err == nil {
		t.Fatalf("expected create session to be rejected")
	}
	if err.Error() != "you already joined an active companion session: 别人的同行" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompanionServiceJoinSessionRejectsWhenAlreadyJoinedAnotherActiveSession(t *testing.T) {
	svc, _, userRepo := newCompanionServiceForTest(t)
	ctx := context.Background()
	if _, err := userRepo.CreateIfNotExists(ctx, &models.User{ID: 1003, Nickname: "third"}); err != nil {
		t.Fatalf("CreateIfNotExists failed: %v", err)
	}

	created1, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "第一场同行"})
	if err != nil {
		t.Fatalf("CreateSession #1 returned error: %v", err)
	}
	if _, err := svc.JoinSession(ctx, 1002, JoinCompanionSessionInput{JoinToken: created1.Join.JoinToken}); err != nil {
		t.Fatalf("JoinSession #1 returned error: %v", err)
	}

	created2, err := svc.CreateSession(ctx, 1003, CreateCompanionSessionInput{Title: "第二场同行"})
	if err != nil {
		t.Fatalf("CreateSession #2 returned error: %v", err)
	}

	_, err = svc.JoinSession(ctx, 1002, JoinCompanionSessionInput{JoinToken: created2.Join.JoinToken})
	if err == nil {
		t.Fatalf("expected join session to be rejected")
	}
	if err.Error() != "you already joined an active companion session: 第一场同行" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeCompanionMQTTBrokerURL(t *testing.T) {
	cases := map[string]string{
		"mqtt://127.0.0.1:1883":      "tcp://127.0.0.1:1883",
		"mqtts://example.com:8883":   "ssl://example.com:8883",
		"ws://example.com:8083/mqtt": "ws://example.com:8083/mqtt",
		"":                           "",
	}
	for input, want := range cases {
		if got := normalizeCompanionMQTTBrokerURL(input); got != want {
			t.Fatalf("normalizeCompanionMQTTBrokerURL(%q)=%q want %q", input, got, want)
		}
	}
}

func TestCompanionServiceListHistory(t *testing.T) {
	svc, _, userRepo := newCompanionServiceForTest(t)
	ctx := context.Background()
	if _, err := userRepo.CreateIfNotExists(ctx, &models.User{ID: 1003, Nickname: "third", AvatarURL: "https://example.com/third.png"}); err != nil {
		t.Fatalf("CreateIfNotExists failed: %v", err)
	}

	created1, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "第一场", TrackType: "徒步", LocateAddr: "北京市海淀区颐和园"})
	if err != nil {
		t.Fatalf("CreateSession #1 returned error: %v", err)
	}
	if _, err := svc.JoinSession(ctx, 1002, JoinCompanionSessionInput{JoinToken: created1.Join.JoinToken}); err != nil {
		t.Fatalf("JoinSession #1 returned error: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	if err := svc.EndSession(ctx, 1001, created1.Session.SessionID); err != nil {
		t.Fatalf("EndSession #1 returned error: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	created2, err := svc.CreateSession(ctx, 1002, CreateCompanionSessionInput{Title: "第二场", TrackType: "跑步"})
	if err != nil {
		t.Fatalf("CreateSession #2 returned error: %v", err)
	}
	if _, err := svc.JoinSession(ctx, 1001, JoinCompanionSessionInput{JoinToken: created2.Join.JoinToken}); err != nil {
		t.Fatalf("JoinSession #2 returned error: %v", err)
	}
	if _, err := svc.JoinSession(ctx, 1003, JoinCompanionSessionInput{JoinToken: created2.Join.JoinToken}); err != nil {
		t.Fatalf("JoinSession #2 third returned error: %v", err)
	}
	if err := svc.LeaveSession(ctx, 1003, created2.Session.SessionID); err != nil {
		t.Fatalf("LeaveSession #2 third returned error: %v", err)
	}

	page1, err := svc.ListHistory(ctx, 1001, ListCompanionHistoryInput{Limit: 1})
	if err != nil {
		t.Fatalf("ListHistory page1 returned error: %v", err)
	}
	if page1.TotalCount != 2 {
		t.Fatalf("expected total_count=2, got %d", page1.TotalCount)
	}
	if len(page1.Items) != 1 || page1.Items[0].SessionID != created2.Session.SessionID {
		t.Fatalf("unexpected page1 items: %+v", page1.Items)
	}
	if page1.Items[0].ParticipantCount != 2 {
		t.Fatalf("expected active participant_count=2, got %d", page1.Items[0].ParticipantCount)
	}
	if len(page1.Items[0].Participants) != 2 {
		t.Fatalf("expected 2 active participants, got %d", len(page1.Items[0].Participants))
	}
	for _, p := range page1.Items[0].Participants {
		if p.UserID == 1003 {
			t.Fatalf("expected left participant 1003 excluded from active participants: %+v", page1.Items[0].Participants)
		}
	}
	if page1.Items[0].TrackType != "running" {
		t.Fatalf("expected active track_type=running, got %q", page1.Items[0].TrackType)
	}
	if page1.Items[0].DurationSeconds < 0 {
		t.Fatalf("expected non-negative active duration_seconds, got %d", page1.Items[0].DurationSeconds)
	}
	if !page1.HasMore || page1.NextCursor == "" {
		t.Fatalf("expected page1 has_more with next cursor, got %+v", page1)
	}

	page2, err := svc.ListHistory(ctx, 1001, ListCompanionHistoryInput{Limit: 1, Cursor: page1.NextCursor})
	if err != nil {
		t.Fatalf("ListHistory page2 returned error: %v", err)
	}
	if len(page2.Items) != 1 || page2.Items[0].SessionID != created1.Session.SessionID {
		t.Fatalf("unexpected page2 items: %+v", page2.Items)
	}
	if page2.Items[0].Status != models.CompanionSessionStatusEnded {
		t.Fatalf("expected ended status, got %q", page2.Items[0].Status)
	}
	if page2.Items[0].ParticipantCount != 2 || len(page2.Items[0].Participants) != 2 {
		t.Fatalf("expected ended participant history preserved, got %+v", page2.Items[0])
	}
	if page2.Items[0].TrackType != "hiking" {
		t.Fatalf("expected ended track_type=hiking, got %q", page2.Items[0].TrackType)
	}
	if page2.Items[0].LocateAddr != "北京市海淀区颐和园" {
		t.Fatalf("expected ended locate_addr=北京市海淀区颐和园, got %q", page2.Items[0].LocateAddr)
	}
	if page2.Items[0].DurationSeconds <= 0 {
		t.Fatalf("expected positive ended duration_seconds, got %d", page2.Items[0].DurationSeconds)
	}
	if page2.HasMore || page2.NextCursor != "" {
		t.Fatalf("expected page2 end, got %+v", page2)
	}

	if _, err := svc.ListHistory(ctx, 1001, ListCompanionHistoryInput{Cursor: "bad-cursor"}); err == nil {
		t.Fatalf("expected invalid cursor error")
	}
}

func TestCompanionServiceListHistoryRewritesAvatarURLToStaticAsset(t *testing.T) {
	svc, _, userRepo := newCompanionServiceForTest(t)
	ctx := context.Background()
	avatarURL := "https://track-avatar.oss-cn-beijing.aliyuncs.com/avatar/1002.png"
	guest, err := userRepo.FindByID(ctx, 1002)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	guest.AvatarURL = avatarURL
	if err := userRepo.Update(ctx, guest); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	cacheDir := t.TempDir()
	cache, err := NewAssetCacheService(cacheDir, "/api/v1/static/avatars", []string{".png", ".jpg", ".jpeg", ".webp"}, ".png")
	if err != nil {
		t.Fatalf("NewAssetCacheService failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "1002.png"), []byte("avatar"), 0o644); err != nil {
		t.Fatalf("seed avatar cache failed: %v", err)
	}
	svc.SetAvatarCache(cache)

	created, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "头像同行"})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if _, err := svc.JoinSession(ctx, 1002, JoinCompanionSessionInput{JoinToken: created.Join.JoinToken}); err != nil {
		t.Fatalf("JoinSession returned error: %v", err)
	}

	page, err := svc.ListHistory(ctx, 1001, ListCompanionHistoryInput{Limit: 1})
	if err != nil {
		t.Fatalf("ListHistory returned error: %v", err)
	}
	if len(page.Items) != 1 || len(page.Items[0].Participants) != 2 {
		t.Fatalf("unexpected history page: %+v", page)
	}
	for _, p := range page.Items[0].Participants {
		if p.UserID == 1002 {
			if p.AvatarURL != "/api/v1/static/avatars/1002.png" {
				t.Fatalf("expected rewritten avatar url, got %q", p.AvatarURL)
			}
			return
		}
	}
	t.Fatalf("expected participant 1002 in history page: %+v", page.Items[0].Participants)
}

func TestAssetCacheEnsureCachedConcurrentSameKeySingleDownload(t *testing.T) {
	cacheDir := t.TempDir()
	cache, err := NewAssetCacheService(cacheDir, "/api/v1/static/avatars", []string{".png", ".jpg", ".jpeg", ".webp"}, ".png")
	if err != nil {
		t.Fatalf("NewAssetCacheService failed: %v", err)
	}
	downloader := &blockingAssetDownloader{started: make(chan struct{}), release: make(chan struct{})}
	cache.SetDownloader(downloader)

	const workers = 8
	results := make(chan string, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			results <- cache.EnsureCached(ctx, 1001, "1001", "https://track-avatar.oss-cn-beijing.aliyuncs.com/avatar/1001.png")
			errs <- ctx.Err()
		}()
	}

	select {
	case <-downloader.started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected downloader to start")
	}
	close(downloader.release)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("concurrent EnsureCached calls did not finish")
	}
	close(results)
	close(errs)

	if downloader.called.Load() != 1 {
		t.Fatalf("expected downloader called once, got %d", downloader.called.Load())
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("unexpected context error: %v", err)
		}
	}
	for result := range results {
		if result != "/api/v1/static/avatars/1001.png" {
			t.Fatalf("unexpected cached url: %q", result)
		}
	}
	if _, ok := cache.Exists("1001"); !ok {
		t.Fatalf("expected avatar cache file exists")
	}
}

func TestAssetCacheEnsureCachedRecoversPanicWithoutDeadlock(t *testing.T) {
	cacheDir := t.TempDir()
	cache, err := NewAssetCacheService(cacheDir, "/api/v1/static/avatars", []string{".png", ".jpg", ".jpeg", ".webp"}, ".png")
	if err != nil {
		t.Fatalf("NewAssetCacheService failed: %v", err)
	}
	downloader := &panicAssetDownloader{}
	cache.SetDownloader(downloader)

	const workers = 4
	results := make(chan string, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			results <- cache.EnsureCached(ctx, 1001, "1002", "https://track-avatar.oss-cn-beijing.aliyuncs.com/avatar/1002.png")
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("EnsureCached should not deadlock when downloader panics")
	}
	close(results)

	if downloader.called.Load() < 1 {
		t.Fatalf("expected downloader to be called at least once")
	}
	for result := range results {
		if result != "" {
			t.Fatalf("expected empty result on panic, got %q", result)
		}
	}
}

// rawPublishedMessage 用于 danmaku 等非 ControlEvent 的广播 payload，
// 因为 mockCompanionControlPublisher 会强制把 payload 反序列化成 CompanionControlEvent，
// 这里用一个独立的“原始 bytes”记录器以避免类型耦合。
type rawPublishedMessage struct {
	topic   string
	payload []byte
}

type rawMockPublisher struct {
	messages []rawPublishedMessage
	err      error
}

func (m *rawMockPublisher) Publish(_ context.Context, topic string, payload []byte) error {
	if m.err != nil {
		return m.err
	}
	clone := make([]byte, len(payload))
	copy(clone, payload)
	m.messages = append(m.messages, rawPublishedMessage{topic: topic, payload: clone})
	return nil
}

func (m *rawMockPublisher) Close() error { return nil }

// danmakuTestSetup 创建一个 owner 已 create + member 已 join 的会话，
// 并为指定 user 颁发 MQTT 凭证，供 ingest 测试用 client_id/username 复核 principal。
func danmakuTestSetup(t *testing.T, svc *CompanionService, ownerID, joinerID int64) (sessionID string, joinerCreds *CompanionMQTTCredentials) {
	t.Helper()
	svc.SetMQTTOptions(CompanionMQTTOptions{
		BrokerURL:        "mqtt://127.0.0.1:1883",
		TopicPrefix:      "companion",
		CredentialSecret: "test_companion_secret",
	})
	ctx := context.Background()
	created, err := svc.CreateSession(ctx, ownerID, CreateCompanionSessionInput{Title: "danmaku"})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if _, err := svc.JoinSession(ctx, joinerID, JoinCompanionSessionInput{JoinToken: created.Join.JoinToken}); err != nil {
		t.Fatalf("JoinSession failed: %v", err)
	}
	creds, err := svc.IssueMQTTCredentials(ctx, joinerID, created.Session.SessionID)
	if err != nil {
		t.Fatalf("IssueMQTTCredentials failed: %v", err)
	}
	return created.Session.SessionID, creds
}

func TestCompanionServiceIngestDanmakuPublishesBroadcast(t *testing.T) {
	svc, _, _ := newCompanionServiceForTest(t)
	pub := &rawMockPublisher{}
	svc.SetDanmakuPublisher(pub)
	sessionID, creds := danmakuTestSetup(t, svc, 1001, 1002)

	if err := svc.IngestDanmakuFromMQTT(context.Background(), CompanionMQTTDanmakuIngestInput{
		SessionID: sessionID,
		UserID:    1002,
		Content:   "hello world",
		ClientID:  creds.ClientID,
		Username:  creds.Username,
	}); err != nil {
		t.Fatalf("IngestDanmakuFromMQTT returned error: %v", err)
	}
	if len(pub.messages) != 1 {
		t.Fatalf("expected 1 published broadcast, got %d", len(pub.messages))
	}
	expectedTopic := "companion/" + sessionID + "/danmaku"
	if pub.messages[0].topic != expectedTopic {
		t.Fatalf("unexpected broadcast topic %q (want %q)", pub.messages[0].topic, expectedTopic)
	}
	var event CompanionDanmakuBroadcast
	if err := json.Unmarshal(pub.messages[0].payload, &event); err != nil {
		t.Fatalf("unmarshal danmaku payload failed: %v", err)
	}
	if event.SessionID != sessionID || event.UserID != 1002 || event.Content != "hello world" {
		t.Fatalf("unexpected broadcast payload: %+v", event)
	}
	if event.MessageID == 0 {
		t.Fatalf("expected non-zero message_id assigned by repo")
	}
}

func TestCompanionServiceIngestDanmakuRejectsPrincipalMismatch(t *testing.T) {
	svc, _, _ := newCompanionServiceForTest(t)
	pub := &rawMockPublisher{}
	svc.SetDanmakuPublisher(pub)
	sessionID, creds := danmakuTestSetup(t, svc, 1001, 1002)

	// principal 绑定的是 1002，但 ingest 声明 user_id=1001 -> 拒绝。
	err := svc.IngestDanmakuFromMQTT(context.Background(), CompanionMQTTDanmakuIngestInput{
		SessionID: sessionID,
		UserID:    1001,
		Content:   "spoofed",
		ClientID:  creds.ClientID,
		Username:  creds.Username,
	})
	if err == nil {
		t.Fatalf("expected forbidden error for principal mismatch, got nil")
	}
	if len(pub.messages) != 0 {
		t.Fatalf("expected no broadcast for rejected ingest, got %d", len(pub.messages))
	}
}

func TestCompanionServiceIngestDanmakuRejectsOversizedContent(t *testing.T) {
	svc, _, _ := newCompanionServiceForTest(t)
	pub := &rawMockPublisher{}
	svc.SetDanmakuPublisher(pub)
	sessionID, creds := danmakuTestSetup(t, svc, 1001, 1002)

	long := make([]rune, companionDanmakuMaxContentLength+1)
	for i := range long {
		long[i] = '中'
	}
	err := svc.IngestDanmakuFromMQTT(context.Background(), CompanionMQTTDanmakuIngestInput{
		SessionID: sessionID,
		UserID:    1002,
		Content:   string(long),
		ClientID:  creds.ClientID,
		Username:  creds.Username,
	})
	if err == nil {
		t.Fatalf("expected oversized content to be rejected")
	}
	if len(pub.messages) != 0 {
		t.Fatalf("expected no broadcast for rejected ingest, got %d", len(pub.messages))
	}
}

func TestCompanionServiceIngestDanmakuEnforcesRateLimit(t *testing.T) {
	svc, _, _ := newCompanionServiceForTest(t)
	pub := &rawMockPublisher{}
	svc.SetDanmakuPublisher(pub)
	sessionID, creds := danmakuTestSetup(t, svc, 1001, 1002)

	for i := 0; i < companionDanmakuRateLimitMax; i++ {
		if err := svc.IngestDanmakuFromMQTT(context.Background(), CompanionMQTTDanmakuIngestInput{
			SessionID: sessionID,
			UserID:    1002,
			Content:   "msg",
			ClientID:  creds.ClientID,
			Username:  creds.Username,
		}); err != nil {
			t.Fatalf("ingest #%d failed: %v", i, err)
		}
	}
	if len(pub.messages) != companionDanmakuRateLimitMax {
		t.Fatalf("expected %d broadcasts, got %d", companionDanmakuRateLimitMax, len(pub.messages))
	}
	err := svc.IngestDanmakuFromMQTT(context.Background(), CompanionMQTTDanmakuIngestInput{
		SessionID: sessionID,
		UserID:    1002,
		Content:   "msg",
		ClientID:  creds.ClientID,
		Username:  creds.Username,
	})
	if err == nil {
		t.Fatalf("expected rate limit error, got nil")
	}
	if len(pub.messages) != companionDanmakuRateLimitMax {
		t.Fatalf("expected broadcast count to remain %d after rate limit, got %d", companionDanmakuRateLimitMax, len(pub.messages))
	}
}

func TestCompanionServiceIngestDanmakuRejectsSensitiveWord(t *testing.T) {
	svc, _, _ := newCompanionServiceForTest(t)
	pub := &rawMockPublisher{}
	svc.SetDanmakuPublisher(pub)
	sessionID, creds := danmakuTestSetup(t, svc, 1001, 1002)

	// "badword" / "test_block_word" 来自内置词库占位项，命中即整条拒绝。
	err := svc.IngestDanmakuFromMQTT(context.Background(), CompanionMQTTDanmakuIngestInput{
		SessionID: sessionID,
		UserID:    1002,
		Content:   "hello BadWord here", // 大小写不敏感
		ClientID:  creds.ClientID,
		Username:  creds.Username,
	})
	if err == nil {
		t.Fatalf("expected sensitive word rejection, got nil")
	}
	if err.Error() != "content contains sensitive content" {
		t.Fatalf("unexpected error message: %v", err)
	}
	if len(pub.messages) != 0 {
		t.Fatalf("expected no broadcast for rejected ingest, got %d", len(pub.messages))
	}
}

func TestCompanionServiceIngestDanmakuRejectsWhenDisabled(t *testing.T) {
	svc, _, _ := newCompanionServiceForTest(t)
	pub := &rawMockPublisher{}
	svc.SetDanmakuPublisher(pub)
	ctrl := &mockCompanionControlPublisher{}
	svc.SetControlPublisher(ctrl)
	sessionID, creds := danmakuTestSetup(t, svc, 1001, 1002)

	// owner 关闭弹幕开关。
	if _, err := svc.SetSessionDanmakuEnabled(context.Background(), 1001, sessionID, false); err != nil {
		t.Fatalf("SetSessionDanmakuEnabled returned error: %v", err)
	}

	err := svc.IngestDanmakuFromMQTT(context.Background(), CompanionMQTTDanmakuIngestInput{
		SessionID: sessionID,
		UserID:    1002,
		Content:   "hi",
		ClientID:  creds.ClientID,
		Username:  creds.Username,
	})
	if err == nil {
		t.Fatalf("expected danmaku disabled error, got nil")
	}
	if err.Error() != "danmaku disabled" {
		t.Fatalf("unexpected error message: %v", err)
	}
	if len(pub.messages) != 0 {
		t.Fatalf("expected no broadcast when danmaku disabled, got %d", len(pub.messages))
	}
}

func TestCompanionServiceIngestDanmakuEnforcesSessionRateLimit(t *testing.T) {
	svc, repo, _ := newCompanionServiceForTest(t)
	pub := &rawMockPublisher{}
	svc.SetDanmakuPublisher(pub)
	sessionID, creds := danmakuTestSetup(t, svc, 1001, 1002)

	// 直接预置上限条数的 session 记录 (UserID=9999 非 ingest 调用方，
	// 避免触发单成员限速)，之后下一条由当前成员发起即触发 session 级限速。
	now := time.Now()
	for i := 0; i < companionDanmakuSessionRateLimitMax; i++ {
		if err := repo.InsertDanmaku(context.Background(), &models.CompanionDanmaku{
			SessionID: sessionID,
			UserID:    9999,
			Content:   "preset",
			CreatedAt: now,
		}); err != nil {
			t.Fatalf("seed danmaku #%d failed: %v", i, err)
		}
	}

	err := svc.IngestDanmakuFromMQTT(context.Background(), CompanionMQTTDanmakuIngestInput{
		SessionID: sessionID,
		UserID:    1002,
		Content:   "msg",
		ClientID:  creds.ClientID,
		Username:  creds.Username,
	})
	if err == nil {
		t.Fatalf("expected session rate limit error, got nil")
	}
	if err.Error() != "session danmaku rate limit exceeded" {
		t.Fatalf("unexpected error message: %v", err)
	}
	if len(pub.messages) != 0 {
		t.Fatalf("expected no broadcast after session limit, got %d", len(pub.messages))
	}
}

func TestCompanionServiceSetSessionDanmakuEnabledOwnerOnly(t *testing.T) {
	svc, _, _ := newCompanionServiceForTest(t)
	ctrl := &mockCompanionControlPublisher{}
	svc.SetControlPublisher(ctrl)
	ctx := context.Background()

	created, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "toggle"})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if _, err := svc.JoinSession(ctx, 1002, JoinCompanionSessionInput{JoinToken: created.Join.JoinToken}); err != nil {
		t.Fatalf("JoinSession returned error: %v", err)
	}

	// 非 owner 调用 → forbidden。
	if _, err := svc.SetSessionDanmakuEnabled(ctx, 1002, created.Session.SessionID, false); err == nil {
		t.Fatalf("expected forbidden error for non-owner")
	} else if err != repository.ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if len(ctrl.messages) != 0 {
		t.Fatalf("expected no control event published for non-owner, got %d", len(ctrl.messages))
	}
}

func TestCompanionServiceSetSessionDanmakuEnabledNotActive(t *testing.T) {
	svc, _, _ := newCompanionServiceForTest(t)
	ctrl := &mockCompanionControlPublisher{}
	svc.SetControlPublisher(ctrl)
	ctx := context.Background()

	created, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "toggle"})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if err := svc.EndSession(ctx, 1001, created.Session.SessionID); err != nil {
		t.Fatalf("EndSession returned error: %v", err)
	}
	beforeCount := len(ctrl.messages)

	if _, err := svc.SetSessionDanmakuEnabled(ctx, 1001, created.Session.SessionID, false); err == nil {
		t.Fatalf("expected error when session ended")
	} else if err.Error() != "companion session already ended" {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ctrl.messages) != beforeCount {
		t.Fatalf("expected no extra control events, got delta=%d", len(ctrl.messages)-beforeCount)
	}
}

func TestCompanionServiceSetSessionDanmakuEnabledIdempotent(t *testing.T) {
	svc, _, _ := newCompanionServiceForTest(t)
	ctrl := &mockCompanionControlPublisher{}
	svc.SetControlPublisher(ctrl)
	ctx := context.Background()

	created, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "toggle"})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	// 默认 enabled=true，再次设置 true → 幂等，不广播。
	state, err := svc.SetSessionDanmakuEnabled(ctx, 1001, created.Session.SessionID, true)
	if err != nil {
		t.Fatalf("SetSessionDanmakuEnabled returned error: %v", err)
	}
	if state == nil || state.Session == nil || !state.Session.DanmakuEnabled {
		t.Fatalf("expected danmaku still enabled, got %+v", state)
	}
	if len(ctrl.messages) != 0 {
		t.Fatalf("expected no control event for idempotent toggle, got %d", len(ctrl.messages))
	}
}

func TestCompanionServiceSetSessionDanmakuEnabledPublishesControlEvent(t *testing.T) {
	svc, _, _ := newCompanionServiceForTest(t)
	ctrl := &mockCompanionControlPublisher{}
	svc.SetControlPublisher(ctrl)
	ctx := context.Background()

	created, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "toggle"})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	state, err := svc.SetSessionDanmakuEnabled(ctx, 1001, created.Session.SessionID, false)
	if err != nil {
		t.Fatalf("SetSessionDanmakuEnabled returned error: %v", err)
	}
	if state == nil || state.Session == nil || state.Session.DanmakuEnabled {
		t.Fatalf("expected danmaku disabled in returned state, got %+v", state)
	}
	if len(ctrl.messages) != 1 {
		t.Fatalf("expected 1 control event published, got %d", len(ctrl.messages))
	}
	msg := ctrl.messages[0]
	if msg.topic != "companion/"+created.Session.SessionID+"/control" {
		t.Fatalf("unexpected control topic %q", msg.topic)
	}
	if msg.event.Event != CompanionControlEventDanmakuToggled {
		t.Fatalf("unexpected control event: %+v", msg.event)
	}
	if msg.event.OperatorUserID != 1001 || msg.event.SessionID != created.Session.SessionID {
		t.Fatalf("unexpected event metadata: %+v", msg.event)
	}
	if msg.event.Reason != "danmaku_disabled" {
		t.Fatalf("unexpected reason: %q", msg.event.Reason)
	}
	if msg.event.Enabled == nil || *msg.event.Enabled != false {
		t.Fatalf("expected enabled=false in event, got %+v", msg.event.Enabled)
	}

	// 再次切到 true，应再发布一条事件，reason=danmaku_enabled。
	if _, err := svc.SetSessionDanmakuEnabled(ctx, 1001, created.Session.SessionID, true); err != nil {
		t.Fatalf("SetSessionDanmakuEnabled returned error: %v", err)
	}
	if len(ctrl.messages) != 2 {
		t.Fatalf("expected 2 control events after re-enable, got %d", len(ctrl.messages))
	}
	last := ctrl.messages[1]
	if last.event.Reason != "danmaku_enabled" {
		t.Fatalf("unexpected re-enable reason: %q", last.event.Reason)
	}
	if last.event.Enabled == nil || *last.event.Enabled != true {
		t.Fatalf("expected enabled=true in re-enable event, got %+v", last.event.Enabled)
	}
}

func TestCompanionServiceCreateAndListEvents(t *testing.T) {
	svc, _, _ := newCompanionServiceForTest(t)
	ctx := context.Background()
	created, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "events"})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if _, err := svc.JoinSession(ctx, 1002, JoinCompanionSessionInput{JoinToken: created.Join.JoinToken}); err != nil {
		t.Fatalf("JoinSession returned error: %v", err)
	}

	eventTime := created.Session.StartedAt.Add(time.Minute)
	event, err := svc.CreateEvent(ctx, 1001, created.Session.SessionID, CreateCompanionEventInput{
		EventType:     "member_disconnected",
		TargetUserID:  1002,
		Title:         "成员断线",
		Content:       "guest offline",
		EventTime:     eventTime,
		ClientEventID: "evt-1",
		Metadata:      json.RawMessage(`{"last_seen_seconds":31}`),
	})
	if err != nil {
		t.Fatalf("CreateEvent returned error: %v", err)
	}
	if event.ID <= 0 || event.Metadata == nil {
		t.Fatalf("unexpected event: %+v", event)
	}

	again, err := svc.CreateEvent(ctx, 1001, created.Session.SessionID, CreateCompanionEventInput{
		EventType:     "member_disconnected",
		ClientEventID: "evt-1",
	})
	if err != nil {
		t.Fatalf("CreateEvent idempotent retry returned error: %v", err)
	}
	if again.ID != event.ID {
		t.Fatalf("expected idempotent retry to return event %d, got %d", event.ID, again.ID)
	}

	page, err := svc.ListEvents(ctx, 1001, created.Session.SessionID, ListCompanionEventsInput{Limit: 1})
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != event.ID {
		t.Fatalf("unexpected events page: %+v", page)
	}
	if page.Items[0].Metadata == nil {
		t.Fatalf("expected metadata in event response")
	}
}

func TestCompanionServiceCreateEventOwnerOnly(t *testing.T) {
	svc, _, _ := newCompanionServiceForTest(t)
	ctx := context.Background()
	created, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "events"})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	_, err = svc.CreateEvent(ctx, 1002, created.Session.SessionID, CreateCompanionEventInput{
		EventType:     "notice_sent",
		ClientEventID: "evt-forbidden",
	})
	if !errors.Is(err, repository.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestCompanionServiceCreateEventLimit(t *testing.T) {
	svc, _, _ := newCompanionServiceForTest(t)
	ctx := context.Background()
	created, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "events"})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	for i := 0; i < companionEventMaxPerSession; i++ {
		_, err := svc.CreateEvent(ctx, 1001, created.Session.SessionID, CreateCompanionEventInput{
			EventType:     "custom",
			ClientEventID: "evt-limit-" + strconv.Itoa(i),
		})
		if err != nil {
			t.Fatalf("CreateEvent #%d returned error: %v", i, err)
		}
	}
	existing, err := svc.CreateEvent(ctx, 1001, created.Session.SessionID, CreateCompanionEventInput{
		EventType:     "custom",
		ClientEventID: "evt-limit-0",
	})
	if err != nil || existing == nil {
		t.Fatalf("expected idempotent retry to succeed after limit, got event=%+v err=%v", existing, err)
	}
	_, err = svc.CreateEvent(ctx, 1001, created.Session.SessionID, CreateCompanionEventInput{
		EventType:     "custom",
		ClientEventID: "evt-over-limit",
	})
	if err == nil || err.Error() != "companion event limit exceeded" {
		t.Fatalf("expected limit exceeded, got %v", err)
	}
}

func TestCompanionServiceListNearbySessions(t *testing.T) {
	svc, repo, userRepo := newCompanionServiceForTest(t)
	ctx := context.Background()

	// 准备远端房间的 owner 用户。
	if _, err := userRepo.CreateIfNotExists(ctx, &models.User{ID: 2001, Nickname: "远端 owner"}); err != nil {
		t.Fatalf("create far owner failed: %v", err)
	}

	// owner=1001 创建房间（近：北京天安门）。
	near, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "近的房间", Visibility: "public"})
	if err != nil {
		t.Fatalf("CreateSession near returned error: %v", err)
	}
	if err := repo.UpsertPosition(ctx, &models.CompanionLivePosition{
		SessionID:        near.Session.SessionID,
		UserID:           1001,
		Latitude:         39.9087,
		Longitude:        116.3975,
		CoordinateSystem: "wgs84",
		RecordedAt:       time.Now(),
		Source:           "test",
	}); err != nil {
		t.Fatalf("UpsertPosition near failed: %v", err)
	}

	// owner=2001 创建房间（远：上海外滩，距北京 ~1000km）。
	far, err := svc.CreateSession(ctx, 2001, CreateCompanionSessionInput{Title: "远的房间", Visibility: "public"})
	if err != nil {
		t.Fatalf("CreateSession far returned error: %v", err)
	}
	if err := repo.UpsertPosition(ctx, &models.CompanionLivePosition{
		SessionID:        far.Session.SessionID,
		UserID:           2001,
		Latitude:         31.2397,
		Longitude:        121.4990,
		CoordinateSystem: "wgs84",
		RecordedAt:       time.Now(),
		Source:           "test",
	}); err != nil {
		t.Fatalf("UpsertPosition far failed: %v", err)
	}

	// 查询者站在天安门附近，半径 5km：只应返回近的房间。
	page, err := svc.ListNearbySessions(ctx, 1002, ListCompanionNearbyInput{
		Latitude:  39.9090,
		Longitude: 116.3980,
	})
	if err != nil {
		t.Fatalf("ListNearbySessions returned error: %v", err)
	}
	if page == nil || len(page.Items) != 1 {
		t.Fatalf("expected 1 nearby item, got %+v", page)
	}
	item := page.Items[0]
	if item.SessionID != near.Session.SessionID {
		t.Fatalf("unexpected session_id: %q", item.SessionID)
	}
	if item.JoinToken == "" {
		t.Fatalf("expected join_token to be returned")
	}
	if item.Anchor == nil || item.Anchor.DistanceM <= 0 || item.Anchor.DistanceM > 5000 {
		t.Fatalf("unexpected anchor: %+v", item.Anchor)
	}
	if item.MaxMembers == 0 {
		t.Fatalf("expected max_members to be set, got %d", item.MaxMembers)
	}
	if item.MemberCount != 1 || len(item.Members) != 1 || item.Members[0].Role != models.CompanionMemberRoleOwner {
		t.Fatalf("unexpected members: count=%d, %+v", item.MemberCount, item.Members)
	}
	if page.RadiusM != defaultCompanionNearbyRadiusMeters {
		t.Fatalf("unexpected default radius: %v", page.RadiusM)
	}

	// 扩大半径到 2000km：远房间也应被包含。
	page, err = svc.ListNearbySessions(ctx, 1002, ListCompanionNearbyInput{
		Latitude:     39.9090,
		Longitude:    116.3980,
		RadiusMeters: maxCompanionNearbyRadiusMeters,
	})
	if err != nil {
		t.Fatalf("ListNearbySessions wide returned error: %v", err)
	}
	// 上海距离北京约 1000km，远超 maxCompanionNearbyRadiusMeters(20km)，仍应被过滤。
	if len(page.Items) != 1 {
		t.Fatalf("expected only the near item within max radius, got %d", len(page.Items))
	}
	if page.RadiusM != maxCompanionNearbyRadiusMeters {
		t.Fatalf("expected radius capped to max, got %v", page.RadiusM)
	}
}

func TestCompanionServiceListNearbySessionsRejectsInvalidLatitude(t *testing.T) {
	svc, _, _ := newCompanionServiceForTest(t)
	if _, err := svc.ListNearbySessions(context.Background(), 1001, ListCompanionNearbyInput{Latitude: 100}); err == nil {
		t.Fatalf("expected error for out-of-range latitude")
	}
}

func TestCompanionServiceListNearbySessionsSkipsWithoutOwnerPosition(t *testing.T) {
	svc, _, _ := newCompanionServiceForTest(t)
	ctx := context.Background()
	if _, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "未上传位置"}); err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	page, err := svc.ListNearbySessions(ctx, 1002, ListCompanionNearbyInput{Latitude: 39.9, Longitude: 116.4})
	if err != nil {
		t.Fatalf("ListNearbySessions returned error: %v", err)
	}
	if page == nil || len(page.Items) != 0 {
		t.Fatalf("expected empty page when owner has no position, got %+v", page)
	}
}

func TestCompanionServiceCreateSessionDefaultsPrivate(t *testing.T) {
	svc, _, _ := newCompanionServiceForTest(t)
	ctx := context.Background()
	state, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "默认私密"})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if state.Session.Visibility != models.CompanionSessionVisibilityPrivate {
		t.Fatalf("expected default visibility=private, got %q", state.Session.Visibility)
	}
}

func TestCompanionServiceCreateSessionRejectsInvalidVisibility(t *testing.T) {
	svc, _, _ := newCompanionServiceForTest(t)
	if _, err := svc.CreateSession(context.Background(), 1001, CreateCompanionSessionInput{Visibility: "secret"}); err == nil {
		t.Fatalf("expected error for invalid visibility")
	}
}

func TestCompanionServiceJoinSessionByIDPublic(t *testing.T) {
	svc, _, userRepo := newCompanionServiceForTest(t)
	ctx := context.Background()
	if _, err := userRepo.CreateIfNotExists(ctx, &models.User{ID: 1002, Nickname: "joiner"}); err != nil {
		t.Fatalf("create joiner failed: %v", err)
	}
	created, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "公开房间", Visibility: "public"})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	state, err := svc.JoinSession(ctx, 1002, JoinCompanionSessionInput{SessionID: created.Session.SessionID})
	if err != nil {
		t.Fatalf("JoinSession by session_id returned error: %v", err)
	}
	if state.Session.SessionID != created.Session.SessionID {
		t.Fatalf("unexpected session id after join")
	}
}

func TestCompanionServiceJoinSessionByIDPrivateForbidden(t *testing.T) {
	svc, _, userRepo := newCompanionServiceForTest(t)
	ctx := context.Background()
	if _, err := userRepo.CreateIfNotExists(ctx, &models.User{ID: 1002, Nickname: "joiner"}); err != nil {
		t.Fatalf("create joiner failed: %v", err)
	}
	created, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "私密房间"})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	_, err = svc.JoinSession(ctx, 1002, JoinCompanionSessionInput{SessionID: created.Session.SessionID})
	if err == nil {
		t.Fatalf("expected error joining private room by session_id")
	}
	// 仍可凭 join_token 加入私密房间。
	if _, err := svc.JoinSession(ctx, 1002, JoinCompanionSessionInput{JoinToken: created.Session.JoinToken}); err != nil {
		t.Fatalf("JoinSession by join_token to private room failed: %v", err)
	}
}

func TestCompanionServiceJoinSessionRequiresTokenOrID(t *testing.T) {
	svc, _, _ := newCompanionServiceForTest(t)
	if _, err := svc.JoinSession(context.Background(), 1001, JoinCompanionSessionInput{}); err == nil {
		t.Fatalf("expected error when neither join_token nor session_id provided")
	}
}

func TestCompanionServiceListNearbyExcludesPrivate(t *testing.T) {
	svc, repo, userRepo := newCompanionServiceForTest(t)
	ctx := context.Background()
	if _, err := userRepo.CreateIfNotExists(ctx, &models.User{ID: 2001, Nickname: "私密 owner"}); err != nil {
		t.Fatalf("create private owner failed: %v", err)
	}
	// 私密房间在附近不出现。
	private, err := svc.CreateSession(ctx, 2001, CreateCompanionSessionInput{Title: "私密房间"})
	if err != nil {
		t.Fatalf("CreateSession private returned error: %v", err)
	}
	if err := repo.UpsertPosition(ctx, &models.CompanionLivePosition{
		SessionID:        private.Session.SessionID,
		UserID:           2001,
		Latitude:         39.9087,
		Longitude:        116.3975,
		CoordinateSystem: "wgs84",
		RecordedAt:       time.Now(),
		Source:           "test",
	}); err != nil {
		t.Fatalf("UpsertPosition private failed: %v", err)
	}
	page, err := svc.ListNearbySessions(ctx, 1002, ListCompanionNearbyInput{Latitude: 39.9090, Longitude: 116.3980})
	if err != nil {
		t.Fatalf("ListNearbySessions returned error: %v", err)
	}
	if page == nil || len(page.Items) != 0 {
		t.Fatalf("expected nearby to exclude private rooms, got %+v", page)
	}
}
