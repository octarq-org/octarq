package dns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/dnsprovider"
)

func TestCreateDDNSTokenErrors(t *testing.T) {
	p, mkCtx := setupFreshTestDB(t)
	ctx := context.Background()
	accID := uint(0)

	// Member forbidden.
	member := httptest.NewRequest(http.MethodPost, "/", nil)
	member.Header.Set("X-Org-ID", "1")
	member.Header.Set("X-Role", "member")
	in := &createDDNSTokenInput{Ctx: mkCtx(member)}
	in.Body.DomainID = 1
	if _, err := p.createDDNSToken(ctx, in); statusOf(t, err) != http.StatusForbidden {
		t.Fatalf("member create: want 403")
	}

	// Missing domain id -> 400.
	in = &createDDNSTokenInput{Ctx: mkAdmin(t, mkCtx)}
	if _, err := p.createDDNSToken(ctx, in); statusOf(t, err) != http.StatusBadRequest {
		t.Fatalf("no domain id: want 400")
	}

	// Unknown domain (including another org's) -> 404.
	p.db.Create(&Domain{OrgID: 2, Name: "v.com"})
	in = &createDDNSTokenInput{Ctx: mkAdmin(t, mkCtx)}
	in.Body.DomainID = 1
	if _, err := p.createDDNSToken(ctx, in); statusOf(t, err) != http.StatusNotFound {
		t.Fatalf("foreign/unknown domain: want 404")
	}

	// Blank record name -> 400.
	acc := ProviderAccount{OrgID: 1, Name: "acc", Type: "cloudflare"}
	if err := p.db.Create(&acc).Error; err != nil {
		t.Fatal(err)
	}
	dom := Domain{OrgID: 1, Name: "ddns.com", ProviderAccountID: acc.ID}
	if err := p.db.Create(&dom).Error; err != nil {
		t.Fatal(err)
	}
	in = &createDDNSTokenInput{Ctx: mkAdmin(t, mkCtx)}
	in.Body.DomainID = dom.ID
	if _, err := p.createDDNSToken(ctx, in); statusOf(t, err) != http.StatusBadRequest {
		t.Fatalf("blank record name: want 400")
	}

	// Invalid record type -> 400.
	in.Body.RecordName = "home.ddns.com"
	in.Body.RecordType = "CNAME"
	if _, err := p.createDDNSToken(ctx, in); statusOf(t, err) != http.StatusBadRequest {
		t.Fatalf("bad type: want 400")
	}

	// Record type defaults to A when omitted.
	in.Body.RecordType = ""
	ok, err := p.createDDNSToken(ctx, in)
	if err != nil || ok.Body.RecordType != "A" || ok.Body.Secret == "" {
		t.Fatalf("default type A: %v %+v", err, ok.Body)
	}
	accID = ok.Body.ID

	// Deleting a token that does not exist (or is another org's) -> 404.
	delIn := &deleteDDNSTokenInput{Ctx: mkAdmin(t, mkCtx), ID: accID + 999}
	if _, err := p.deleteDDNSToken(ctx, delIn); statusOf(t, err) != http.StatusNotFound {
		t.Fatalf("delete missing: want 404")
	}
}

func TestDeleteDDNSTokenGates(t *testing.T) {
	p, mkCtx := setupFreshTestDB(t)
	ctx := context.Background()

	noOrg := httptest.NewRequest(http.MethodDelete, "/", nil)
	noOrg.Header.Set("X-Org-ID", "0")
	noOrg.Header.Set("X-Role", "admin")
	if _, err := p.deleteDDNSToken(ctx, &deleteDDNSTokenInput{Ctx: mkCtx(noOrg), ID: 1}); statusOf(t, err) != http.StatusUnauthorized {
		t.Fatalf("org0 delete: want 401")
	}
	member := httptest.NewRequest(http.MethodDelete, "/", nil)
	member.Header.Set("X-Org-ID", "1")
	member.Header.Set("X-Role", "member")
	if _, err := p.deleteDDNSToken(ctx, &deleteDDNSTokenInput{Ctx: mkCtx(member), ID: 1}); statusOf(t, err) != http.StatusForbidden {
		t.Fatalf("member delete: want 403")
	}
}

func TestUpdateDDNSProviderFailurePaths(t *testing.T) {
	p, mkCtx := setupFreshTestDB(t)
	_ = mkCtx

	// A token whose domain has no provider account: providerFor fails -> dnserr.
	createHarnessToken := func(t *testing.T, p *Plugin) (string, Domain) {
		t.Helper()
		dom := Domain{OrgID: 1, Name: "no-provider.com"}
		if err := p.db.Create(&dom).Error; err != nil {
			t.Fatal(err)
		}
		secret, _ := generateDDNSSecret()
		tok := DDNSToken{OrgID: 1, DomainID: dom.ID, RecordName: "h.no-provider.com", RecordType: "A", TokenHash: hashDDNSSecret(secret)}
		if err := p.db.Create(&tok).Error; err != nil {
			t.Fatal(err)
		}
		return secret, dom
	}

	secret, _ := createHarnessToken(t, p)
	req := httptest.NewRequest(http.MethodGet, "/api/dns/ddns/update?token="+secret+"&ip=1.2.3.4", nil)
	rec := httptest.NewRecorder()
	ctx := humago.NewContext(nil, req, rec)
	_, err := p.updateDDNS(context.Background(), &updateDDNSInput{Token: secret, IP: "1.2.3.4", Ctx: ctx})
	if err != nil || strings.TrimSpace(rec.Body.String()) != "dnserr" {
		t.Fatalf("provider-less domain: err=%v body=%q", err, rec.Body.String())
	}
}

func TestUpdateDDNSRecordFailurePaths(t *testing.T) {
	p, mkCtx := setupFreshGateTestDB(t)
	_ = mkCtx
	fake := &fakeDNSProvider{listRecErr: errBoom}
	provName := registerFakeProvider(t, "ddns-rec-fail-prov", fake)
	acc := ProviderAccount{OrgID: 1, Name: "acc", Type: provName, Config: "{}"}
	if err := p.db.Create(&acc).Error; err != nil {
		t.Fatal(err)
	}
	dom := Domain{OrgID: 1, Name: "rec-fail.com", ProviderAccountID: acc.ID, ZoneID: "z"}
	if err := p.db.Create(&dom).Error; err != nil {
		t.Fatal(err)
	}
	secret, _ := generateDDNSSecret()
	if err := p.db.Create(&DDNSToken{OrgID: 1, DomainID: dom.ID, RecordName: "h.rec-fail.com", RecordType: "A", TokenHash: hashDDNSSecret(secret)}).Error; err != nil {
		t.Fatal(err)
	}

	call := func() (int, string) {
		req := httptest.NewRequest(http.MethodGet, "/api/dns/ddns/update?token="+secret+"&ip=1.2.3.4", nil)
		rec := httptest.NewRecorder()
		p.updateDDNS(context.Background(), &updateDDNSInput{Token: secret, IP: "1.2.3.4", Ctx: humago.NewContext(nil, req, rec)})
		return rec.Code, strings.TrimSpace(rec.Body.String())
	}

	// ListRecords failing -> dnserr.
	if code, body := call(); code != http.StatusOK || body != "dnserr" {
		t.Fatalf("list records fail: code=%d body=%q", code, body)
	}

	// Update failure -> dnserr (the existing record must differ from the
	// requested IP, otherwise the no-change short-circuit fires first).
	fake.listRecErr = nil
	fake.records = []dnsprovider.Record{{ID: "r1", Type: "A", Name: "h.rec-fail.com", Content: "9.9.9.9"}}
	fake.updateErr = errBoom
	if code, body := call(); code != http.StatusOK || body != "dnserr" {
		t.Fatalf("update fail: code=%d body=%q", code, body)
	}

	// Create failure on a missing record -> dnserr.
	fake.records = nil
	fake.updateErr = nil
	fake.createErr = errBoom
	if code, body := call(); code != http.StatusOK || body != "dnserr" {
		t.Fatalf("create fail: code=%d body=%q", code, body)
	}
}

func TestNormalizeHostIPv6(t *testing.T) {
	cases := map[string]string{
		"[::1]:8080":          "[::1]", // bracketed literal: cut after "]"
		"GO.Example.com:8443": "go.example.com",
		"go.example.com.":     "go.example.com",
		"2001:db8::1":         "2001:db8:", // unbracketed literal: last-colon cut
	}
	for in, want := range cases {
		if got := NormalizeHost(in); got != want {
			t.Errorf("NormalizeHost(%q) = %q, want %q", in, got, want)
		}
	}
}
