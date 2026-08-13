package dns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/tenancy"
	"github.com/octarq-org/octarq/origin"
)

// seedBase writes the base_domain setting the reservation reads. Models are
// migrated by setupFullDNSTestDB, which includes the settings table.
func seedBase(t *testing.T, p *Plugin, base string) {
	t.Helper()
	if err := p.db.Create(&models.Setting{Key: models.BaseDomainSetting, Value: base}).Error; err != nil {
		t.Fatalf("seed base domain: %v", err)
	}
}

// Guard 4: when a base domain is configured, the base itself and every label
// under it are reserved — no org may register any of them by hand. This covers
// the base, an unprovisioned future tenant, and a retired slug, none of which
// has a Domain row for the unique index to defend.
func TestCreateDomainRejectsReservedBaseZone(t *testing.T) {
	p, mkCtx := setupFullDNSTestDB(t)
	seedBase(t, p, "app.octarq.org")
	acc := ProviderAccount{OrgID: 1, Name: "P", Type: "cloudflare"}
	if err := p.db.Create(&acc).Error; err != nil {
		t.Fatalf("seed provider account: %v", err)
	}
	// A retired slug, parked in OrgSlugHistory like the rename path leaves it.
	if err := p.db.Create(&models.OrgSlugHistory{Slug: "oldslug", OrgID: 1, RetiredAt: time.Now()}).Error; err != nil {
		t.Fatalf("seed slug history: %v", err)
	}

	ctx := context.Background()
	rejected := []string{
		"app.octarq.org",         // the base itself
		"nobody.app.octarq.org",  // a label that may become a tenant's address
		"oldslug.app.octarq.org", // a retired slug's address — still reserved
	}
	for _, name := range rejected {
		req := httptest.NewRequest(http.MethodPost, "/api/dns/domains", nil)
		_, err := p.createDomain(ctx, &CreateDomainInput{
			Ctx:  mkCtx(req),
			Body: domainDTO{Name: name, ProviderAccountID: acc.ID},
		})
		if err == nil {
			t.Errorf("createDomain(%q) succeeded — the reserved base zone must not be manually registrable", name)
		}
		var n int64
		p.db.Model(&Domain{}).Where("name = ?", name).Count(&n)
		if n != 0 {
			t.Errorf("createDomain(%q) wrote a row despite rejection", name)
		}
	}

	// An unrelated custom domain is untouched by the reservation.
	req := httptest.NewRequest(http.MethodPost, "/api/dns/domains", nil)
	if _, err := p.createDomain(ctx, &CreateDomainInput{
		Ctx:  mkCtx(req),
		Body: domainDTO{Name: "customer.com", ProviderAccountID: acc.ID},
	}); err != nil {
		t.Errorf("createDomain(customer.com) = %v, want success", err)
	}
}

// Guard 1: with no base domain configured nothing is reserved and today's
// behaviour is preserved exactly.
func TestCreateDomainAllowsBaseZoneWhenUnconfigured(t *testing.T) {
	p, mkCtx := setupFullDNSTestDB(t)
	acc := ProviderAccount{OrgID: 1, Name: "P", Type: "cloudflare"}
	if err := p.db.Create(&acc).Error; err != nil {
		t.Fatalf("seed provider account: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/dns/domains", nil)
	if _, err := p.createDomain(context.Background(), &CreateDomainInput{
		Ctx:  mkCtx(req),
		Body: domainDTO{Name: "app.octarq.org", ProviderAccountID: acc.ID},
	}); err != nil {
		t.Errorf("createDomain with no base configured = %v, want success (today's behaviour)", err)
	}
}

// underBaseZone is the decision function both createDomain and syncDomains rely
// on. syncDomains reaches a live provider for its zone list, which tests cannot
// stub, so this pins the predicate directly on top of the end-to-end create
// rejection above.
func TestUnderBaseZonePredicate(t *testing.T) {
	p, _ := setupFullDNSTestDB(t)
	seedBase(t, p, "app.octarq.org")

	if !underBaseZone(p.db, "app.octarq.org") {
		t.Error("the base itself must be reserved")
	}
	if !underBaseZone(p.db, "acme7x.app.octarq.org") {
		t.Error("a subdomain of the base must be reserved")
	}
	if !underBaseZone(p.db, "deep.sub.app.octarq.org") {
		t.Error("any depth under the base must be reserved")
	}
	// Scheme/port/case must not smuggle a label past the reservation.
	if !underBaseZone(p.db, "HTTPS://Acme7x.App.Octarq.Org:8443") {
		t.Error("a normalized subdomain of the base must be reserved")
	}
	if underBaseZone(p.db, "octarq.org") {
		t.Error("the apex above the base is not itself reserved")
	}
	if underBaseZone(p.db, "app.octarq.org.evil.com") {
		t.Error("a suffix lookalike of the base must not be reserved")
	}
	if underBaseZone(p.db, "customer.com") {
		t.Error("an unrelated domain must not be reserved")
	}

	// No base configured → nothing is reserved. The subtest opens its own DB:
	// setupFullDNSTestDB keys the in-memory DSN on t.Name(), so a fresh name
	// means fresh storage, and the base seeded above does not leak in.
	t.Run("unconfigured", func(t *testing.T) {
		p2, _ := setupFullDNSTestDB(t)
		if underBaseZone(p2.db, "app.octarq.org") {
			t.Error("without a configured base no zone is reserved")
		}
	})
}

// Guard 6 (trap 5): purging a workspace removes its provisioned tenant
// subdomain rows, so the hostname stops resolving to the deleted org while the
// bystander org's address is untouched.
func TestPurgeRemovesProvisionedSubdomain(t *testing.T) {
	p, _ := setupFullDNSTestDB(t)
	seedBase(t, p, "app.example.com")
	if _, _, err := tenancy.Provision(p.db, 42, "doomed"); err != nil {
		t.Fatalf("provision doomed org: %v", err)
	}
	if _, _, err := tenancy.Provision(p.db, 43, "bystander"); err != nil {
		t.Fatalf("provision bystander org: %v", err)
	}
	if owner, ok := origin.OwnerOf(p.db, "doomed.app.example.com"); !ok || owner != 42 {
		t.Fatalf("OwnerOf before purge = (%d, %v), want (42, true)", owner, ok)
	}

	if err := p.purge(42); err != nil {
		t.Fatalf("purge: %v", err)
	}

	var n int64
	p.db.Model(&Domain{}).Where("name = ?", "doomed.app.example.com").Count(&n)
	if n != 0 {
		t.Errorf("purged workspace's subdomain row still present")
	}
	if owner, ok := origin.OwnerOf(p.db, "doomed.app.example.com"); ok || owner != 0 {
		t.Errorf("OwnerOf after purge = (%d, %v), want (0, false)", owner, ok)
	}
	if owner, ok := origin.OwnerOf(p.db, "bystander.app.example.com"); !ok || owner != 43 {
		t.Errorf("bystander subdomain after purge = (%d, %v), want (43, true)", owner, ok)
	}
}
