package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/internal/csrf"
)

// TestCSRFCookieTTLMatchesSessionTTL pins the one duplicated constant in the
// pair. csrf.CookieTTL can't read sessionTTL (auth imports csrf, not the other
// way round), so nothing but this test stops the two drifting — and a token
// cookie that expires before its session leaves the user unable to write.
func TestCSRFCookieTTLMatchesSessionTTL(t *testing.T) {
	if csrf.CookieTTL != sessionTTL {
		t.Fatalf("csrf.CookieTTL = %v, sessionTTL = %v — the token must live exactly as long as the session it is derived from", csrf.CookieTTL, sessionTTL)
	}
}

// TestSetCookieIssuesBothHalves: a session is useless to the dashboard without
// the readable token beside it, so setCookie must emit the pair — with matching
// Secure, or the two split across HTTP/HTTPS and every write 403s.
func TestSetCookieIssuesBothHalves(t *testing.T) {
	m := testManager(t)
	rec := httptest.NewRecorder()
	m.setCookie(rec, httptest.NewRequest(http.MethodGet, "https://app.example.com/", nil), "tok")

	var session, token *http.Cookie
	for _, c := range rec.Result().Cookies() {
		switch c.Name {
		case cookieName:
			session = c
		case csrf.CookieName:
			token = c
		}
	}
	if session == nil {
		t.Fatal("no session cookie")
	}
	if token == nil {
		t.Fatalf("no %s cookie beside the session", csrf.CookieName)
	}
	if token.HttpOnly {
		t.Error("token cookie is HttpOnly — the frontend cannot read it, so it can never be echoed back")
	}
	if !session.HttpOnly {
		t.Error("session cookie lost HttpOnly")
	}
	if token.Secure != session.Secure {
		t.Errorf("Secure mismatch: session %v, token %v", session.Secure, token.Secure)
	}
	if got, want := token.Value, csrf.GenerateToken(m.cfg.SecretKey, "tok"); got != want {
		t.Errorf("token = %q, want the HMAC of the session token (%q)", got, want)
	}
}

// TestClearExpiresBothHalves: logging out must not leave a stale token cookie
// pointing at a dead session.
func TestClearExpiresBothHalves(t *testing.T) {
	m := testManager(t)
	rec := httptest.NewRecorder()
	m.Clear(httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil), rec)

	cleared := map[string]bool{}
	for _, c := range rec.Result().Cookies() {
		if c.MaxAge < 0 {
			cleared[c.Name] = true
		}
	}
	for _, name := range []string{cookieName, csrf.CookieName} {
		if !cleared[name] {
			t.Errorf("%s was not expired on logout", name)
		}
	}
}
