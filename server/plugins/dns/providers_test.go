package dns

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
)

var errBoom = errors.New("boom")

// setupProviderDB is setupFullDNSTestDB plus the encrypt seam the provider
// CRUD paths need (the base harness only wires decrypt).
func setupProviderDB(t *testing.T) (*Plugin, func(req *http.Request) huma.Context) {
	t.Helper()
	p, mkCtx := setupFreshTestDB(t)
	p.encrypt = func(b []byte) (string, error) { return string(b), nil }
	return p, mkCtx
}

var auditCalls []string

func collectAudit(p *Plugin) {
	auditCalls = nil
	p.audit = func(r *http.Request, action, targetType string, targetID uint, meta map[string]any) {
		auditCalls = append(auditCalls, action)
	}
}

func mkAdmin(t *testing.T, mkCtx func(*http.Request) huma.Context) huma.Context {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Org-ID", "1")
	req.Header.Set("X-Role", "admin")
	return mkCtx(req)
}

func TestProviderAccountCRUDFlow(t *testing.T) {
	p, mkCtx := setupProviderDB(t)
	ctx := context.Background()
	collectAudit(p)

	// Create with credentials.
	created, err := p.createProviderAccount(ctx, &CreateProviderAccountInput{
		Ctx:  mkAdmin(t, mkCtx),
		Body: providerAccountDTO{Name: "  Personal CF  ", Type: "cloudflare", Config: map[string]any{"apiToken": "tok"}},
	})
	if err != nil {
		t.Fatalf("createProviderAccount: %v", err)
	}
	if created.Body.ID == 0 || created.Body.Name != "Personal CF" || !created.Body.HasCredentials {
		t.Fatalf("created account wrong: %+v", created.Body)
	}
	if len(auditCalls) != 1 || auditCalls[0] != "provider.create" {
		t.Errorf("expected provider.create audit, got %v", auditCalls)
	}
	var stored ProviderAccount
	if err := p.db.First(&stored, created.Body.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Config != `{"apiToken":"tok"}` {
		t.Errorf("stored encrypted config = %q", stored.Config)
	}

	// List, including a second account without credentials.
	p.db.Create(&ProviderAccount{OrgID: 1, Name: "No Creds", Type: "dnspod"})
	out, err := p.listProviderAccounts(ctx, &ListProviderAccountsInput{Ctx: mkAdmin(t, mkCtx)})
	if err != nil {
		t.Fatalf("listProviderAccounts: %v", err)
	}
	if len(out.Body) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(out.Body))
	}
	creds := map[uint]bool{}
	for _, a := range out.Body {
		creds[a.ID] = a.HasCredentials
	}
	if !creds[created.Body.ID] || creds[created.Body.ID+1] {
		t.Errorf("HasCredentials flags wrong: %v", creds)
	}

	// Update name and config.
	updated, err := p.updateProviderAccount(ctx, &UpdateProviderAccountInput{
		Ctx:  mkAdmin(t, mkCtx),
		ID:   created.Body.ID,
		Body: providerAccountDTO{Name: "Renamed", Config: map[string]any{"apiToken": "new"}},
	})
	if err != nil {
		t.Fatalf("updateProviderAccount: %v", err)
	}
	if updated.Body.Name != "Renamed" || updated.Body.Config != `{"apiToken":"new"}` {
		t.Errorf("updated account = %+v", updated.Body)
	}
	if len(auditCalls) != 2 || auditCalls[1] != "provider.update" {
		t.Errorf("expected provider.update audit, got %v", auditCalls)
	}

	// Delete.
	del, err := p.deleteProviderAccount(ctx, &DeleteProviderAccountInput{Ctx: mkAdmin(t, mkCtx), ID: created.Body.ID})
	if err != nil || !del.Body["ok"] {
		t.Fatalf("deleteProviderAccount: %v", err)
	}
	if len(auditCalls) != 3 || auditCalls[2] != "provider.delete" {
		t.Errorf("expected provider.delete audit, got %v", auditCalls)
	}
	var n int64
	p.db.Model(&ProviderAccount{}).Where("id = ?", created.Body.ID).Count(&n)
	if n != 0 {
		t.Errorf("account row still present after delete")
	}
}

func TestProviderAccountValidationAndGates(t *testing.T) {
	p, mkCtx := setupProviderDB(t)
	ctx := context.Background()
	acc := ProviderAccount{OrgID: 1, Name: "Other", Type: "cloudflare"}
	if err := p.db.Create(&acc).Error; err != nil {
		t.Fatal(err)
	}

	member := httptest.NewRequest(http.MethodPost, "/", nil)
	member.Header.Set("X-Org-ID", "1")
	member.Header.Set("X-Role", "member")

	// No name/type → 400.
	_, err := p.createProviderAccount(ctx, &CreateProviderAccountInput{
		Ctx:  mkAdmin(t, mkCtx),
		Body: providerAccountDTO{Name: "  ", Type: "cloudflare"},
	})
	if got := statusOf(t, err); got != http.StatusBadRequest {
		t.Fatalf("blank name: want 400, got %d", got)
	}

	// Member → 403.
	_, err = p.createProviderAccount(ctx, &CreateProviderAccountInput{
		Ctx: mkCtx(member), Body: providerAccountDTO{Name: "n", Type: "t"},
	})
	if got := statusOf(t, err); got != http.StatusForbidden {
		t.Fatalf("member create: want 403, got %d", got)
	}

	// org 0 → 401.
	noOrg := httptest.NewRequest(http.MethodPost, "/", nil)
	noOrg.Header.Set("X-Org-ID", "0")
	noOrg.Header.Set("X-Role", "admin")
	_, err = p.createProviderAccount(ctx, &CreateProviderAccountInput{
		Ctx: mkCtx(noOrg), Body: providerAccountDTO{Name: "n", Type: "t"},
	})
	if got := statusOf(t, err); got != http.StatusUnauthorized {
		t.Fatalf("org0 create: want 401, got %d", got)
	}

	// Nil context → 500.
	_, err = p.createProviderAccount(ctx, &CreateProviderAccountInput{Body: providerAccountDTO{Name: "n", Type: "t"}})
	if got := statusOf(t, err); got != http.StatusInternalServerError {
		t.Fatalf("nil ctx: want 500, got %d", got)
	}

	// Another org's account is invisible to this org's admin: update and
	// delete both 404, and nothing may be modified.
	other := ProviderAccount{OrgID: 2, Name: "Victim", Type: "cloudflare"}
	if err := p.db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	foreign := httptest.NewRequest(http.MethodPut, "/", nil)
	foreign.Header.Set("X-Org-ID", "1")
	foreign.Header.Set("X-Role", "admin")
	_, err = p.updateProviderAccount(ctx, &UpdateProviderAccountInput{Ctx: mkCtx(foreign), ID: other.ID, Body: providerAccountDTO{Name: "hijack"}})
	if got := statusOf(t, err); got != http.StatusNotFound {
		t.Fatalf("cross-org update: want 404, got %d", got)
	}
	_, err = p.deleteProviderAccount(ctx, &DeleteProviderAccountInput{Ctx: mkCtx(foreign), ID: other.ID})
	if got := statusOf(t, err); got != http.StatusNotFound {
		t.Fatalf("cross-org delete: want 404, got %d", got)
	}
	var after ProviderAccount
	p.db.First(&after, other.ID)
	if after.Name != "Victim" {
		t.Errorf("cross-org attacker modified the victim row: %+v", after)
	}
}

func TestDeleteProviderAccountInUseByDomain(t *testing.T) {
	p, mkCtx := setupProviderDB(t)
	ctx := context.Background()
	acc := ProviderAccount{OrgID: 1, Name: "Used", Type: "cloudflare"}
	if err := p.db.Create(&acc).Error; err != nil {
		t.Fatal(err)
	}
	p.db.Create(&Domain{OrgID: 1, Name: "used.com", ProviderAccountID: acc.ID})

	_, err := p.deleteProviderAccount(ctx, &DeleteProviderAccountInput{Ctx: mkAdmin(t, mkCtx), ID: acc.ID})
	if got := statusOf(t, err); got != http.StatusConflict {
		t.Fatalf("delete in-use account: want 409, got %d", got)
	}
	// The row must still exist.
	var n int64
	p.db.Model(&ProviderAccount{}).Where("id = ?", acc.ID).Count(&n)
	if n != 1 {
		t.Errorf("in-use account was deleted: count=%d", n)
	}
}

func TestProviderAccountEncryptFailure(t *testing.T) {
	p, mkCtx := setupProviderDB(t)
	p.encrypt = func([]byte) (string, error) { return "", errBoom }
	ctx := context.Background()

	_, err := p.createProviderAccount(ctx, &CreateProviderAccountInput{
		Ctx: mkAdmin(t, mkCtx), Body: providerAccountDTO{Name: "n", Type: "t", Config: map[string]any{"k": "v"}},
	})
	if got := statusOf(t, err); got != http.StatusInternalServerError {
		t.Fatalf("encrypt failure: want 500, got %d", got)
	}

	// Empty config maps to no stored credentials at all.
	p.encrypt = func(b []byte) (string, error) { return string(b), nil }
	created, err := p.createProviderAccount(ctx, &CreateProviderAccountInput{
		Ctx: mkAdmin(t, mkCtx), Body: providerAccountDTO{Name: "n", Type: "t"},
	})
	if err != nil {
		t.Fatalf("create without config: %v", err)
	}
	if created.Body.HasCredentials {
		t.Error("empty config must yield HasCredentials=false")
	}
}

func TestEncryptConfig(t *testing.T) {
	p := &Plugin{encrypt: func(b []byte) (string, error) { return "enc[" + string(b) + "]", nil }}
	if s, err := p.encryptConfig(nil); err != nil || s != "" {
		t.Errorf("encryptConfig(nil) = %q, %v; want empty", s, err)
	}
	s, err := p.encryptConfig(map[string]any{"a": "b"})
	if err != nil || s != `enc[{"a":"b"}]` {
		t.Errorf("encryptConfig(map) = %q, %v", s, err)
	}
	// An unserializable value surfaces the marshal error.
	p.encrypt = func(b []byte) (string, error) { return "", errBoom }
	_, err = p.encryptConfig(map[string]any{"ch": make(chan int)})
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Errorf("encryptConfig marshal err = %v, want unsupported-type error", err)
	}
}
