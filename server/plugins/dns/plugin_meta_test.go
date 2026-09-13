package dns

import (
	"context"
	"testing"

	"github.com/octarq-org/octarq/plugin"
)

func TestDNSPluginMetaPurgeExportAndMCP(t *testing.T) {
	t.Parallel()

	p, _ := setupFullDNSTestDB(t)
	ctx := context.Background()

	if p.Name() != "dns" {
		t.Errorf("Name = %q", p.Name())
	}
	desc := p.Describe()
	if desc.Title == "" {
		t.Errorf("Describe Title empty")
	}
	if len(p.Models()) == 0 {
		t.Error("Models empty")
	}
	if len(p.Menus()) == 0 {
		t.Error("Menus empty")
	}
	if len(p.Actions()) == 0 {
		t.Error("Actions empty")
	}
	if p.HelpDocsFS() == nil {
		t.Error("HelpDocsFS is nil")
	}

	acc := ProviderAccount{OrgID: 1, Name: "Prov 1", Type: "cloudflare"}
	p.db.Create(&acc)
	dom := Domain{OrgID: 1, Name: "testdom.com", ProviderAccountID: acc.ID}
	p.db.Create(&dom)
	tok := DDNSToken{OrgID: 1, DomainID: dom.ID, Label: "Token 1"}
	p.db.Create(&tok)

	exp := p.exportData(1)
	if exp == nil || len(exp["domains"].([]Domain)) == 0 {
		t.Errorf("exportData failed: %+v", exp)
	}

	domExp, err := p.mcpExportDomains(ctx, 1)
	if err != nil || domExp == nil {
		t.Errorf("mcpExportDomains failed: %v, %+v", err, domExp)
	}

	// MCP Tool Calls with context org 1
	ctxOrg := plugin.WithOrgID(ctx, 1)
	_, domsOut, err := p.mcpListDomains(ctxOrg, nil, listDomainsInput{})
	if err != nil || domsOut == nil {
		t.Errorf("mcpListDomains error=%v, out=%+v", err, domsOut)
	}

	// MCP Tool Calls without org context -> error
	if _, _, err := p.mcpListDomains(ctx, nil, listDomainsInput{}); err == nil {
		t.Error("expected error when calling mcpListDomains with no org")
	}

	if err := p.purge(1); err != nil {
		t.Fatalf("purge error: %v", err)
	}
	exp2 := p.exportData(1)
	if len(exp2["domains"].([]Domain)) != 0 {
		t.Errorf("expected 0 domains after purge, got %v", exp2)
	}
}
