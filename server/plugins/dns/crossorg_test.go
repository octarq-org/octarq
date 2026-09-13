package dns

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/dnsprovider"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

func setupCrossOrgDNSDB(t *testing.T) (*Plugin, *gorm.DB) {
	t.Helper()
	db := freshMemDB(t)
	p := New()
	p.db = db
	p.audit = func(r *http.Request, action, targetType string, targetID uint, meta map[string]any) {}
	p.decrypt = func(s string) ([]byte, error) { return []byte(s), nil }
	p.orgID = func(r *http.Request) uint {
		if val := r.Header.Get("X-Org-ID"); val != "" {
			var id uint
			fmt.Sscanf(val, "%d", &id)
			return id
		}
		return 1
	}
	p.requireRole = func(r *http.Request, role string) bool {
		return r.Header.Get("X-Role") != "member"
	}
	return p, db
}

func makeDNSOrgCtx(orgID uint, role string) huma.Context {
	req := httptest.NewRequest(http.MethodGet, "/api/domains", nil)
	req.Header.Set("X-Org-ID", fmt.Sprintf("%d", orgID))
	req.Header.Set("X-Role", role)
	return humago.NewContext(nil, req, httptest.NewRecorder())
}

func TestDNSCrossOrgIsolation(t *testing.T) {
	p, db := setupCrossOrgDNSDB(t)
	ctx := context.Background()

	const org1 uint = 1
	const org2 uint = 2

	// Setup fake DNS provider
	fake := &fakeDNSProvider{
		records: []dnsprovider.Record{
			{ID: "rec-org2-1", Type: "A", Name: "app.org2.example.com", Content: "2.2.2.2", TTL: 300},
		},
	}
	provName := registerFakeProvider(t, "crossorg-dns-prov", fake)

	// Org 1 resources
	acc1 := ProviderAccount{OrgID: org1, Name: "org1-cf", Type: provName, Config: "{}"}
	db.Create(&acc1)
	dom1 := Domain{OrgID: org1, Name: "org1.example.com", ProviderAccountID: acc1.ID, ZoneID: "zone-1", Note: "Org1 Domain"}
	db.Create(&dom1)
	tok1 := DDNSToken{
		OrgID: org1, DomainID: dom1.ID, RecordName: "ddns1", RecordType: "A",
		TokenHash: "hash1", Label: "Org1 DDNS", CreatedAt: time.Now(),
	}
	db.Create(&tok1)

	// Org 2 resources
	acc2 := ProviderAccount{OrgID: org2, Name: "org2-cf", Type: provName, Config: "{}"}
	db.Create(&acc2)
	dom2 := Domain{OrgID: org2, Name: "org2.example.com", ProviderAccountID: acc2.ID, ZoneID: "zone-2", Note: "Org2 Domain"}
	db.Create(&dom2)
	tok2 := DDNSToken{
		OrgID: org2, DomainID: dom2.ID, RecordName: "ddns2", RecordType: "A",
		TokenHash: "hash2", Label: "Org2 DDNS", CreatedAt: time.Now(),
	}
	db.Create(&tok2)

	ctxOrg1Admin := makeDNSOrgCtx(org1, "admin")
	ctxOrg2Admin := makeDNSOrgCtx(org2, "admin")

	// ==================== 1. Domain Endpoints Isolation ====================

	// 1a. List Domains: Org 1 must only see dom1
	domListOut, err := p.listDomains(ctx, &ListDomainsInput{Ctx: ctxOrg1Admin})
	if err != nil {
		t.Fatalf("listDomains: %v", err)
	}
	if len(domListOut.Body) != 1 || domListOut.Body[0].ID != dom1.ID {
		t.Errorf("listDomains: Org 1 saw domains %+v, want only dom1 (%d)", domListOut.Body, dom1.ID)
	}

	// 1b. Create Domain with Org 2's Provider Account: must be rejected
	_, err = p.createDomain(ctx, &CreateDomainInput{
		Ctx: ctxOrg1Admin,
		Body: domainDTO{
			Name:              "rogue.example.com",
			ProviderAccountID: acc2.ID,
		},
	})
	if err == nil {
		t.Errorf("createDomain: expected error when Org 1 binds Org 2 provider account, got nil")
	}
	var rogue models.Org
	var count int64
	db.Model(&Domain{}).Where("name = ?", "rogue.example.com").Count(&count)
	if count != 0 {
		t.Errorf("createDomain: rogue domain was created using Org 2 provider account")
	}

	// 1c. Update Domain: Org 1 cannot update Org 2's domain
	maliciousNote := "malicious-domain-update"
	_, err = p.updateDomain(ctx, &UpdateDomainInput{
		Ctx:  ctxOrg1Admin,
		ID:   dom2.ID,
		Body: domainDTO{Note: maliciousNote},
	})
	if err == nil {
		t.Errorf("updateDomain: expected error when Org 1 updates Org 2 domain, got nil")
	}
	var dom2Reload Domain
	db.First(&dom2Reload, dom2.ID)
	if dom2Reload.Note == maliciousNote {
		t.Fatalf("updateDomain: Org 2 domain was modified by Org 1")
	}

	// 1d. Update Domain: Org 1 cannot rebind its domain to Org 2's provider account
	_, err = p.updateDomain(ctx, &UpdateDomainInput{
		Ctx:  ctxOrg1Admin,
		ID:   dom1.ID,
		Body: domainDTO{ProviderAccountID: acc2.ID},
	})
	if err == nil {
		t.Errorf("updateDomain: expected error when Org 1 rebinds to Org 2 provider account, got nil")
	}
	var dom1Reload Domain
	db.First(&dom1Reload, dom1.ID)
	if dom1Reload.ProviderAccountID == acc2.ID {
		t.Fatalf("updateDomain: Org 1 domain bound to Org 2 provider account")
	}

	// 1e. Delete Domain: Org 1 cannot delete Org 2's domain
	_, err = p.deleteDomain(ctx, &DeleteDomainInput{
		Ctx: ctxOrg1Admin,
		ID:  dom2.ID,
	})
	if err == nil {
		t.Errorf("deleteDomain: expected error when Org 1 deletes Org 2 domain, got nil")
	}
	db.Model(&Domain{}).Where("id = ?", dom2.ID).Count(&count)
	if count == 0 {
		t.Fatalf("deleteDomain: Org 2 domain was deleted by Org 1")
	}

	// ==================== 2. Record Endpoints Isolation ====================

	// 2a. List Records: Org 1 cannot list records for Org 2's domain
	_, err = p.listRecords(ctx, &ListRecordsInput{
		Ctx: ctxOrg1Admin,
		ID:  dom2.ID,
	})
	if err == nil {
		t.Errorf("listRecords: expected error when Org 1 lists records of Org 2 domain, got nil")
	}

	// 2b. Create Record: Org 1 cannot create a record under Org 2's domain
	_, err = p.createRecord(ctx, &CreateRecordInput{
		Ctx: ctxOrg1Admin,
		ID:  dom2.ID,
		Body: dnsprovider.Record{
			Type:    "A",
			Name:    "injected.org2.example.com",
			Content: "9.9.9.9",
		},
	})
	if err == nil {
		t.Errorf("createRecord: expected error when Org 1 creates record in Org 2 domain, got nil")
	}
	for _, cr := range fake.created {
		if cr.Name == "injected.org2.example.com" {
			t.Fatalf("createRecord: record was injected into Org 2 domain zone")
		}
	}

	// 2c. Update Record: Org 1 cannot update a record under Org 2's domain
	_, err = p.updateRecord(ctx, &UpdateRecordInput{
		Ctx: ctxOrg1Admin,
		ID:  dom2.ID,
		RID: "rec-org2-1",
		Body: dnsprovider.Record{
			Type:    "A",
			Name:    "app.org2.example.com",
			Content: "6.6.6.6",
		},
	})
	if err == nil {
		t.Errorf("updateRecord: expected error when Org 1 updates record in Org 2 domain, got nil")
	}
	for _, up := range fake.updated {
		if up.ID == "rec-org2-1" {
			t.Fatalf("updateRecord: Org 2 record was updated by Org 1")
		}
	}

	// 2d. Delete Record: Org 1 cannot delete a record under Org 2's domain
	_, err = p.deleteRecord(ctx, &DeleteRecordInput{
		Ctx: ctxOrg1Admin,
		ID:  dom2.ID,
		RID: "rec-org2-1",
	})
	if err == nil {
		t.Errorf("deleteRecord: expected error when Org 1 deletes record in Org 2 domain, got nil")
	}
	for _, del := range fake.deleted {
		if del == "rec-org2-1" {
			t.Fatalf("deleteRecord: Org 2 record was deleted by Org 1")
		}
	}

	// ==================== 3. DDNS Token Endpoints Isolation ====================

	// 3a. List DDNS Tokens: Org 1 only sees tok1
	ddnsListOut, err := p.listDDNSTokens(ctx, &listDDNSTokensInput{Ctx: ctxOrg1Admin})
	if err != nil {
		t.Fatalf("listDDNSTokens: %v", err)
	}
	if len(ddnsListOut.Body) != 1 || ddnsListOut.Body[0].ID != tok1.ID {
		t.Errorf("listDDNSTokens: Org 1 saw DDNS tokens %+v, want only tok1 (%d)", ddnsListOut.Body, tok1.ID)
	}

	// 3b. Create DDNS Token (ddns_crud.go:84): Org 1 cannot create token for Org 2's domain
	createTokIn := &createDDNSTokenInput{
		Ctx: ctxOrg1Admin,
	}
	createTokIn.Body.DomainID = dom2.ID
	createTokIn.Body.RecordName = "rogue-ddns"
	createTokIn.Body.RecordType = "A"
	createTokIn.Body.Label = "Rogue Token"

	_, err = p.createDDNSToken(ctx, createTokIn)
	if err == nil {
		t.Errorf("createDDNSToken: expected error when Org 1 creates token for Org 2 domain, got nil")
	}
	db.Model(&DDNSToken{}).Where("record_name = ?", "rogue-ddns").Count(&count)
	if count != 0 {
		t.Fatalf("createDDNSToken: DDNS token created against Org 2 domain")
	}

	// 3c. Delete DDNS Token (ddns_crud.go:169): Org 1 cannot delete Org 2's DDNS token
	_, err = p.deleteDDNSToken(ctx, &deleteDDNSTokenInput{
		Ctx: ctxOrg1Admin,
		ID:  tok2.ID,
	})
	if err == nil {
		t.Errorf("deleteDDNSToken: expected error when Org 1 deletes Org 2 token, got nil")
	}
	db.Model(&DDNSToken{}).Where("id = ?", tok2.ID).Count(&count)
	if count == 0 {
		t.Fatalf("deleteDDNSToken: Org 2 DDNS token was deleted by Org 1")
	}

	// ==================== 4. Reverse Checks (Org 2 vs Org 1) ====================

	// Org 2 cannot update Org 1 domain
	_, err = p.updateDomain(ctx, &UpdateDomainInput{Ctx: ctxOrg2Admin, ID: dom1.ID, Body: domainDTO{Note: "x"}})
	if err == nil {
		t.Errorf("updateDomain: expected error for Org 2 updating Org 1 domain")
	}

	// Org 2 cannot delete Org 1 domain
	_, err = p.deleteDomain(ctx, &DeleteDomainInput{Ctx: ctxOrg2Admin, ID: dom1.ID})
	if err == nil {
		t.Errorf("deleteDomain: expected error for Org 2 deleting Org 1 domain")
	}

	// Org 2 cannot create DDNS token under Org 1 domain
	createTokIn2 := &createDDNSTokenInput{Ctx: ctxOrg2Admin}
	createTokIn2.Body.DomainID = dom1.ID
	createTokIn2.Body.RecordName = "rogue-ddns-2"
	_, err = p.createDDNSToken(ctx, createTokIn2)
	if err == nil {
		t.Errorf("createDDNSToken: expected error for Org 2 on Org 1 domain")
	}

	// Org 2 cannot delete Org 1 DDNS token
	_, err = p.deleteDDNSToken(ctx, &deleteDDNSTokenInput{Ctx: ctxOrg2Admin, ID: tok1.ID})
	if err == nil {
		t.Errorf("deleteDDNSToken: expected error for Org 2 deleting Org 1 token")
	}
	_ = rogue
}
