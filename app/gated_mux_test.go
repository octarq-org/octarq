package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
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

// TestGatedAPI verifies Huma routes registered via gatedAPI/gatedAdapter are blocked
// (404) only when the caller's workspace has the plugin disabled.
func TestGatedAPI(t *testing.T) {
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
			realMux := http.NewServeMux()
			config := huma.DefaultConfig("Test API", "1.0.0")
			realAPI := humago.New(realMux, config)

			enabledFunc := func(_ *http.Request, _ string) (bool, bool) {
				return tc.allowed, tc.scoped
			}

			gAPI := &gatedAPI{
				API: realAPI,
				gAdapter: &gatedAdapter{
					Adapter: realAPI.Adapter(),
					plugin:  "fake",
					enabled: enabledFunc,
				},
			}

			// Register a Huma route on the gated API
			type TestInput struct{}
			type TestOutput struct {
				Body struct {
					Message string `json:"message"`
				}
			}

			huma.Register(gAPI, huma.Operation{
				Method: "GET",
				Path:   "/api/fake-huma",
			}, func(ctx context.Context, input *TestInput) (*TestOutput, error) {
				resp := &TestOutput{}
				resp.Body.Message = "hello"
				return resp, nil
			})

			rec := httptest.NewRecorder()
			realMux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/fake-huma", nil))
			if rec.Code != tc.wantStatus {
				t.Fatalf("got %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}
