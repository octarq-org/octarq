package dns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/plugin"
)

func TestListDomainsPaginationAndSearch(t *testing.T) {
	p, mkCtx := setupFreshTestDB(t)
	accID := seedProviderAccount(t, p)
	ctx := context.Background()
	for i, n := range []string{"alpha.com", "beta.com", "gamma.com", "delta.com", "note-events.com"} {
		p.db.Create(&Domain{OrgID: 1, Name: n, ProviderAccountID: accID, Note: "order-" + string(rune('a'+i))})
	}

	req := httptest.NewRequest(http.MethodGet, "/api/domains", nil)

	// Search narrows by name or note.
	out, err := p.listDomains(ctx, &ListDomainsInput{Ctx: mkCtx(req), Q: "order-"})
	if err != nil || len(out.Body) != 5 {
		t.Fatalf("list by note: %v, %d rows", err, len(out.Body))
	}
	out, err = p.listDomains(ctx, &ListDomainsInput{Ctx: mkCtx(req), Q: "alpha"})
	if err != nil || len(out.Body) != 1 || out.Body[0].Name != "alpha.com" {
		t.Fatalf("list by name: %v, %+v", err, out.Body)
	}

	// Limit and offset page through the rows.
	out, err = p.listDomains(ctx, &ListDomainsInput{Ctx: mkCtx(req), Limit: 2})
	if err != nil || len(out.Body) != 2 {
		t.Fatalf("limit=2: %v, %d rows", err, len(out.Body))
	}
	out, err = p.listDomains(ctx, &ListDomainsInput{Ctx: mkCtx(req), Limit: 2, Offset: 4})
	if err != nil || len(out.Body) != 1 {
		t.Fatalf("offset=4 limit=2: %v, %d rows", err, len(out.Body))
	}
	// Out-of-range limits clamp to the 500 cap and a negative request falls
	// back to the default of 50.
	out, err = p.listDomains(ctx, &ListDomainsInput{Ctx: mkCtx(req), Limit: 5000})
	if err != nil || len(out.Body) != 5 {
		t.Fatalf("clamped limit: %v, %d rows", err, len(out.Body))
	}
}

func TestCreateDomainVerifyZone(t *testing.T) {
	p, mkCtx := setupFreshGateTestDB(t)
	fake := &fakeDNSProvider{verifyName: "verified.example.com"}
	provName := registerFakeProvider(t, "verify-zone-prov", fake)
	accID := ProviderAccount{OrgID: 1, Name: "acc", Type: provName, Config: "{}"}
	if err := p.db.Create(&accID).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	admin := mkCtx("admin")

	// A zone id present triggers a best-effort VerifyZone before the row is
	// written; failure blocks creation and fires domain.verify_failed.
	var events []struct {
		org  uint
		name string
		data map[string]any
	}
	p.publishEvent = func(o uint, n string, d any) {
		events = append(events, struct {
			org  uint
			name string
			data map[string]any
		}{o, n, d.(map[string]any)})
	}
	fake.verifyErr = errBoom
	_, err := p.createDomain(ctx, &CreateDomainInput{Ctx: admin, Body: domainDTO{Name: "v.example.com", ProviderAccountID: accID.ID, ZoneID: "z9"}})
	if statusOf(t, err) != http.StatusBadRequest {
		t.Fatalf("verify failure: want 400, got %v", err)
	}
	if len(events) != 1 || events[0].name != "domain.verify_failed" {
		t.Fatalf("verify_failed event not fired: %+v", events)
	}
	var n int64
	p.db.Model(&Domain{}).Where("name = ?", "v.example.com").Count(&n)
	if n != 0 {
		t.Errorf("verify-failed create still wrote a row")
	}

	// A successful verify allows creation.
	fake.verifyErr = nil
	out, err := p.createDomain(ctx, &CreateDomainInput{Ctx: admin, Body: domainDTO{Name: "ok.example.com", ProviderAccountID: accID.ID, ZoneID: "z9"}})
	if err != nil || out.Body.ID == 0 {
		t.Fatalf("create with verify: %v", err)
	}
	if len(events) < 2 || events[1].name != "domain.create" {
		t.Errorf("domain.create event not fired: %+v", events)
	}
}

func TestCreateDomainConflict(t *testing.T) {
	p, mkCtx := setupFreshGateTestDB(t)
	accID := seedProviderAccount(t, p)
	p.db.Create(&Domain{OrgID: 1, Name: "dup.com", ProviderAccountID: accID})
	ctx := context.Background()
	_, err := p.createDomain(ctx, &CreateDomainInput{Ctx: mkCtx("admin"), Body: domainDTO{Name: "dup.com", ProviderAccountID: accID}})
	if statusOf(t, err) != http.StatusConflict {
		t.Fatalf("duplicate: want 409, got %v", err)
	}
}

func TestUpdateDomainSwitchesProviderAccount(t *testing.T) {
	p, mkCtx := setupFreshGateTestDB(t)
	fake := &fakeDNSProvider{}
	provName := registerFakeProvider(t, "update-switch-prov", fake)
	acc1 := ProviderAccount{OrgID: 1, Name: "a1", Type: provName, Config: "{}"}
	acc2 := ProviderAccount{OrgID: 1, Name: "a2", Type: provName, Config: "{}"}
	foreign := ProviderAccount{OrgID: 2, Name: "foreign", Type: provName, Config: "{}"}
	for _, a := range []*ProviderAccount{&acc1, &acc2, &foreign} {
		if err := p.db.Create(a).Error; err != nil {
			t.Fatal(err)
		}
	}
	dom := Domain{OrgID: 1, Name: "switch.com", ProviderAccountID: acc1.ID}
	if err := p.db.Create(&dom).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	admin := mkCtx("admin")

	// Switching to the org's own second account works.
	out, err := p.updateDomain(ctx, &UpdateDomainInput{Ctx: admin, ID: dom.ID, Body: domainDTO{ProviderAccountID: acc2.ID}})
	if err != nil || out.Body.ProviderAccountID != acc2.ID {
		t.Fatalf("switch account: %v %+v", err, out.Body)
	}

	// Binding another org's account is refused with 404 and leaves the row
	// pointing at the original account.
	_, err = p.updateDomain(ctx, &UpdateDomainInput{Ctx: admin, ID: dom.ID, Body: domainDTO{ProviderAccountID: foreign.ID}})
	if statusOf(t, err) != http.StatusNotFound {
		t.Fatalf("foreign account: want 404, got %v", err)
	}
	var after Domain
	p.db.First(&after, dom.ID)
	if after.ProviderAccountID != acc2.ID {
		t.Errorf("foreign account was bound to the domain: %+v", after)
	}

	// A nonexistent domain is a 404 too.
	_, err = p.updateDomain(ctx, &UpdateDomainInput{Ctx: admin, ID: 99999, Body: domainDTO{ProviderAccountID: acc1.ID}})
	if statusOf(t, err) != http.StatusNotFound {
		t.Fatalf("missing domain: want 404, got %v", err)
	}
}

func TestDeleteDomainGates(t *testing.T) {
	p, mkCtx := setupFreshGateTestDB(t)
	accID := seedProviderAccount(t, p)
	dom := Domain{OrgID: 1, Name: "bye.com", ProviderAccountID: accID}
	if err := p.db.Create(&dom).Error; err != nil {
		t.Fatal(err)
	}
	foreign := Domain{OrgID: 2, Name: "theirs.com", ProviderAccountID: accID}
	if err := p.db.Create(&foreign).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// Member -> 403.
	if _, err := p.deleteDomain(ctx, &DeleteDomainInput{Ctx: mkCtx("member"), ID: dom.ID}); statusOf(t, err) != http.StatusForbidden {
		t.Fatalf("member delete: want 403")
	}
	// Another org's domain -> 404, and the row survives.
	if _, err := p.deleteDomain(ctx, &DeleteDomainInput{Ctx: mkCtx("admin"), ID: foreign.ID}); statusOf(t, err) != http.StatusNotFound {
		t.Fatalf("cross-org delete: want 404")
	}
	var n int64
	p.db.Model(&Domain{}).Where("id = ?", foreign.ID).Count(&n)
	if n != 1 {
		t.Errorf("cross-org delete removed the row")
	}
	// Admin delete of own domain succeeds and drops the row.
	if _, err := p.deleteDomain(ctx, &DeleteDomainInput{Ctx: mkCtx("admin"), ID: dom.ID}); err != nil {
		t.Fatalf("admin delete: %v", err)
	}
	if err := p.db.First(&Domain{}, dom.ID).Error; err == nil {
		t.Error("domain row still present after delete")
	}
}

func TestVerifyDomainDNSFiresFailureEvent(t *testing.T) {
	p, mkCtx := setupFreshTestDB(t)
	const orgID = uint(1)
	dom := Domain{OrgID: orgID, Name: "flaky.com", ForMail: true}
	if err := p.db.Create(&dom).Error; err != nil {
		t.Fatal(err)
	}
	var events []string
	var eventData map[string]any
	p.publishEvent = func(o uint, n string, d any) {
		events = append(events, n)
		eventData = d.(map[string]any)
	}
	p.lookupTXT = func(name string) ([]string, error) { return []string{"v=spf1 -all"}, nil }

	req := httptest.NewRequest(http.MethodGet, "/api/domains/1/verify-dns", nil)
	out, err := p.verifyDomainDNS(context.Background(), &VerifyDomainDNSInput{Ctx: mkCtx(req), ID: dom.ID})
	if err != nil {
		t.Fatalf("verifyDomainDNS: %v", err)
	}
	// SPF is healthy but DMARC is missing on the apex, so the failure event
	// must list the host.
	if len(events) == 0 || events[0] != "domain.verify_failed" {
		t.Fatalf("expected domain.verify_failed, got %v", events)
	}
	hosts := eventData["hosts"].([]string)
	if len(hosts) != 1 || hosts[0] != "flaky.com" {
		t.Errorf("failure event hosts = %v, want [flaky.com]", hosts)
	}
	// The response still reports what it found.
	body := out.Body
	spf := body["spf"].(dnsRecordStatus)
	if !spf.Healthy {
		t.Errorf("spf should be healthy: %+v", spf)
	}
	if dmarc := body["dmarc"].(dnsRecordStatus); dmarc.Set {
		t.Errorf("dmarc should be unset: %+v", dmarc)
	}
}

func TestVerifyDomainDNSGates(t *testing.T) {
	p, _ := setupFreshTestDB(t)
	foreign := Domain{OrgID: 2, Name: "theirs.com"}
	if err := p.db.Create(&foreign).Error; err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/domains/1/verify-dns", nil)
	req.Header.Set("X-Org-ID", "1")
	ctx := humago.NewContext(nil, req, httptest.NewRecorder())
	if _, err := p.verifyDomainDNS(context.Background(), &VerifyDomainDNSInput{Ctx: ctx, ID: foreign.ID}); statusOf(t, err) != http.StatusNotFound {
		t.Fatalf("cross-org verify: want 404, got %v", err)
	}
}

func TestProviderForErrors(t *testing.T) {
	p, _ := setupFreshTestDB(t)

	// No provider account configured.
	_, err := p.providerFor(Domain{OrgID: 1, Name: "x.com"})
	if err == nil || err.Error() != "domain has no provider account configured" {
		t.Fatalf("no account: %v", err)
	}

	// Account exists but belongs to a different org.
	acc := ProviderAccount{OrgID: 2, Name: "v", Type: "cloudflare", Config: "{}"}
	if err := p.db.Create(&acc).Error; err != nil {
		t.Fatal(err)
	}
	_, err = p.providerFor(Domain{OrgID: 1, Name: "x.com", ProviderAccountID: acc.ID})
	if err == nil || err.Error() != "provider account not found" {
		t.Fatalf("cross-org account: %v", err)
	}

	// Empty stored config.
	acc1 := ProviderAccount{OrgID: 1, Name: "empty", Type: "cloudflare", Config: ""}
	if err := p.db.Create(&acc1).Error; err != nil {
		t.Fatal(err)
	}
	_, err = p.providerFor(Domain{OrgID: 1, Name: "x.com", ProviderAccountID: acc1.ID})
	if err == nil || err.Error() != "provider account has no credentials configured" {
		t.Fatalf("empty config: %v", err)
	}

	// Decryption failure.
	p.decrypt = func(string) ([]byte, error) { return nil, errBoom }
	acc2 := ProviderAccount{OrgID: 1, Name: "enc", Type: "cloudflare", Config: "encrypted"}
	if err := p.db.Create(&acc2).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := p.providerFor(Domain{OrgID: 1, Name: "x.com", ProviderAccountID: acc2.ID}); err == nil {
		t.Fatal("decrypt failure must propagate")
	}

	// Unknown provider type.
	p.decrypt = func(s string) ([]byte, error) { return []byte(s), nil }
	acc3 := ProviderAccount{OrgID: 1, Name: "ghost", Type: "nope", Config: "{}"}
	if err := p.db.Create(&acc3).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := p.providerFor(Domain{OrgID: 1, Name: "x.com", ProviderAccountID: acc3.ID}); err == nil {
		t.Fatal("unknown provider type must error")
	}
}

func TestOverviewCounts(t *testing.T) {
	p, _ := setupFreshTestDB(t)
	accID := seedProviderAccount(t, p)
	p.db.Create(&Domain{OrgID: 1, Name: "a.com", ProviderAccountID: accID, ForLink: true})
	p.db.Create(&Domain{OrgID: 1, Name: "b.com", ProviderAccountID: accID, ForMail: true})
	p.db.Create(&Domain{OrgID: 2, Name: "c.com", ProviderAccountID: accID})

	ov := p.overview(1, false)
	if ov["domains"] != int64(2) {
		t.Errorf("domains = %v, want 2", ov["domains"])
	}
	if ov["linkDomains"] != int64(1) || ov["mailDomains"] != int64(1) {
		t.Errorf("link/mail counts = %v/%v, want 1/1", ov["linkDomains"], ov["mailDomains"])
	}
}

func TestMountWiresWebhookEvents(t *testing.T) {
	p := New()
	var registered []plugin.WebhookEventDef
	reg := plugin.NewRegistry()
	p.Mount(nil, &plugin.Context{
		RegisterWebhookEvent: func(d plugin.WebhookEventDef) { registered = append(registered, d) },
		Provide:              reg.Provide,
		Lookup:               reg.Lookup,
	})
	if len(registered) != 2 {
		t.Fatalf("expected 2 webhook events, got %v", registered)
	}
}
