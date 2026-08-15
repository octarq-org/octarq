package dns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Guard 4: host lists are the serving surface of their own domain — every
// entry must be the domain's Name or a subdomain of it. Domain.Name is unique
// across the table, so this is what makes it impossible for two orgs to
// legally claim the same hostname through their lists (the JSON columns have
// no uniqueness constraint of their own).
func TestCreateDomainRejectsHostListsOutsideZone(t *testing.T) {
	p, mkCtx := setupFullDNSTestDB(t)
	acc := ProviderAccount{OrgID: 1, Name: "P", Type: "cloudflare"}
	if err := p.db.Create(&acc).Error; err != nil {
		t.Fatalf("seed provider account: %v", err)
	}

	ctx := context.Background()
	enabled := true
	rejected := []domainDTO{
		{Name: "example.com", ProviderAccountID: acc.ID, LinkHosts: &[]hostEntry{{Host: "victim.test", Enabled: &enabled}}},
		{Name: "example.com", ProviderAccountID: acc.ID, MailHosts: &[]hostEntry{{Host: "evil.net", Enabled: &enabled}}},
		// A bare suffix must not smuggle an outside host in: "notexample.com"
		// is not a subdomain of "example.com".
		{Name: "example.com", ProviderAccountID: acc.ID, LinkHosts: &[]hostEntry{{Host: "notexample.com", Enabled: &enabled}}},
		// Normalization must not smuggle one in either.
		{Name: "example.com", ProviderAccountID: acc.ID, MailHosts: &[]hostEntry{{Host: "HTTPS://Victim.Test:8443", Enabled: &enabled}}},
	}
	for _, dto := range rejected {
		req := httptest.NewRequest(http.MethodPost, "/api/dns/domains", nil)
		if _, err := p.createDomain(ctx, &CreateDomainInput{Ctx: mkCtx(req), Body: dto}); err == nil {
			t.Errorf("createDomain accepted an outside-zone host list entry: %+v", dto)
		}
		var n int64
		p.db.Model(&Domain{}).Where("name = ?", "example.com").Count(&n)
		if n != 0 {
			t.Fatalf("a rejected createDomain wrote a row")
		}
	}

	// The apex itself and subdomains of it are inside the zone and accepted.
	req := httptest.NewRequest(http.MethodPost, "/api/dns/domains", nil)
	if _, err := p.createDomain(ctx, &CreateDomainInput{
		Ctx: mkCtx(req),
		Body: domainDTO{Name: "example.com", ProviderAccountID: acc.ID,
			LinkHosts: &[]hostEntry{{Host: "example.com", Enabled: &enabled}, {Host: "go.example.com", Enabled: &enabled}},
			MailHosts: &[]hostEntry{{Host: "mail.example.com", Enabled: &enabled}}},
	}); err != nil {
		t.Errorf("createDomain with apex/subdomain host lists = %v, want success", err)
	}
}

// The same zone rule must hold when an existing domain's host lists are
// edited, and a rejected edit must not write anything.
func TestUpdateDomainRejectsHostListsOutsideZone(t *testing.T) {
	p, mkCtx := setupFullDNSTestDB(t)
	acc := ProviderAccount{OrgID: 1, Name: "P", Type: "cloudflare"}
	if err := p.db.Create(&acc).Error; err != nil {
		t.Fatalf("seed provider account: %v", err)
	}
	dom := Domain{OrgID: 1, Name: "example.com", ProviderAccountID: acc.ID}
	if err := p.db.Create(&dom).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}

	ctx := context.Background()
	enabled := true
	req := httptest.NewRequest(http.MethodPut, "/api/dns/domains/1", nil)
	if _, err := p.updateDomain(ctx, &UpdateDomainInput{
		Ctx:  mkCtx(req),
		ID:   dom.ID,
		Body: domainDTO{LinkHosts: &[]hostEntry{{Host: "victim.test", Enabled: &enabled}}},
	}); err == nil {
		t.Error("updateDomain accepted an outside-zone link host")
	}
	var after Domain
	if err := p.db.First(&after, dom.ID).Error; err != nil {
		t.Fatalf("reload domain: %v", err)
	}
	if len(after.LinkHosts) != 0 {
		t.Errorf("a rejected update still wrote the host list: %+v", after.LinkHosts)
	}

	// In-zone hosts still work on update.
	req2 := httptest.NewRequest(http.MethodPut, "/api/dns/domains/1", nil)
	if _, err := p.updateDomain(ctx, &UpdateDomainInput{
		Ctx:  mkCtx(req2),
		ID:   dom.ID,
		Body: domainDTO{LinkHosts: &[]hostEntry{{Host: "go.example.com", Enabled: &enabled}}, MailHosts: &[]hostEntry{{Host: "mail.example.com", Enabled: &enabled}}},
	}); err != nil {
		t.Errorf("updateDomain with in-zone host lists = %v, want success", err)
	}
	if err := p.db.First(&after, dom.ID).Error; err != nil {
		t.Fatalf("reload domain: %v", err)
	}
	if len(after.LinkHosts) != 1 || after.LinkHosts[0].Host != "go.example.com" {
		t.Errorf("accepted update did not persist the host list: %+v", after.LinkHosts)
	}
}
