package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
)

func TestWebhooksEventsAndCRUD(t *testing.T) {
	srv, db := newTestHandler(t)
	adminCookies := loginCookies(t, srv)

	// Create member
	memberUser := models.User{Email: "memberwh@example.com"}
	db.Create(&memberUser)
	db.Create(&models.OrgMember{OrgID: 1, UserID: memberUser.ID, Role: "member"})
	memberCookies := sessionCookies(t, memberUser.ID, 1)

	// 1. List Webhook Events unauth -> 401
	rec := do(srv, "GET", "/api/webhooks/events", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth webhook events: got %d, want 401", rec.Code)
	}

	// 2. List Webhook Events auth -> 200
	rec = do(srv, "GET", "/api/webhooks/events", adminCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("auth webhook events: got %d (%s)", rec.Code, rec.Body.String())
	}

	// 3. List Webhooks unauth -> 401
	rec = do(srv, "GET", "/api/webhooks", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth list webhooks: got %d, want 401", rec.Code)
	}

	// 4. Member list webhooks -> 403
	rec = do(srv, "GET", "/api/webhooks", memberCookies, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member list webhooks: got %d, want 403", rec.Code)
	}

	// 5. Create Webhook validation: empty URL -> 400
	rec = do(srv, "POST", "/api/webhooks", adminCookies, `{"name":"test","url":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create webhook empty url: got %d, want 400", rec.Code)
	}

	// Member create webhook -> 403
	rec = do(srv, "POST", "/api/webhooks", memberCookies, `{"name":"test","url":"https://example.com/wh"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member create webhook: got %d, want 403", rec.Code)
	}

	// Admin create webhook without secret -> auto generates secret
	rec = do(srv, "POST", "/api/webhooks", adminCookies, `{
		"name": "Audit Webhook",
		"url": "https://example.com/audit-webhook",
		"events": "member.role, member.remove"
	}`)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("admin create webhook: got %d (%s)", rec.Code, rec.Body.String())
	}
	var created models.Webhook
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created webhook: %v", err)
	}
	if created.Secret == "" || created.Name != "Audit Webhook" {
		t.Errorf("created webhook mismatch: %+v", created)
	}

	// 6. List webhooks -> decrypted secret returned to authenticated caller
	rec = do(srv, "GET", "/api/webhooks", adminCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list webhooks: got %d", rec.Code)
	}
	var hooks []models.Webhook
	if err := json.Unmarshal(rec.Body.Bytes(), &hooks); err != nil {
		t.Fatalf("unmarshal webhooks: %v", err)
	}
	if len(hooks) != 1 || hooks[0].Secret != created.Secret {
		t.Errorf("list webhooks secret mismatch: got %+v", hooks)
	}

	// 7. Update Webhook
	// Unauth -> 401
	rec = do(srv, "PUT", fmt.Sprintf("/api/webhooks/%d", created.ID), nil, `{"name":"test","url":"https://example.com"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth update webhook: got %d, want 401", rec.Code)
	}

	// Update empty name/url -> 400
	rec = do(srv, "PUT", fmt.Sprintf("/api/webhooks/%d", created.ID), adminCookies, `{"name":"","url":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty name update webhook: got %d, want 400", rec.Code)
	}

	// Update not found -> 404
	rec = do(srv, "PUT", "/api/webhooks/999999", adminCookies, `{"name":"test","url":"https://example.com"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("update nonexistent webhook: got %d, want 404", rec.Code)
	}

	// Update name, events, custom secret
	rec = do(srv, "PUT", fmt.Sprintf("/api/webhooks/%d", created.ID), adminCookies, `{
		"name": "Renamed Webhook",
		"url": "https://example.com/wh2",
		"secret": "new-custom-secret-hex",
		"events": "*"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update webhook: got %d (%s)", rec.Code, rec.Body.String())
	}

	// Verify updated secret in list
	rec = do(srv, "GET", "/api/webhooks", adminCookies, "")
	_ = json.Unmarshal(rec.Body.Bytes(), &hooks)
	if hooks[0].Secret != "new-custom-secret-hex" || hooks[0].Name != "Renamed Webhook" {
		t.Errorf("updated webhook mismatch: %+v", hooks[0])
	}

	// 8. Delete Webhook
	// Unauth -> 401
	rec = do(srv, "DELETE", fmt.Sprintf("/api/webhooks/%d", created.ID), nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth delete webhook: got %d, want 401", rec.Code)
	}

	// Member -> 403
	rec = do(srv, "DELETE", fmt.Sprintf("/api/webhooks/%d", created.ID), memberCookies, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member delete webhook: got %d, want 403", rec.Code)
	}

	rec = do(srv, "DELETE", "/api/webhooks/999999", adminCookies, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete nonexistent webhook: got %d, want 404", rec.Code)
	}

	rec = do(srv, "DELETE", fmt.Sprintf("/api/webhooks/%d", created.ID), adminCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete webhook: got %d (%s)", rec.Code, rec.Body.String())
	}
	var count int64
	db.Model(&models.Webhook{}).Where("id = ?", created.ID).Count(&count)
	if count != 0 {
		t.Errorf("webhook still exists after delete")
	}

	// 9. Secret helper functions
	h, _, _ := newTestHandlerRaw(t)
	plain, err := h.webhookSecretPlaintext("")
	if err != nil || plain != "" {
		t.Errorf("webhookSecretPlaintext(\"\") = (%q, %v)", plain, err)
	}

	enc, err := h.encryptWebhookSecret("test-secret")
	if err != nil {
		t.Fatalf("encryptWebhookSecret: %v", err)
	}
	dec, err := h.webhookSecretPlaintext(enc)
	if err != nil || dec != "test-secret" {
		t.Errorf("decrypt webhook secret roundtrip = (%q, %v)", dec, err)
	}

	// 10. Nil Ctx calls
	ctx := context.Background()
	if _, err := h.listWebhookEvents(ctx, &ListWebhookEventsInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in listWebhookEvents")
	}
	if _, err := h.listWebhooks(ctx, &ListWebhooksInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in listWebhooks")
	}
	if _, err := h.createWebhook(ctx, &CreateWebhookInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in createWebhook")
	}
	if _, err := h.updateWebhook(ctx, &UpdateWebhookInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in updateWebhook")
	}
	if _, err := h.deleteWebhook(ctx, &DeleteWebhookInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in deleteWebhook")
	}
}
