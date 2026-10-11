package notification

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/internal/eventbus"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

func setupTestDB(t testing.TB) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatalf("automigrate models: %v", err)
	}
	return db
}

func TestMatchEventPattern(t *testing.T) {
	tests := []struct {
		pattern   string
		eventType string
		want      bool
	}{
		{"", "any", false},
		{"any", "", false},
		{"*", "anything", true},
		{"security.login_failed", "security.login_failed", true},
		{"security.login_failed", "security.login_succeeded", false},
		{"security.*", "security.login_failed", true},
		{"security.*", "security.auth.failed", true},
		{"security.*", "cron.job_failed", false},
		{"*.job_failed", "cron.job_failed", true},
		{"*.job_failed", "backup.job_failed", true},
		{"*.job_failed", "backup.success", false},
		// Edge cases that fall through path.Match (due to slashes or malformed patterns)
		{"security.*", "security", true},
		{"security.*", "security.auth/failed", true},
		{"*.job_failed", "job_failed", true},
		{"*.job_failed", "cron/backup.job_failed", true},
		{"sec[urity.*", "sec[urity.failed", true},
		{"*.fa[iled", "job.fa[iled", true},
		// Whitespace trimming
		{"  security.*  ", "  security.login_failed  ", true},
	}

	for _, tt := range tests {
		got := MatchEventPattern(tt.pattern, tt.eventType)
		if got != tt.want {
			t.Errorf("MatchEventPattern(%q, %q) = %v; want %v", tt.pattern, tt.eventType, got, tt.want)
		}
	}
}

func TestPreferencesStoreAndResolution(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	userID := "user_42"

	// 1. Unconfigured preferences -> fallback to DefaultChannels
	channels, err := ResolveChannels(ctx, db, userID, "security.login_failed")
	if err != nil {
		t.Fatalf("ResolveChannels failed: %v", err)
	}
	if len(channels) != 2 || channels[0] != "in_app" || channels[1] != "email" {
		t.Errorf("expected default channels [in_app email], got %v", channels)
	}

	// 2. Save preferences
	items := []PreferenceItem{
		{EventPattern: "*", Channels: []string{"in_app"}},
		{EventPattern: "security.*", Channels: []string{"email", "telegram"}},
		{EventPattern: "security.critical", Channels: []string{"email", "sms"}},
	}
	if err := SavePreferences(ctx, db, userID, items); err != nil {
		t.Fatalf("SavePreferences failed: %v", err)
	}

	// Read back
	prefs, err := GetPreferences(ctx, db, userID)
	if err != nil {
		t.Fatalf("GetPreferences failed: %v", err)
	}
	if len(prefs) != 3 {
		t.Fatalf("expected 3 preferences, got %d", len(prefs))
	}

	// Exact match wins
	ch, err := ResolveChannels(ctx, db, userID, "security.critical")
	if err != nil || len(ch) != 2 || ch[0] != "email" || ch[1] != "sms" {
		t.Errorf("expected [email sms] for security.critical, got %v (err=%v)", ch, err)
	}

	// Prefix glob match wins over universal wildcard
	ch, err = ResolveChannels(ctx, db, userID, "security.login_failed")
	if err != nil || len(ch) != 2 || ch[0] != "email" || ch[1] != "telegram" {
		t.Errorf("expected [email telegram] for security.login_failed, got %v (err=%v)", ch, err)
	}

	// Non-security event matches universal wildcard "*"
	ch, err = ResolveChannels(ctx, db, userID, "cron.daily_backup")
	if err != nil || len(ch) != 1 || ch[0] != "in_app" {
		t.Errorf("expected [in_app] for cron event, got %v (err=%v)", ch, err)
	}

	// Update existing pattern
	updateItems := []PreferenceItem{
		{EventPattern: "security.*", Channels: []string{"in_app", "email", "slack"}},
	}
	if err := SavePreferences(ctx, db, userID, updateItems); err != nil {
		t.Fatalf("update SavePreferences failed: %v", err)
	}

	ch, err = ResolveChannels(ctx, db, userID, "security.login_failed")
	if err != nil || len(ch) != 3 || ch[2] != "slack" {
		t.Errorf("expected updated channels [in_app email slack], got %v", ch)
	}

	// Nil / empty checks
	if _, err := GetPreferences(ctx, nil, userID); err == nil {
		t.Error("expected error with nil db in GetPreferences")
	}
	if err := SavePreferences(ctx, nil, userID, items); err == nil {
		t.Error("expected error with nil db in SavePreferences")
	}
	if err := SavePreferences(ctx, db, "", items); err == nil {
		t.Error("expected error with empty userID in SavePreferences")
	}
}

type mockChannel struct {
	name      string
	disp      string
	schema    string
	sendFunc  func(ctx context.Context, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error
	sendCount int32
}

func (m *mockChannel) Name() string        { return m.name }
func (m *mockChannel) DisplayName() string { return m.disp }
func (m *mockChannel) ConfigSchema() json.RawMessage {
	if m.schema == "" {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(m.schema)
}
func (m *mockChannel) Send(ctx context.Context, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error {
	atomic.AddInt32(&m.sendCount, 1)
	if m.sendFunc != nil {
		return m.sendFunc(ctx, recipient, payload)
	}
	return nil
}

func TestNewDispatcher(t *testing.T) {
	d := NewDispatcher()

	if d.maxRetries != 3 {
		t.Errorf("expected maxRetries to be 3, got %d", d.maxRetries)
	}
	if d.baseBackoff != 50*time.Millisecond {
		t.Errorf("expected baseBackoff to be 50ms, got %v", d.baseBackoff)
	}
	if d.sleepFunc == nil {
		t.Errorf("expected sleepFunc to be set, got nil")
	}
}

func TestDispatcher_RetryAndBackoff(t *testing.T) {
	d := NewDispatcher()
	d.SetBaseBackoff(1 * time.Millisecond) // fast for testing
	d.SetMaxRetries(3)

	var failures int32
	mockCh := &mockChannel{
		name: "flaky",
		disp: "Flaky Channel",
		sendFunc: func(ctx context.Context, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error {
			f := atomic.AddInt32(&failures, 1)
			if f < 3 {
				return errors.New("transient network timeout")
			}
			return nil // succeed on attempt 3
		},
	}

	rec := plugin.NotificationRecipient{UserID: "1", OrgID: "1"}
	payload := plugin.NotificationPayload{EventType: "test.retry", Title: "Retry Test"}

	// Synchronous delivery with retries
	err := d.DispatchSync(context.Background(), mockCh, rec, payload)
	if err != nil {
		t.Fatalf("expected eventual success on 3rd attempt, got %v", err)
	}
	if atomic.LoadInt32(&mockCh.sendCount) != 3 {
		t.Errorf("expected 3 attempts, got %d", mockCh.sendCount)
	}

	// Test permanent failure callback
	var permanentErr error
	var failMu sync.Mutex
	d.SetOnFailure(func(err error, ch plugin.NotificationChannel, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) {
		failMu.Lock()
		permanentErr = err
		failMu.Unlock()
	})

	failingCh := &mockChannel{
		name: "broken",
		sendFunc: func(ctx context.Context, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error {
			return errors.New("permanent server crash")
		},
	}

	err = d.DispatchSync(context.Background(), failingCh, rec, payload)
	if err == nil {
		t.Fatal("expected error on permanent failure")
	}
	if atomic.LoadInt32(&failingCh.sendCount) != 3 {
		t.Errorf("expected 3 attempts for permanent failure, got %d", failingCh.sendCount)
	}
	failMu.Lock()
	if permanentErr == nil || permanentErr.Error() != "permanent server crash" {
		t.Errorf("unexpected permanent error recorded: %v", permanentErr)
	}
	failMu.Unlock()

	// Test asynchronous Dispatch with Wait()
	asyncCh := &mockChannel{name: "async"}
	d.Dispatch(context.Background(), asyncCh, rec, payload)
	d.Wait()
	if atomic.LoadInt32(&asyncCh.sendCount) != 1 {
		t.Errorf("expected 1 async delivery, got %d", asyncCh.sendCount)
	}

	// Dispatch with nil channel
	d.Dispatch(context.Background(), nil, rec, payload)
	if err := d.DispatchSync(context.Background(), nil, rec, payload); err == nil {
		t.Error("expected error when dispatching to nil channel")
	}

	// Test context cancellation
	cancellingCtx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	if err := d.DispatchSync(cancellingCtx, failingCh, rec, payload); !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestNewInAppChannel(t *testing.T) {
	db := setupTestDB(t)
	ch := NewInAppChannel(db)
	if ch == nil {
		t.Fatal("expected non-nil InAppChannel")
	}
	if ch.db != db {
		t.Errorf("expected db field to be %v, got %v", db, ch.db)
	}

	nilDBCh := NewInAppChannel(nil)
	if nilDBCh == nil {
		t.Fatal("expected non-nil InAppChannel")
	}
	if nilDBCh.db != nil {
		t.Errorf("expected db field to be nil, got %v", nilDBCh.db)
	}
}

func TestInAppChannel(t *testing.T) {
	db := setupTestDB(t)
	ch := NewInAppChannel(db)

	if ch.Name() != "in_app" {
		t.Errorf("expected in_app, got %s", ch.Name())
	}
	if ch.DisplayName() != "In-App Notification" {
		t.Errorf("expected In-App Notification, got %s", ch.DisplayName())
	}
	if len(ch.ConfigSchema()) == 0 {
		t.Error("expected non-empty config schema")
	}

	// Subscribe to eventbus to verify SSE envelope publication
	subCh, cancel := eventbus.Subscribe(eventbus.SubscribeOpts{
		Keys: []string{"notification"},
	})
	defer cancel()

	rec := plugin.NotificationRecipient{
		UserID: "user_100",
		OrgID:  "1",
	}
	payload := plugin.NotificationPayload{
		EventType: "security.login_failed",
		Title:     "Suspicious login attempt",
		Body:      "Login failed from IP 1.2.3.4",
		Priority:  "high",
		Data:      map[string]interface{}{"ip": "1.2.3.4"},
	}

	if err := ch.Send(context.Background(), rec, payload); err != nil {
		t.Fatalf("in_app send failed: %v", err)
	}

	// Verify database record
	var notif models.Notification
	if err := db.Where("user_id = ?", "user_100").First(&notif).Error; err != nil {
		t.Fatalf("query notification from db failed: %v", err)
	}
	if notif.Title != "Suspicious login attempt" || notif.Priority != "high" {
		t.Errorf("unexpected notification row: %+v", notif)
	}
	if notif.ReadAt != nil {
		t.Errorf("expected read_at to be nil initially, got %v", notif.ReadAt)
	}

	// Verify envelope received on eventbus
	select {
	case env := <-subCh:
		if env.Key != "notification" {
			t.Errorf("expected key notification, got %s", env.Key)
		}
		if env.OrgID != 1 {
			t.Errorf("expected OrgID 1, got %d", env.OrgID)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for eventbus notification envelope")
	}

	// Test nil db error
	nilDBCh := NewInAppChannel(nil)
	if err := nilDBCh.Send(context.Background(), rec, payload); err == nil {
		t.Error("expected error with nil db")
	}
}

func TestNewEmailChannel(t *testing.T) {
	db := setupTestDB(t)

	var called1, called2 bool
	mockSender1 := func(ctx context.Context, orgID uint, to, subject, htmlBody, textBody string) error {
		called1 = true
		return nil
	}

	mockSender2 := func(ctx context.Context, orgID uint, to, subject, htmlBody, textBody string) error {
		called2 = true
		return nil
	}

	ch := NewEmailChannel(db, mockSender1)

	if ch.db != db {
		t.Errorf("expected db to be assigned correctly, got %v", ch.db)
	}

	if ch.sender != nil {
		_ = ch.sender(context.Background(), 1, "to", "sub", "html", "text")
	}
	if !called1 {
		t.Errorf("expected sender to be initialized to mockSender1")
	}

	ch.SetSender(mockSender2)

	if ch.sender != nil {
		_ = ch.sender(context.Background(), 1, "to", "sub", "html", "text")
	}
	if !called2 {
		t.Errorf("expected sender to be updated to mockSender2")
	}
}
