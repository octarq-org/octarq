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

	// Disabling x.example under Org A should NOT disable shortlink resolution for Org B
	if engine.linkHostDisabled("x.example") {
		t.Fatal("expected linkHostDisabled to return false for Org B's enabled link host, but got true")
	}
}
