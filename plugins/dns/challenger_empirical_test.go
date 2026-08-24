package dns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/octarq-org/octarq/internal/dnsprovider"
)

// TestFinding4_SyncDomainsIgnoresDBCreateConflict demonstrates that when
// syncDomains encounters a unique constraint conflict (e.g. domain registered by Org 1),
// it ignores p.db.Create error, increments created++, reports false positive success,
// and evicts the cache of the existing domain.
func TestFinding4_SyncDomainsIgnoresDBCreateConflict(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFreshTestDB(t)

	// 1. Org 1 legitimately owns "contested.com"
	domOrg1 := Domain{
		OrgID: 1,
		Name:  "contested.com",
	}
	if err := p.db.Create(&domOrg1).Error; err != nil {
		t.Fatalf("setup Org 1 domain: %v", err)
	}

	// 2. Org 2 has a provider account that lists "contested.com"
	fake := &fakeDNSProvider{
		zones: []dnsprovider.Zone{
			{ID: "z-contested", Name: "contested.com"},
		},
	}
	provName := registerFakeProvider(t, "sync-conflict-prov", fake)
	accOrg2 := ProviderAccount{OrgID: 2, Name: "org2-acc", Type: provName, Config: "{}"}
	if err := p.db.Create(&accOrg2).Error; err != nil {
		t.Fatalf("setup Org 2 provider: %v", err)
	}

	// 3. Org 2 runs syncDomains
	req := httptest.NewRequest(http.MethodPost, "/api/domains/sync", nil)
	req.Header.Set("X-Org-ID", "2")
	req.Header.Set("X-Role", "admin")

	out, err := p.syncDomains(context.Background(), &SyncDomainsInput{
		Ctx: mkCtx(req),
		Body: struct {
			ProviderAccountID uint `json:"providerAccountId,omitempty"`
		}{accOrg2.ID},
	})
	if err != nil {
		t.Fatalf("syncDomains returned unexpected error: %v", err)
	}

	body := out.Body
	t.Logf("syncDomains returned body: %+v", body)

	var org2Doms []Domain
	p.db.Where("owner_id = ? AND name = ?", 2, "contested.com").Find(&org2Doms)

	if len(org2Doms) == 0 && body["created"] == 0 {
		t.Logf("[FIXED Finding 4] syncDomains correctly reports created=0 after conflict")
	} else if body["created"] == 1 {
		t.Errorf("expected created=0 after fix, got 1")
	} else if len(org2Doms) > 0 {
		t.Errorf("unexpected: Org 2 domain was actually created (unique constraint violated)")
	}
}

// TestFinding6_OrphanedDDNSTokensOnDomainDelete demonstrates that deleting
// a Domain leaves orphaned DDNSToken records in the database.
func TestFinding6_OrphanedDDNSTokensOnDomainDelete(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFreshTestDB(t)

	// 1. Create a domain for Org 1
	dom := Domain{
		OrgID: 1,
		Name:  "ddns-domain.com",
	}
	if err := p.db.Create(&dom).Error; err != nil {
		t.Fatalf("create domain: %v", err)
	}

	// 2. Create DDNS tokens associated with this domain
	tok1 := DDNSToken{
		OrgID:      1,
		DomainID:   dom.ID,
		RecordName: "home.ddns-domain.com",
		RecordType: "A",
		TokenHash:  "hash-tok-1",
		Label:      "Home Server",
		CreatedAt:  time.Now(),
	}
	tok2 := DDNSToken{
		OrgID:      1,
		DomainID:   dom.ID,
		RecordName: "vpn.ddns-domain.com",
		RecordType: "AAAA",
		TokenHash:  "hash-tok-2",
		Label:      "VPN Gateway",
		CreatedAt:  time.Now(),
	}
	p.db.Create(&tok1)
	p.db.Create(&tok2)

	var tokenCountBefore int64
	p.db.Model(&DDNSToken{}).Where("domain_id = ? AND owner_id = ?", dom.ID, 1).Count(&tokenCountBefore)
	if tokenCountBefore != 2 {
		t.Fatalf("expected 2 tokens before delete, got %d", tokenCountBefore)
	}

	// 3. Delete Domain
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/domains/1", nil)
	reqDel.Header.Set("X-Org-ID", "1")
	reqDel.Header.Set("X-Role", "admin")
	delOut, err := p.deleteDomain(context.Background(), &DeleteDomainInput{
		Ctx: mkCtx(reqDel),
		ID:  dom.ID,
	})
	if err != nil || !delOut.Body["ok"] {
		t.Fatalf("deleteDomain failed: %v", err)
	}

	var tokenCountAfter int64
	p.db.Model(&DDNSToken{}).Where("domain_id = ? AND owner_id = ?", dom.ID, 1).Count(&tokenCountAfter)

	if tokenCountAfter == 0 {
		t.Logf("[FIXED Finding 6] Deleting Domain ID=%d correctly cleaned %d orphaned DDNSToken rows", dom.ID, 2)
	} else {
		t.Errorf("expected 0 orphaned tokens after fix, got %d", tokenCountAfter)
	}

	reqList := httptest.NewRequest(http.MethodGet, "/api/dns/ddns", nil)
	reqList.Header.Set("X-Org-ID", "1")
	listOut, err := p.listDDNSTokens(context.Background(), &listDDNSTokensInput{Ctx: mkCtx(reqList)})
	if err != nil {
		t.Fatalf("listDDNSTokens failed: %v", err)
	}
	if len(listOut.Body) != 0 {
		t.Errorf("expected 0 tokens after domain delete, got %d", len(listOut.Body))
	}
}
