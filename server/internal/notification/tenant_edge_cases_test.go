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

	ctxWithTenant := plugin.WithOrgID(context.Background(), 12)
	err = SendDirect(ctxWithTenant, "test_direct", `{"custom":"param"}`, "Package SendDirect msg")
	if err != nil {
		t.Fatalf("package SendDirect failed: %v", err)
	}
	if receivedRecipient.OrgID != "12" || receivedPayload.Body != "Package SendDirect msg" {
		t.Errorf("package SendDirect mismatch: %+v, %+v", receivedRecipient, receivedPayload)
	}

	err = SendDirectWithTenant(context.Background(), 15, "test_direct", `{}`, "Tenant msg")
	if err != nil {
		t.Fatalf("package SendDirectWithTenant failed: %v", err)
	}
	if receivedRecipient.OrgID != "15" || receivedPayload.Body != "Tenant msg" {
		t.Errorf("package SendDirectWithTenant mismatch: %+v, %+v", receivedRecipient, receivedPayload)
	}

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

	err = r.SendDirectTo(context.Background(), "non_existent_channel", plugin.NotificationRecipient{}, plugin.NotificationPayload{})
	if err == nil || (!strings.Contains(err.Error(), "channel not registered") && !strings.Contains(err.Error(), "unknown notification channel type")) {
		t.Errorf("expected unknown channel error, got %v", err)
	}
}

func TestNotification_PreferencesEdgeCases(t *testing.T) {
	db := setupTestDB(t)

	if err := SavePreferences(context.Background(), nil, "user1", []PreferenceItem{}); err == nil {
		t.Error("expected error with nil db in SavePreferences")
	}
	if err := SavePreferences(context.Background(), db, "", []PreferenceItem{}); err == nil {
		t.Error("expected error with empty userID in SavePreferences")
	}

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

	db.Create(&models.NotificationChannel{
		OrgID:       50,
		Type:        "telegram",
		VaultItemID: 1,
		Config:      `{"chatId":"999"}`,
		Enabled:     true,
	})

	ch := NewTelegramChannel(db)
	ch.SetVaultService(&stubVaultService{secrets: map[uint]map[string]any{
		1: {"botToken": "invalid-token"},
	}})
	err := ch.Send(context.Background(), plugin.NotificationRecipient{}, plugin.NotificationPayload{
		OrgID: "50",
		Body:  "alert",
	})
	if err == nil || !strings.Contains(err.Error(), "failed to initialize telegram notifier") {
		t.Errorf("expected initialization error resolving org from payload, got %v", err)
	}

	err = ch.Send(context.Background(), plugin.NotificationRecipient{OrgID: "abc"}, plugin.NotificationPayload{Body: "alert"})
	if err == nil || !strings.Contains(err.Error(), "missing telegram credentials") {
		t.Errorf("expected missing credentials for unparseable orgID, got %v", err)
	}
}

func TestMultiTenant_WebhookChannel_ContextOrgID_And_SystemFallback(t *testing.T) {
	db := setupTestDB(t)

	db.Create(&models.NotificationChannel{
		OrgID:   1,
		Type:    "webhook",
		Config:  `{"url":"https://system.example.com/webhook"}`,
		Enabled: true,
	})

	db.Create(&models.NotificationChannel{
		OrgID:   80,
		Type:    "webhook",
		Config:  `{"url":"https://org80.example.com/webhook"}`,
		Enabled: true,
	})

	ch := NewWebhookChannel(db)

	ctx80 := plugin.WithOrgID(context.Background(), 80)
	err := ch.Send(ctx80, plugin.NotificationRecipient{}, plugin.NotificationPayload{Body: "tenant alert"})
	if err != nil && strings.Contains(err.Error(), "missing webhook url") {
		t.Errorf("expected webhookURL to be resolved from context OrgID, got error: %v", err)
	}

	err = ch.Send(context.Background(), plugin.NotificationRecipient{}, plugin.NotificationPayload{
		EventType: "system",
		Body:      "kernel error",
	})
	if err != nil && strings.Contains(err.Error(), "missing webhook url") {
		t.Errorf("expected system alert to fall back to Org 1 webhook, got error: %v", err)
	}
}
