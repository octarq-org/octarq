package notification

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

// mockSlackChannel replicates the exact Pro slackChannel SPI driver contract
// and DB fallback behavior to verify fail-closed tenant isolation.
type mockSlackChannel struct {
	router *Router
	sent   map[uint][]string
	mu     sync.Mutex
}

var _ plugin.NotificationChannel = (*mockSlackChannel)(nil)

func (c *mockSlackChannel) Name() string                  { return "slack" }
func (c *mockSlackChannel) DisplayName() string           { return "Slack" }
func (c *mockSlackChannel) ConfigSchema() json.RawMessage { return nil }

func (c *mockSlackChannel) Send(ctx context.Context, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error {
	webhookURL := ""

	// 1. Direct config override
	if recipient.Config != nil {
		if v, ok := recipient.Config["webhook_url"].(string); ok && strings.TrimSpace(v) != "" {
			webhookURL = strings.TrimSpace(v)
		} else if v, ok := recipient.Config["webhookUrl"].(string); ok && strings.TrimSpace(v) != "" {
			webhookURL = strings.TrimSpace(v)
		}
	}

	// 2. Fallback to tenant's saved Slack channel settings in database
	if webhookURL == "" && c.router != nil && c.router.db != nil {
		targetOrg := strings.TrimSpace(recipient.OrgID)
		if targetOrg == "" {
			targetOrg = strings.TrimSpace(payload.OrgID)
		}
		if targetOrg != "" {
			if orgNum, err := strconv.ParseUint(targetOrg, 10, 64); err == nil && orgNum > 0 {
				var row models.NotificationChannel
				if err := c.router.db.WithContext(ctx).Where("owner_id = ? AND type = ? AND enabled = ?", orgNum, "slack", true).First(&row).Error; err == nil && row.Config != "" {
					plain, decErr := ConfigPlaintext(row.Config)
					if decErr == nil {
						var m struct {
							WebhookURL string `json:"webhookUrl"`
						}
						if json.Unmarshal([]byte(plain), &m) == nil && m.WebhookURL != "" {
							webhookURL = strings.TrimSpace(m.WebhookURL)
						}
					}
				}
			}
		}
	}

	if webhookURL == "" {
		return errors.New("slack: missing webhook url")
	}

	// Record the webhook URL invocation under the tenant ID parsed from targetOrg
	targetOrg := strings.TrimSpace(recipient.OrgID)
	if targetOrg == "" {
		targetOrg = strings.TrimSpace(payload.OrgID)
	}
	orgNum, _ := strconv.ParseUint(targetOrg, 10, 64)

	c.mu.Lock()
	if c.sent == nil {
		c.sent = make(map[uint][]string)
	}
	c.sent[uint(orgNum)] = append(c.sent[uint(orgNum)], webhookURL)
	c.mu.Unlock()

	return nil
}

// TestMultiTenant_WebhookIsolation verifies that SendDirect and WebhookChannel strictly
// isolate tenant webhook configurations and fail-closed when unconfigured or cross-tenant.
func TestMultiTenant_WebhookIsolation(t *testing.T) {
	SetConfigDecryptor(func(s string) (string, bool) { return s, true })
	t.Cleanup(func() { SetConfigDecryptor(nil) })

	db := setupTestDB(t)
	r := NewRouter(db)

	var hook1CallCount, hook2CallCount int32
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		atomic.AddInt32(&hook1CallCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv1.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		atomic.AddInt32(&hook2CallCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv2.Close()

	// Seed Webhook channel for Org 10
	cfg1, _ := json.Marshal(map[string]string{"url": srv1.URL})
	if err := db.Create(&models.NotificationChannel{
		OrgID:   10,
		Name:    "Org 10 Webhook",
		Type:    "webhook",
		Config:  string(cfg1),
		Enabled: true,
	}).Error; err != nil {
		t.Fatalf("failed to seed channel for Org 10: %v", err)
	}

	// Seed Webhook channel for Org 20
	cfg2, _ := json.Marshal(map[string]string{"url": srv2.URL})
	if err := db.Create(&models.NotificationChannel{
		OrgID:   20,
		Name:    "Org 20 Webhook",
		Type:    "webhook",
		Config:  string(cfg2),
		Enabled: true,
	}).Error; err != nil {
		t.Fatalf("failed to seed channel for Org 20: %v", err)
	}

	// 1. Tenant 10 triggers SendDirect using context OrgID 10
	ctx10 := plugin.WithOrgID(context.Background(), 10)
	if err := r.SendDirect(ctx10, "webhook", "{}", "Alert for Org 10"); err != nil {
		t.Fatalf("SendDirect for Org 10 failed: %v", err)
	}
	if atomic.LoadInt32(&hook1CallCount) != 1 {
		t.Errorf("expected 1 call to hook1, got %d", hook1CallCount)
	}
	if atomic.LoadInt32(&hook2CallCount) != 0 {
		t.Errorf("expected 0 calls to hook2, got %d", hook2CallCount)
	}

	// 2. Tenant 20 triggers SendDirect using context OrgID 20
	ctx20 := plugin.WithOrgID(context.Background(), 20)
	if err := r.SendDirect(ctx20, "webhook", "{}", "Alert for Org 20"); err != nil {
		t.Fatalf("SendDirect for Org 20 failed: %v", err)
	}
	if atomic.LoadInt32(&hook1CallCount) != 1 {
		t.Errorf("expected hook1 to remain at 1, got %d", hook1CallCount)
	}
	if atomic.LoadInt32(&hook2CallCount) != 1 {
		t.Errorf("expected 1 call to hook2, got %d", hook2CallCount)
	}

	// 3. Tenant 30 (unconfigured) must FAIL CLOSED and NEVER borrow Org 10, Org 20, or Org 1
	ctx30 := plugin.WithOrgID(context.Background(), 30)
	err := r.SendDirect(ctx30, "webhook", "{}", "Alert for Org 30")
	if err == nil || !strings.Contains(err.Error(), "missing webhook url") {
		t.Fatalf("expected missing webhook url error for unconfigured Org 30, got: %v", err)
	}

	// 4. SendDirect with no tenant context in ctx and empty config must FAIL CLOSED
	err = r.SendDirect(context.Background(), "webhook", "{}", "Alert with no org")
	if err == nil || !strings.Contains(err.Error(), "missing webhook url") {
		t.Fatalf("expected missing webhook url error when context has no org, got: %v", err)
	}

	// Verify no unintended webhook dispatches occurred
	if atomic.LoadInt32(&hook1CallCount) != 1 {
		t.Errorf("hook1 was illegally invoked, count=%d", hook1CallCount)
	}
	if atomic.LoadInt32(&hook2CallCount) != 1 {
		t.Errorf("hook2 was illegally invoked, count=%d", hook2CallCount)
	}
}

// TestMultiTenant_SlackChannelFailClosed verifies that custom SPI channels like Slack
// never borrow Org 1 configuration when invoked by other tenants or without explicit org context.
func TestMultiTenant_SlackChannelFailClosed(t *testing.T) {
	SetConfigDecryptor(func(s string) (string, bool) { return s, true })
	t.Cleanup(func() { SetConfigDecryptor(nil) })

	db := setupTestDB(t)
	r := NewRouter(db)

	slackDriver := &mockSlackChannel{router: r}
	if err := r.RegisterChannel(slackDriver); err != nil {
		t.Fatalf("failed to register mockSlackChannel: %v", err)
	}

	// Seed Org 1 (Default System Org) Slack config
	cfgOrg1, _ := json.Marshal(map[string]string{"webhookUrl": "https://hooks.slack.com/services/ORG1/HOOK"})
	if err := db.Create(&models.NotificationChannel{
		OrgID:   1,
		Name:    "Org 1 Slack",
		Type:    "slack",
		Config:  string(cfgOrg1),
		Enabled: true,
	}).Error; err != nil {
		t.Fatalf("failed to seed Slack channel for Org 1: %v", err)
	}

	// Seed Org 2 Slack config
	cfgOrg2, _ := json.Marshal(map[string]string{"webhookUrl": "https://hooks.slack.com/services/ORG2/HOOK"})
	if err := db.Create(&models.NotificationChannel{
		OrgID:   2,
		Name:    "Org 2 Slack",
		Type:    "slack",
		Config:  string(cfgOrg2),
		Enabled: true,
	}).Error; err != nil {
		t.Fatalf("failed to seed Slack channel for Org 2: %v", err)
	}

	// 1. Org 2 triggers alert via SendDirect with OrgID 2 in context
	ctx2 := plugin.WithOrgID(context.Background(), 2)
	if err := r.SendDirect(ctx2, "slack", "{}", "Critical alert for Org 2"); err != nil {
		t.Fatalf("SendDirect for Org 2 failed: %v", err)
	}
	slackDriver.mu.Lock()
	if len(slackDriver.sent[2]) != 1 || slackDriver.sent[2][0] != "https://hooks.slack.com/services/ORG2/HOOK" {
		t.Errorf("expected Org 2 hook to be used, got: %v", slackDriver.sent[2])
	}
	if len(slackDriver.sent[1]) != 0 {
		t.Errorf("Org 1 Slack hook was borrowed unexpectedly! sent=%v", slackDriver.sent[1])
	}
	slackDriver.mu.Unlock()

	// 2. Org 3 (has NO Slack configured) triggers alert
	// CRITICAL VULNERABILITY CHECK: Must NOT borrow Org 1 Slack hook!
	ctx3 := plugin.WithOrgID(context.Background(), 3)
	err := r.SendDirect(ctx3, "slack", "{}", "Critical alert for Org 3")
	if err == nil || !strings.Contains(err.Error(), "missing webhook url") {
		t.Fatalf("expected missing webhook url error for Org 3, got: %v", err)
	}
	slackDriver.mu.Lock()
	if len(slackDriver.sent[1]) != 0 {
		t.Fatalf("CRITICAL SECURITY VIOLATION: Org 1 Slack webhook was borrowed by Org 3!")
	}
	slackDriver.mu.Unlock()

	// 3. Unauthenticated / empty context without org must fail closed
	err = r.SendDirect(context.Background(), "slack", "{}", "Orphan alert")
	if err == nil || !strings.Contains(err.Error(), "missing webhook url") {
		t.Fatalf("expected missing webhook url error for empty context, got: %v", err)
	}
	slackDriver.mu.Lock()
	if len(slackDriver.sent[1]) != 0 {
		t.Fatalf("CRITICAL SECURITY VIOLATION: Org 1 Slack webhook was borrowed by empty context!")
	}
	slackDriver.mu.Unlock()

	// 4. SendDirectWithTenant explicitly targeting Org 1 succeeds for Org 1 only
	if err := r.SendDirectWithTenant(context.Background(), 1, "slack", "{}", "Legitimate Org 1 alert"); err != nil {
		t.Fatalf("SendDirectWithTenant for Org 1 failed: %v", err)
	}
	slackDriver.mu.Lock()
	if len(slackDriver.sent[1]) != 1 {
		t.Errorf("expected 1 delivery for Org 1, got %d", len(slackDriver.sent[1]))
	}
	slackDriver.mu.Unlock()
}

// TestMultiTenant_EmitAndPreferencesIsolation verifies that Emit routes to the correct
// tenant workspace members and uses that tenant's configured preferences and channels.
func TestMultiTenant_EmitAndPreferencesIsolation(t *testing.T) {
	db := setupTestDB(t)
	r := NewRouter(db)

	var org10Dispatched, org20Dispatched int32
	ch10 := &mockChannel{
		name: "channel_org10",
		sendFunc: func(ctx context.Context, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error {
			if recipient.OrgID != "10" {
				t.Errorf("expected recipient OrgID 10, got %q", recipient.OrgID)
			}
			atomic.AddInt32(&org10Dispatched, 1)
			return nil
		},
	}
	ch20 := &mockChannel{
		name: "channel_org20",
		sendFunc: func(ctx context.Context, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error {
			if recipient.OrgID != "20" {
				t.Errorf("expected recipient OrgID 20, got %q", recipient.OrgID)
			}
			atomic.AddInt32(&org20Dispatched, 1)
			return nil
		},
	}
	_ = r.RegisterChannel(ch10)
	_ = r.RegisterChannel(ch20)

	// Create users and workspace memberships
	user10 := models.User{ID: 101, Email: "user10@example.com"}
	user20 := models.User{ID: 201, Email: "user20@example.com"}
	db.Create(&user10)
	db.Create(&user20)
	db.Create(&models.OrgMember{UserID: 101, OrgID: 10, Role: "admin"})
	db.Create(&models.OrgMember{UserID: 201, OrgID: 20, Role: "admin"})

	// Configure preferences:
	// User 101 routes security alerts to channel_org10
	// User 201 routes security alerts to channel_org20
	_ = SavePreferences(context.Background(), db, "101", []PreferenceItem{
		{EventPattern: "security.*", Channels: []string{"channel_org10"}},
	})
	_ = SavePreferences(context.Background(), db, "201", []PreferenceItem{
		{EventPattern: "security.*", Channels: []string{"channel_org20"}},
	})

	// Emit security alert for Org 10
	ctx10 := plugin.WithOrgID(context.Background(), 10)
	err := r.Emit(ctx10, plugin.NotificationPayload{
		EventType: "security.login_failed",
		Title:     "Failed Login Attempt",
		Body:      "IP blocked",
		OrgID:     "10",
	})
	if err != nil {
		t.Fatalf("Emit for Org 10 failed: %v", err)
	}
	r.Wait()

	if atomic.LoadInt32(&org10Dispatched) != 1 {
		t.Errorf("expected 1 dispatch to Org 10 channel, got %d", org10Dispatched)
	}
	if atomic.LoadInt32(&org20Dispatched) != 0 {
		t.Errorf("expected 0 dispatches to Org 20 channel, got %d", org20Dispatched)
	}

	// Emit security alert for Org 20
	ctx20 := plugin.WithOrgID(context.Background(), 20)
	err = r.Emit(ctx20, plugin.NotificationPayload{
		EventType: "security.login_failed",
		Title:     "Failed Login Attempt",
		Body:      "IP blocked",
		OrgID:     "20",
	})
	if err != nil {
		t.Fatalf("Emit for Org 20 failed: %v", err)
	}
	r.Wait()

	if atomic.LoadInt32(&org10Dispatched) != 1 {
		t.Errorf("expected Org 10 channel dispatches to remain 1, got %d", org10Dispatched)
	}
	if atomic.LoadInt32(&org20Dispatched) != 1 {
		t.Errorf("expected 1 dispatch to Org 20 channel, got %d", org20Dispatched)
	}
}

func TestNotification_SendDirectTo_And_PackageEmit(t *testing.T) {
	db := setupTestDB(t)
	r := NewRouter(db)

	var receivedRecipient plugin.NotificationRecipient
	var receivedPayload plugin.NotificationPayload
	mockCh := &mockChannel{
		name: "test_direct",
		sendFunc: func(ctx context.Context, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error {
			receivedRecipient = recipient
			receivedPayload = payload
			return nil
		},
	}
	_ = r.RegisterChannel(mockCh)

	oldDefault := DefaultRouter()
	SetDefaultRouter(r)
	t.Cleanup(func() { SetDefaultRouter(oldDefault) })

	// Test r.SendDirectTo
	err := r.SendDirectTo(context.Background(), "test_direct", plugin.NotificationRecipient{
		UserID: "user_99",
		OrgID:  "12",
	}, plugin.NotificationPayload{
		Title: "Direct Alert",
		Body:  "Direct Body",
	})
	if err != nil {
		t.Fatalf("SendDirectTo failed: %v", err)
	}
	if receivedRecipient.UserID != "user_99" || receivedRecipient.OrgID != "12" {
		t.Errorf("SendDirectTo recipient mismatch: %+v", receivedRecipient)
	}
	if receivedPayload.Title != "Direct Alert" {
		t.Errorf("SendDirectTo payload mismatch: %+v", receivedPayload)
	}

	// Test package-level SendDirect
	ctxWithTenant := plugin.WithOrgID(context.Background(), 12)
	err = SendDirect(ctxWithTenant, "test_direct", `{"custom":"param"}`, "Package SendDirect msg")
	if err != nil {
		t.Fatalf("package SendDirect failed: %v", err)
	}
	if receivedRecipient.OrgID != "12" || receivedPayload.Body != "Package SendDirect msg" {
		t.Errorf("package SendDirect mismatch: %+v, %+v", receivedRecipient, receivedPayload)
	}

	// Test package-level SendDirectWithTenant
	err = SendDirectWithTenant(context.Background(), 15, "test_direct", `{}`, "Tenant msg")
	if err != nil {
		t.Fatalf("package SendDirectWithTenant failed: %v", err)
	}
	if receivedRecipient.OrgID != "15" || receivedPayload.Body != "Tenant msg" {
		t.Errorf("package SendDirectWithTenant mismatch: %+v, %+v", receivedRecipient, receivedPayload)
	}

	// Test package-level SendDirectTo
	err = SendDirectTo(context.Background(), "test_direct", plugin.NotificationRecipient{
		UserID: "user_100",
		OrgID:  "16",
	}, plugin.NotificationPayload{Body: "Pkg SendDirectTo"})
	if err != nil {
		t.Fatalf("package SendDirectTo failed: %v", err)
	}
	if receivedRecipient.UserID != "user_100" || receivedRecipient.OrgID != "16" {
		t.Errorf("package SendDirectTo mismatch: %+v", receivedRecipient)
	}

	// Test error when channel is not registered
	err = r.SendDirectTo(context.Background(), "non_existent_channel", plugin.NotificationRecipient{}, plugin.NotificationPayload{})
	if err == nil || (!strings.Contains(err.Error(), "channel not registered") && !strings.Contains(err.Error(), "unknown notification channel type")) {
		t.Errorf("expected unknown channel error, got %v", err)
	}
}

func TestNotification_PreferencesEdgeCases(t *testing.T) {
	db := setupTestDB(t)

	// Test SavePreferences errors
	if err := SavePreferences(context.Background(), nil, "user1", []PreferenceItem{}); err == nil {
		t.Error("expected error with nil db in SavePreferences")
	}
	if err := SavePreferences(context.Background(), db, "", []PreferenceItem{}); err == nil {
		t.Error("expected error with empty userID in SavePreferences")
	}

	// Test ResetPreferences
	if err := ResetPreferences(context.Background(), nil, "user1"); err == nil {
		t.Error("expected error with nil db in ResetPreferences")
	}
	if err := ResetPreferences(context.Background(), db, ""); err == nil {
		t.Error("expected error with empty userID in ResetPreferences")
	}

	_ = SavePreferences(context.Background(), db, "user1", []PreferenceItem{
		{EventPattern: "*", Channels: []string{"in_app"}},
	})
	if err := ResetPreferences(context.Background(), db, "user1"); err != nil {
		t.Fatalf("ResetPreferences failed: %v", err)
	}
	prefs, _ := GetPreferences(context.Background(), db, "user1")
	if len(prefs) != 0 {
		t.Errorf("expected 0 preferences after reset, got %d", len(prefs))
	}
}

func TestTelegramChannel_OrgPayloadFallback(t *testing.T) {
	db := setupTestDB(t)
	SetConfigDecryptor(func(s string) (string, bool) { return s, true })
	t.Cleanup(func() { SetConfigDecryptor(nil) })

	db.Create(&models.NotificationChannel{
		OrgID:   50,
		Type:    "telegram",
		Config:  `{"botToken":"invalid-token","chatId":"999"}`,
		Enabled: true,
	})

	ch := NewTelegramChannel(db)
	// targetOrg resolved from payload.OrgID when recipient.OrgID is empty
	err := ch.Send(context.Background(), plugin.NotificationRecipient{}, plugin.NotificationPayload{
		OrgID: "50",
		Body:  "alert",
	})
	if err == nil || !strings.Contains(err.Error(), "failed to initialize telegram notifier") {
		t.Errorf("expected initialization error resolving org from payload, got %v", err)
	}

	// unparseable orgID
	err = ch.Send(context.Background(), plugin.NotificationRecipient{OrgID: "abc"}, plugin.NotificationPayload{Body: "alert"})
	if err == nil || !strings.Contains(err.Error(), "missing telegram credentials") {
		t.Errorf("expected missing credentials for unparseable orgID, got %v", err)
	}
}
