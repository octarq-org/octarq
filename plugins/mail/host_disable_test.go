package mail

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugins/dns"
	"gorm.io/gorm"
)

func TestMailHostDisabledTenantIsolation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(append(models.AllModels(), &Mailbox{}, &dns.Domain{})...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db.Where("1 = 1").Delete(&Mailbox{})
	db.Where("1 = 1").Delete(&dns.Domain{})

	// Org B (orgID=2) owns victim.com with mailHost "x.example" enabled
	db.Create(&dns.Domain{
		OrgID:   2,
		Name:    "victim.com",
		ForMail: true,
		MailHosts: models.HostList{
			models.Host{Host: "x.example", Enabled: true},
		},
	})

	// Org A (orgID=1) lists the SAME hostname on its own mail domain, disabled.
	// Both orgs must be for_mail, or the two assertions below would be answered
	// by the for_mail filter rather than by org scoping — and the test would pass
	// without proving anything.
	db.Create(&dns.Domain{
		OrgID:   1,
		Name:    "attacker.com",
		ForMail: true,
		MailHosts: models.HostList{
			models.Host{Host: "x.example", Enabled: false},
		},
	})

	p := &Plugin{db: db}

	// Org B's listing is enabled, so B is not disabled — even though A disabled
	// the same hostname on its own domain.
	if p.mailHostDisabled(2, "x.example") {
		t.Fatal("org A disabling x.example must not disable mail for org B")
	}

	// The converse, which is what makes the assertion above non-vacuous: without
	// per-org scoping both calls return the same answer and only one can hold.
	if !p.mailHostDisabled(1, "x.example") {
		t.Fatal("org A disabled x.example on its own domain; it must read as disabled for A")
	}

	// A host no tenant lists is unmanaged, never disabled.
	if p.mailHostDisabled(1, "unlisted.example") {
		t.Fatal("an unlisted host must not be reported as disabled")
	}
}
