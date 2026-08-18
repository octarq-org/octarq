package links

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/tenancy"
	"github.com/octarq-org/octarq/plugins/dns"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Link hosts on a multi-tenant install are org-scoped, but the host="" pool is
// shared across every tenant: tenant A's slug blocks tenant B forever, and the
// resulting 409 leaks which slugs exist. These tests pin both sides of the fix:
// Provision must make the tenant subdomain a real link host, and the handlers
// must refuse to fall back into the shared pool once one exists.
//
// Each test opens its own in-memory DB (unlike setupOwnershipTestDB, which
// shares one DB across the whole package): a base-domain setting seeded here
// must neither see nor leak state from other tests in the package.
func openTenantTestPlugin(t *testing.T) (*gorm.DB, *Plugin) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(append(models.AllModels(), &Link{}, &LinkEvent{}, &dns.Domain{}, &dns.ProviderAccount{})...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	p := New()
	p.db = db
	p.auth.OrgID = func(r *http.Request) uint {
		if val := r.Header.Get("X-Org-ID"); val != "" {
			var id uint
			fmt.Sscanf(val, "%d", &id)
			return id
		}
		return 1
	}
	return db, p
}

// A newly provisioned tenant subdomain must be a usable link host for its org:
// tenancy.Provision writes ForLink + LinkHosts, and ownsHost must see them.
func TestProvisionedTenantSubdomainIsLinkHost(t *testing.T) {
	db, p := openTenantTestPlugin(t)
	db.Create(&models.Setting{Key: models.BaseDomainSetting, Value: "app.example.com"})

	if _, ok, err := tenancy.Provision(db, 7, "acme7x"); err != nil || !ok {
		t.Fatalf("Provision = (ok=%v, err=%v), want provisioned", ok, err)
	}
	if !p.ownsHost(7, "acme7x.app.example.com") {
		t.Fatal("tenant subdomain is not a usable link host for its org")
	}
	if p.ownsHost(1, "acme7x.app.example.com") {
		t.Fatal("tenant subdomain is visible to a different org")
	}
}

// base + link hosts: host="" must be refused on every write path, so no tenant
// can drop back into the cross-tenant shared namespace.
func TestEmptyHostRejectedWhenInstanceMultiTenant(t *testing.T) {
	db, p := openTenantTestPlugin(t)
	db.Create(&models.Setting{Key: models.BaseDomainSetting, Value: "app.example.com"})
	db.Create(&dns.Domain{
		OrgID:     1,
		Name:      "acme.app.example.com",
		ForLink:   true,
		LinkHosts: models.HostList{{Host: "acme.app.example.com", Enabled: true}},
	})

	ctx := context.Background()

	// createLink with host="" → 400
	{
		req := httptest.NewRequest(http.MethodPost, "/api/links", nil)
		req.Header.Set("X-Org-ID", "1")
		input := &CreateLinkInput{
			Ctx:  humago.NewContext(nil, req, httptest.NewRecorder()),
			Body: linkDTO{Host: "", Slug: "naked", Target: "https://t.example/x"},
		}
		_, err := p.createLink(ctx, input)
		if err == nil || !strings.Contains(err.Error(), "host is required") {
			t.Fatalf("createLink(host='') err = %v, want 400 host-required", err)
		}
	}

	// quickCreateLink with host="" → 400
	{
		req := httptest.NewRequest(http.MethodPost, "/api/links/quick", nil)
		req.Header.Set("X-Org-ID", "1")
		input := &QuickCreateLinkInput{
			Ctx:  humago.NewContext(nil, req, httptest.NewRecorder()),
			Body: QuickCreateLinkBody{URL: "https://t.example/y"},
		}
		_, err := p.quickCreateLink(ctx, input)
		if err == nil || !strings.Contains(err.Error(), "host is required") {
			t.Fatalf("quickCreateLink(host='') err = %v, want 400 host-required", err)
		}
	}

	// updateLink moving an existing link to host="" → 400
	{
		req := httptest.NewRequest(http.MethodPost, "/api/links", nil)
		req.Header.Set("X-Org-ID", "1")
		create := &CreateLinkInput{
			Ctx:  humago.NewContext(nil, req, httptest.NewRecorder()),
			Body: linkDTO{Host: "acme.app.example.com", Slug: "homed", Target: "https://t.example/homed"},
		}
		out, err := p.createLink(ctx, create)
		if err != nil {
			t.Fatalf("seed link: %v", err)
		}
		reqUp := httptest.NewRequest(http.MethodPut, "/api/links/1", nil)
		reqUp.Header.Set("X-Org-ID", "1")
		upd := &UpdateLinkInput{
			Ctx:  humago.NewContext(nil, reqUp, httptest.NewRecorder()),
			ID:   out.Body.ID,
			Body: linkDTO{Host: "", Slug: "homed", Target: "https://t.example/homed"},
		}
		_, err = p.updateLink(ctx, upd)
		if err == nil || !strings.Contains(err.Error(), "host is required") {
			t.Fatalf("updateLink(host='') err = %v, want 400 host-required", err)
		}
	}
}

// Without a base domain the identical request must still succeed: self-hosted
// single-tenant installs keep their neutral-host links (mail click tracking and
// the shared dashboard host depend on it). Without this twin, a test passes by
// refusing host="" unconditionally.
func TestEmptyHostStillAllowedWithoutBase(t *testing.T) {
	db, p := openTenantTestPlugin(t)
	t.Setenv(models.BaseDomainEnv, "") // pin: no env fallback either
	db.Create(&dns.Domain{
		OrgID:     1,
		Name:      "mybrand.com",
		ForLink:   true,
		LinkHosts: models.HostList{{Host: "go.mybrand.com", Enabled: true}},
	})

	ctx := context.Background()
	req := httptest.NewRequest(http.MethodPost, "/api/links", nil)
	req.Header.Set("X-Org-ID", "1")
	input := &CreateLinkInput{
		Ctx:  humago.NewContext(nil, req, httptest.NewRecorder()),
		Body: linkDTO{Host: "", Slug: "legacy", Target: "https://t.example/legacy"},
	}
	out, err := p.createLink(ctx, input)
	if err != nil {
		t.Fatalf("createLink(host='') without base domain err = %v, want success", err)
	}
	if out.Body.Host != "" {
		t.Fatalf("got host %q, want empty host stored", out.Body.Host)
	}
}
