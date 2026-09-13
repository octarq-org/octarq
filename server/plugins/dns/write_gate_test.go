package dns

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/dnsprovider"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

// setupGateTestDB initialises a minimal in-memory plugin with role and orgID wired.
func setupGateTestDB(t *testing.T) (*Plugin, func(role string) huma.Context) {
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

	mkCtx := func(role string) huma.Context {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.Header.Set("X-Org-ID", "1")
		r.Header.Set("X-Role", role)
		return humago.NewContext(nil, r, httptest.NewRecorder())
	}
	return p, mkCtx
}

// TestCreateRecordRoleGate verifies that member is denied and admin passes through.
func TestCreateRecordRoleGate(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupGateTestDB(t)

	// Seed domain
	acc := ProviderAccount{OrgID: 1, Name: "CF", Type: "cloudflare"}
	p.db.Create(&acc)
	dom := Domain{OrgID: 1, Name: "gate-test.com", ProviderAccountID: acc.ID, ZoneID: "zone-abc"}
	p.db.Create(&dom)

	in := &CreateRecordInput{
		ID:   dom.ID,
		Body: dnsprovider.Record{Type: "A", Content: "1.2.3.4", Name: "gate-test.com"},
	}

	// member → must be refused before hitting the provider
	in.Ctx = mkCtx("member")
	_, err := p.createRecord(context.Background(), in)
	if err == nil {
		t.Fatal("createRecord: expected 403 for member, got nil error")
		return
	}
	if he, ok := err.(huma.StatusError); !ok || he.GetStatus() != 403 {
		t.Fatalf("createRecord: expected 403, got %v", err)
	}

	// admin → provider will error (no real creds) but the gate must pass
	in.Ctx = mkCtx("admin")
	_, err = p.createRecord(context.Background(), in)
	// We expect a provider-level error (e.g. no creds), NOT a 403
	if err == nil {
		t.Fatal("createRecord: expected provider error for admin with bad creds, got nil")
		return
	}
	if he, ok := err.(huma.StatusError); ok && he.GetStatus() == 403 {
		t.Fatalf("createRecord: admin must not be refused with 403, got %v", err)
	}
}

// TestUpdateRecordRoleGate verifies that member is denied and admin passes through.
func TestUpdateRecordRoleGate(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupGateTestDB(t)

	acc := ProviderAccount{OrgID: 1, Name: "CF2", Type: "cloudflare"}
	p.db.Create(&acc)
	dom := Domain{OrgID: 1, Name: "gate-upd.com", ProviderAccountID: acc.ID, ZoneID: "zone-upd"}
	p.db.Create(&dom)

	in := &UpdateRecordInput{
		ID:   dom.ID,
		RID:  "rec-123",
		Body: dnsprovider.Record{Type: "A", Content: "5.6.7.8", Name: "gate-upd.com"},
	}

	// member → 403
	in.Ctx = mkCtx("member")
	_, err := p.updateRecord(context.Background(), in)
	if err == nil {
		t.Fatal("updateRecord: expected 403 for member, got nil error")
		return
	}
	if he, ok := err.(huma.StatusError); !ok || he.GetStatus() != 403 {
		t.Fatalf("updateRecord: expected 403, got %v", err)
	}

	// admin → reaches provider (may error on creds, but not 403)
	in.Ctx = mkCtx("admin")
	_, err = p.updateRecord(context.Background(), in)
	if err == nil {
		t.Fatal("updateRecord: expected provider error for admin with bad creds, got nil")
		return
	}
	if he, ok := err.(huma.StatusError); ok && he.GetStatus() == 403 {
		t.Fatalf("updateRecord: admin must not be refused with 403, got %v", err)
	}
}

// TestProviderErrSanitization verifies that a recognisable marker in the
// upstream error does NOT appear in the HTTP response body, but the log still
// contains it (providerErr calls log.Printf before returning).
func TestProviderErrSanitization(t *testing.T) {
	t.Parallel()
	p := &Plugin{}

	marker := "CF-RAY:abc123xyz-secret"
	rawErr := errors.New("cloudflare: create record: 400 Bad Request: " + marker)

	returned := p.providerErr("create record", rawErr)
	if returned == nil {
		t.Fatal("providerErr returned nil")
	}

	// The HTTP response body must NOT contain the marker.
	if strings.Contains(returned.Error(), marker) {
		t.Errorf("provider raw text leaked into response: %v", returned.Error())
	}
	// It must still contain the action name so the user knows what failed.
	if !strings.Contains(returned.Error(), "create record") {
		t.Errorf("response should contain the action name, got: %v", returned.Error())
	}
}
