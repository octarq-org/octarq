package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/server/internal/csrf"
)

func TestCSRFCookieTTLMatchesSessionTTL(t *testing.T) {
	if csrf.CookieTTL != sessionTTL {
		t.Fatalf("csrf.CookieTTL = %v, sessionTTL = %v — the token must live exactly as long as the session it is derived from", csrf.CookieTTL, sessionTTL)
	}
}

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
	if session == nil || !session.HttpOnly {
		t.Error("no session cookie, or session cookie lost HttpOnly")
	}
	if token == nil || token.HttpOnly {
		t.Errorf("no %s cookie beside the session, or it is HttpOnly — the frontend cannot read it, so it can never be echoed back", csrf.CookieName)
	}
	if token == nil || token.Secure != session.Secure {
		t.Errorf("Secure mismatch: session %v, token %v", session != nil, token != nil)
	}
	if token == nil || token.Value != csrf.GenerateToken(m.cfg.SecretKey, "tok") {
		t.Errorf("no token cookie, or token %v is not the HMAC of the session token", token)
	}
}

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
