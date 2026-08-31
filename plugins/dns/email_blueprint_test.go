package dns

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/dnsprovider"
)

// setupBlueprintPlugin creates a fresh plugin wired for blueprint tests.
// orgID comes from X-Org-ID header (defaults to 1); admin gate checks X-Role=="admin".
func setupBlueprintPlugin(t *testing.T) *Plugin {
	t.Helper()
	p := New()
	p.db = freshMemDB(t)
	p.audit = func(r *http.Request, action, targetType string, targetID uint, meta map[string]any) {}
	p.publishEvent = func(orgID uint, event string, data any) {}
	p.decrypt = func(s string) ([]byte, error) { return []byte(s), nil }
	p.orgID = func(r *http.Request) uint {
		var id uint
		fmt.Sscanf(r.Header.Get("X-Org-ID"), "%d", &id)
		if id == 0 {
			id = 1
		}
		return id
	}
	p.requireRole = func(r *http.Request, role string) bool {
		return r.Header.Get("X-Role") == "admin"
	}
	return p
}

// seedBlueprintDomain creates a ProviderAccount and Domain for the given org,
// backed by the named fake provider. Returns the domain ID.
func seedBlueprintDomain(t *testing.T, p *Plugin, orgID uint, domainName, provName string) uint {
	t.Helper()
	acc := ProviderAccount{OrgID: orgID, Name: domainName + "-acc", Type: provName, Config: "{}"}
	if err := p.db.Create(&acc).Error; err != nil {
		t.Fatalf("seedBlueprintDomain: create account: %v", err)
	}
	dom := Domain{OrgID: orgID, Name: domainName, ProviderAccountID: acc.ID, ZoneID: "zone-" + domainName}
	if err := p.db.Create(&dom).Error; err != nil {
		t.Fatalf("seedBlueprintDomain: create domain: %v", err)
	}
	return dom.ID
}

// TestBlueprintAllMissingWhenProviderEmpty: domain with provider but empty
// records list → all 4 blueprint records have status "missing".
func TestBlueprintAllMissingWhenProviderEmpty(t *testing.T) {
	p := setupBlueprintPlugin(t)
	fake := &fakeDNSProvider{records: []dnsprovider.Record{}}
	provName := registerFakeProvider(t, "bp-empty-prov", fake)
	domID := seedBlueprintDomain(t, p, 1, "empty.example.com", provName)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Org-ID", "1")
	req.Header.Set("X-Role", "admin")
	out, err := p.emailBlueprint(context.Background(), &EmailBlueprintInput{
		Ctx: humago.NewContext(nil, req, httptest.NewRecorder()),
		ID:  domID,
	})
	if err != nil {
		t.Fatalf("emailBlueprint: %v", err)
	}
	if len(out.Body) != 4 {
		t.Fatalf("expected 4 records, got %d", len(out.Body))
	}
	for _, rec := range out.Body {
		if rec.Status != BlueprintStatusMissing {
			t.Errorf("record %s %s: want missing, got %s", rec.Type, rec.Name, rec.Status)
		}
	}
}

// TestBlueprintStatusOKWhenRecordsMatch: provider returns all 4 matching
// records → all blueprint records report status "ok".
func TestBlueprintStatusOKWhenRecordsMatch(t *testing.T) {
	p := setupBlueprintPlugin(t)

	// Return records matching the blueprint template, using FQDN names as
	// a real provider would.
	apex := "match.example.com"
	mxP10 := 10
	mxP53 := 53
	live := []dnsprovider.Record{
		{ID: "r1", Type: "MX", Name: apex, Content: "route1.mx.cloudflare.net", TTL: 1, Priority: &mxP10},
		{ID: "r2", Type: "MX", Name: apex, Content: "route2.mx.cloudflare.net", TTL: 1, Priority: &mxP53},
		{ID: "r3", Type: "TXT", Name: apex, Content: "v=spf1 include:_spf.mx.cloudflare.net ~all", TTL: 1},
		{ID: "r4", Type: "TXT", Name: "_dmarc." + apex, Content: "v=DMARC1; p=none; sp=none;", TTL: 1},
	}
	fake := &fakeDNSProvider{records: live}
	provName := registerFakeProvider(t, "bp-ok-prov", fake)
	domID := seedBlueprintDomain(t, p, 1, apex, provName)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Org-ID", "1")
	req.Header.Set("X-Role", "admin")
	out, err := p.emailBlueprint(context.Background(), &EmailBlueprintInput{
		Ctx: humago.NewContext(nil, req, httptest.NewRecorder()),
		ID:  domID,
	})
	if err != nil {
		t.Fatalf("emailBlueprint: %v", err)
	}
	for _, rec := range out.Body {
		if rec.Status != BlueprintStatusOK {
			t.Errorf("record %s %s: want ok, got %s (content=%q)", rec.Type, rec.Name, rec.Status, rec.Content)
		}
	}
}

// TestApplyBlueprintMemberForbidden: member role → 403.
func TestApplyBlueprintMemberForbidden(t *testing.T) {
	p := setupBlueprintPlugin(t)
	fake := &fakeDNSProvider{}
	provName := registerFakeProvider(t, "bp-member-prov", fake)
	domID := seedBlueprintDomain(t, p, 1, "member.example.com", provName)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Org-ID", "1")
	req.Header.Set("X-Role", "member")
	_, err := p.applyEmailBlueprint(context.Background(), &ApplyEmailBlueprintInput{
		Ctx: humago.NewContext(nil, req, httptest.NewRecorder()),
		ID:  domID,
	})
	if err == nil {
		t.Fatal("expected 403, got nil")
	}
	if st := statusOf(t, err); st != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %v", st, err)
	}
}

// TestApplyBlueprintAdminAppliesAll: admin with empty provider → applied=4, skipped=0.
func TestApplyBlueprintAdminAppliesAll(t *testing.T) {
	p := setupBlueprintPlugin(t)
	fake := &fakeDNSProvider{records: []dnsprovider.Record{}}
	provName := registerFakeProvider(t, "bp-admin-prov", fake)
	domID := seedBlueprintDomain(t, p, 1, "admin.example.com", provName)

	var auditCalls int
	p.audit = func(r *http.Request, action, targetType string, targetID uint, meta map[string]any) {
		if action == "email_blueprint_applied" {
			auditCalls++
		}
	}
	var eventFired string
	p.publishEvent = func(orgID uint, event string, data any) {
		eventFired = event
	}

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Org-ID", "1")
	req.Header.Set("X-Role", "admin")
	out, err := p.applyEmailBlueprint(context.Background(), &ApplyEmailBlueprintInput{
		Ctx: humago.NewContext(nil, req, httptest.NewRecorder()),
		ID:  domID,
	})
	if err != nil {
		t.Fatalf("applyEmailBlueprint: %v", err)
	}
	if !out.Body.OK {
		t.Error("expected ok=true")
	}
	if out.Body.Applied != 4 {
		t.Errorf("expected applied=4, got %d", out.Body.Applied)
	}
	if out.Body.Skipped != 0 {
		t.Errorf("expected skipped=0, got %d", out.Body.Skipped)
	}
	if len(fake.created) != 4 {
		t.Errorf("expected 4 provider CreateRecord calls, got %d", len(fake.created))
	}
	if auditCalls != 1 {
		t.Errorf("expected 1 audit call, got %d", auditCalls)
	}
	if eventFired != "domain.email_blueprint_applied" {
		t.Errorf("expected event domain.email_blueprint_applied, got %q", eventFired)
	}
}

// TestApplyBlueprintMultiTenantIsolation: org2 token cannot access org1's domain → 404.
func TestApplyBlueprintMultiTenantIsolation(t *testing.T) {
	p := setupBlueprintPlugin(t)
	fake := &fakeDNSProvider{}
	provName := registerFakeProvider(t, "bp-mt-prov", fake)
	// Seed domain under org 1.
	org1DomID := seedBlueprintDomain(t, p, 1, "org1-mt.example.com", provName)

	// Org 2 attempts to apply to org 1's domain.
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Org-ID", "2")
	req.Header.Set("X-Role", "admin")
	_, err := p.applyEmailBlueprint(context.Background(), &ApplyEmailBlueprintInput{
		Ctx: humago.NewContext(nil, req, httptest.NewRecorder()),
		ID:  org1DomID,
	})
	if err == nil {
		t.Fatal("expected 404 for cross-org POST, got nil")
	}
	if st := statusOf(t, err); st != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %v", st, err)
	}

	// Also check GET blueprint.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("X-Org-ID", "2")
	req2.Header.Set("X-Role", "admin")
	_, err = p.emailBlueprint(context.Background(), &EmailBlueprintInput{
		Ctx: humago.NewContext(nil, req2, httptest.NewRecorder()),
		ID:  org1DomID,
	})
	if err == nil {
		t.Fatal("expected 404 for cross-org GET blueprint, got nil")
	}
	if st := statusOf(t, err); st != http.StatusNotFound {
		t.Fatalf("expected 404 for GET, got %d: %v", st, err)
	}
}

// TestApplyBlueprintIdempotency: when all records already match (status ok),
// apply reports applied=0, skipped=4 and calls CreateRecord zero times.
func TestApplyBlueprintIdempotency(t *testing.T) {
	p := setupBlueprintPlugin(t)

	apex := "idem.example.com"
	mxP10 := 10
	mxP53 := 53
	live := []dnsprovider.Record{
		{ID: "i1", Type: "MX", Name: apex, Content: "route1.mx.cloudflare.net", TTL: 1, Priority: &mxP10},
		{ID: "i2", Type: "MX", Name: apex, Content: "route2.mx.cloudflare.net", TTL: 1, Priority: &mxP53},
		{ID: "i3", Type: "TXT", Name: apex, Content: "v=spf1 include:_spf.mx.cloudflare.net ~all", TTL: 1},
		{ID: "i4", Type: "TXT", Name: "_dmarc." + apex, Content: "v=DMARC1; p=none; sp=none;", TTL: 1},
	}
	fake := &fakeDNSProvider{records: live}
	provName := registerFakeProvider(t, "bp-idem-prov", fake)
	domID := seedBlueprintDomain(t, p, 1, apex, provName)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Org-ID", "1")
	req.Header.Set("X-Role", "admin")
	out, err := p.applyEmailBlueprint(context.Background(), &ApplyEmailBlueprintInput{
		Ctx: humago.NewContext(nil, req, httptest.NewRecorder()),
		ID:  domID,
	})
	if err != nil {
		t.Fatalf("applyEmailBlueprint: %v", err)
	}
	if out.Body.Applied != 0 {
		t.Errorf("expected applied=0, got %d", out.Body.Applied)
	}
	if out.Body.Skipped != 4 {
		t.Errorf("expected skipped=4, got %d", out.Body.Skipped)
	}
	if len(fake.created) != 0 {
		t.Errorf("expected 0 CreateRecord calls, got %d", len(fake.created))
	}
}

// TestApplyBlueprintNoProviderReturns400: domain without ProviderAccountID → 400 with
// a friendly message containing "no DNS provider".
func TestApplyBlueprintNoProviderReturns400(t *testing.T) {
	p := setupBlueprintPlugin(t)

	// Create a domain without a ProviderAccountID.
	dom := Domain{OrgID: 1, Name: "noprov.example.com", ProviderAccountID: 0, ZoneID: ""}
	if err := p.db.Create(&dom).Error; err != nil {
		t.Fatalf("create domain: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Org-ID", "1")
	req.Header.Set("X-Role", "admin")
	_, err := p.applyEmailBlueprint(context.Background(), &ApplyEmailBlueprintInput{
		Ctx: humago.NewContext(nil, req, httptest.NewRecorder()),
		ID:  dom.ID,
	})
	if err == nil {
		t.Fatal("expected 400, got nil")
	}
	if st := statusOf(t, err); st != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %v", st, err)
	}
	if !strings.Contains(err.Error(), "no DNS provider") {
		t.Errorf("expected error to mention 'no DNS provider', got: %v", err)
	}
}
