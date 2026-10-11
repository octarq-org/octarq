package notification

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

func TestEmailChannel(t *testing.T) {
	db := setupTestDB(t)

	// Create user in DB
	user := models.User{
		ID:    7,
		Email: "alice@example.com",
	}
	db.Create(&user)

	var sentTo, sentSubject, sentText string
	var sentOrg uint
	mockSender := func(ctx context.Context, orgID uint, to, subject, htmlBody, textBody string) error {
		sentOrg = orgID
		sentTo = to
		sentSubject = subject
		sentText = textBody
		return nil
	}

	ch := NewEmailChannel(db, mockSender)
	if ch.Name() != "email" || ch.DisplayName() != "Email" {
		t.Errorf("unexpected name or display name: %s / %s", ch.Name(), ch.DisplayName())
	}
	if len(ch.ConfigSchema()) == 0 {
		t.Error("expected non-empty config schema")
	}

	// Send to user looked up from DB
	rec := plugin.NotificationRecipient{UserID: "7", OrgID: "2"}
	payload := plugin.NotificationPayload{
		EventType: "cron.backup_done",
		Title:     "Daily Backup Succeeded",
		Body:      "Backup completed in 4s",
	}

	if err := ch.Send(context.Background(), rec, payload); err != nil {
		t.Fatalf("send email failed: %v", err)
	}
	if sentTo != "alice@example.com" {
		t.Errorf("expected to alice@example.com, got %s", sentTo)
	}
	if sentSubject != "Daily Backup Succeeded" {
		t.Errorf("expected subject Daily Backup Succeeded, got %s", sentSubject)
	}
	if sentText != "Backup completed in 4s" {
		t.Errorf("expected text 'Backup completed in 4s', got %s", sentText)
	}
	if sentOrg != 2 {
		t.Errorf("expected org 2, got %d", sentOrg)
	}

	// Send with config email override
	recOverride := plugin.NotificationRecipient{
		UserID: "7",
		OrgID:  "1",
		Config: map[string]interface{}{"email": "override@example.com"},
	}
	if err := ch.Send(context.Background(), recOverride, payload); err != nil {
		t.Fatalf("send override email failed: %v", err)
	}
	if sentTo != "override@example.com" {
		t.Errorf("expected to override@example.com, got %s", sentTo)
	}

	// Failure when user not found and no override
	recNotFound := plugin.NotificationRecipient{UserID: "9999"}
	if err := ch.Send(context.Background(), recNotFound, payload); err == nil {
		t.Error("expected error when recipient email cannot be resolved")
	}

	// Failure when sender is nil
	chNoSender := NewEmailChannel(db, nil)
	if err := chNoSender.Send(context.Background(), recOverride, payload); err == nil {
		t.Error("expected error when sender is nil")
	}
}

func TestRouterAndEmit(t *testing.T) {
	db := setupTestDB(t)

	// Create users and org
	u1 := models.User{ID: 10, Email: "user10@example.com", IsInstanceAdmin: true}
	u2 := models.User{ID: 20, Email: "user20@example.com"}
	db.Create(&u1)
	db.Create(&u2)

	mem1 := models.OrgMember{OrgID: 5, UserID: 10, Role: "owner"}
	mem2 := models.OrgMember{OrgID: 5, UserID: 20, Role: "member"}
	db.Create(&mem1)
	db.Create(&mem2)

	router := NewRouter(db)
	router.Dispatcher().SetBaseBackoff(1 * time.Millisecond)

	// Test channel management
	channels := router.ListChannels()
	if len(channels) < 2 {
		t.Fatalf("expected at least 2 built-in channels, got %d", len(channels))
	}
	if _, ok := router.GetChannel("in_app"); !ok {
		t.Error("expected in_app channel registered")
	}
	if _, ok := router.GetChannel("email"); !ok {
		t.Error("expected email channel registered")
	} else if ec, ok := router.GetChannel("email"); ok {
		if emailCh, ok := ec.(*EmailChannel); ok {
			emailCh.SetSender(func(ctx context.Context, orgID uint, to, subject, htmlBody, textBody string) error {
				return nil
			})
		}
	}

	// Register custom channel
	customCh := &mockChannel{name: "webhook", disp: "Webhook"}
	if err := router.RegisterChannel(customCh); err != nil {
		t.Fatalf("RegisterChannel failed: %v", err)
	}
	if err := router.RegisterChannel(nil); err == nil {
		t.Error("expected error for nil channel")
	}
	if err := router.RegisterChannel(&mockChannel{name: ""}); err == nil {
		t.Error("expected error for empty channel name")
	}

	// Configure preferences for user 10
	_ = SavePreferences(context.Background(), db, "10", []PreferenceItem{
		{EventPattern: "security.*", Channels: []string{"webhook"}},
	})

	// 1. Emit to specific user
	payload := plugin.NotificationPayload{
		EventType: "security.token_revoked",
		Title:     "Token Revoked",
		Body:      "Your API token was revoked",
		UserID:    "10",
		OrgID:     "5",
	}
	if err := router.Emit(context.Background(), payload); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	router.Wait()
	if atomic.LoadInt32(&customCh.sendCount) != 1 {
		t.Errorf("expected 1 webhook delivery for user 10, got %d", customCh.sendCount)
	}

	// 2. Emit to Org (should fan out to both members)
	orgPayload := plugin.NotificationPayload{
		EventType: "billing.invoice_ready",
		Title:     "Invoice Ready",
		Body:      "Monthly invoice ready",
		OrgID:     "5",
	}
	if err := router.Emit(context.Background(), orgPayload); err != nil {
		t.Fatalf("Emit to org failed: %v", err)
	}
	router.Wait()

	// Verify in_app notifications created for user 20 (fallback DefaultChannels has in_app)
	var notifs []models.Notification
	db.Where("user_id = ?", "20").Find(&notifs)
	if len(notifs) != 1 {
		t.Errorf("expected 1 in_app notification for user 20, got %d", len(notifs))
	}

	// 3. Emit with empty EventType -> error
	if err := router.Emit(context.Background(), plugin.NotificationPayload{}); err == nil {
		t.Error("expected error with empty eventType")
	}
	if err := router.EmitTo(context.Background(), plugin.NotificationRecipient{UserID: "1"}, plugin.NotificationPayload{}); err == nil {
		t.Error("expected error with empty eventType in EmitTo")
	}

	// Test global DefaultRouter & Emit functions
	oldDefault := DefaultRouter()
	SetDefaultRouter(router)
	defer SetDefaultRouter(oldDefault)

	if DefaultRouter() != router {
		t.Error("SetDefaultRouter did not set default router")
	}

	if err := Emit(context.Background(), payload); err != nil {
		t.Fatalf("global Emit failed: %v", err)
	}
	router.Wait()

	if err := EmitTo(context.Background(), plugin.NotificationRecipient{UserID: "10", OrgID: "5"}, payload); err != nil {
		t.Fatalf("global EmitTo failed: %v", err)
	}
	router.Wait()
}

func TestRouter_EdgeCases(t *testing.T) {
	db := setupTestDB(t)
	d := NewDispatcher()
	d.SetMaxRetries(0)   // clamp to 1
	d.SetBaseBackoff(-1) // clamp to 0

	r := NewRouter(db, WithDispatcher(d), WithDispatcher(nil))
	if r.Dispatcher() != d {
		t.Error("expected custom dispatcher")
	}

	// Emit with no active channels
	emptyRouter := &Router{
		channels:   make(map[string]plugin.NotificationChannel),
		dispatcher: d,
	}
	err := emptyRouter.EmitTo(context.Background(), plugin.NotificationRecipient{UserID: "1"}, plugin.NotificationPayload{
		EventType: "test.event",
	})
	if err == nil {
		t.Error("expected error when no channels matched")
	}

	// Emit targeting payload.Data["userId"]
	var receivedUserID string
	testCh := &mockChannel{
		name: "test_target",
		sendFunc: func(ctx context.Context, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error {
			receivedUserID = recipient.UserID
			return nil
		},
	}
	_ = r.RegisterChannel(testCh)
	_ = SavePreferences(context.Background(), db, "99", []PreferenceItem{
		{EventPattern: "*", Channels: []string{"test_target"}},
	})

	err = r.Emit(context.Background(), plugin.NotificationPayload{
		EventType: "user.data_target",
		Data:      map[string]interface{}{"userId": "99"},
	})
	if err != nil {
		t.Fatalf("emit with data userId failed: %v", err)
	}
	r.Wait()
	if receivedUserID != "99" {
		t.Errorf("expected receivedUserID to be 99, got %s", receivedUserID)
	}

	// Emit targeting payload.Data["orgId"]
	orgUser := models.User{ID: 55, Email: "orguser@example.com"}
	db.Create(&orgUser)
	db.Create(&models.OrgMember{OrgID: 88, UserID: 55})
	_ = SavePreferences(context.Background(), db, "55", []PreferenceItem{
		{EventPattern: "*", Channels: []string{"test_target"}},
	})

	err = r.Emit(context.Background(), plugin.NotificationPayload{
		EventType: "org.data_target",
		Data:      map[string]interface{}{"orgId": "88"},
	})
	if err != nil {
		t.Fatalf("emit with data orgId failed: %v", err)
	}
	r.Wait()

	// Emit fallback to instance admin
	adminUser := models.User{ID: 77, Email: "admin77@example.com", IsInstanceAdmin: true}
	db.Create(&adminUser)
	_ = SavePreferences(context.Background(), db, "77", []PreferenceItem{
		{EventPattern: "*", Channels: []string{"test_target"}},
	})

	err = r.Emit(context.Background(), plugin.NotificationPayload{
		EventType: "admin.broadcast",
	})
	if err != nil {
		t.Fatalf("emit fallback to admin failed: %v", err)
	}
	r.Wait()

	// Emit fallback with no db
	noDBRouter := NewRouter(nil)
	var noDBReceived bool
	noDBCh := &mockChannel{
		name: "in_app",
		sendFunc: func(ctx context.Context, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error {
			noDBReceived = true
			return nil
		},
	}
	_ = noDBRouter.RegisterChannel(noDBCh)
	if err := noDBRouter.Emit(context.Background(), plugin.NotificationPayload{EventType: "nodb.event"}); err != nil {
		t.Fatalf("emit with no db failed: %v", err)
	}
	noDBRouter.Wait()
	if !noDBReceived {
		t.Error("expected noDB delivery to default recipient")
	}
}

func TestPreferences_SaveEmptyPatternAndFallback(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Empty pattern items are skipped
	items := []PreferenceItem{
		{EventPattern: "", Channels: []string{"email"}},
		{EventPattern: "valid.*", Channels: nil},
	}
	if err := SavePreferences(ctx, db, "123", items); err != nil {
		t.Fatalf("save with empty pattern failed: %v", err)
	}

	prefs, err := GetPreferences(ctx, db, "123")
	if err != nil || len(prefs) != 1 {
		t.Fatalf("expected 1 pref, got %v (err=%v)", prefs, err)
	}

	// ResolveChannels with empty result fallback
	ch, err := ResolveChannels(ctx, db, "123", "valid.foo")
	if err != nil || len(ch) != 2 || ch[0] != "in_app" || ch[1] != "email" {
		t.Errorf("expected fallback to default channels, got %v", ch)
	}
}

func TestNewRouter(t *testing.T) {
	db := setupTestDB(t)

	// Test default initialization
	r := NewRouter(db)
	if r == nil {
		t.Fatal("NewRouter returned nil")
	}

	// Verify built-in channels are registered
	if _, ok := r.GetChannel("in_app"); !ok {
		t.Error("expected in_app channel to be registered by default")
	}
	if _, ok := r.GetChannel("email"); !ok {
		t.Error("expected email channel to be registered by default")
	}

	// Verify default dispatcher is set
	if r.Dispatcher() == nil {
		t.Error("expected default dispatcher to be initialized")
	}

	// Test initialization with custom dispatcher option
	customDisp := NewDispatcher()
	rCustom := NewRouter(db, WithDispatcher(customDisp))
	if rCustom.Dispatcher() != customDisp {
		t.Error("expected custom dispatcher to be set via WithDispatcher option")
	}

	// Test WithDispatcher with nil dispatcher (should not override default)
	rNilDisp := NewRouter(db, WithDispatcher(nil))
	if rNilDisp.Dispatcher() == nil {
		t.Error("expected dispatcher not to be nil when WithDispatcher(nil) is passed")
	}
}

func TestWithDispatcher(t *testing.T) {
	db := setupTestDB(t)

	// Verify default dispatcher is set when no options are provided
	rDefault := NewRouter(db)
	if rDefault.Dispatcher() == nil {
		t.Error("NewRouter should initialize a default dispatcher")
	}

	// Verify WithDispatcher correctly overwrites the dispatcher
	customDispatcher := NewDispatcher()
	rCustom := NewRouter(db, WithDispatcher(customDispatcher))
	if rCustom.Dispatcher() != customDispatcher {
		t.Error("WithDispatcher should set the custom dispatcher")
	}

	// Verify WithDispatcher(nil) does not overwrite an existing dispatcher
	rNil := NewRouter(db, WithDispatcher(customDispatcher), WithDispatcher(nil))
	if rNil.Dispatcher() != customDispatcher {
		t.Error("WithDispatcher(nil) should not overwrite an existing dispatcher")
	}
	if rNil.Dispatcher() == nil {
		t.Error("dispatcher should not be nil")
	}
}
