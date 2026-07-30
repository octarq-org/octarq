package dns_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/plugins/dns"
	"gorm.io/gorm"
)

// mountForManager mounts the dns plugin and returns the plugin.DNSManager it
// provides, plus the db it is backed by.
func mountForManager(t *testing.T, dbName string) (*gorm.DB, plugin.DNSManager) {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open("file:"+dbName+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	p := dns.New()
	if err := gdb.AutoMigrate(p.Models()...); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

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

	v, ok := reg.Lookup(plugin.ServiceDNSManager)
	if !ok {
		t.Fatal("dns plugin did not provide the DNS manager service")
	}
	mgr, ok := v.(plugin.DNSManager)
	if !ok {
		t.Fatalf("service %q is not a plugin.DNSManager: %T", plugin.ServiceDNSManager, v)
	}
	return gdb, mgr
}

// TestManagerScopesDomainToOrg pins the seam's own tenant boundary: every
// operation resolves the domain scoped to the org it was handed, so a consumer
// that forgets to check ownership cannot reach another tenant's zone. Reaching
// providerFor ("no provider account configured") means the domain resolved;
// "record not found" means the scope refused it.
func TestManagerScopesDomainToOrg(t *testing.T) {
	gdb, mgr := mountForManager(t, "memdb_dns_manager_scope")

	own := dns.Domain{OrgID: 1, Name: "mine.com"}
	other := dns.Domain{OrgID: 2, Name: "victim.com"}
	if err := gdb.Create(&own).Error; err != nil {
		t.Fatalf("seed own domain: %v", err)
	}
	if err := gdb.Create(&other).Error; err != nil {
		t.Fatalf("seed other domain: %v", err)
	}

	ctx := context.Background()
	rec := plugin.DNSRecord{Type: "TXT", Name: "_acme-challenge.victim.com", Content: "x"}

	// Org 1 reaching for org 2's domain: refused before any provider is built.
	crossOrg := map[string]error{}
	_, err := mgr.List(ctx, 1, other.ID)
	crossOrg["List"] = err
	_, err = mgr.Set(ctx, 1, other.ID, rec)
	crossOrg["Set"] = err
	crossOrg["Delete"] = mgr.Delete(ctx, 1, other.ID, "rec-1")

	for op, err := range crossOrg {
		if err == nil {
			t.Errorf("%s reached another org's domain and returned no error", op)
			continue
		}
		if !strings.Contains(err.Error(), "record not found") {
			t.Errorf("%s: expected the domain lookup to refuse the id, got %q", op, err)
		}
	}

	// No org at all is a caller bug, not a wildcard.
	if _, err := mgr.List(ctx, 0, own.ID); err == nil || !strings.Contains(err.Error(), "org id is required") {
		t.Errorf("List with no org: expected an org-required error, got %v", err)
	}

	// The caller's own domain resolves and gets as far as its provider.
	if _, err := mgr.List(ctx, 1, own.ID); err == nil || !strings.Contains(err.Error(), "no provider account") {
		t.Errorf("List on the caller's own domain: expected to reach providerFor, got %v", err)
	}
}
