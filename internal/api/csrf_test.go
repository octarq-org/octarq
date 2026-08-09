package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/internal/csrf"
)

// TestCSRFGuard exercises both halves of the guard over one matrix: the
// Origin/Referer check (which fires for ANY cookie-bearing request) and the
// double-submit token (which fires only for the dashboard session cookie). The
// cases the guard exists for are the octarq_customer rows — a second browser
// session that the old session-cookie-only trigger left uncovered — and the
// missing/wrong-token rows.
func TestCSRFGuard(t *testing.T) {
	secret := "test-secret"
	guarded := CSRFGuard(secret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	const host = "app.example.com"
	sessionCookie := &http.Cookie{Name: sessionCookieName, Value: "x"}
	customerCookie := &http.Cookie{Name: "octarq_customer", Value: "y"}
	validCSRFToken := csrf.GenerateToken(secret, "x")

	cases := []struct {
		name       string
		method     string
		cookies    []*http.Cookie
		origin     string
		referer    string
		header     string // X-CSRF-Token value
		want       int
		wantCookie bool // Whether we expect octarq_csrf to be set
	}{
		{"safe GET cross-origin passes and heals token", http.MethodGet, []*http.Cookie{sessionCookie}, "https://evil.example", "", "", http.StatusOK, true},
		{"cookie + no origin/referer passes (non-browser)", http.MethodPost, []*http.Cookie{sessionCookie}, "", "", validCSRFToken, http.StatusOK, false},
		{"cookie + referer fallback same-origin passes", http.MethodPost, []*http.Cookie{sessionCookie}, "", "https://" + host + "/admin/", validCSRFToken, http.StatusOK, false},
		{"cookie + referer fallback cross-origin blocked", http.MethodPost, []*http.Cookie{sessionCookie}, "", "https://evil.example/x", validCSRFToken, http.StatusForbidden, false},
		{"cookie + malformed origin blocked", http.MethodPost, []*http.Cookie{sessionCookie}, "://nonsense", "", validCSRFToken, http.StatusForbidden, false},
		{"cookie + origin wins over referer", http.MethodPost, []*http.Cookie{sessionCookie}, "https://evil.example", "https://" + host, validCSRFToken, http.StatusForbidden, false},

		{"session cookie + cross origin blocked", http.MethodPost, []*http.Cookie{sessionCookie}, "https://evil.example", "", validCSRFToken, http.StatusForbidden, false},
		{"PUT cross origin blocked", http.MethodPut, []*http.Cookie{sessionCookie}, "https://evil.example", "", validCSRFToken, http.StatusForbidden, false},
		{"DELETE cross origin blocked", http.MethodDelete, []*http.Cookie{sessionCookie}, "https://evil.example", "", validCSRFToken, http.StatusForbidden, false},
		{"PATCH cross origin blocked", http.MethodPatch, []*http.Cookie{sessionCookie}, "https://evil.example", "", validCSRFToken, http.StatusForbidden, false},

		// The buyer portal's own session. It never sees a token (nothing mints one
		// for it), so it must pass on same-origin and be refused cross-origin
		// purely on Origin — that is the hole the any-cookie trigger closes.
		{"customer cookie + cross origin", http.MethodPost, []*http.Cookie{customerCookie}, "https://evil.example", "", "", http.StatusForbidden, false},
		{"customer cookie + same origin", http.MethodPost, []*http.Cookie{customerCookie}, "https://" + host, "", "", http.StatusOK, false},
		{"customer cookie + cross origin DELETE", http.MethodDelete, []*http.Cookie{customerCookie}, "https://evil.example", "", "", http.StatusForbidden, false},
		{"unrelated cookie only + cross origin blocked", http.MethodPost, []*http.Cookie{{Name: "some_plugin_session", Value: "z"}}, "https://evil.example", "", "", http.StatusForbidden, false},
		{"no cookie + cross origin POST", http.MethodPost, nil, "https://evil.example", "", "", http.StatusOK, false},
		{"no cookie + cross origin DELETE", http.MethodDelete, nil, "https://evil.example", "", "", http.StatusOK, false},
		{"session cookie + same origin POST + correct token", http.MethodPost, []*http.Cookie{sessionCookie}, "https://" + host, "", validCSRFToken, http.StatusOK, false},
		{"session cookie + same origin POST + missing token", http.MethodPost, []*http.Cookie{sessionCookie}, "https://" + host, "", "", http.StatusForbidden, false},
		{"session cookie + same origin POST + wrong token", http.MethodPost, []*http.Cookie{sessionCookie}, "https://" + host, "", "wrong", http.StatusForbidden, false},
		{"session GET + missing csrf cookie heals", http.MethodGet, []*http.Cookie{sessionCookie}, "https://" + host, "", "", http.StatusOK, true},
		{"session POST + missing csrf cookie blocked, no heal", http.MethodPost, []*http.Cookie{sessionCookie}, "https://" + host, "", "", http.StatusForbidden, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(c.method, "https://"+host+"/api/links", nil)
			req.Host = host
			for _, cookie := range c.cookies {
				req.AddCookie(cookie)
			}
			if c.origin != "" {
				req.Header.Set("Origin", c.origin)
			}
			if c.referer != "" {
				req.Header.Set("Referer", c.referer)
			}
			if c.header != "" {
				req.Header.Set(csrf.HeaderName, c.header)
			}
			rec := httptest.NewRecorder()
			guarded.ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Errorf("got %d, want %d", rec.Code, c.want)
			}

			hasHealCookie := false
			for _, cookie := range rec.Result().Cookies() {
				if cookie.Name == csrf.CookieName {
					hasHealCookie = true
					if cookie.Value != validCSRFToken && c.wantCookie {
						t.Errorf("healed token = %q, want %q", cookie.Value, validCSRFToken)
					}
				}
			}
			if c.wantCookie && !hasHealCookie {
				t.Errorf("expected octarq_csrf cookie to be set")
			} else if !c.wantCookie && hasHealCookie {
				t.Errorf("did not expect octarq_csrf cookie to be set")
			}
		})
	}
}
