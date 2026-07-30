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

	// Org A (orgID=1) has a domain with forMail=false and mailHost "x.example" disabled
	db.Create(&dns.Domain{
		OrgID:   1,
		Name:    "attacker.com",
		ForMail: false, // NOT enabled for mail
		MailHosts: models.HostList{
			models.Host{Host: "x.example", Enabled: false},
		},
	})

	p := &Plugin{db: db}

	// Disabling x.example under Org A's non-mail domain should NOT disable mail for Org B
	if p.mailHostDisabled("x.example") {
		t.Fatal("expected mailHostDisabled to return false for Org B's enabled mail host, but got true")
	}
}
