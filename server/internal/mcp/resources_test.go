package mcp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

type testLinkRow struct {
	ID        uint      `gorm:"primaryKey"`
	OrgID     uint      `gorm:"column:owner_id"`
	Host      string    `gorm:"column:host"`
	Slug      string    `gorm:"column:slug"`
	Title     string    `gorm:"column:title"`
	Clicks    int64     `gorm:"column:clicks"`
	Enabled   bool      `gorm:"column:enabled"`
	Archived  bool      `gorm:"column:archived"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (testLinkRow) TableName() string { return "links" }

type testLinkEventRow struct {
	ID        uint      `gorm:"primaryKey"`
	LinkID    uint      `gorm:"column:link_id"`
	IsBot     bool      `gorm:"column:is_bot"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (testLinkEventRow) TableName() string { return "link_events" }

type testDomainRow struct {
	ID                uint            `gorm:"primaryKey"`
	OrgID             uint            `gorm:"column:owner_id"`
	Name              string          `gorm:"column:name"`
	ProviderAccountID uint            `gorm:"column:provider_account_id"`
	ZoneID            string          `gorm:"column:zone_id"`
	ForMail           bool            `gorm:"column:for_mail"`
	ForLink           bool            `gorm:"column:for_link"`
	LinkHosts         models.HostList `gorm:"column:link_hosts;type:text"`
	MailHosts         models.HostList `gorm:"column:mail_hosts;type:text"`
	CreatedAt         time.Time       `gorm:"column:created_at"`
	UpdatedAt         time.Time       `gorm:"column:updated_at"`
}

func (testDomainRow) TableName() string { return "domains" }

func setupResourcesTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "mcp_resources.db")
	gdb, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite test db: %v", err)
	}
	if err := gdb.AutoMigrate(&testLinkRow{}, &testLinkEventRow{}, &testDomainRow{}); err != nil {
		t.Fatalf("failed to auto migrate tables: %v", err)
	}
	return gdb
}

func TestMCPResources_ListAndRead(t *testing.T) {
	ctx := context.Background()
	gdb := setupResourcesTestDB(t)

	// Seed data for Org 1
	now := time.Now()
	l1 := testLinkRow{OrgID: 1, Host: "go.corp.com", Slug: "blog", Title: "Blog", Clicks: 30, Enabled: true, Archived: false, CreatedAt: now}
	l2 := testLinkRow{OrgID: 1, Host: "go.corp.com", Slug: "promo", Title: "Promo", Clicks: 20, Enabled: true, Archived: false, CreatedAt: now}
	lArchived := testLinkRow{OrgID: 1, Host: "go.corp.com", Slug: "old", Title: "Old", Clicks: 5, Enabled: false, Archived: true, CreatedAt: now}
	// Seed link for Org 2
	lOrg2 := testLinkRow{OrgID: 2, Host: "link.other.com", Slug: "secret", Title: "Secret", Clicks: 100, Enabled: true, Archived: false, CreatedAt: now}
	gdb.Create(&l1)
	gdb.Create(&l2)
	gdb.Create(&lArchived)
	gdb.Create(&lOrg2)

	// Seed events for l1
	gdb.Create(&testLinkEventRow{LinkID: l1.ID, IsBot: false, CreatedAt: now.Add(-2 * 24 * time.Hour)})  // within 7d
	gdb.Create(&testLinkEventRow{LinkID: l1.ID, IsBot: false, CreatedAt: now.Add(-10 * 24 * time.Hour)}) // within 30d
	gdb.Create(&testLinkEventRow{LinkID: l1.ID, IsBot: true, CreatedAt: now.Add(-1 * 24 * time.Hour)})   // bot, excluded
	gdb.Create(&testLinkEventRow{LinkID: lOrg2.ID, IsBot: false, CreatedAt: now.Add(-1 * 24 * time.Hour)})

	// Seed domains for Org 1
	d1 := testDomainRow{
		OrgID:             1,
		Name:              "corp.com",
		ProviderAccountID: 1,
		ZoneID:            "zone-123",
		ForMail:           true,
		ForLink:           true,
		LinkHosts:         models.HostList{{Host: "go.corp.com", Enabled: true}},
		MailHosts:         models.HostList{{Host: "corp.com", Enabled: true}},
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	dPendingZone := testDomainRow{
		OrgID:             1,
		Name:              "pendingzone.com",
		ProviderAccountID: 1,
		ZoneID:            "",
		ForMail:           true,
		ForLink:           false,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	dPendingProv := testDomainRow{
		OrgID:             1,
		Name:              "pendingprov.com",
		ProviderAccountID: 0,
		ZoneID:            "",
		ForMail:           false,
		ForLink:           true,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	// Seed domain for Org 2
	dOrg2 := testDomainRow{
		OrgID:             2,
		Name:              "othercorp.com",
		ProviderAccountID: 2,
		ZoneID:            "zone-other",
		ForMail:           true,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	gdb.Create(&d1)
	gdb.Create(&dPendingZone)
	gdb.Create(&dPendingProv)
	gdb.Create(&dOrg2)

	// Create MCP server instance for Org 1
	server := NewServerInstance(gdb, 1, nil)
	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1.0"}, nil)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Close()
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	// 1. Verify ListResources
	resList, err := clientSession.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("ListResources failed: %v", err)
	}
	resourceMap := make(map[string]*mcp.Resource)
	for _, r := range resList.Resources {
		resourceMap[r.URI] = r
	}

	if rTraffic, ok := resourceMap[ResourceURITrafficMetrics]; !ok {
		t.Errorf("missing resource %s", ResourceURITrafficMetrics)
	} else {
		if rTraffic.MIMEType != "application/json" {
			t.Errorf("expected mimeType application/json, got %s", rTraffic.MIMEType)
		}
		if rTraffic.Name != "traffic_metrics" {
			t.Errorf("expected name traffic_metrics, got %s", rTraffic.Name)
		}
	}

	if rDomains, ok := resourceMap[ResourceURIDomainsHealth]; !ok {
		t.Errorf("missing resource %s", ResourceURIDomainsHealth)
	} else {
		if rDomains.MIMEType != "application/json" {
			t.Errorf("expected mimeType application/json, got %s", rDomains.MIMEType)
		}
		if rDomains.Name != "domains_health" {
			t.Errorf("expected name domains_health, got %s", rDomains.Name)
		}
	}

	// 2. Read octarq://metrics/traffic
	readTraffic, err := clientSession.ReadResource(ctx, &mcp.ReadResourceParams{URI: ResourceURITrafficMetrics})
	if err != nil {
		t.Fatalf("ReadResource(traffic) failed: %v", err)
	}
	if len(readTraffic.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(readTraffic.Contents))
	}
	var trafficData TrafficMetricsResource
	if err := json.Unmarshal([]byte(readTraffic.Contents[0].Text), &trafficData); err != nil {
		t.Fatalf("unmarshal traffic resource JSON: %v", err)
	}
	if trafficData.OrgID != 1 {
		t.Errorf("expected OrgID = 1, got %d", trafficData.OrgID)
	}
	if trafficData.TotalLinks != 3 {
		t.Errorf("expected TotalLinks = 3, got %d", trafficData.TotalLinks)
	}
	if trafficData.ActiveLinks != 2 {
		t.Errorf("expected ActiveLinks = 2, got %d", trafficData.ActiveLinks)
	}
	if trafficData.TotalClicks != 55 {
		t.Errorf("expected TotalClicks = 55 (30+20+5), got %d", trafficData.TotalClicks)
	}
	if trafficData.Clicks7d != 1 {
		t.Errorf("expected Clicks7d = 1 (excluding bot), got %d", trafficData.Clicks7d)
	}
	if trafficData.Clicks30d != 2 {
		t.Errorf("expected Clicks30d = 2, got %d", trafficData.Clicks30d)
	}
	if len(trafficData.TopLinks) != 2 {
		t.Errorf("expected 2 active top links, got %d", len(trafficData.TopLinks))
	}

	// 3. Read octarq://domains/health
	readDomains, err := clientSession.ReadResource(ctx, &mcp.ReadResourceParams{URI: ResourceURIDomainsHealth})
	if err != nil {
		t.Fatalf("ReadResource(domains) failed: %v", err)
	}
	if len(readDomains.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(readDomains.Contents))
	}
	var domainsData DomainsHealthResource
	if err := json.Unmarshal([]byte(readDomains.Contents[0].Text), &domainsData); err != nil {
		t.Fatalf("unmarshal domains health resource JSON: %v", err)
	}
	if domainsData.OrgID != 1 {
		t.Errorf("expected OrgID = 1, got %d", domainsData.OrgID)
	}
	if domainsData.TotalDomains != 3 {
		t.Errorf("expected TotalDomains = 3, got %d", domainsData.TotalDomains)
	}
	if domainsData.ActiveMailDomains != 2 {
		t.Errorf("expected ActiveMailDomains = 2, got %d", domainsData.ActiveMailDomains)
	}
	if domainsData.ActiveLinkDomains != 2 {
		t.Errorf("expected ActiveLinkDomains = 2, got %d", domainsData.ActiveLinkDomains)
	}

	statusMap := make(map[string]string)
	for _, d := range domainsData.Domains {
		statusMap[d.Name] = d.Status
	}
	if statusMap["corp.com"] != "healthy" {
		t.Errorf("expected corp.com to be healthy, got %s", statusMap["corp.com"])
	}
	if statusMap["pendingzone.com"] != "pending_zone" {
		t.Errorf("expected pendingzone.com to be pending_zone, got %s", statusMap["pendingzone.com"])
	}
	if statusMap["pendingprov.com"] != "pending_provider" {
		t.Errorf("expected pendingprov.com to be pending_provider, got %s", statusMap["pendingprov.com"])
	}

	// 4. Test reading unknown URI
	_, errNotFound := clientSession.ReadResource(ctx, &mcp.ReadResourceParams{URI: "octarq://unknown/resource"})
	if errNotFound == nil {
		t.Errorf("expected error for unknown resource URI, got nil")
	}
}

func TestMCPResources_EdgeCasesAndTenantIsolation(t *testing.T) {
	ctx := context.Background()
	gdb := setupResourcesTestDB(t)

	sOrg0 := &server{gdb: gdb, orgID: 0}

	// Call handleResourceTraffic with nil params
	if _, err := sOrg0.handleResourceTraffic(ctx, nil); err == nil {
		t.Error("expected error for nil request")
	}

	// Call with unknown URI
	if _, err := sOrg0.handleResourceTraffic(ctx, &mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: "octarq://other"}}); err == nil {
		t.Error("expected error for invalid URI in handleResourceTraffic")
	}
	if _, err := sOrg0.handleResourceDomainsHealth(ctx, &mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: "octarq://other"}}); err == nil {
		t.Error("expected error for invalid URI in handleResourceDomainsHealth")
	}
	if _, err := sOrg0.handleResourceDomainsHealth(ctx, nil); err == nil {
		t.Error("expected error for nil request in handleResourceDomainsHealth")
	}

	// Call with no org in context and server orgID = 0
	if _, err := sOrg0.handleResourceTraffic(ctx, &mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: ResourceURITrafficMetrics}}); err == nil {
		t.Error("expected error when no org present")
	}
	if _, err := sOrg0.handleResourceDomainsHealth(ctx, &mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: ResourceURIDomainsHealth}}); err == nil {
		t.Error("expected error when no org present")
	}

	// Fresh empty DB without tables
	freshDBPath := filepath.Join(t.TempDir(), "fresh.db")
	freshDB, _ := gorm.Open(sqlite.Open(freshDBPath), &gorm.Config{})
	sFresh := &server{gdb: freshDB, orgID: 1}
	resTraffic, err := sFresh.handleResourceTraffic(ctx, &mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: ResourceURITrafficMetrics}})
	if err != nil {
		t.Fatalf("unexpected error on fresh DB: %v", err)
	}
	if len(resTraffic.Contents) == 0 {
		t.Errorf("expected response content on fresh DB")
	}

	resDomains, err := sFresh.handleResourceDomainsHealth(ctx, &mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: ResourceURIDomainsHealth}})
	if err != nil {
		t.Fatalf("unexpected error on fresh DB: %v", err)
	}
	if len(resDomains.Contents) == 0 {
		t.Errorf("expected response content on fresh DB")
	}

	// Verify plugin.WithOrgID takes precedence over server.orgID
	ctxOrg2 := plugin.WithOrgID(ctx, 2)
	resTrafficOrg2, err := sFresh.handleResourceTraffic(ctxOrg2, &mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: ResourceURITrafficMetrics}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var dataOrg2 TrafficMetricsResource
	_ = json.Unmarshal([]byte(resTrafficOrg2.Contents[0].Text), &dataOrg2)
	if dataOrg2.OrgID != 2 {
		t.Errorf("expected OrgID = 2 from context, got %d", dataOrg2.OrgID)
	}
}
