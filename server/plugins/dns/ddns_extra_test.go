package dns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDDNSClientIPAndSecretHelpers(t *testing.T) {
	t.Parallel()

	h := hashDDNSSecret("secret123")
	if len(h) != 64 {
		t.Errorf("hashDDNSSecret output length = %d, want 64", len(h))
	}

	sec, err := generateDDNSSecret()
	if err != nil || len(sec) != 48 {
		t.Fatalf("generateDDNSSecret failed: %v, len=%d", err, len(sec))
	}

	SetTrustProxy(true)
	defer SetTrustProxy(false)

	reqXFF := httptest.NewRequest("GET", "/", nil)
	reqXFF.Header.Set("X-Forwarded-For", "203.0.113.195, 70.41.3.18")
	if ip := clientIP(reqXFF); ip != "203.0.113.195" {
		t.Errorf("clientIP XFF expected 203.0.113.195, got %s", ip)
	}

	reqXRI := httptest.NewRequest("GET", "/", nil)
	reqXRI.Header.Set("X-Real-IP", "198.51.100.1")
	if ip := clientIP(reqXRI); ip != "198.51.100.1" {
		t.Errorf("clientIP XRI expected 198.51.100.1, got %s", ip)
	}

	reqRemote := httptest.NewRequest("GET", "/", nil)
	reqRemote.RemoteAddr = "192.0.2.1:12345"
	if ip := clientIP(reqRemote); ip != "192.0.2.1" {
		t.Errorf("clientIP RemoteAddr expected 192.0.2.1, got %s", ip)
	}
}

func TestDDNSTokenCRUDHandlers(t *testing.T) {
	t.Parallel()

	p, mkCtx := setupFullDNSTestDB(t)
	ctx := context.Background()

	acc := ProviderAccount{OrgID: 1, Name: "Prov", Type: "cloudflare"}
	p.db.Create(&acc)
	dom := Domain{OrgID: 1, Name: "ddns.com", ProviderAccountID: acc.ID}
	p.db.Create(&dom)

	reqCreate := httptest.NewRequest(http.MethodPost, "/api/dns/ddns", nil)
	createIn := &createDDNSTokenInput{
		Ctx: mkCtx(reqCreate),
	}
	createIn.Body.DomainID = dom.ID
	createIn.Body.RecordName = "home.ddns.com"
	createIn.Body.RecordType = "A"
	createIn.Body.Label = "Home IP"

	outCreate, err := p.createDDNSToken(ctx, createIn)
	if err != nil || outCreate.Body.Secret == "" {
		t.Fatalf("createDDNSToken failed: %v, %+v", err, outCreate)
	}
	tokID := outCreate.Body.ID

	reqList := httptest.NewRequest(http.MethodGet, "/api/dns/ddns", nil)
	outList, err := p.listDDNSTokens(ctx, &listDDNSTokensInput{Ctx: mkCtx(reqList)})
	if err != nil || len(outList.Body) != 1 {
		t.Fatalf("listDDNSTokens failed: %v, count=%d", err, len(outList.Body))
	}

	reqDel := httptest.NewRequest(http.MethodDelete, "/api/dns/ddns/1", nil)
	outDel, err := p.deleteDDNSToken(ctx, &deleteDDNSTokenInput{Ctx: mkCtx(reqDel), ID: tokID})
	if err != nil || !outDel.Body.OK {
		t.Fatalf("deleteDDNSToken failed: %v", err)
	}
}
