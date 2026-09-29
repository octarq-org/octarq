package dns

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/internal/dnsprovider"
	"github.com/octarq-org/octarq/server/plugin"
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

func TestMCPUpdateDomainConfig(t *testing.T) {
	p, _ := setupFreshTestDB(t)
	p.db.Create(&Domain{OrgID: 1, Name: "cfg.com", ForMail: false, ForLink: false, Note: "init"})

	ctx := plugin.WithOrgID(context.Background(), 1)
	trueVal := true
	res, anyOut, err := p.mcpUpdateDomainConfig(ctx, nil, updateDomainConfigInput{
		ID:      1,
		ForMail: &trueVal,
		ForLink: &trueVal,
		Note:    "updated note",
	})
	if err != nil {
		t.Fatalf("mcpUpdateDomainConfig: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %v", res)
	}
	out := anyOut.(domainOut)
	if !out.ForMail || !out.ForLink {
		t.Errorf("expected forMail and forLink to be true, got %+v", out)
	}

	// Cross-tenant check: org 2 cannot update org 1's domain
	ctx2 := plugin.WithOrgID(context.Background(), 2)
	_, _, err = p.mcpUpdateDomainConfig(ctx2, nil, updateDomainConfigInput{ID: 1, Note: "hacked"})
	if err == nil {
		t.Error("expected error updating another tenant's domain")
	}
}

type mockDNSProvider struct {
	records []dnsprovider.Record
}

func (m *mockDNSProvider) ListRecords(ctx context.Context, zoneID string) ([]dnsprovider.Record, error) {
	return m.records, nil
}

func (m *mockDNSProvider) CreateRecord(ctx context.Context, zoneID string, r dnsprovider.Record) (dnsprovider.Record, error) {
	r.ID = "rec-created-1"
	m.records = append(m.records, r)
	return r, nil
}

func (m *mockDNSProvider) UpdateRecord(ctx context.Context, zoneID string, r dnsprovider.Record) (dnsprovider.Record, error) {
	for i, existing := range m.records {
		if existing.ID == r.ID {
			m.records[i] = r
			return r, nil
		}
	}
	m.records = append(m.records, r)
	return r, nil
}

func (m *mockDNSProvider) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	for i, existing := range m.records {
		if existing.ID == recordID {
			m.records = append(m.records[:i], m.records[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *mockDNSProvider) ListZones(ctx context.Context) ([]dnsprovider.Zone, error) {
	return nil, nil
}

func (m *mockDNSProvider) VerifyZone(ctx context.Context, zoneID string) (string, error) {
	return "api.example.com", nil
}

func TestMCPDNSRecordsLifecycle(t *testing.T) {
	p, _ := setupFreshTestDB(t)

	mockProv := &mockDNSProvider{
		records: []dnsprovider.Record{
			{ID: "rec-1", Type: "A", Name: "api", Content: "1.2.3.4", TTL: 300},
		},
	}
	dnsprovider.Register("mock-test", func([]byte) (dnsprovider.Provider, error) {
		return mockProv, nil
	})

	acc := ProviderAccount{OrgID: 1, Name: "acc1", Type: "mock-test", Config: "token123"}
	p.db.Create(&acc)
	dom := Domain{OrgID: 1, Name: "api.example.com", ProviderAccountID: acc.ID, ZoneID: "zone-123"}
	p.db.Create(&dom)

	ctx := plugin.WithOrgID(context.Background(), 1)

	// 1. List
	_, listOut, err := p.mcpListDNSRecords(ctx, nil, listDNSRecordsInput{DomainID: dom.ID})
	if err != nil {
		t.Fatalf("mcpListDNSRecords: %v", err)
	}
	recs := listOut.([]dnsprovider.Record)
	if len(recs) != 1 || recs[0].ID != "rec-1" {
		t.Fatalf("expected rec-1, got %+v", recs)
	}

	// 2. Create
	_, createOut, err := p.mcpSetDNSRecord(ctx, nil, setDNSRecordInput{
		DomainID: dom.ID,
		Type:     "A",
		Name:     "sub",
		Content:  "5.6.7.8",
		TTL:      300,
	})
	if err != nil {
		t.Fatalf("mcpSetDNSRecord create: %v", err)
	}
	created := createOut.(dnsprovider.Record)
	if created.ID != "rec-created-1" {
		t.Fatalf("expected rec-created-1, got %+v", created)
	}

	// 3. Update
	_, updateOut, err := p.mcpSetDNSRecord(ctx, nil, setDNSRecordInput{
		DomainID: dom.ID,
		RecordID: created.ID,
		Type:     "A",
		Name:     "sub",
		Content:  "9.9.9.9",
		TTL:      600,
	})
	if err != nil {
		t.Fatalf("mcpSetDNSRecord update: %v", err)
	}
	updated := updateOut.(dnsprovider.Record)
	if updated.Content != "9.9.9.9" {
		t.Fatalf("expected updated content 9.9.9.9, got %+v", updated)
	}

	// 4. Delete
	_, _, err = p.mcpDeleteDNSRecord(ctx, nil, deleteDNSRecordInput{
		DomainID: dom.ID,
		RecordID: created.ID,
	})
	if err != nil {
		t.Fatalf("mcpDeleteDNSRecord: %v", err)
	}
	if len(mockProv.records) != 1 {
		t.Fatalf("expected 1 record left, got %d", len(mockProv.records))
	}
}
