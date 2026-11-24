package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/plugin"
)

type stubPluginEnabled struct {
	answer  bool
	askedOK bool
}

func (s *stubPluginEnabled) PluginEnabled(uint, string) bool {
	s.askedOK = true
	return s.answer
}

func TestPluginGateDoesNotGateAnonymous(t *testing.T) {
	a := &App{auth: auth.New(&config.Config{}, &crypto.Cipher{})}
	stub := &stubPluginEnabled{answer: false}
	gate := a.pluginGate(stub, nil)

	r := httptest.NewRequest(http.MethodPost, "/api/billing/webhook/stripe", nil)
	allowed, scoped := gate(r, "billing")

	if scoped {
		t.Errorf("anonymous request reported as workspace-scoped: gating it would 404 every payment webhook and every buyer")
	}
	if allowed {
		t.Errorf("anonymous request reported allowed=true; the gate should express 'not scoped', not 'permitted'")
	}
	if stub.askedOK {
		t.Errorf("gate consulted the per-workspace toggle for a request with no workspace")
	}
}

func TestPluginGateDoesNotGateReservedOrDeclaredRoutes(t *testing.T) {
	a := &App{auth: auth.New(&config.Config{}, &crypto.Cipher{})}
	stub := &stubPluginEnabled{answer: false}
	matcher, err := newUnscopedMatcher([]plugin.Plugin{unscopedStubPlugin{
		name: "commerce",
		prefixes: []string{
			"/api/customer/",
			"/api/portal/",
			"/api/storefront",
			"/api/storefront/",
			"/api/delivery/",
			"/api/updates/",
			"/api/license",
			"/api/update/",
		},
	}})
	if err != nil {
		t.Fatalf("build matcher: %v", err)
	}
	gate := a.pluginGate(stub, matcher)

	paths := []string{
		"/api/sso/instance/approvals",
		"/api/sso/instance/approvals/2",
		"/api/infra/storage-instance/1",
		"/api/infra/storage-instance/1/objects",
		"/api/instance/menus",
		"/api/instance-settings",
		"/api/sso/login",
		"/api/sso/acme/login",
		"/api/sso/acme/callback",
		"/api/auth/login",
		"/api/mcp/sse",
		"/api/health",
		"/api/status",
		"/api/customer/me",
		"/api/portal/licenses",
		"/api/storefront",
		"/api/storefront/pro",
		"/api/delivery/pkg",
		"/api/updates/app/latest",
		"/api/license",
		"/api/update/status",
	}

	for _, path := range paths {
		stub.askedOK = false
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r = r.WithContext(auth.WithOrgID(r.Context(), 7))

		allowed, scoped := gate(r, "sso")
		if scoped {
			t.Errorf("path %s reported as workspace-scoped: reserved or declared routes must not 404 based on session workspace", path)
		}
		if allowed {
			t.Errorf("path %s reported allowed=true; non-scoped should express 'not scoped'", path)
		}
		if stub.askedOK {
			t.Errorf("path %s consulted per-workspace toggle", path)
		}
	}

	stub.askedOK = false
	undeclared := httptest.NewRequest(http.MethodGet, "/api/billing/config", nil)
	undeclared = undeclared.WithContext(auth.WithOrgID(undeclared.Context(), 7))
	if _, scoped := gate(undeclared, "billing"); !scoped {
		t.Error("an undeclared business route must stay workspace-scoped")
	}
	if !stub.askedOK {
		t.Error("the gate did not consult the toggle for an undeclared route")
	}
}

func TestPluginGateGatesWorkspaceRequests(t *testing.T) {
	a := &App{auth: auth.New(&config.Config{}, &crypto.Cipher{})}

	for _, tc := range []struct{ enabled bool }{{true}, {false}} {
		stub := &stubPluginEnabled{answer: tc.enabled}
		gate := a.pluginGate(stub, nil)

		r := httptest.NewRequest(http.MethodGet, "/api/billing/config", nil)
		r = r.WithContext(auth.WithOrgID(r.Context(), 7))

		allowed, scoped := gate(r, "billing")
		if !scoped {
			t.Fatalf("request carrying org 7 not reported as workspace-scoped")
		}
		if allowed != tc.enabled {
			t.Errorf("toggle=%v: gate said allowed=%v", tc.enabled, allowed)
		}
		if !stub.askedOK {
			t.Errorf("toggle=%v: gate never consulted the per-workspace toggle", tc.enabled)
		}
	}
}
