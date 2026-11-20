package auth

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
			previous := trustProxy.Load()
			trustProxy.Store(tc.trustProxy)
			t.Cleanup(func() { trustProxy.Store(previous) })

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
