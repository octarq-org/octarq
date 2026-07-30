package dns_test

import (
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/plugins/dns"
	"gorm.io/gorm"
)

func TestMigrateLegacyMultiTenant(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open("file:memdb_dns_migrate_legacy?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	p := dns.New()
	if err := gdb.AutoMigrate(p.Models()...); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	// 1. Create legacy columns
	_ = gdb.Exec("ALTER TABLE domains ADD COLUMN provider TEXT")
	_ = gdb.Exec("ALTER TABLE domains ADD COLUMN config TEXT")

	// 2. Insert test data:
	// - Org 2 (A): 1 legacy domain
	// - Org 3 (B): 1 legacy domain
	// - Org 4: 2 legacy domains with different config ciphertext (to verify deduplication per org)
	// - Org 0 (unpopulated owner_id): 1 legacy domain (to verify fallback to org 1)
	err = gdb.Exec(`INSERT INTO domains (id, name, provider, config, owner_id, provider_account_id) VALUES 
		(10, 'org2-domain.com', 'cloudflare', 'enc-token-org2', 2, 0),
		(20, 'org3-domain.com', 'cloudflare', 'enc-token-org3', 3, 0),
		(30, 'org4-domain1.com', 'dnspod', 'enc-token-org4-nonce1', 4, 0),
		(31, 'org4-domain2.com', 'dnspod', 'enc-token-org4-nonce2', 4, 0),
		(40, 'org0-domain.com', 'cloudflare', 'enc-token-org0', 0, 0)`).Error
	if err != nil {
		t.Fatalf("failed to insert legacy data: %v", err)
	}

	// Mount plugin to trigger migrateLegacy()
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("t", "1.0"))
	reg := plugin.NewRegistry()
	p.Mount(nil, &plugin.Context{
		Huma:        api,
		DB:          gdb,
		OrgID:       func(*http.Request) uint { return 1 },
		RequireRole: func(*http.Request, string) bool { return true },
		Provide:     reg.Provide,
		Lookup:      reg.Lookup,
	})

	// Test 1: Orgs 2 and 3 should have separate ProviderAccounts with owner_id 2 and 3
	var acc2, acc3 dns.ProviderAccount
	var d2, d3 dns.Domain
	gdb.Where("name = ?", "org2-domain.com").First(&d2)
	gdb.Where("name = ?", "org3-domain.com").First(&d3)

	if d2.ProviderAccountID == 0 {
		t.Fatalf("org2-domain.com provider_account_id is 0")
	}
	if d3.ProviderAccountID == 0 {
		t.Fatalf("org3-domain.com provider_account_id is 0")
	}

	if err := gdb.First(&acc2, d2.ProviderAccountID).Error; err != nil {
		t.Fatalf("failed to find ProviderAccount for org2 domain: %v", err)
	}
	if err := gdb.First(&acc3, d3.ProviderAccountID).Error; err != nil {
		t.Fatalf("failed to find ProviderAccount for org3 domain: %v", err)
	}

	if acc2.OrgID != 2 {
		t.Errorf("expected acc2.OrgID = 2, got %d", acc2.OrgID)
	}
	if acc3.OrgID != 3 {
		t.Errorf("expected acc3.OrgID = 3, got %d", acc3.OrgID)
	}
	if acc2.ID == acc3.ID {
		t.Errorf("expected different ProviderAccounts for Org 2 and Org 3, but got same ID %d", acc2.ID)
	}

	// Test 2: Org 4 has 2 legacy domains -> should produce only 1 ProviderAccount shared by both
	var d4_1, d4_2 dns.Domain
	gdb.Where("name = ?", "org4-domain1.com").First(&d4_1)
	gdb.Where("name = ?", "org4-domain2.com").First(&d4_2)
	if d4_1.ProviderAccountID == 0 || d4_2.ProviderAccountID == 0 {
		t.Fatalf("org4 domains provider_account_id not set")
	}
	if d4_1.ProviderAccountID != d4_2.ProviderAccountID {
		t.Errorf("org4 domains should share provider account, but got %d and %d", d4_1.ProviderAccountID, d4_2.ProviderAccountID)
	}
	var countOrg4Acc int64
	gdb.Model(&dns.ProviderAccount{}).Where("owner_id = ?", 4).Count(&countOrg4Acc)
	if countOrg4Acc != 1 {
		t.Errorf("expected 1 ProviderAccount for org 4, got %d", countOrg4Acc)
	}

	// Test 3: owner_id = 0 legacy row -> falls back to org 1
	var d0 dns.Domain
	gdb.Where("name = ?", "org0-domain.com").First(&d0)
	if d0.ProviderAccountID == 0 {
		t.Fatalf("org0 domain provider_account_id is 0")
	}
	var acc0 dns.ProviderAccount
	if err := gdb.First(&acc0, d0.ProviderAccountID).Error; err != nil {
		t.Fatalf("failed to find ProviderAccount for org0 domain: %v", err)
	}
	if acc0.OrgID != 1 {
		t.Errorf("expected acc0.OrgID = 1 (fallback), got %d", acc0.OrgID)
	}
}
