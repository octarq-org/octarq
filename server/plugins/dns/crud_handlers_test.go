package dns

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

func setupFullDNSTestDB(t *testing.T) (*Plugin, func(req *http.Request) huma.Context) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(append(models.AllModels(), &Domain{}, &ProviderAccount{}, &DDNSToken{})...); err != nil {
		t.Fatalf("migrate: %v", err)
	}

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

	mkCtx := func(r *http.Request) huma.Context {
		if r.Header.Get("X-Org-ID") == "" {
			r.Header.Set("X-Org-ID", "1")
		}
		return humago.NewContext(nil, r, httptest.NewRecorder())
	}
	return p, mkCtx
}

func TestDNSDomainCRUDHandlers(t *testing.T) {
	t.Parallel()

	p, mkCtx := setupFullDNSTestDB(t)
	ctx := context.Background()

	// Seed ProviderAccount
	acc := ProviderAccount{OrgID: 1, Name: "Test Provider", Type: "cloudflare"}
	p.db.Create(&acc)

	// 1. Create Domain
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/dns/domains", nil)
	createIn := &CreateDomainInput{
		Ctx: mkCtx(reqCreate),
		Body: domainDTO{
			Name:              "mydomain.com",
			ProviderAccountID: acc.ID,
			Note:              "Primary domain",
		},
	}
	outCreate, err := p.createDomain(ctx, createIn)
	if err != nil {
		t.Fatalf("createDomain error: %v", err)
	}
	domID := outCreate.Body.ID

	// 2. List Domains
	reqList := httptest.NewRequest(http.MethodGet, "/api/dns/domains?q=mydomain", nil)
	listOut, err := p.listDomains(ctx, &ListDomainsInput{
		Ctx: mkCtx(reqList),
		Q:   "mydomain",
	})
	if err != nil || len(listOut.Body) != 1 {
		t.Fatalf("listDomains error=%v, count=%d", err, len(listOut.Body))
	}

	// 3. Update Domain
	bTrue := true
	reqUp := httptest.NewRequest(http.MethodPut, "/api/dns/domains/1", nil)
	upOut, err := p.updateDomain(ctx, &UpdateDomainInput{
		Ctx:  mkCtx(reqUp),
		ID:   domID,
		Body: domainDTO{Note: "Updated note", ForMail: &bTrue},
	})
	if err != nil || upOut.Body.Note != "Updated note" || !upOut.Body.ForMail {
		t.Fatalf("updateDomain error=%v, out=%+v", err, upOut)
	}

	// 4. Delete Domain - member forbidden
	reqDelMember := httptest.NewRequest(http.MethodDelete, "/api/dns/domains/1", nil)
	reqDelMember.Header.Set("X-Role", "member")
	_, err = p.deleteDomain(ctx, &DeleteDomainInput{Ctx: mkCtx(reqDelMember), ID: domID})
	if err == nil {
		t.Error("expected 403 when deleting domain as member")
	}

	// Delete Domain - admin success
	reqDelAdmin := httptest.NewRequest(http.MethodDelete, "/api/dns/domains/1", nil)
	reqDelAdmin.Header.Set("X-Role", "admin")
	delOut, err := p.deleteDomain(ctx, &DeleteDomainInput{Ctx: mkCtx(reqDelAdmin), ID: domID})
	if err != nil || !delOut.Body["ok"] {
		t.Fatalf("deleteDomain error=%v", err)
	}
}

func TestDNSProvidersHandler(t *testing.T) {
	t.Parallel()

	p, mkCtx := setupFullDNSTestDB(t)
	ctx := context.Background()

	req := httptest.NewRequest(http.MethodGet, "/api/dns/providers", nil)
	out, err := p.dnsProviders(ctx, &DNSProvidersInput{Ctx: mkCtx(req)})
	if err != nil || len(out.Body) == 0 {
		t.Fatalf("dnsProviders error=%v, count=%d", err, len(out.Body))
	}
}
