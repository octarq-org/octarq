package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/plugin"
)

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

	huma.Register(recAPI, huma.Operation{
		Method: "GET",
		Path:   "/api/core-huma",
	}, func(ctx context.Context, input *TestInput) (*TestOutput, error) {
		return &TestOutput{}, nil
	})

	if routes.Err() == nil {
		t.Errorf("expected collision error on recordingAdapter duplicate register")
	}

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

func TestIsPlatformReservedPath(t *testing.T) {
	for _, path := range []string{
		"/api/instance-settings",
		"/api/instance/menus",
		"/api/sso/login",
		"/api/sso/acme/callback",
		"/api/auth/login",
		"/api/mcp/sse",
		"/api/health",
		"/api/status",
	} {
		if !isPlatformReservedPath(path) {
			t.Errorf("expected %s to be a platform-reserved route", path)
		}
	}
	for _, path := range []string{"/api/customer/info", "/api/links", "/api/billing/config"} {
		if isPlatformReservedPath(path) {
			t.Errorf("expected %s NOT to be a platform-reserved route", path)
		}
	}
}

func TestUnscopedMatcherExactAndPrefix(t *testing.T) {
	matcher, err := newUnscopedMatcher([]plugin.Plugin{
		unscopedStubPlugin{name: "commerce", prefixes: []string{"/api/license", "/api/storefront/"}},
	})
	if err != nil {
		t.Fatalf("build matcher: %v", err)
	}
	for _, path := range []string{"/api/license", "/api/storefront/checkout"} {
		if !matcher.Matches(path) {
			t.Errorf("expected %s to match a declaration", path)
		}
	}
	for _, path := range []string{"/api/license/verify", "/api/storefront", "/api/links"} {
		if matcher.Matches(path) {
			t.Errorf("expected %s NOT to match a declaration", path)
		}
	}

	if _, err := newUnscopedMatcher([]plugin.Plugin{
		unscopedStubPlugin{name: "a", prefixes: []string{"/api/license"}},
		unscopedStubPlugin{name: "b", prefixes: []string{"/api/license"}},
	}); err == nil {
		t.Error("duplicate declarations across plugins must fail startup")
	}

	if _, err := newUnscopedMatcher([]plugin.Plugin{
		unscopedStubPlugin{name: "a", prefixes: []string{"/api"}},
	}); err == nil {
		t.Error("a too-broad declaration must fail startup")
	}
}
