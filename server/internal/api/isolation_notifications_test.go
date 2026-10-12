package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// --- notification channel isolation ---

func TestOrgIsolation_NotificationChannels(t *testing.T) {
	srv, _ := newTestHandler(t)
	org1 := sessionCookies(t, 1, 1)
	org2 := sessionCookies(t, 2, 2)

	rec := do(srv, http.MethodPost, "/api/notification-channels", org1,
		`{"name":"chan-org1","type":"telegram","config":"{}"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create channel: got %d — %s", rec.Code, rec.Body)
	}
	var ch struct {
		ID uint `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &ch)
	path := fmt.Sprintf("/api/notification-channels/%d", ch.ID)

	// Org 2 must not update or delete org 1's channel.
	if rec := do(srv, http.MethodPut, path, org2, `{"name":"hacked","type":"telegram","config":"{}"}`); rec.Code != http.StatusNotFound {
		t.Errorf("cross-org PUT channel: got %d, want 404", rec.Code)
	}
	if rec := do(srv, http.MethodDelete, path, org2, ""); rec.Code != http.StatusNotFound {
		t.Errorf("cross-org DELETE channel: got %d, want 404", rec.Code)
	}
	// Org 2's list must not leak org 1's channel.
	if rec := do(srv, http.MethodGet, "/api/notification-channels", org2, ""); strings.Contains(rec.Body.String(), "chan-org1") {
		t.Error("org2 list leaked org1's notification channel")
	}
}

// --- webhook isolation ---

func TestOrgIsolation_Webhooks(t *testing.T) {
	srv, _ := newTestHandler(t)
	org1 := sessionCookies(t, 1, 1)
	org2 := sessionCookies(t, 2, 2)

	rec := do(srv, http.MethodPost, "/api/webhooks", org1,
		`{"name":"hook-org1","url":"https://example.com/org1"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create webhook: got %d — %s", rec.Code, rec.Body)
	}
	var wh struct {
		ID uint `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &wh)
	path := fmt.Sprintf("/api/webhooks/%d", wh.ID)

	// Org 2 must not update or delete org 1's webhook.
	if rec := do(srv, http.MethodPut, path, org2, `{"name":"hacked"}`); rec.Code != http.StatusNotFound {
		t.Errorf("cross-org PUT webhook: got %d, want 404", rec.Code)
	}
	if rec := do(srv, http.MethodDelete, path, org2, ""); rec.Code != http.StatusNotFound {
		t.Errorf("cross-org DELETE webhook: got %d, want 404", rec.Code)
	}
	// Org 2's list must not leak org 1's webhook.
	if rec := do(srv, http.MethodGet, "/api/webhooks", org2, ""); strings.Contains(rec.Body.String(), "hook-org1") {
		t.Error("org2 list leaked org1's webhook")
	}
}
