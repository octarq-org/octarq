package notification

import (
	"context"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

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

func TestMultiTenant_WebhookChannel_ContextOrgID_And_SystemFallback(t *testing.T) {
	db := setupTestDB(t)
	SetConfigDecryptor(func(s string) (string, bool) { return s, true })
	t.Cleanup(func() { SetConfigDecryptor(nil) })

	// Org 1 (system default)
	db.Create(&models.NotificationChannel{
		OrgID:   1,
		Type:    "webhook",
		Config:  `{"url":"https://system.example.com/webhook"}`,
		Enabled: true,
	})

	// Org 80 (tenant)
	db.Create(&models.NotificationChannel{
		OrgID:   80,
		Type:    "webhook",
		Config:  `{"url":"https://org80.example.com/webhook"}`,
		Enabled: true,
	})

	ch := NewWebhookChannel(db)

	// 1. Context OrgID resolution when recipient.OrgID and payload.OrgID are empty
	ctx80 := plugin.WithOrgID(context.Background(), 80)
	err := ch.Send(ctx80, plugin.NotificationRecipient{}, plugin.NotificationPayload{Body: "tenant alert"})
	if err != nil && strings.Contains(err.Error(), "missing webhook url") {
		t.Errorf("expected webhookURL to be resolved from context OrgID, got error: %v", err)
	}

	// 2. System-level alert fallback: when eventType is "system", falls back to Org 1
	err = ch.Send(context.Background(), plugin.NotificationRecipient{}, plugin.NotificationPayload{
		EventType: "system",
		Body:      "kernel error",
	})
	if err != nil && strings.Contains(err.Error(), "missing webhook url") {
		t.Errorf("expected system alert to fall back to Org 1 webhook, got error: %v", err)
	}
}
