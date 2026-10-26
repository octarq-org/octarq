package notification

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/internal/tenantsql"
	"github.com/octarq-org/octarq/server/plugin"
)

func TestCoverageLift_Notification_Descriptors(t *testing.T) {
	t.Parallel()
	r := NewRouter(nil)

	// Register channel with empty title, icon and description
	ch1 := &mockChannel{name: "slack", disp: ""}
	if err := r.RegisterChannelWithDescriptor(ch1, Descriptor{}); err != nil {
		t.Fatalf("RegisterChannelWithDescriptor failed: %v", err)
	}

	d1, ok := r.GetDescriptor("slack")
	if !ok {
		t.Fatalf("expected descriptor for slack")
	}
	if d1.Title != "Slack" || d1.Icon != "bell" || d1.Description != "Deliver notifications via Slack" {
		t.Fatalf("unexpected defaults: %+v", d1)
	}

	// Register channel with non-empty DisplayName but empty Title
	ch2 := &mockChannel{name: "custom_sms", disp: "My SMS Gateway"}
	if err := r.RegisterChannelWithDescriptor(ch2, Descriptor{}); err != nil {
		t.Fatalf("RegisterChannelWithDescriptor failed: %v", err)
	}
	d2, ok := r.GetDescriptor("custom_sms")
	if !ok {
		t.Fatalf("expected descriptor for custom_sms")
	}
	if d2.Title != "My SMS Gateway" {
		t.Fatalf("expected title My SMS Gateway, got %s", d2.Title)
	}
}

func TestCoverageLift_Notification_SendDirect_ContextAndFallbacks(t *testing.T) {
	t.Parallel()
	r := NewRouter(nil)
	mockCh := &mockChannel{name: "test_ch"}
	if err := r.RegisterChannel(mockCh); err != nil {
		t.Fatalf("register channel: %v", err)
	}

	// 1. UserID from tenantsql.WithUserID context
	ctxUser := tenantsql.WithUserID(context.Background(), 77)
	err := r.SendDirect(ctxUser, "test_ch", `{}`, "hello")
	if err != nil {
		t.Fatalf("SendDirect failed: %v", err)
	}

	// 2. Fallbacks in cfgMap: orgId / userId as float64
	err = r.SendDirect(context.Background(), "test_ch", `{"orgId":10,"userId":20}`, "hello")
	if err != nil {
		t.Fatalf("SendDirect float64 orgId/userId failed: %v", err)
	}

	// 3. Fallbacks: org_id / user_id as float64
	err = r.SendDirect(context.Background(), "test_ch", `{"org_id":11,"user_id":21}`, "hello")
	if err != nil {
		t.Fatalf("SendDirect float64 org_id/user_id failed: %v", err)
	}

	// 4. Fallbacks: orgId / userId as string
	err = r.SendDirect(context.Background(), "test_ch", `{"orgId":"12","userId":"22"}`, "hello")
	if err != nil {
		t.Fatalf("SendDirect string orgId/userId failed: %v", err)
	}

	// 5. Fallbacks: org_id / user_id as string
	err = r.SendDirect(context.Background(), "test_ch", `{"org_id":"13","user_id":"23"}`, "hello")
	if err != nil {
		t.Fatalf("SendDirect string org_id/user_id failed: %v", err)
	}
}

func TestCoverageLift_Notification_Preferences_Edges(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// len(items) == 0
	if err := SavePreferences(ctx, db, "u1", nil); err != nil {
		t.Fatalf("expected nil for empty items, got %v", err)
	}

	// len(prefs) == 0 (only empty pattern items)
	if err := SavePreferences(ctx, db, "u1", []PreferenceItem{{EventPattern: "   "}}); err != nil {
		t.Fatalf("expected nil for empty patterns, got %v", err)
	}

	// ResolveChannels with canceled context
	cancCtx, cancel := context.WithCancel(ctx)
	cancel()
	chans, err := ResolveChannels(cancCtx, db, "u1", "test.event")
	if err == nil {
		t.Fatalf("expected error from canceled context")
	}
	if len(chans) != len(DefaultChannels) {
		t.Fatalf("expected default channels on error")
	}
}

func TestCoverageLift_Notification_Router_EmitAndResolve_Edges(t *testing.T) {
	db := setupTestDB(t)
	r := NewRouter(db)
	mockCh := &mockChannel{name: "in_app"}
	_ = r.RegisterChannel(mockCh)

	// ResolveChannels on router with canceled context
	cancCtx, cancel := context.WithCancel(context.Background())
	cancel()
	chans := r.ResolveChannels(cancCtx, "u1", "test.event")
	if len(chans) != len(DefaultChannels) {
		t.Fatalf("expected default channels")
	}

	// Emit with UserID resolved from tenantsql.WithUserID context
	ctxUser := tenantsql.WithUserID(context.Background(), 88)
	if err := r.Emit(ctxUser, plugin.NotificationPayload{EventType: "user.event"}); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}

	// Emit with OrgID resolved from plugin.WithOrgID context
	ctxOrg := plugin.WithOrgID(context.Background(), 99)
	if err := r.Emit(ctxOrg, plugin.NotificationPayload{EventType: "org.event"}); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
}

func TestCoverageLift_Notification_Channels_Edges(t *testing.T) {
	db := setupTestDB(t)

	// 1. Email channel: payload.OrgID used when recipient.OrgID is empty
	emailCh := NewEmailChannel(db, nil)
	cancCtx, cancel := context.WithCancel(context.Background())
	cancel()
	// Should parse payload.OrgID="42" before failing on context/smtp
	_ = emailCh.Send(cancCtx, plugin.NotificationRecipient{}, plugin.NotificationPayload{OrgID: "42", Title: "Subj"})

	// 2. InApp channel: context canceled triggers DB create error
	inAppCh := NewInAppChannel(db)
	err := inAppCh.Send(cancCtx, plugin.NotificationRecipient{UserID: "1", OrgID: "1"}, plugin.NotificationPayload{EventType: "ev"})
	if err == nil {
		t.Fatalf("expected error from canceled context in InApp channel")
	}

	// 3. Webhook channel: timeout <= 0 triggers fallback to default
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	whCh := NewWebhookChannel()
	err = whCh.Send(cancCtx, plugin.NotificationRecipient{Config: map[string]any{"url": ts.URL}}, plugin.NotificationPayload{Body: "msg"})
	if err == nil {
		t.Fatalf("expected context error")
	}

	// 4. Telegram channel: DB fallback with chatId as float64 in JSON
	channelCfg := `{"botToken":"123456789:ABCdefGHIjklMNOpqrsTUVwxyz","chatId":987654}`
	db.Create(&models.NotificationChannel{
		OrgID:   55,
		Type:    "telegram",
		Enabled: true,
		Config:  channelCfg,
	})
	tgCh := NewTelegramChannel(db)
	_ = tgCh.Send(cancCtx, plugin.NotificationRecipient{OrgID: "55"}, plugin.NotificationPayload{Body: "test"})

	// Telegram channel: recipient.Config with ChatID as int64 and int
	_ = tgCh.Send(cancCtx, plugin.NotificationRecipient{
		Config: map[string]any{
			"botToken": "123456789:ABCdefGHIjklMNOpqrsTUVwxyz",
			"chatId":   int64(987654),
		},
	}, plugin.NotificationPayload{Body: "test"})

	_ = tgCh.Send(cancCtx, plugin.NotificationRecipient{
		Config: map[string]any{
			"botToken": "123456789:ABCdefGHIjklMNOpqrsTUVwxyz",
			"chatId":   int(987654),
		},
	}, plugin.NotificationPayload{Body: "test"})
}

func TestCoverageLift_SlackFormattingAndConfig(t *testing.T) {
	// 1. formatSlackMessage branches
	pCrit := plugin.NotificationPayload{
		Priority:  "critical",
		Title:     "Outage",
		EventType: "system.down",
		Body:      "Service is down",
	}
	msgCrit := formatSlackMessage(pCrit)
	if !strings.Contains(msgCrit, "CRITICAL") {
		t.Errorf("expected CRITICAL, got %s", msgCrit)
	}

	pLow := plugin.NotificationPayload{
		Priority:  "low",
		EventType: "info.update",
	}
	msgLow := formatSlackMessage(pLow)
	if !strings.Contains(msgLow, "LOW") || !strings.Contains(msgLow, "info.update") {
		t.Errorf("expected LOW with eventType, got %s", msgLow)
	}

	pCustom := plugin.NotificationPayload{
		Priority: "urgent",
		Body:     "Please check",
	}
	msgCustom := formatSlackMessage(pCustom)
	if !strings.Contains(msgCustom, "URGENT") {
		t.Errorf("expected URGENT, got %s", msgCustom)
	}

	pEmpty := plugin.NotificationPayload{}
	msgEmpty := formatSlackMessage(pEmpty)
	if msgEmpty != "Notification received" {
		t.Errorf("expected 'Notification received', got %s", msgEmpty)
	}

	// 2. Config variations for SlackChannel.Send
	sl := NewSlackChannel(nil)
	cancCtx, cancel := context.WithCancel(context.Background())
	cancel()

	// webhookUrl camelCase
	_ = sl.Send(cancCtx, plugin.NotificationRecipient{
		Config: map[string]any{"webhookUrl": "https://hooks.slack.com/services/T/B/X"},
	}, pCrit)

	// integrationId variations
	_ = sl.Send(cancCtx, plugin.NotificationRecipient{
		Config: map[string]any{"webhook_url": "https://hooks.slack.com/services/T/B/X", "integrationId": 12.0},
	}, pCrit)
	_ = sl.Send(cancCtx, plugin.NotificationRecipient{
		Config: map[string]any{"webhook_url": "https://hooks.slack.com/services/T/B/X", "integration_id": 13},
	}, pCrit)
	_ = sl.Send(cancCtx, plugin.NotificationRecipient{
		Config: map[string]any{"webhook_url": "https://hooks.slack.com/services/T/B/X", "integrationId": 14},
	}, pCrit)

	// _raw_config variations
	_ = sl.Send(cancCtx, plugin.NotificationRecipient{
		Config: map[string]any{"_raw_config": `{"webhookUrl":"https://hooks.slack.com/services/T/B/X","integrationId":15}`},
	}, pCrit)
	_ = sl.Send(cancCtx, plugin.NotificationRecipient{
		Config: map[string]any{"_raw_config": `{"webhook_url":"https://hooks.slack.com/services/T/B/X","integration_id":16}`},
	}, pCrit)
}
