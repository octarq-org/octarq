package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGatedMux verifies plugin routes are blocked (404) only when the caller's
// workspace has the plugin disabled; enabled and no-workspace requests pass.
func TestGatedMux(t *testing.T) {
	cases := []struct {
		name            string
		allowed, scoped bool
		wantStatus      int
	}{
		{"disabled for workspace → blocked", false, true, http.StatusNotFound},
		{"enabled for workspace → allowed", true, true, http.StatusOK},
		{"no workspace context → allowed", false, false, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			real := http.NewServeMux()
			gm := &gatedMux{
				real:   real,
				plugin: "fake",
				enabled: func(_ *http.Request, _ string) (bool, bool) {
					return tc.allowed, tc.scoped
				},
			}
			gm.HandleFunc("/api/fake", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			rec := httptest.NewRecorder()
			real.ServeHTTP(rec, httptest.NewRequest("GET", "/api/fake", nil))
			if rec.Code != tc.wantStatus {
				t.Fatalf("got %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}
