package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
	if page1.Items[0].TrackType != "跑步" {
		t.Fatalf("expected active track_type=跑步, got %q", page1.Items[0].TrackType)
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
	if page2.Items[0].TrackType != "徒步" {
		t.Fatalf("expected ended track_type=徒步, got %q", page2.Items[0].TrackType)
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
