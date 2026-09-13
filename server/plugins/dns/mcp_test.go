package dns

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/plugin"
)

func TestRegisterMCPAddsListDomains(t *testing.T) {
	p, _ := setupFreshTestDB(t)
	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	// Must not panic; a second registration must be idempotent.
	p.RegisterMCP(srv)
	p.RegisterMCP(srv)

	ctx := plugin.WithOrgID(context.Background(), 7)
	acc := ProviderAccount{OrgID: 7, Name: "mcp-acc", Type: "cloudflare"}
	if err := p.db.Create(&acc).Error; err != nil {
		t.Fatal(err)
	}
	if err := p.db.Create(&Domain{OrgID: 7, Name: "mcp.example.com", ProviderAccountID: acc.ID, ForLink: true}).Error; err != nil {
		t.Fatal(err)
	}
	res, doms, err := p.mcpListDomains(ctx, nil, listDomainsInput{})
	if err != nil {
		t.Fatalf("mcpListDomains: %v", err)
	}
	list := doms.([]domainOut)
	if len(list) != 1 || list[0].Name != "mcp.example.com" || !list[0].ForLink {
		t.Fatalf("mcpListDomains = %+v", list)
	}
	var body []map[string]any
	if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &body); err != nil {
		t.Fatalf("tool result is not a JSON array: %v", err)
	}
	if len(body) != 1 || body[0]["name"] != "mcp.example.com" {
		t.Fatalf("tool result payload = %+v", body)
	}

	// Export mirrors the same rows.
	exported, err := p.mcpExportDomains(ctx, 7)
	if err != nil || len(exported.([]Domain)) != 1 {
		t.Fatalf("mcpExportDomains: %v %+v", err, exported)
	}
}

func TestMCPExportScopedToOrg(t *testing.T) {
	p, _ := setupFreshTestDB(t)
	p.db.Create(&Domain{OrgID: 1, Name: "mine.com"})
	p.db.Create(&Domain{OrgID: 2, Name: "v.com"})
	exported, err := p.mcpExportDomains(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	list := exported.([]Domain)
	if len(list) != 1 || list[0].Name != "v.com" {
		t.Errorf("export must be scoped to the org: %+v", list)
	}
}

func TestJSONResultMarshalError(t *testing.T) {
	_, _, err := jsonResult(make(chan int))
	if err == nil {
		t.Fatal("jsonResult of an unsupported value must error")
		return
	}
}
