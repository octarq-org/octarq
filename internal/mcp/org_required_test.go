package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/plugin"
)

// No MCP tool may fall back to a default tenant when the context carries no org.
//
// Such a fallback used to exist in every tool ("if orgID == 0 { orgID = 1 }") and
// was harmless in the one place it was written for — the stdio CLI, where there
// is a single local operator. But the same code runs on the networked
// transports, where an absent org means an unidentified caller and defaulting it
// hands them the bootstrap tenant's data. The fix was to stop stdio being the
// odd caller out: RunWithPlugins now puts its org on the context like every
// other transport, so no tool needs a fallback at all.
func TestExportRefusesWithoutOrgInContext(t *testing.T) {
	s := &server{}
	_, _, err := s.exportData(context.Background(), nil, exportInput{Resource: "links"})
	if err == nil {
		t.Fatal("exportData served a request with no org in context; it must refuse rather than pick a tenant")
	}
	if !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("unexpected error %q", err)
	}
}

// The stdio CLI is the only caller with no HTTP request behind it, so it is the
// one that would break if the entry point stopped seeding the context. Pin it:
// without this, removing the fallbacks silently turns `octarq mcp` into a
// command that refuses every tool call.
func TestStdioContextCarriesTheOperatorOrg(t *testing.T) {
	ctx := plugin.WithOrgID(context.Background(), stdioOrgID)
	if got := plugin.OrgIDFromContext(ctx); got != stdioOrgID {
		t.Fatalf("stdio context org = %d, want %d", got, stdioOrgID)
	}
	if stdioOrgID == 0 {
		t.Fatal("stdioOrgID must be a real tenant; zero would make every stdio tool call refuse")
	}
}
