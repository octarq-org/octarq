package dns

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/dnsprovider"
	"github.com/octarq-org/octarq/server/plugin"
)

func TestCoverageLift_DNS_Host(t *testing.T) {
	t.Parallel()
	p := New()
	if p.Host() != nil {
		t.Fatalf("expected nil host initially")
	}
	p.SetHost(nil)
	if p.Host() != nil {
		t.Fatalf("expected nil host after SetHost(nil)")
	}
}

func TestCoverageLift_DNS_ErrorsAndHelpers(t *testing.T) {
	t.Parallel()
	p, _ := setupFullDNSTestDB(t)

	// cleanProviderError invalid/validation/parameter
	if got := cleanProviderError("invalid argument provided"); got != "invalid record configuration or parameters" {
		t.Fatalf("unexpected cleanProviderError: %s", got)
	}
	if got := cleanProviderError("parameter value error"); got != "invalid record configuration or parameters" {
		t.Fatalf("unexpected cleanProviderError: %s", got)
	}

	// providerErr with err == nil
	if err := p.providerErr("test_action", nil); err == nil {
		t.Fatalf("expected error from providerErr")
	}

	// hostsOutsideZone with empty name
	if got := hostsOutsideZone("", nil, nil); got != "" {
		t.Fatalf("expected empty string for empty domain name, got %s", got)
	}

	// ownsProviderAccount with id == 0
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if p.ownsProviderAccount(r, 0) {
		t.Fatalf("expected false for id=0")
	}
}

func TestCoverageLift_DNS_EmailBlueprint(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullDNSTestDB(t)
	ctx := context.Background()

	// NormaliseRecordName fallback
	if got := normaliseRecordName("otherdomain.org", "example.com"); got != "otherdomain.org" {
		t.Fatalf("unexpected normaliseRecordName: %s", got)
	}

	// Resolve methods
	req := httptest.NewRequest(http.MethodGet, "/domains/1/email-blueprint", nil)
	humaCtx := mkCtx(req)

	ebInput := &EmailBlueprintInput{ID: 1}
	if errs := ebInput.Resolve(humaCtx); len(errs) > 0 {
		t.Fatalf("unexpected resolve errors: %v", errs)
	}
	if ebInput.Ctx != humaCtx {
		t.Fatalf("expected Ctx to be stored")
	}

	applyInput := &ApplyEmailBlueprintInput{ID: 1}
	if errs := applyInput.Resolve(humaCtx); len(errs) > 0 {
		t.Fatalf("unexpected resolve errors: %v", errs)
	}

	// emailBlueprint error branches: nil Ctx, orgID == 0
	if _, err := p.emailBlueprint(ctx, &EmailBlueprintInput{}); err == nil {
		t.Fatalf("expected error for nil Ctx")
	}

	reqNoOrg := httptest.NewRequest(http.MethodGet, "/domains/1/email-blueprint", nil)
	reqNoOrg.Header.Set("X-Org-ID", "0")
	ctxNoOrg := humago.NewContext(nil, reqNoOrg, httptest.NewRecorder())
	if _, err := p.emailBlueprint(ctx, &EmailBlueprintInput{Ctx: ctxNoOrg, ID: 1}); err == nil {
		t.Fatalf("expected error for orgID == 0")
	}

	// emailBlueprint: domain without provider account
	dom := Domain{
		OrgID:             1,
		Name:              "unconfigured.example.com",
		ProviderAccountID: 0,
	}
	if err := p.db.Create(&dom).Error; err != nil {
		t.Fatalf("create domain: %v", err)
	}

	reqDom := httptest.NewRequest(http.MethodGet, "/domains/1/email-blueprint", nil)
	reqDom.Header.Set("X-Org-ID", "1")
	ctxDom := humago.NewContext(nil, reqDom, httptest.NewRecorder())
	out, err := p.emailBlueprint(ctx, &EmailBlueprintInput{Ctx: ctxDom, ID: dom.ID})
	if err != nil {
		t.Fatalf("expected success for domain without provider, got %v", err)
	}
	if len(out.Body) == 0 || out.Body[0].Status != BlueprintStatusMissing {
		t.Fatalf("expected all missing status")
	}

	// applyEmailBlueprint: nil Ctx, orgID == 0, dom with ProviderAccountID == 0
	if _, err := p.applyEmailBlueprint(ctx, &ApplyEmailBlueprintInput{}); err == nil {
		t.Fatalf("expected error for nil Ctx")
	}
	if _, err := p.applyEmailBlueprint(ctx, &ApplyEmailBlueprintInput{Ctx: ctxNoOrg, ID: 1}); err == nil {
		t.Fatalf("expected error for orgID == 0")
	}
	if _, err := p.applyEmailBlueprint(ctx, &ApplyEmailBlueprintInput{Ctx: ctxDom, ID: dom.ID}); err == nil {
		t.Fatalf("expected error for domain without provider")
	}
}

func TestCoverageLift_DNS_MCP(t *testing.T) {
	t.Parallel()
	p, _ := setupFullDNSTestDB(t)
	ctx := plugin.WithOrgID(context.Background(), 1)

	// mcpUpdateDomainConfig: ID == 0
	if _, _, err := p.mcpUpdateDomainConfig(ctx, nil, updateDomainConfigInput{ID: 0}); err == nil {
		t.Fatalf("expected error for ID == 0")
	}

	// mcpListDNSRecords: DomainID == 0
	if _, _, err := p.mcpListDNSRecords(ctx, nil, listDNSRecordsInput{DomainID: 0}); err == nil {
		t.Fatalf("expected error for DomainID == 0")
	}

	// mcpSetDNSRecord: DomainID == 0
	if _, _, err := p.mcpSetDNSRecord(ctx, nil, setDNSRecordInput{DomainID: 0}); err == nil {
		t.Fatalf("expected error for DomainID == 0")
	}

	// mcpDeleteDNSRecord: DomainID == 0, RecordID == ""
	if _, _, err := p.mcpDeleteDNSRecord(ctx, nil, deleteDNSRecordInput{DomainID: 0}); err == nil {
		t.Fatalf("expected error for DomainID == 0")
	}
	if _, _, err := p.mcpDeleteDNSRecord(ctx, nil, deleteDNSRecordInput{DomainID: 1, RecordID: ""}); err == nil {
		t.Fatalf("expected error for empty RecordID")
	}

	// Domain without zoneID
	acc := ProviderAccount{OrgID: 1, Name: "Acc", Type: "cloudflare"}
	p.db.Create(&acc)
	dom := Domain{OrgID: 1, Name: "nozone.example.com", ProviderAccountID: acc.ID, ZoneID: ""}
	p.db.Create(&dom)

	if _, _, err := p.mcpListDNSRecords(ctx, nil, listDNSRecordsInput{DomainID: dom.ID}); err == nil {
		t.Fatalf("expected error for missing ZoneID")
	}
	if _, _, err := p.mcpSetDNSRecord(ctx, nil, setDNSRecordInput{DomainID: dom.ID, Type: "A", Name: "sub", Content: "1.2.3.4"}); err == nil {
		t.Fatalf("expected error for missing ZoneID")
	}
	if _, _, err := p.mcpDeleteDNSRecord(ctx, nil, deleteDNSRecordInput{DomainID: dom.ID, RecordID: "rec1"}); err == nil {
		t.Fatalf("expected error for missing ZoneID")
	}

	// mcpExportDomains
	res, err := p.mcpExportDomains(ctx, 1)
	if err != nil {
		t.Fatalf("mcpExportDomains failed: %v", err)
	}
	if res == nil {
		t.Fatalf("expected non-nil export result")
	}
}

func TestCoverageLift_DNS_ProvidersAndDomains(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullDNSTestDB(t)
	ctx := context.Background()

	// Resolve implementations
	upProvIn := &UpdateProviderAccountInput{ID: 1}
	if errs := upProvIn.Resolve(mkCtx(httptest.NewRequest(http.MethodPut, "/providers/1", nil))); len(errs) > 0 {
		t.Fatalf("resolve failed: %v", errs)
	}
	delProvIn := &DeleteProviderAccountInput{ID: 1}
	if errs := delProvIn.Resolve(mkCtx(httptest.NewRequest(http.MethodDelete, "/providers/1", nil))); len(errs) > 0 {
		t.Fatalf("resolve failed: %v", errs)
	}
	upDomIn := &UpdateDomainInput{ID: 1}
	if errs := upDomIn.Resolve(mkCtx(httptest.NewRequest(http.MethodPut, "/domains/1", nil))); len(errs) > 0 {
		t.Fatalf("resolve failed: %v", errs)
	}

	// updateProviderAccount error branches
	if _, err := p.updateProviderAccount(ctx, &UpdateProviderAccountInput{}); err == nil {
		t.Fatalf("expected error for nil Ctx")
	}
	reqNoOrg := httptest.NewRequest(http.MethodPut, "/providers/1", nil)
	reqNoOrg.Header.Set("X-Org-ID", "0")
	ctxNoOrg := humago.NewContext(nil, reqNoOrg, httptest.NewRecorder())
	if _, err := p.updateProviderAccount(ctx, &UpdateProviderAccountInput{Ctx: ctxNoOrg, ID: 1}); err == nil {
		t.Fatalf("expected error for orgID == 0")
	}
	reqMember := httptest.NewRequest(http.MethodPut, "/providers/1", nil)
	reqMember.Header.Set("X-Org-ID", "1")
	p.requireRole = func(r *http.Request, role string) bool { return false }
	ctxMember := humago.NewContext(nil, reqMember, httptest.NewRecorder())
	if _, err := p.updateProviderAccount(ctx, &UpdateProviderAccountInput{Ctx: ctxMember, ID: 1}); err == nil {
		t.Fatalf("expected forbidden for non-admin")
	}

	// deleteProviderAccount error branches
	if _, err := p.deleteProviderAccount(ctx, &DeleteProviderAccountInput{}); err == nil {
		t.Fatalf("expected error for nil Ctx")
	}
	if _, err := p.deleteProviderAccount(ctx, &DeleteProviderAccountInput{Ctx: ctxNoOrg, ID: 1}); err == nil {
		t.Fatalf("expected error for orgID == 0")
	}
	if _, err := p.deleteProviderAccount(ctx, &DeleteProviderAccountInput{Ctx: ctxMember, ID: 1}); err == nil {
		t.Fatalf("expected forbidden for non-admin")
	}

	// Restore admin role
	p.requireRole = func(r *http.Request, role string) bool { return true }
	reqAdmin := httptest.NewRequest(http.MethodDelete, "/providers/999", nil)
	reqAdmin.Header.Set("X-Org-ID", "1")
	ctxAdmin := humago.NewContext(nil, reqAdmin, httptest.NewRecorder())
	if _, err := p.deleteProviderAccount(ctx, &DeleteProviderAccountInput{Ctx: ctxAdmin, ID: 999}); err == nil {
		t.Fatalf("expected 404 for non-existent provider")
	}

	// updateDomain error branches
	if _, err := p.updateDomain(ctx, &UpdateDomainInput{}); err == nil {
		t.Fatalf("expected error for nil Ctx")
	}
	if _, err := p.updateDomain(ctx, &UpdateDomainInput{Ctx: ctxNoOrg, ID: 1}); err == nil {
		t.Fatalf("expected error for orgID == 0")
	}
	p.requireRole = func(r *http.Request, role string) bool { return false }
	if _, err := p.updateDomain(ctx, &UpdateDomainInput{Ctx: ctxMember, ID: 1}); err == nil {
		t.Fatalf("expected forbidden for non-admin")
	}
	p.requireRole = func(r *http.Request, role string) bool { return true }

	// createDomain with explicit ForLink / ForMail flags
	forLink := true
	forMail := false
	acc := ProviderAccount{OrgID: 1, Name: "Switches Prov", Type: "cloudflare"}
	p.db.Create(&acc)

	createIn := &CreateDomainInput{
		Ctx: ctxAdmin,
		Body: domainDTO{
			Name:              "switches.example.com",
			ProviderAccountID: acc.ID,
			ForLink:           &forLink,
			ForMail:           &forMail,
		},
	}
	outDom, err := p.createDomain(ctx, createIn)
	if err != nil {
		t.Fatalf("createDomain with flags failed: %v", err)
	}
	if !outDom.Body.ForLink || outDom.Body.ForMail {
		t.Fatalf("flags did not take effect")
	}

	// updateDomain with ZoneID, ForLink, ForMail, Note
	zoneID := "zone_123"
	forLinkUpdated := false
	forMailUpdated := true
	upDomOut, err := p.updateDomain(ctx, &UpdateDomainInput{
		Ctx: ctxAdmin,
		ID:  outDom.Body.ID,
		Body: domainDTO{
			ZoneID:  zoneID,
			ForLink: &forLinkUpdated,
			ForMail: &forMailUpdated,
			Note:    "updated note",
		},
	})
	if err != nil {
		t.Fatalf("updateDomain failed: %v", err)
	}
	if upDomOut.Body.ZoneID != zoneID || upDomOut.Body.Note != "updated note" {
		t.Fatalf("domain was not updated properly")
	}

	// deleteDomain non-existent
	delReq := httptest.NewRequest(http.MethodDelete, "/domains/9999", nil)
	delReq.Header.Set("X-Org-ID", "1")
	delCtx := humago.NewContext(nil, delReq, httptest.NewRecorder())
	if _, err := p.deleteDomain(ctx, &DeleteDomainInput{Ctx: delCtx, ID: 9999}); err == nil {
		t.Fatalf("expected 404 for non-existent domain")
	}
}

func TestCoverageLift_DNS_DDNSAndRecords(t *testing.T) {
	t.Parallel()
	p, _ := setupFullDNSTestDB(t)
	ctx := context.Background()

	// deleteDDNSToken error branches
	if _, err := p.deleteDDNSToken(ctx, &deleteDDNSTokenInput{}); err == nil {
		t.Fatalf("expected error for nil Ctx")
	}
	reqNoOrg := httptest.NewRequest(http.MethodDelete, "/ddns/1", nil)
	reqNoOrg.Header.Set("X-Org-ID", "0")
	ctxNoOrg := humago.NewContext(nil, reqNoOrg, httptest.NewRecorder())
	if _, err := p.deleteDDNSToken(ctx, &deleteDDNSTokenInput{Ctx: ctxNoOrg, ID: 1}); err == nil {
		t.Fatalf("expected error for orgID == 0")
	}
	reqMember := httptest.NewRequest(http.MethodDelete, "/ddns/1", nil)
	reqMember.Header.Set("X-Org-ID", "1")
	p.requireRole = func(r *http.Request, role string) bool { return false }
	ctxMember := humago.NewContext(nil, reqMember, httptest.NewRecorder())
	if _, err := p.deleteDDNSToken(ctx, &deleteDDNSTokenInput{Ctx: ctxMember, ID: 1}); err == nil {
		t.Fatalf("expected forbidden for non-admin")
	}
	p.requireRole = func(r *http.Request, role string) bool { return true }

	// updateDDNS error branches
	if _, err := p.updateDDNS(ctx, &updateDDNSInput{}); err == nil {
		t.Fatalf("expected error for nil Ctx")
	}

	rec := httptest.NewRecorder()
	reqEmptyTok := httptest.NewRequest(http.MethodGet, "/ddns/update", nil)
	ctxEmptyTok := humago.NewContext(nil, reqEmptyTok, rec)
	_, _ = p.updateDDNS(ctx, &updateDDNSInput{Ctx: ctxEmptyTok})
	if rec.Body.String() != "badauth" {
		t.Fatalf("expected badauth response, got %s", rec.Body.String())
	}

	// Records error branches
	domNoProv := Domain{OrgID: 1, Name: "noprov.example.com", ProviderAccountID: 0}
	p.db.Create(&domNoProv)

	reqAdmin := httptest.NewRequest(http.MethodGet, "/", nil)
	reqAdmin.Header.Set("X-Org-ID", "1")
	ctxAdmin := humago.NewContext(nil, reqAdmin, httptest.NewRecorder())

	if _, err := p.listRecords(ctx, &ListRecordsInput{Ctx: ctxAdmin, ID: domNoProv.ID}); err == nil {
		t.Fatalf("expected error for domain without provider")
	}
	if _, err := p.deleteRecord(ctx, &DeleteRecordInput{Ctx: ctxAdmin, ID: domNoProv.ID, RID: "r1"}); err == nil {
		t.Fatalf("expected error for domain without provider")
	}

	// updateRecord validation failure
	acc := ProviderAccount{OrgID: 1, Name: "Acc", Type: "cloudflare"}
	p.db.Create(&acc)
	domProv := Domain{OrgID: 1, Name: "prov.example.com", ProviderAccountID: acc.ID, ZoneID: "zone1"}
	p.db.Create(&domProv)

	upRecIn := &UpdateRecordInput{
		Ctx: ctxAdmin,
		ID:  domProv.ID,
		RID: "r1",
		Body: dnsprovider.Record{
			Type: "INVALID_TYPE",
			Name: "sub",
		},
	}
	if _, err := p.updateRecord(ctx, upRecIn); err == nil {
		t.Fatalf("expected validation error for invalid record type")
	}

	// syncDomains decrypt failure & Resolve
	accBadKey := ProviderAccount{OrgID: 1, Name: "BadKey", Type: "cloudflare", Config: "bad_secret"}
	p.db.Create(&accBadKey)
	p.decrypt = func(string) ([]byte, error) { return nil, errors.New("decrypt error") }
	syncIn := &SyncDomainsInput{Ctx: ctxAdmin}
	syncIn.Body.ProviderAccountID = accBadKey.ID
	if errs := syncIn.Resolve(ctxAdmin); len(errs) > 0 {
		t.Fatalf("syncDomains Resolve failed: %v", errs)
	}
	if _, err := p.syncDomains(ctx, syncIn); err == nil {
		t.Fatalf("expected decrypt error in syncDomains")
	}
}
