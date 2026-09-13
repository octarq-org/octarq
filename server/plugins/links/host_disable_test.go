package links

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugins/dns"
	"gorm.io/gorm"
)

func TestLinkHostDisabledTenantIsolation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(append(models.AllModels(), &Link{}, &dns.Domain{})...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db.Where("1 = 1").Delete(&Link{})
	db.Where("1 = 1").Delete(&dns.Domain{})

	// Org B (orgID=2) owns victim.com with linkHost "x.example" enabled
	db.Create(&dns.Domain{
		OrgID:   2,
		Name:    "victim.com",
		ForLink: true,
		LinkHosts: models.HostList{
			models.Host{Host: "x.example", Enabled: true},
		},
	})

	// Org A (orgID=1) owns attacker.com with linkHost "x.example" disabled
	db.Create(&dns.Domain{
		OrgID:   1,
		Name:    "attacker.com",
		ForLink: true,
		LinkHosts: models.HostList{
			models.Host{Host: "x.example", Enabled: false},
		},
	})

	engine := NewEngine(db, mockCtx())

	// Org B's listing is enabled, so B is not disabled — even though A disabled
	// the same hostname on its own domain.
	if engine.linkHostDisabled(2, "x.example") {
		t.Fatal("org A disabling x.example must not disable it for org B")
	}

	// And the converse, which is what makes the assertion above non-vacuous:
	// A's own listing IS disabled, so A must still see it as disabled. Without
	// per-org scoping both calls return the same answer and only one of these
	// two can hold.
	if !engine.linkHostDisabled(1, "x.example") {
		t.Fatal("org A disabled x.example on its own domain; it must read as disabled for A")
	}

	// A host no tenant lists is unmanaged, never disabled.
	if engine.linkHostDisabled(1, "unlisted.example") {
		t.Fatal("an unlisted host must not be reported as disabled")
	}
}
