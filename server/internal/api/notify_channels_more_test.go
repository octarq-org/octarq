package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/internal/notification"
)

func TestSplitChannelConfig(t *testing.T) {
	meta, secrets, redacted, err := splitChannelConfig(`{"bot_token":"secret123","channel_id":"#general"}`)
	if err != nil {
		t.Fatalf("splitChannelConfig: %v", err)
	}
	if secrets["bot_token"] != "secret123" || meta["channel_id"] != "#general" || len(redacted) != 0 {
		t.Fatalf("unexpected split: meta=%v secrets=%v redacted=%v", meta, secrets, redacted)
	}

	meta, secrets, redacted, err = splitChannelConfig(`{"bot_token":"[REDACTED]","channel_id":"#alerts"}`)
	if err != nil {
		t.Fatalf("splitChannelConfig redacted: %v", err)
	}
	if len(secrets) != 0 || meta["channel_id"] != "#alerts" || len(redacted) != 1 {
		t.Fatalf("unexpected redacted split: meta=%v secrets=%v redacted=%v", meta, secrets, redacted)
	}

	if _, _, _, err := splitChannelConfig("not-json"); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if got := marshalChannelConfigMap(nil); got != "{}" {
		t.Errorf("marshalChannelConfigMap(nil) = %q, want {}", got)
	}
}

func TestNotificationChannelsHTTPFlow(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)
	adminCookies := loginCookies(t, srv)

	// Create member
	memberUser := models.User{Email: "notadmin@example.com"}
	db.Create(&memberUser)
	db.Create(&models.OrgMember{OrgID: 1, UserID: memberUser.ID, Role: "member"})
	memberCookies := sessionCookies(t, memberUser.ID, 1)

	// 1. List Types unauthenticated -> 401
	rec := do(srv, "GET", "/api/notification-channel-types", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth list types: got %d, want 401", rec.Code)
	}

	// 2. List Types authenticated -> 200
	rec = do(srv, "GET", "/api/notification-channel-types", adminCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("auth list types: got %d (%s)", rec.Code, rec.Body.String())
	}

	// 3. Create Channel validation
	// Empty name or type -> 400
	rec = do(srv, "POST", "/api/notification-channels", adminCookies, `{"name":"","type":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create channel empty fields: got %d, want 400", rec.Code)
	}

	// Member role -> 403
	rec = do(srv, "POST", "/api/notification-channels", memberCookies, `{"name":"Webhook","type":"webhook"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member create channel: got %d, want 403", rec.Code)
	}

	// Admin create valid channel
	rec = do(srv, "POST", "/api/notification-channels", adminCookies, `{
		"name": "Dev Ops Slack",
		"type": "webhook",
		"config": "{\"webhook_url\":\"https://hooks.slack.com/services/123\",\"token\":\"supersecret\"}",
		"enabled": true
	}`)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("admin create channel: got %d (%s)", rec.Code, rec.Body.String())
	}
	var created models.NotificationChannel
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created channel: %v", err)
	}
	if created.Name != "Dev Ops Slack" || created.Type != "webhook" {
		t.Errorf("created channel mismatch: %+v", created)
	}

	// 4. List Channels -> verify secret is redacted
	rec = do(srv, "GET", "/api/notification-channels", adminCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list channels: got %d", rec.Code)
	}
	var channels []models.NotificationChannel
	if err := json.Unmarshal(rec.Body.Bytes(), &channels); err != nil {
		t.Fatalf("unmarshal channels: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("got %d channels, want 1", len(channels))
	}
	if !strings.Contains(channels[0].Config, "[REDACTED]") {
		t.Errorf("channel config not redacted in list: %q", channels[0].Config)
	}

	// 5. Update Channel with [REDACTED] config -> preserves secret in DB
	rec = do(srv, "PUT", fmt.Sprintf("/api/notification-channels/%d", created.ID), adminCookies, `{
		"name": "Updated Slack",
		"config": "{\"webhook_url\":\"https://hooks.slack.com/services/456\",\"token\":\"[REDACTED]\"}"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update channel with [REDACTED]: got %d (%s)", rec.Code, rec.Body.String())
	}

	// Check Vault has real token preserved and DB row does not store it
	var updated models.NotificationChannel
	db.First(&updated, created.ID)
	if strings.Contains(updated.Config, "supersecret") {
		t.Fatalf("DB config column leaked secret: %q", updated.Config)
	}
	resolved, err := notification.ResolveChannelConfigJSON(context.Background(), h.vault(), updated)
	if err != nil {
		t.Fatalf("ResolveChannelConfigJSON: %v", err)
	}
	if !strings.Contains(resolved, "supersecret") || !strings.Contains(resolved, "services/456") {
		t.Errorf("preserved secret or updated webhook_url missing in Vault: %q", resolved)
	}

	// Update nonexistent channel -> 404
	rec = do(srv, "PUT", "/api/notification-channels/999999", adminCookies, `{"name":"test"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("update nonexistent channel: got %d, want 404", rec.Code)
	}

	// 6. Delete Channel
	// unauth -> 401
	rec = do(srv, "DELETE", fmt.Sprintf("/api/notification-channels/%d", created.ID), nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth delete channel: got %d, want 401", rec.Code)
	}

	// member -> 403
	rec = do(srv, "DELETE", fmt.Sprintf("/api/notification-channels/%d", created.ID), memberCookies, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member delete channel: got %d, want 403", rec.Code)
	}

	rec = do(srv, "DELETE", "/api/notification-channels/999999", adminCookies, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete nonexistent channel: got %d, want 404", rec.Code)
	}

	rec = do(srv, "DELETE", fmt.Sprintf("/api/notification-channels/%d", created.ID), adminCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete channel: got %d (%s)", rec.Code, rec.Body.String())
	}
	var count int64
	db.Model(&models.NotificationChannel{}).Where("id = ?", created.ID).Count(&count)
	if count != 0 {
		t.Errorf("channel still exists after delete")
	}

	// 7. Test notification channel
	// unauth -> 401
	rec = do(srv, "POST", "/api/notification-channels/999999/test", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth test channel: got %d, want 401", rec.Code)
	}

	// member -> 403
	rec = do(srv, "POST", "/api/notification-channels/999999/test", memberCookies, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member test channel: got %d, want 403", rec.Code)
	}

	// not found -> 404
	rec = do(srv, "POST", "/api/notification-channels/999999/test", adminCookies, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("test nonexistent channel: got %d, want 404", rec.Code)
	}

	// 8. Nil Ctx calls
	ctx := context.Background()
	if _, err := h.listNotificationChannelTypes(ctx, &ListNotificationChannelTypesInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in listNotificationChannelTypes")
	}
	if _, err := h.listNotificationChannels(ctx, &ListNotificationChannelsInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in listNotificationChannels")
	}
	if _, err := h.createNotificationChannel(ctx, &CreateNotificationChannelInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in createNotificationChannel")
	}
	if _, err := h.updateNotificationChannel(ctx, &UpdateNotificationChannelInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in updateNotificationChannel")
	}
	if _, err := h.deleteNotificationChannel(ctx, &DeleteNotificationChannelInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in deleteNotificationChannel")
	}
	if _, err := h.testNotificationChannel(ctx, &TestNotificationChannelInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in testNotificationChannel")
	}
}
