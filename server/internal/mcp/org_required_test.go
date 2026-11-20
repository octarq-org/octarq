package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

func TestExportRefusesWithoutOrgInContext(t *testing.T) {
	s := &server{}
	_, _, err := s.exportData(context.Background(), nil, exportInput{Resource: "links"})
	if err == nil {
		t.Fatal("exportData served a request with no org in context; it must refuse rather than pick a tenant")
		return
	}
	if !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestStdioContextCarriesTheOperatorOrg(t *testing.T) {
	ctx := plugin.WithOrgID(context.Background(), stdioOrgID)
	if got := plugin.OrgIDFromContext(ctx); got != stdioOrgID {
		t.Fatalf("stdio context org = %d, want %d", got, stdioOrgID)
	}
	if stdioOrgID == 0 {
		t.Fatal("stdioOrgID must be a real tenant; zero would make every stdio tool call refuse")
	}
}
