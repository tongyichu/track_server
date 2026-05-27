package service

import (
	"context"
	"encoding/json"
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

func TestCompanionServiceEndPublishesSessionEnded(t *testing.T) {
	svc, _, _ := newCompanionServiceForTest(t)
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
}

func TestCompanionServiceAutoEndPublishesSessionEndedAfterLastLeave(t *testing.T) {
	svc, _, _ := newCompanionServiceForTest(t)
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

	created1, err := svc.CreateSession(ctx, 1001, CreateCompanionSessionInput{Title: "第一场"})
	if err != nil {
		t.Fatalf("CreateSession #1 returned error: %v", err)
	}
	if _, err := svc.JoinSession(ctx, 1002, JoinCompanionSessionInput{JoinToken: created1.Join.JoinToken}); err != nil {
		t.Fatalf("JoinSession #1 returned error: %v", err)
	}
	if err := svc.EndSession(ctx, 1001, created1.Session.SessionID); err != nil {
		t.Fatalf("EndSession #1 returned error: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	created2, err := svc.CreateSession(ctx, 1002, CreateCompanionSessionInput{Title: "第二场"})
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
	if page2.HasMore || page2.NextCursor != "" {
		t.Fatalf("expected page2 end, got %+v", page2)
	}

	if _, err := svc.ListHistory(ctx, 1001, ListCompanionHistoryInput{Cursor: "bad-cursor"}); err == nil {
		t.Fatalf("expected invalid cursor error")
	}
}
