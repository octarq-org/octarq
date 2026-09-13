package auth

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSessionCookieSecureTrustsProxyOnlyWhenTold pins the one input to the
// cookie's Secure attribute that an attacker can set: X-Forwarded-Proto.
//
// Believing it unconditionally would let anyone talking plain HTTP to octarq
// have their own session cookie marked Secure — which the browser then refuses
// to send back over that same plain-HTTP connection, so the user logs in and is
// immediately 401 on everything. It is only trustworthy when the operator has
// declared that a reverse proxy in front of octarq sets it (OCTARQ_TRUST_PROXY),
// which is the same declaration that governs the client-IP headers.
func TestSessionCookieSecureTrustsProxyOnlyWhenTold(t *testing.T) {
	cases := []struct {
		name       string
		tls        bool
		forwarded  string
		trustProxy bool
		want       bool
	}{
		{name: "plain http", want: false},
		{name: "real TLS", tls: true, want: true},
		{name: "forged X-Forwarded-Proto is ignored", forwarded: "https", trustProxy: false, want: false},
		{name: "trusted X-Forwarded-Proto is honoured", forwarded: "https", trustProxy: true, want: true},
		{name: "trusted proxy reporting http", forwarded: "http", trustProxy: true, want: false},
		{name: "trusted proxy chain uses the client-side entry", forwarded: "https, http", trustProxy: true, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := testManager(t)
			// trustProxy is package state set from config by New; set it
			// directly and put it back, so one case cannot leak into the next.
			previous := trustProxy
			trustProxy = tc.trustProxy
			t.Cleanup(func() { trustProxy = previous })

			req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
			if tc.tls {
				req.TLS = &tls.ConnectionState{}
			}
			if tc.forwarded != "" {
				req.Header.Set("X-Forwarded-Proto", tc.forwarded)
			}
			rec := httptest.NewRecorder()

			m.SetSessionFromRequest(req, rec, 1, 1)

			cookies := rec.Result().Cookies()
			if len(cookies) != 2 {
				t.Fatalf("expected 2 cookies, got %d", len(cookies))
			}
			for _, cookie := range cookies {
				if got := cookie.Secure; got != tc.want {
					t.Errorf("Secure for %s = %v, want %v", cookie.Name, got, tc.want)
				}
			}
		})
	}
}
