package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/internal/crypto"
)

// stubPluginEnabled records what the gate asked about and answers a fixed value.
type stubPluginEnabled struct {
	answer  bool
	askedOK bool
}

func (s *stubPluginEnabled) PluginEnabled(uint, string) bool {
	s.askedOK = true
	return s.answer
}

// TestPluginGateDoesNotGateAnonymous pins an implicit contract (P3-21).
//
// A request with no workspace in session gets scoped=false, which makes gatedMux
// serve the route rather than 404 it. That reads like a hole and is not one:
// public plugin routes — payment webhooks, the buyer portal, license activation
// — carry no session, so there is no workspace whose toggle could be consulted.
// They authenticate themselves instead (webhook signature, customer cookie).
//
// The reason to pin it is the direction of the failure. Someone "hardening" this
// to scoped=true would 404 every inbound Stripe webhook and every buyer, and the
// symptom would be missing revenue rather than an error anyone sees.
func TestPluginGateDoesNotGateAnonymous(t *testing.T) {
	a := &App{auth: auth.New(&config.Config{}, &crypto.Cipher{})}
	stub := &stubPluginEnabled{answer: false}
	gate := a.pluginGate(stub)

	// No session cookie, no bearer token → no org.
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

// TestPluginGateGatesWorkspaceRequests is the other half: once a workspace IS
// resolved, the toggle decides. Without this, a gate that returned
// (false, false) unconditionally would satisfy the test above.
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
