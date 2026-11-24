package api

import (
	"net/http"
	"testing"
)

func TestProtectedRoutesRejectAnonymous(t *testing.T) {
	srv, _ := newTestHandler(t)

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/settings", ""},
		{http.MethodPut, "/api/settings", "{}"},
		{http.MethodGet, "/api/settings/inbound-token", ""},
		{http.MethodGet, "/api/overview", ""},
		{http.MethodGet, "/api/account/export", ""},
		{http.MethodGet, "/api/tokens", ""},
		{http.MethodGet, "/api/notifications", ""},
		{http.MethodGet, "/api/plugins", ""},
		{http.MethodGet, "/api/webhooks", ""},
	}

	for _, tc := range cases {
		rec := do(srv, tc.method, tc.path, nil, tc.body)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s anonymous: got %d, want 401 (%s)", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}
