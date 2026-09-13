package dns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/origin"
)

// TestDomainWritesInvalidateOriginCache pins that the domains table and
// origin's view of it cannot drift apart.
//
// origin caches its answers for minutes because the endpoints asking are
// unauthenticated and a Host-header sprayer would otherwise be a free
// full-table scan. That cache is only safe if every write here drops it. The
// case that bites hardest is the first domain on a fresh instance: until one
// exists, origin has no whitelist to check against and falls back to using the
// request Host verbatim, and a stale "no domains registered" answer keeps that
// fallback alive — accepting forged Hosts — after the operator has registered
// the domain meant to close it.
func TestDomainWritesInvalidateOriginCache(t *testing.T) {
	p, mkCtx := setupFullDNSTestDB(t)
	ctx := context.Background()
	rv := origin.NewResolver(p.db)

	// Warm the cache while the instance has no domains at all. Both answers
	// below are now held for ttl unless a write invalidates them.
	if rv.AnyRegistered() {
		t.Fatal("a database with no domain rows reported a registered whitelist")
	}
	if _, ok := rv.OwnerOf("app.mydomain.com"); ok {
		t.Fatal("OwnerOf found an owner before any domain was created")
	}

	acc := ProviderAccount{OrgID: 1, Name: "Test Provider", Type: "cloudflare"}
	if err := p.db.Create(&acc).Error; err != nil {
		t.Fatalf("seed provider account: %v", err)
	}

	create := httptest.NewRequest(http.MethodPost, "/api/dns/domains", nil)
	if _, err := p.createDomain(ctx, &CreateDomainInput{
		Ctx:  mkCtx(create),
		Body: domainDTO{Name: "mydomain.com", ProviderAccountID: acc.ID},
	}); err != nil {
		t.Fatalf("createDomain: %v", err)
	}

	if !rv.AnyRegistered() {
		t.Error("after creating a domain origin still reports no whitelist: the request Host is still being honoured verbatim")
	}
	org, ok := rv.OwnerOf("app.mydomain.com")
	if !ok || org != 1 {
		t.Errorf("OwnerOf(app.mydomain.com) = (%d, %v) after create, want (1, true)", org, ok)
	}

	var created Domain
	if err := p.db.Where("name = ?", "mydomain.com").First(&created).Error; err != nil {
		t.Fatalf("read back created domain: %v", err)
	}

	del := httptest.NewRequest(http.MethodDelete, "/api/dns/domains", nil)
	if _, err := p.deleteDomain(ctx, &DeleteDomainInput{Ctx: mkCtx(del), ID: created.ID}); err != nil {
		t.Fatalf("deleteDomain: %v", err)
	}

	if _, ok := rv.OwnerOf("app.mydomain.com"); ok {
		t.Error("a deleted domain still resolves to its former owner")
	}
}

// TestPurgeInvalidatesOriginCache covers workspace deletion, which drops the
// rows in bulk without ever loading them — the path a GORM hook would miss.
func TestPurgeInvalidatesOriginCache(t *testing.T) {
	p, _ := setupFullDNSTestDB(t)
	rv := origin.NewResolver(p.db)

	if err := p.db.Create(&Domain{OrgID: 1, Name: "purgeme.example"}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	origin.ClearDomainCache("purgeme.example")
	if org, ok := rv.OwnerOf("app.purgeme.example"); !ok || org != 1 {
		t.Fatalf("OwnerOf = (%d, %v) before purge, want (1, true); the assertion below would be vacuous", org, ok)
	}

	if err := p.purge(1); err != nil {
		t.Fatalf("purge: %v", err)
	}
	if _, ok := rv.OwnerOf("app.purgeme.example"); ok {
		t.Error("a purged workspace's hostname still resolves to it")
	}
}
