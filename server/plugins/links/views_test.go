package links_test

import (
	"strings"
	"testing"

	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/plugins/links"
)

func TestRegisterViews(t *testing.T) {
	var registered []plugin.TenantView

	pctx := &plugin.Context{
		RegisterTenantView: func(v plugin.TenantView) {
			registered = append(registered, v)
		},
	}

	links.RegisterViews(pctx)

	if len(registered) != 2 {
		t.Fatalf("expected 2 views registered, got %d", len(registered))
	}

	viewsMap := make(map[string]plugin.TenantView)
	for _, v := range registered {
		viewsMap[v.Name] = v
	}

	// tenant_links
	tl, ok := viewsMap["tenant_links"]
	if !ok {
		t.Fatal("tenant_links view not registered")
	}
	if len(tl.Columns) == 0 {
		t.Error("tenant_links columns should not be empty")
	}
	if len(tl.Sensitive) == 0 || tl.Sensitive[0] != "password" {
		t.Errorf("tenant_links sensitive = %v, want ['password']", tl.Sensitive)
	}
	defTL := tl.Definition(42)
	if !strings.Contains(defTL, "WHERE owner_id = 42") {
		t.Errorf("tenant_links definition missing WHERE owner_id = 42: %s", defTL)
	}

	// tenant_link_events
	tle, ok := viewsMap["tenant_link_events"]
	if !ok {
		t.Fatal("tenant_link_events view not registered")
	}
	if len(tle.Columns) == 0 {
		t.Error("tenant_link_events columns should not be empty")
	}
	if len(tle.Sensitive) == 0 || tle.Sensitive[0] != "fingerprint" {
		t.Errorf("tenant_link_events sensitive = %v, want ['fingerprint']", tle.Sensitive)
	}
	defTLE := tle.Definition(42)
	if !strings.Contains(defTLE, "WHERE l.owner_id = 42") {
		t.Errorf("tenant_link_events definition missing WHERE l.owner_id = 42: %s", defTLE)
	}

	// Nil ctx or nil RegisterTenantView should not panic
	links.RegisterViews(nil)
	links.RegisterViews(&plugin.Context{})
}
