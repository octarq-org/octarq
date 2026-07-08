package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCSRFGuard exercises the cross-origin guard that protects cookie-authed,
// state-changing requests. The matrix covers the three ways a request can be
// waved through (safe method, no session cookie, same-origin) and the ways a
// browser-driven cross-site request is refused.
func TestCSRFGuard(t *testing.T) {
	guarded := CSRFGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	const host = "app.example.com"
	sessionCookie := &http.Cookie{Name: sessionCookieName, Value: "x"}

	cases := []struct {
		name       string
		method     string
		withCookie bool
		origin     string
		referer    string
		want       int
	}{
		{"safe GET cross-origin passes", http.MethodGet, true, "https://evil.example", "", http.StatusOK},
		{"no session cookie passes (bearer/webhook)", http.MethodPost, false, "https://evil.example", "", http.StatusOK},
		{"cookie + no origin/referer passes (non-browser)", http.MethodPost, true, "", "", http.StatusOK},
		{"cookie + same origin passes", http.MethodPost, true, "https://" + host, "", http.StatusOK},
		{"cookie + cross origin blocked", http.MethodPost, true, "https://evil.example", "", http.StatusForbidden},
		{"cookie + referer fallback same-origin passes", http.MethodPost, true, "", "https://" + host + "/admin/", http.StatusOK},
		{"cookie + referer fallback cross-origin blocked", http.MethodPost, true, "", "https://evil.example/x", http.StatusForbidden},
		{"cookie + malformed origin blocked", http.MethodPost, true, "://nonsense", "", http.StatusForbidden},
		{"cookie + origin wins over referer", http.MethodPost, true, "https://evil.example", "https://" + host, http.StatusForbidden},
		{"PUT cross origin blocked", http.MethodPut, true, "https://evil.example", "", http.StatusForbidden},
		{"DELETE cross origin blocked", http.MethodDelete, true, "https://evil.example", "", http.StatusForbidden},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(c.method, "https://"+host+"/api/links", nil)
			req.Host = host
			if c.withCookie {
				req.AddCookie(sessionCookie)
			}
			if c.origin != "" {
				req.Header.Set("Origin", c.origin)
			}
			if c.referer != "" {
				req.Header.Set("Referer", c.referer)
			}
			rec := httptest.NewRecorder()
			guarded.ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Errorf("got %d, want %d", rec.Code, c.want)
			}
		})
	}
}
