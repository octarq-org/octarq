package dns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/dnsprovider"
	"github.com/octarq-org/octarq/plugin"
)

// TestResolveMethods pins the trivial input.Resolve contract every handler
// depends on: it stores the huma.Context on the input and reports no errors.
func TestResolveMethods(t *testing.T) {
	ctx := huma.Context(nil)

	for _, resolve := range []func() []error{
		func() []error { var i ListProviderAccountsInput; return i.Resolve(ctx) },
		func() []error { var i CreateProviderAccountInput; return i.Resolve(ctx) },
		func() []error { var i UpdateProviderAccountInput; return i.Resolve(ctx) },
		func() []error { var i DeleteProviderAccountInput; return i.Resolve(ctx) },
		func() []error { var i DNSProvidersInput; return i.Resolve(ctx) },
		func() []error { var i SyncDomainsInput; return i.Resolve(ctx) },
		func() []error { var i ListDomainsInput; return i.Resolve(ctx) },
		func() []error { var i CreateDomainInput; return i.Resolve(ctx) },
		func() []error { var i UpdateDomainInput; return i.Resolve(ctx) },
		func() []error { var i DeleteDomainInput; return i.Resolve(ctx) },
		func() []error { var i ListRecordsInput; return i.Resolve(ctx) },
		func() []error { var i CreateRecordInput; return i.Resolve(ctx) },
		func() []error { var i UpdateRecordInput; return i.Resolve(ctx) },
		func() []error { var i DeleteRecordInput; return i.Resolve(ctx) },
		func() []error { var i VerifyDomainDNSInput; return i.Resolve(ctx) },
		func() []error { var i listDDNSTokensInput; return i.Resolve(ctx) },
		func() []error { var i createDDNSTokenInput; return i.Resolve(ctx) },
		func() []error { var i deleteDDNSTokenInput; return i.Resolve(ctx) },
		func() []error { var i updateDDNSInput; return i.Resolve(ctx) },
	} {
		if errs := resolve(); errs != nil {
			t.Errorf("Resolve returned unexpected errors: %v", errs)
		}
	}
}

// withOrgCtx builds a huma.Context from a request with the given org header.
func withOrgCtx(t *testing.T, org string) huma.Context {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Org-ID", org)
	return humago.NewContext(nil, req, httptest.NewRecorder())
}

func noCtx() huma.Context { return nil }

func TestRecordHandlerGateMatrix(t *testing.T) {
	p, _ := setupFreshTestDB(t)
	fake := &fakeDNSProvider{}
	provName := registerFakeProvider(t, "rec-gate-prov", fake)
	_, domID := seedDomainForProvider(t, p, "gate.example.com", "zone", provName)
	ctx := context.Background()
	body := dnsprovider.Record{Type: "A", Name: "x.gate.example.com", Content: "1.1.1.1"}

	for _, c := range []struct {
		name string
		org  string
		role string
		nilc bool
		want int
	}{
		{"no context", "1", "admin", true, 500},
		{"org zero", "0", "admin", false, 401},
		{"member create", "1", "member", false, 403},
	} {
		var hctx huma.Context
		if c.nilc {
			hctx = noCtx()
		} else {
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set("X-Org-ID", c.org)
			if c.role != "" {
				req.Header.Set("X-Role", c.role)
			}
			hctx = humago.NewContext(nil, req, httptest.NewRecorder())
		}
		cases := map[string]error{}
		_, cases["create"] = p.createRecord(ctx, &CreateRecordInput{Ctx: hctx, ID: domID, Body: body})
		_, cases["update"] = p.updateRecord(ctx, &UpdateRecordInput{Ctx: hctx, ID: domID, RID: "r", Body: body})
		_, cases["delete"] = p.deleteRecord(ctx, &DeleteRecordInput{Ctx: hctx, ID: domID, RID: "r"})
		for op, err := range cases {
			if got := statusOf(t, err); got != c.want {
				t.Errorf("%s[%s]: want %d, got %d (%v)", op, c.name, c.want, got, err)
			}
		}
	}
}

func TestReadHandlerGateMatrix(t *testing.T) {
	p, _ := setupFreshTestDB(t)
	_, domID := seedDomainForProvider(t, p, "reads.example.com", "zone", "cloudflare")
	ctx := context.Background()
	accID := seedProviderAccount(t, p)

	// Records list + domains list + provider-account list + ddns list: the
	// shared gates are nil context (500) and org 0 (401).
	if _, err := p.listRecords(ctx, &ListRecordsInput{Ctx: noCtx(), ID: domID}); statusOf(t, err) != 500 {
		t.Errorf("listRecords nil ctx")
	}
	if _, err := p.listRecords(ctx, &ListRecordsInput{Ctx: withOrgCtx(t, "0"), ID: domID}); statusOf(t, err) != 401 {
		t.Errorf("listRecords org0")
	}

	if _, err := p.listDomains(ctx, &ListDomainsInput{Ctx: noCtx()}); statusOf(t, err) != 500 {
		t.Errorf("listDomains nil ctx")
	}
	if _, err := p.listDomains(ctx, &ListDomainsInput{Ctx: withOrgCtx(t, "0")}); statusOf(t, err) != 401 {
		t.Errorf("listDomains org0")
	}

	if _, err := p.listProviderAccounts(ctx, &ListProviderAccountsInput{Ctx: noCtx()}); statusOf(t, err) != 500 {
		t.Errorf("listProviderAccounts nil ctx")
	}
	if _, err := p.listProviderAccounts(ctx, &ListProviderAccountsInput{Ctx: withOrgCtx(t, "0")}); statusOf(t, err) != 401 {
		t.Errorf("listProviderAccounts org0")
	}

	if _, err := p.listDDNSTokens(ctx, &listDDNSTokensInput{Ctx: noCtx()}); statusOf(t, err) != 500 {
		t.Errorf("listDDNSTokens nil ctx")
	}

	if _, err := p.verifyDomainDNS(ctx, &VerifyDomainDNSInput{Ctx: withOrgCtx(t, "0"), ID: domID}); statusOf(t, err) != 401 {
		t.Errorf("verifyDomainDNS org0")
	}
	if _, err := p.verifyDomainDNS(ctx, &VerifyDomainDNSInput{Ctx: noCtx()}); statusOf(t, err) != 500 {
		t.Errorf("verifyDomainDNS nil ctx")
	}
	_, _ = accID, ctx
}

func TestWriteDomainGateMatrix(t *testing.T) {
	p, _ := setupFreshTestDB(t)
	accID := seedProviderAccount(t, p)
	ctx := context.Background()

	// createDomain gates: nil ctx, org 0, member.
	for _, c := range []struct {
		wal  string
		org  string
		role string
		want int
	}{
		{"nil", "1", "admin", 500},
		{"org0", "0", "admin", 401},
		{"member", "1", "member", 403},
	} {
		var hctx huma.Context
		if c.wal == "nil" {
			hctx = nil
		} else {
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set("X-Org-ID", c.org)
			if c.role != "" {
				req.Header.Set("X-Role", c.role)
			}
			hctx = humago.NewContext(nil, req, httptest.NewRecorder())
		}
		_, err := p.createDomain(ctx, &CreateDomainInput{Ctx: hctx, Body: domainDTO{Name: "x.example.com", ProviderAccountID: accID}})
		if statusOf(t, err) != c.want {
			t.Errorf("createDomain[%s]: want %d, got %v", c.wal, c.want, err)
		}
		if c.wal != "member" {
			continue
		}
	}
	_ = p

	// Blank name with an admin + account still rejects with 400.
	adminReq := httptest.NewRequest(http.MethodPost, "/", nil)
	adminReq.Header.Set("X-Org-ID", "1")
	adminReq.Header.Set("X-Role", "admin")
	_, err := p.createDomain(ctx, &CreateDomainInput{Ctx: humago.NewContext(nil, adminReq, httptest.NewRecorder()), Body: domainDTO{Name: "   ", ProviderAccountID: accID}})
	if statusOf(t, err) != 400 {
		t.Errorf("blank domain name: want 400, got %v", err)
	}
	// ProviderAccountID 0 fails validation with 400 before any ownership check.
	_, err = p.createDomain(ctx, &CreateDomainInput{Ctx: humago.NewContext(nil, adminReq, httptest.NewRecorder()), Body: domainDTO{Name: "ok.example.com"}})
	if statusOf(t, err) != 400 {
		t.Errorf("account 0 validation: want 400, got %v", err)
	}
}

func TestDeleteDomainAndSyncGates(t *testing.T) {
	p, _ := setupFreshTestDB(t)
	accID := seedProviderAccount(t, p)
	ctx := context.Background()
	dom := Domain{OrgID: 1, Name: "gated.com", ProviderAccountID: accID}
	if err := p.db.Create(&dom).Error; err != nil {
		t.Fatal(err)
	}

	// deleteDomain: nil ctx (500) and org 0 (401).
	if _, err := p.deleteDomain(ctx, &DeleteDomainInput{Ctx: noCtx(), ID: dom.ID}); statusOf(t, err) != 500 {
		t.Errorf("deleteDomain nil ctx")
	}
	if _, err := p.deleteDomain(ctx, &DeleteDomainInput{Ctx: withOrgCtx(t, "0"), ID: dom.ID}); statusOf(t, err) != 401 {
		t.Errorf("deleteDomain org0")
	}

	// syncDomains: nil ctx (500), org 0 (401), member (403).
	body := struct {
		ProviderAccountID uint `json:"providerAccountId,omitempty"`
	}{0}
	if _, err := p.syncDomains(ctx, &SyncDomainsInput{Ctx: noCtx(), Body: body}); statusOf(t, err) != 500 {
		t.Errorf("syncDomains nil ctx")
	}
	if _, err := p.syncDomains(ctx, &SyncDomainsInput{Ctx: withOrgCtx(t, "0"), Body: body}); statusOf(t, err) != 401 {
		t.Errorf("syncDomains org0")
	}
	member := httptest.NewRequest(http.MethodPost, "/", nil)
	member.Header.Set("X-Org-ID", "1")
	member.Header.Set("X-Role", "member")
	if _, err := p.syncDomains(ctx, &SyncDomainsInput{Ctx: humago.NewContext(nil, member, httptest.NewRecorder()), Body: body}); statusOf(t, err) != 403 {
		t.Errorf("syncDomains member")
	}

	// createDDNSToken: nil ctx (500) and org 0 (401).
	if _, err := p.createDDNSToken(ctx, &createDDNSTokenInput{Ctx: noCtx()}); statusOf(t, err) != 500 {
		t.Errorf("createDDNSToken nil ctx")
	}
	if _, err := p.createDDNSToken(ctx, &createDDNSTokenInput{Ctx: withOrgCtx(t, "0")}); statusOf(t, err) != 401 {
		t.Errorf("createDDNSToken org0")
	}
	// deleteDDNSToken: nil ctx (500).
	if _, err := p.deleteDDNSToken(ctx, &deleteDDNSTokenInput{Ctx: noCtx(), ID: 1}); statusOf(t, err) != 500 {
		t.Errorf("deleteDDNSToken nil ctx")
	}
}

func TestUpdateDDNSMissingIP(t *testing.T) {
	p, _ := setupFreshTestDB(t)
	acc := ProviderAccount{OrgID: 1, Name: "acc", Type: "cloudflare", Config: "{}"}
	if err := p.db.Create(&acc).Error; err != nil {
		t.Fatal(err)
	}
	dom := Domain{OrgID: 1, Name: "noip.com", ProviderAccountID: acc.ID, ZoneID: "z"}
	if err := p.db.Create(&dom).Error; err != nil {
		t.Fatal(err)
	}
	secret, _ := generateDDNSSecret()
	if err := p.db.Create(&DDNSToken{OrgID: 1, DomainID: dom.ID, RecordName: "h.noip.com", RecordType: "A", TokenHash: hashDDNSSecret(secret)}).Error; err != nil {
		t.Fatal(err)
	}

	// Valid token but no IP anywhere (no query, no form, empty RemoteAddr) ->
	// dnserr through the empty-ip fallback.
	req := httptest.NewRequest(http.MethodGet, "/api/dns/ddns/update?token="+secret, nil)
	req.RemoteAddr = ""
	rec := httptest.NewRecorder()
	_, err := p.updateDDNS(context.Background(), &updateDDNSInput{Token: secret, Ctx: humago.NewContext(nil, req, rec)})
	if err != nil || strings.TrimSpace(rec.Body.String()) != "dnserr" {
		t.Fatalf("missing-ip: err=%v body=%q", err, rec.Body.String())
	}
}

func TestManagerSetProviderFailure(t *testing.T) {
	p, _ := setupFreshTestDB(t)
	fake := &fakeDNSProvider{updateErr: errBoom, createErr: errBoom}
	provName := registerFakeProvider(t, "mgr-fail-prov", fake)
	_, domID := seedDomainForProvider(t, p, "mgr-fail.example.com", "z", provName)
	ctx := context.Background()

	// Update branch.
	if _, err := p.DNSManager().Set(ctx, 1, domID, plugin.DNSRecord{ID: "r1", Type: "A", Name: "x", Content: "1.1.1.1"}); err == nil {
		t.Error("Set(update) must propagate provider error")
	}
	// Create branch.
	if _, err := p.DNSManager().Set(ctx, 1, domID, plugin.DNSRecord{Type: "A", Name: "x", Content: "1.1.1.1"}); err == nil {
		t.Error("Set(create) must propagate provider error")
	}
}
