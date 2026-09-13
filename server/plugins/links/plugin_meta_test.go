package links

import (
	"context"
	"testing"

	"github.com/octarq-org/octarq/plugin"
)

func TestLinksPluginMetaPurgeExportAndMCP(t *testing.T) {
	t.Parallel()

	p, _ := setupFullLinksTestDB(t)
	ctx := context.Background()

	if p.Name() != "links" {
		t.Errorf("Name = %q", p.Name())
	}
	desc := p.Describe()
	if desc.Title != "Short Links" {
		t.Errorf("Describe Title = %q", desc.Title)
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

	lk := Link{OrgID: 1, Slug: "test-slug", Target: "https://example.com"}
	p.db.Create(&lk)
	ev := LinkEvent{LinkID: lk.ID, IP: "127.0.0.1"}
	p.db.Create(&ev)

	exp := p.exportData(1)
	if exp == nil || len(exp["links"].([]Link)) == 0 {
		t.Errorf("exportData failed: %+v", exp)
	}

	lkExp, err := p.mcpExportLinks(ctx, 1)
	if err != nil || lkExp == nil {
		t.Errorf("mcpExportLinks failed: %v, %+v", err, lkExp)
	}

	// MCP Tool Calls with context org 1
	ctxOrg := plugin.WithOrgID(ctx, 1)
	_, lksOut, err := p.mcpListLinks(ctxOrg, nil, listLinksInput{})
	if err != nil || lksOut == nil {
		t.Errorf("mcpListLinks error=%v, out=%+v", err, lksOut)
	}

	// MCP Tool Calls without org context -> error
	if _, _, err := p.mcpListLinks(ctx, nil, listLinksInput{}); err == nil {
		t.Error("expected error when calling mcpListLinks with no org")
	}

	if err := p.purge(1); err != nil {
		t.Fatalf("purge error: %v", err)
	}
	exp2 := p.exportData(1)
	if len(exp2["links"].([]Link)) != 0 {
		t.Errorf("expected 0 links after purge, got %v", exp2)
	}
}
