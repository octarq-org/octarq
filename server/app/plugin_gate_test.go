package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/crypto"
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
	gate := a.pluginGate(stub)

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

func TestPluginGateDoesNotGateInstanceOrPublicRoutes(t *testing.T) {
	a := &App{auth: auth.New(&config.Config{}, &crypto.Cipher{})}
	stub := &stubPluginEnabled{answer: false}
	gate := a.pluginGate(stub)

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
		"/api/customer/me",
		"/api/portal/licenses",
		"/api/storefront",
		"/api/storefront/pro",
		"/api/delivery/pkg",
		"/api/updates/app/latest",
		"/api/webhook/cloud/stripe",
		"/api/license",
		"/api/update/status",
	}

	for _, path := range paths {
		stub.askedOK = false
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r = r.WithContext(auth.WithOrgID(r.Context(), 7))

		allowed, scoped := gate(r, "sso")
		if scoped {
			t.Errorf("path %s reported as workspace-scoped: instance/public routes must not 404 based on session workspace", path)
		}
		if allowed {
			t.Errorf("path %s reported allowed=true; non-scoped should express 'not scoped'", path)
		}
		if stub.askedOK {
			t.Errorf("path %s consulted per-workspace toggle", path)
		}
	}
}

func TestPluginGateGatesWorkspaceRequests(t *testing.T) {
	a := &App{auth: auth.New(&config.Config{}, &crypto.Cipher{})}

	for _, tc := range []struct{ enabled bool }{{true}, {false}} {
		stub := &stubPluginEnabled{answer: tc.enabled}
		gate := a.pluginGate(stub)

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
