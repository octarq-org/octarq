package app

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

// A plugin route registers on the mux outside the core middleware, so it must
// resolve its own caller. Without AuthenticateRequest a plugin can never admit
// an "oct_" API token, and every automated caller is forced onto a cookie.
func TestPluginContext_AuthenticateRequestAdmitsAnAPIToken(t *testing.T) {
	a := bootApp(t)
	if err := a.gdb.AutoMigrate(&models.Token{}); err != nil {
		t.Fatalf("AutoMigrate token: %v", err)
	}

	const rawToken = "oct_" + "plugincontexttoken0123456789ab"
	tok := models.Token{
		UserID: 7,
		OrgID:  3,
		Hash:   models.HashToken(rawToken),
		Prefix: rawToken[:8],
		Name:   "automation",
	}
	if err := a.gdb.Create(&tok).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}

	pctx := a.buildPluginContext(pluginContextParams{auth: a.auth, gdb: a.gdb})
	if pctx.AuthenticateRequest == nil {
		t.Fatal("the plugin context has no AuthenticateRequest hook")
	}

	r := httptest.NewRequest("GET", "/api/oruq/overview", nil)
	r.Header.Set("Authorization", "Bearer "+rawToken)
	authed, ok := pctx.AuthenticateRequest(r)
	if !ok {
		t.Fatal("a valid API token was refused")
	}
	if got := plugin.OrgIDFromContext(authed.Context()); got != tok.OrgID {
		t.Fatalf("resolved workspace %d, want %d", got, tok.OrgID)
	}
}

// A token that is absent or unknown must not authenticate, or the hook would
// turn every anonymous plugin request into a workspace request.
func TestPluginContext_AuthenticateRequestRefusesAnUnknownToken(t *testing.T) {
	a := bootApp(t)
	if err := a.gdb.AutoMigrate(&models.Token{}); err != nil {
		t.Fatalf("AutoMigrate token: %v", err)
	}

	pctx := a.buildPluginContext(pluginContextParams{auth: a.auth, gdb: a.gdb})
	if pctx.AuthenticateRequest == nil {
		t.Fatal("the plugin context has no AuthenticateRequest hook")
	}

	for name, header := range map[string]string{
		"missing": "",
		"unknown": "Bearer " + strings.Repeat("oct_", 8),
	} {
		r := httptest.NewRequest("GET", "/api/oruq/overview", nil)
		if header != "" {
			r.Header.Set("Authorization", header)
		}
		if _, ok := pctx.AuthenticateRequest(r); ok {
			t.Fatalf("%s credential was accepted", name)
		}
	}
}
