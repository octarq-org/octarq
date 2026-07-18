package api

import (
	"net/http"
	"testing"
)

// TestCreateLinkRejectsDangerousExpiredURL asserts a javascript: ExpiredURL is
// refused at write time — it must never reach a stored link (which is later
// emitted verbatim in a 302 Location header on expiry).
func TestCreateLinkRejectsDangerousExpiredURL(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	body := `{"target":"https://ok.example","expiredUrl":"javascript:alert(1)"}`
	rec := do(srv, "POST", "/api/links", cookies, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create with javascript: expiredUrl: want 400, got %d (%s)", rec.Code, rec.Body.String())
	}

	// A valid http(s) ExpiredURL is accepted.
	rec = do(srv, "POST", "/api/links", cookies, `{"target":"https://ok.example","expiredUrl":"https://exp.example"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create with valid expiredUrl: want 201, got %d (%s)", rec.Code, rec.Body.String())
	}
}

// TestUpdateLinkRejectsDangerousExpiredURL asserts update rejects a dangerous
// ExpiredURL too.
func TestUpdateLinkRejectsDangerousExpiredURL(t *testing.T) {
	srv, _ := newTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := do(srv, "POST", "/api/links", cookies, `{"target":"https://ok.example"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("seed create: got %d (%s)", rec.Code, rec.Body.String())
	}
	// Slug 1 (first link) — update by id 1.
	rec = do(srv, "PUT", "/api/links/1", cookies, `{"target":"https://ok.example","expiredUrl":"data:text/html,<script>x</script>"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("update with data: expiredUrl: want 400, got %d (%s)", rec.Code, rec.Body.String())
	}
}
