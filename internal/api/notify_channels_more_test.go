package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
)

func TestMergeConfigPreservingSecrets(t *testing.T) {
	// 1. Top-level secret preservation
	oldJSON := `{"bot_token":"secret123","channel_id":"#general"}`
	newJSON := `{"bot_token":"[REDACTED]","channel_id":"#alerts"}`
	merged := mergeConfigPreservingSecrets(oldJSON, newJSON)

	var m map[string]any
	if err := json.Unmarshal([]byte(merged), &m); err != nil {
		t.Fatalf("unmarshal merged: %v", err)
	}
	if m["bot_token"] != "secret123" {
		t.Errorf("bot_token = %v, want secret123", m["bot_token"])
	}
	if m["channel_id"] != "#alerts" {
		t.Errorf("channel_id = %v, want #alerts", m["channel_id"])
	}

	// 2. Nested secret preservation
	oldNested := `{"auth":{"api_key":"supersecret"},"name":"test"}`
	newNested := `{"auth":{"api_key":"[REDACTED]"},"name":"updated"}`
	mergedNested := mergeConfigPreservingSecrets(oldNested, newNested)
	if err := json.Unmarshal([]byte(mergedNested), &m); err != nil {
		t.Fatalf("unmarshal nested merged: %v", err)
	}
	authMap, ok := m["auth"].(map[string]any)
	if !ok || authMap["api_key"] != "supersecret" {
		t.Errorf("nested api_key mismatch: %+v", m)
	}

	// 3. Updating secret explicitly
	newSecret := `{"bot_token":"brandnewsecret","channel_id":"#alerts"}`
	mergedUpdate := mergeConfigPreservingSecrets(oldJSON, newSecret)
	if err := json.Unmarshal([]byte(mergedUpdate), &m); err != nil {
		t.Fatalf("unmarshal mergedUpdate: %v", err)
	}
	if m["bot_token"] != "brandnewsecret" {
		t.Errorf("bot_token = %v, want brandnewsecret", m["bot_token"])
	}

	// 4. Invalid JSON fallback
	if got := mergeConfigPreservingSecrets("invalid", `{"a":"b"}`); got != `{"a":"b"}` {
		t.Errorf("invalid oldJSON: got %q", got)
	}
	if got := mergeConfigPreservingSecrets(`{"a":"b"}`, "invalid"); got != "invalid" {
		t.Errorf("invalid newJSON: got %q", got)
	}
}

func TestRedactConfigSecrets(t *testing.T) {
	if got := redactConfigSecrets(""); got != "{}" {
		t.Errorf("redactConfigSecrets(\"\") = %q, want {}", got)
	}
	if got := redactConfigSecrets("not-json"); got != "not-json" {
		t.Errorf("redactConfigSecrets(not-json) = %q", got)
	}

	cfg := `{"api_token":"xyz123","webhook_secret":"s3cr3t","smtp_password":"pw","url":"https://example.com","nested":{"sub_token":"abc"}}`
	redacted := redactConfigSecrets(cfg)

	var m map[string]any
	if err := json.Unmarshal([]byte(redacted), &m); err != nil {
		t.Fatalf("unmarshal redacted: %v", err)
	}
	if m["api_token"] != "[REDACTED]" || m["webhook_secret"] != "[REDACTED]" || m["smtp_password"] != "[REDACTED]" {
		t.Errorf("unredacted secrets: %+v", m)
	}
	if m["url"] != "https://example.com" {
		t.Errorf("url modified: %v", m["url"])
	}
	nested := m["nested"].(map[string]any)
	if nested["sub_token"] != "[REDACTED]" {
		t.Errorf("nested token not redacted: %+v", nested)
	}
}

func TestNotificationChannelsHTTPFlow(t *testing.T) {
	srv, db := newTestHandler(t)
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

	// Check DB row has real token preserved
	h, _, _ := newTestHandlerRaw(t)
	var updated models.NotificationChannel
	db.First(&updated, created.ID)
	plain, err := h.channelConfigPlaintext(updated.Config)
	if err != nil {
		t.Fatalf("channelConfigPlaintext: %v", err)
	}
	if !strings.Contains(plain, "supersecret") {
		t.Errorf("preserved secret missing in DB: %q", plain)
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

	// Helper edge cases
	if r := redactConfigSecrets(""); r != "{}" {
		t.Errorf("redactConfigSecrets empty = %q, want {}", r)
	}
	if r := redactConfigSecrets("invalid-json"); r != "invalid-json" {
		t.Errorf("redactConfigSecrets invalid = %q", r)
	}
	if m := mergeConfigPreservingSecrets("invalid-json", `{"k":"v"}`); m != `{"k":"v"}` {
		t.Errorf("mergeConfigPreservingSecrets invalid old = %q", m)
	}
	if m := mergeConfigPreservingSecrets(`{"k":"v"}`, "invalid-json"); m != "invalid-json" {
		t.Errorf("mergeConfigPreservingSecrets invalid new = %q", m)
	}

	plainCfg, err := h.channelConfigPlaintext("")
	if err != nil || plainCfg != "" {
		t.Errorf("channelConfigPlaintext(\"\") = (%q, %v)", plainCfg, err)
	}
	encCfg, err := h.encryptChannelConfig("{\"token\":\"123\"}")
	if err != nil || encCfg == "" {
		t.Errorf("encryptChannelConfig: %v", err)
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
