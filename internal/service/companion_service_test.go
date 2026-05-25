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
