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

func TestRecordingAPI_And_Collisions(t *testing.T) {
	realMux := http.NewServeMux()
	config := huma.DefaultConfig("Test API", "1.0.0")
	realAPI := humago.New(realMux, config)
	routes := newRouteRegistry()

	recAPI := &recordingAPI{
		API: realAPI,
		rAdapter: &recordingAdapter{
			Adapter: realAPI.Adapter(),
			routes:  routes,
			owner:   "core-plugin",
		},
	}

	type TestInput struct{}
	type TestOutput struct {
		Body struct {
			Message string `json:"message"`
		}
	}

	huma.Register(recAPI, huma.Operation{
		Method: "GET",
		Path:   "/api/core-huma",
	}, func(ctx context.Context, input *TestInput) (*TestOutput, error) {
		return &TestOutput{}, nil
	})

	// Duplicate route on recordingAPI should be caught by routes
	huma.Register(recAPI, huma.Operation{
		Method: "GET",
		Path:   "/api/core-huma",
	}, func(ctx context.Context, input *TestInput) (*TestOutput, error) {
		return &TestOutput{}, nil
	})

	if routes.Err() == nil {
		t.Errorf("expected collision error on recordingAdapter duplicate register")
	}

	// Test collision on gatedAdapter
	gRoutes := newRouteRegistry()
	gAPI := &gatedAPI{
		API: realAPI,
		gAdapter: &gatedAdapter{
			Adapter: realAPI.Adapter(),
			plugin:  "gated1",
			routes:  gRoutes,
			owner:   "gated1",
			enabled: func(r *http.Request, p string) (bool, bool) { return true, true },
		},
	}
	huma.Register(gAPI, huma.Operation{
		Method: "POST",
		Path:   "/api/gated-dup",
	}, func(ctx context.Context, input *TestInput) (*TestOutput, error) {
		return &TestOutput{}, nil
	})
	huma.Register(gAPI, huma.Operation{
		Method: "POST",
		Path:   "/api/gated-dup",
	}, func(ctx context.Context, input *TestInput) (*TestOutput, error) {
		return &TestOutput{}, nil
	})
	if gRoutes.Err() == nil {
		t.Errorf("expected collision error on gatedAdapter duplicate register")
	}
}

func TestIsNonWorkspaceRoute(t *testing.T) {
	if !isNonWorkspaceRoute("/api/instance-settings") {
		t.Errorf("expected /api/instance-settings to be non-workspace route")
	}
	if !isNonWorkspaceRoute("/api/sso/login") {
		t.Errorf("expected /api/sso/login to be non-workspace route")
	}
	if !isNonWorkspaceRoute("/api/customer/info") {
		t.Errorf("expected /api/customer/info to be non-workspace route")
	}
	if isNonWorkspaceRoute("/api/links") {
		t.Errorf("expected /api/links to NOT be non-workspace route")
	}
}
