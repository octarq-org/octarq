package mail_test

import (
	"strings"
	"testing"

	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/plugins/mail"
)

func TestRegisterViews(t *testing.T) {
	var registered []plugin.TenantView

	pctx := &plugin.Context{
		RegisterTenantView: func(v plugin.TenantView) {
			registered = append(registered, v)
		},
	}

	mail.RegisterViews(pctx)

	if len(registered) != 1 {
		t.Fatalf("expected 1 view registered, got %d", len(registered))
	}

	te := registered[0]
	if te.Name != "tenant_emails" {
		t.Errorf("got name %q, want tenant_emails", te.Name)
	}
	if len(te.Columns) == 0 {
		t.Error("tenant_emails columns should not be empty")
	}

	defTE := te.Definition(99)
	if !strings.Contains(defTE, "WHERE m.owner_id = 99") {
		t.Errorf("tenant_emails definition missing WHERE m.owner_id = 99: %s", defTE)
	}

	// Nil ctx or nil RegisterTenantView should not panic
	mail.RegisterViews(nil)
	mail.RegisterViews(&plugin.Context{})
}
