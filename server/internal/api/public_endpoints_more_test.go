package api

import (
	"context"
	"net/http"
	"testing"
)

func TestPublicEndpointsMore(t *testing.T) {
	// 1. PublicEndpoint String
	ep := PublicEndpoint{Method: "GET", Path: "/api/health", Reason: "exact"}
	if s := ep.String(); s != "GET /api/health  (exact)" {
		t.Errorf("ep.String() = %q", s)
	}

	// 2. isPublicPath tests
	if !isPublicPath("/non-api/doc") {
		t.Error("expected non-api path to be public")
	}
	if !isPublicPath("/api/webhook/stripe/123") {
		t.Error("expected webhook subtree to be public")
	}
	if isPublicPath("/api/tokens") {
		t.Error("expected /api/tokens to not be public")
	}

	// 3. matchedPrefix
	if p := matchedPrefix("/api/webhook/stripe/123"); p != "/api/webhook/" {
		t.Errorf("matchedPrefix(webhook) = %q, want /api/webhook/", p)
	}
	if p := matchedPrefix("/api/unknown"); p != "" {
		t.Errorf("matchedPrefix(unknown) = %q, want empty", p)
	}

	// 4. templateMatches
	if templateMatches([]string{"api", "{slug}", "setting"}, []string{"api", "", "setting"}) {
		t.Error("templateMatches should not match empty param segment")
	}
	if templateMatches([]string{"api", "a"}, []string{"api", "b"}) {
		t.Error("expected mismatching literal segments to return false")
	}

	// 5. DataRetentionDays and Handler helper methods
	h, srv, db := newTestHandlerRaw(t)
	if d := h.DataRetentionDays(); d != DefaultRetentionDays {
		t.Errorf("DataRetentionDays() = %d, want %d", d, DefaultRetentionDays)
	}

	h.setSetting(keyDataRetentionDays, "60")
	if d := h.DataRetentionDays(); d != 60 {
		t.Errorf("DataRetentionDays() = %d, want 60", d)
	}

	h.setSetting(keyDataRetentionDays, "invalid")
	if d := h.DataRetentionDays(); d != DefaultRetentionDays {
		t.Errorf("DataRetentionDays() invalid = %d, want %d", d, DefaultRetentionDays)
	}

	hNilLookup := &Handler{}
	if s, ok := hNilLookup.LookupService("test"); ok || s != nil {
		t.Errorf("LookupService(nil) = (%v, %v)", s, ok)
	}
	if api := h.Huma(); api == nil {
		t.Error("Huma() = nil, want the registered humaAPI")
	}

	// queue handlers test
	if h.queue != nil {
		_ = h.queue.Enqueue(context.Background(), "abuse.notify", []byte("invalid-json"))
	}

	// 6. Test /api/v1/ alias router
	recV1 := do(srv, "GET", "/api/v1/status", nil, "")
	if recV1.Code != http.StatusOK {
		t.Errorf("expected 200 on /api/v1/status, got %d", recV1.Code)
	}
	_ = db
}
