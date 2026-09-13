package dns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	humago "github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/dnsprovider"
	"github.com/octarq-org/octarq/origin"
)

// patchedSyncDomains simulates the proposed remediation in AUDIT_REPORT.md (Finding RES-02)
func (p *Plugin) patchedSyncDomains(ctx context.Context, input *SyncDomainsInput, evictedNames *[]string) (*SyncDomainsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r = r.WithContext(ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("forbidden: admin role required to sync domains")
	}
	if input.Body.ProviderAccountID == 0 {
		return nil, huma.Error400BadRequest("providerAccountId is required")
	}
	var acc ProviderAccount
	if err := p.db.Where("id = ? AND owner_id = ?", input.Body.ProviderAccountID, p.orgID(r)).First(&acc).Error; err != nil {
		return nil, huma.Error404NotFound("provider account not found")
	}

	creds, err := p.decrypt(acc.Config)
	if err != nil {
		return nil, huma.Error500InternalServerError("stored API token could not be decrypted")
	}

	prov, err := dnsprovider.New(acc.Type, creds)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	zones, err := prov.ListZones(r.Context())
	if err != nil {
		return nil, p.providerErr("list zones", err)
	}
	var created, updated int
	for _, z := range zones {
		name := strings.ToLower(z.Name)
		if underBaseZone(p.db, name) {
			continue
		}
		var dom Domain
		if p.db.Where("name = ? AND owner_id = ?", name, p.orgID(r)).First(&dom).Error == nil {
			dom.ZoneID = z.ID
			dom.ProviderAccountID = acc.ID
			p.db.Save(&dom)
			updated++
			forgetOrigin(name)
			if evictedNames != nil {
				*evictedNames = append(*evictedNames, "updated:"+name)
			}
		} else {
			if err := p.db.Create(&Domain{
				OrgID: p.orgID(r),
				Name:  name, ProviderAccountID: acc.ID, ZoneID: z.ID,
			}).Error; err == nil {
				created++
				forgetOrigin(name)
				if evictedNames != nil {
					*evictedNames = append(*evictedNames, "created:"+name)
				}
			}
		}
	}
	return &SyncDomainsOutput{
		Body: map[string]any{
			"ok": true, "total": len(zones), "created": created, "updated": updated,
		},
	}, nil
}

// TestVerification_RES02_Remediation verifies the refined RES-02 remediation in AUDIT_REPORT.md
func TestVerification_RES02_Remediation(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFreshTestDB(t)
	rv := origin.NewResolver(p.db)

	// 1. Setup Org 1 owning "existing-org1.com" (to be updated)
	domOrg1 := Domain{
		OrgID:             1,
		Name:              "existing-org1.com",
		ProviderAccountID: 10,
		ZoneID:            "old-zone-id",
	}
	if err := p.db.Create(&domOrg1).Error; err != nil {
		t.Fatalf("setup Org 1 existing domain: %v", err)
	}

	// 2. Setup Org 2 owning "contested-org2.com" (to simulate unique constraint collision against Org 1)
	domOrg2 := Domain{
		OrgID:             2,
		Name:              "contested-org2.com",
		ProviderAccountID: 20,
		ZoneID:            "org2-zone-id",
	}
	if err := p.db.Create(&domOrg2).Error; err != nil {
		t.Fatalf("setup Org 2 domain: %v", err)
	}

	// 3. Configure a fake DNS Provider for Org 1 with 3 zones:
	// - "existing-org1.com" (should hit Update branch)
	// - "brand-new-org1.com" (should hit Create branch - success)
	// - "contested-org2.com" (should hit Create branch - fail with unique constraint error)
	fake := &fakeDNSProvider{
		zones: []dnsprovider.Zone{
			{ID: "z-updated", Name: "existing-org1.com"},
			{ID: "z-new", Name: "brand-new-org1.com"},
			{ID: "z-contested", Name: "contested-org2.com"},
		},
	}
	provName := registerFakeProvider(t, "sync-res02-prov", fake)
	accOrg1 := ProviderAccount{OrgID: 1, Name: "org1-acc", Type: provName, Config: "{}"}
	if err := p.db.Create(&accOrg1).Error; err != nil {
		t.Fatalf("setup Org 1 provider: %v", err)
	}

	// Warm origin resolver cache
	ownerOld, _ := rv.OwnerOf("app.existing-org1.com")
	if ownerOld != 1 {
		t.Fatalf("expected existing-org1.com owner=1, got %d", ownerOld)
	}
	ownerContested, _ := rv.OwnerOf("app.contested-org2.com")
	if ownerContested != 2 {
		t.Fatalf("expected contested-org2.com owner=2, got %d", ownerContested)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/domains/sync", nil)
	req.Header.Set("X-Org-ID", "1")
	req.Header.Set("X-Role", "admin")

	var evicted []string
	out, err := p.patchedSyncDomains(context.Background(), &SyncDomainsInput{
		Ctx: mkCtx(req),
		Body: struct {
			ProviderAccountID uint `json:"providerAccountId,omitempty"`
		}{accOrg1.ID},
	}, &evicted)

	if err != nil {
		t.Fatalf("patchedSyncDomains returned unexpected error: %v", err)
	}

	body := out.Body
	t.Logf("patchedSyncDomains returned output: %+v", body)
	t.Logf("Evicted items: %+v", evicted)

	// Verify counts: total=3, updated=1, created=1 (contested must not count as created)
	if body["total"] != 3 {
		t.Errorf("expected total 3, got %v", body["total"])
	}
	if body["updated"] != 1 {
		t.Errorf("expected updated 1, got %v", body["updated"])
	}
	if body["created"] != 1 {
		t.Errorf("expected created 1, got %v", body["created"])
	}

	// Verify DB state for updated domain
	var updatedDom Domain
	p.db.First(&updatedDom, domOrg1.ID)
	if updatedDom.ZoneID != "z-updated" || updatedDom.ProviderAccountID != accOrg1.ID {
		t.Errorf("updated domain mismatch: %+v", updatedDom)
	}

	// Verify DB state for newly created domain
	var createdDom Domain
	if err := p.db.Where("owner_id = ? AND name = ?", 1, "brand-new-org1.com").First(&createdDom).Error; err != nil {
		t.Errorf("expected created domain brand-new-org1.com in DB, got error: %v", err)
	}

	// Verify DB state for contested domain (Org 1 must NOT have a row for contested-org2.com)
	var contestedRows []Domain
	p.db.Where("owner_id = ? AND name = ?", 1, "contested-org2.com").Find(&contestedRows)
	if len(contestedRows) != 0 {
		t.Errorf("contested domain was improperly inserted for Org 1!")
	}

	// Verify Origin Cache Invalidation list
	// Must contain updated:existing-org1.com and created:brand-new-org1.com
	// Must NOT contain contested-org2.com
	if len(evicted) != 2 {
		t.Fatalf("expected exactly 2 evictions, got %d: %+v", len(evicted), evicted)
	}
	if evicted[0] != "updated:existing-org1.com" {
		t.Errorf("expected updated eviction, got %s", evicted[0])
	}
	if evicted[1] != "created:brand-new-org1.com" {
		t.Errorf("expected created eviction, got %s", evicted[1])
	}

	// Verify newly created domain is recognized by resolver after cache invalidation
	ownerNew, ok := rv.OwnerOf("app.brand-new-org1.com")
	if !ok || ownerNew != 1 {
		t.Errorf("expected brand-new-org1.com to resolve to org 1, got ok=%v org=%d", ok, ownerNew)
	}

	// Verify contested domain remains owned by Org 2
	ownerContestedAfter, ok := rv.OwnerOf("app.contested-org2.com")
	if !ok || ownerContestedAfter != 2 {
		t.Errorf("expected contested-org2.com to remain owned by org 2, got ok=%v org=%d", ok, ownerContestedAfter)
	}
}
