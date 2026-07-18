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

func TestDNSPluginMigrationLegacy(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open("file:memdb_dns_legacy?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("expected no error opening DB, got %v", err)
	}

	p := dns.New()
	if err := gdb.AutoMigrate(p.Models()...); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	// 1. Manually create the legacy domains table columns
	_ = gdb.Exec("ALTER TABLE domains ADD COLUMN provider TEXT")
	_ = gdb.Exec("ALTER TABLE domains ADD COLUMN config TEXT")

	// 2. Insert some legacy domain rows
	err = gdb.Exec(`INSERT INTO domains (name, provider, config, owner_id) VALUES 
		('legacy1.com', 'cloudflare', 'enc-token-1', 1),
		('legacy2.com', 'cloudflare', 'enc-token-1', 1),
		('legacy3.com', '', '', 1)`).Error
	if err != nil {
		t.Fatalf("failed to insert legacy data: %v", err)
	}

	// 3. Mount plugin — it should detect legacy columns and migrate them
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("t", "1.0"))
	reg := plugin.NewRegistry()
	p.Mount(nil, &plugin.Context{
		Huma:    api,
		DB:      gdb,
		OrgID:   func(*http.Request) uint { return 1 },
		Provide: reg.Provide,
		Lookup:  reg.Lookup,
	})

	// 4. Verify that ProviderAccounts were created and domains were updated
	var count int64
	gdb.Model(&dns.ProviderAccount{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 migrated provider account, got %d", count)
	}

	var d1 dns.Domain
	gdb.Where("name = ?", "legacy1.com").First(&d1)
	if d1.ProviderAccountID == 0 {
		t.Errorf("legacy1.com provider_account_id was not updated")
	}

	var d2 dns.Domain
	gdb.Where("name = ?", "legacy2.com").First(&d2)
	if d2.ProviderAccountID != d1.ProviderAccountID {
		t.Errorf("legacy2.com should share the same provider account, got different ID")
	}
}
