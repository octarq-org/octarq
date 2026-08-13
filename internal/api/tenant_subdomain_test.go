package api

import (
	"encoding/json"
	"net/http"
	"testing"

	dnsmodels "github.com/octarq-org/octarq/plugins/dns"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/tenancy"
	"github.com/octarq-org/octarq/origin"
	"gorm.io/gorm"
)

// setBaseDomain writes the shared tenant-subdomain base as the instance
// settings API would.
func setBaseDomain(t *testing.T, db *gorm.DB, base string) {
	t.Helper()
	if err := db.Save(&models.Setting{Key: models.BaseDomainSetting, Value: base}).Error; err != nil {
		t.Fatalf("set base domain: %v", err)
	}
}

// Guard 1: with no base domain configured, creating an org provisions no
// domains row — behaviour identical to today.
func TestCreateOrgWithNoBaseProvisionsNothing(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := do(srv, "POST", "/api/orgs", cookies, `{"name":"Second Org"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create org: got %d (%s)", rec.Code, rec.Body.String())
	}
	var org models.Org
	if err := json.Unmarshal(rec.Body.Bytes(), &org); err != nil {
		t.Fatalf("decode org: %v", err)
	}
	var n int64
	db.Model(&dnsmodels.Domain{}).Where("owner_id = ?", org.ID).Count(&n)
	if n != 0 {
		t.Fatalf("creating an org with no base domain wrote %d domains row(s)", n)
	}
}

// The base domain is a runtime instance setting: setting it through the admin
// API must (a) round-trip and (b) make the workspace settings report the org's
// tenant subdomain.
func TestInstanceSettingsBaseDomainRoundTrip(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := loginCookies(t, srv)

	rec := do(srv, "PUT", "/api/instance-settings", cookies, `{"baseDomain":"app.octarq.org"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put instance settings: got %d (%s)", rec.Code, rec.Body.String())
	}
	var got struct {
		BaseDomain string `json:"baseDomain"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode put response: %v", err)
	}
	if got.BaseDomain != "app.octarq.org" {
		t.Fatalf("baseDomain after put = %q, want app.octarq.org", got.BaseDomain)
	}

	var org models.Org
	db.First(&org, 1)
	rec = do(srv, "GET", "/api/settings", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get settings: got %d", rec.Code)
	}
	var s struct {
		OrgSlug         string `json:"orgSlug"`
		TenantSubdomain string `json:"tenantSubdomain"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatalf("decode get settings: %v", err)
	}
	if s.OrgSlug == "" {
		t.Fatal("no org slug in settings response")
	}
	if want := s.OrgSlug + ".app.octarq.org"; s.TenantSubdomain != want {
		t.Fatalf("tenantSubdomain = %q, want %q", s.TenantSubdomain, want)
	}

	// Clearing the setting turns the feature off again.
	rec = do(srv, "PUT", "/api/instance-settings", cookies, `{"baseDomain":""}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear base domain: got %d", rec.Code)
	}
	rec = do(srv, "GET", "/api/settings", cookies, "")
	json.Unmarshal(rec.Body.Bytes(), &s)
	if s.TenantSubdomain != "" {
		t.Fatalf("tenantSubdomain after clearing base = %q, want \"\"", s.TenantSubdomain)
	}
}

// Guard 2: with a base domain configured, POST /api/orgs provisions the
// <slug>.<base> row and origin resolves the hostname to the new org.
func TestCreateOrgProvisionsTenantSubdomain(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := loginCookies(t, srv)
	setBaseDomain(t, db, "app.octarq.org")

	rec := do(srv, "POST", "/api/orgs", cookies, `{"name":"Second Org"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create org: got %d (%s)", rec.Code, rec.Body.String())
	}
	var org models.Org
	if err := json.Unmarshal(rec.Body.Bytes(), &org); err != nil {
		t.Fatalf("decode org: %v", err)
	}

	want := org.Slug + ".app.octarq.org"
	var dom dnsmodels.Domain
	if err := db.Where("name = ?", want).First(&dom).Error; err != nil {
		t.Fatalf("no domain row %q after org creation: %v", want, err)
	}
	if dom.OrgID != org.ID {
		t.Errorf("domain row owner = %d, want %d", dom.OrgID, org.ID)
	}
	if owner, ok := origin.OwnerOf(db, want); !ok || owner != org.ID {
		t.Errorf("OwnerOf(%q) = (%d, %v), want (%d, true)", want, owner, ok, org.ID)
	}
}

// Guard 2 via the self-serve register path — the most common way a new tenant
// arrives on a cloud instance.
func TestRegisterProvisionsTenantSubdomain(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)
	setBaseDomain(t, db, "app.octarq.org")

	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"founder@example.com","password":"password123","orgName":"Acme"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("register: got %d (%s)", rec.Code, rec.Body.String())
	}

	var org models.Org
	if err := db.Where("name = ?", "Acme").First(&org).Error; err != nil {
		t.Fatalf("registered org not found: %v", err)
	}
	want := org.Slug + ".app.octarq.org"
	var dom dnsmodels.Domain
	if err := db.Where("name = ?", want).First(&dom).Error; err != nil {
		t.Fatalf("no domain row %q after register: %v", want, err)
	}
	if dom.OrgID != org.ID {
		t.Errorf("domain row owner = %d, want %d", dom.OrgID, org.ID)
	}
}

// Trap 4: renaming the slug must move the tenant subdomain — the old address
// goes offline (it must not keep resolving, and must not be claimable), and
// the new one is provisioned for the same org.
func TestRenameMovesTenantSubdomain(t *testing.T) {
	db, do, owner, _, _ := orgSlugFixture(t)
	setBaseDomain(t, db, "app.example.com")
	if _, _, err := tenancy.Provision(db, 1, "owner-example-com"); err != nil {
		t.Fatalf("provision org 1: %v", err)
	}

	rec := do("PUT", "/api/org/slug", owner, `{"slug":"acme"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("rename: got %d (%s)", rec.Code, rec.Body.String())
	}

	var n int64
	db.Model(&dnsmodels.Domain{}).Where("name = ?", "owner-example-com.app.example.com").Count(&n)
	if n != 0 {
		t.Errorf("the old subdomain row survived the rename")
	}
	if ownerID, ok := origin.OwnerOf(db, "owner-example-com.app.example.com"); ok || ownerID != 0 {
		t.Errorf("old subdomain after rename = (%d, %v), want (0, false)", ownerID, ok)
	}
	if ownerID, ok := origin.OwnerOf(db, "acme.app.example.com"); !ok || ownerID != 1 {
		t.Errorf("new subdomain after rename = (%d, %v), want (1, true)", ownerID, ok)
	}
}

// Trap 6: a subdomain collision must fail the whole operation with a clear
// conflict, and roll everything back — the slug stays, and the org's existing
// address keeps resolving.
func TestRenameCollisionOnSubdomainRollsBack(t *testing.T) {
	db, do, owner, _, _ := orgSlugFixture(t)
	setBaseDomain(t, db, "app.example.com")
	if _, _, err := tenancy.Provision(db, 1, "owner-example-com"); err != nil {
		t.Fatalf("provision org 1: %v", err)
	}
	// A stale row already claims the target address — e.g. a registration that
	// predates the base domain and nobody cleaned up. The slug itself is free.
	if err := db.Create(&dnsmodels.Domain{OrgID: 2, Name: "ghost.app.example.com"}).Error; err != nil {
		t.Fatalf("seed stale subdomain: %v", err)
	}

	rec := do("PUT", "/api/org/slug", owner, `{"slug":"ghost"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("rename to a claimed subdomain = %d, want 409 (%s)", rec.Code, rec.Body.String())
	}

	var org models.Org
	db.First(&org, 1)
	if org.Slug != "owner-example-com" {
		t.Fatalf("slug changed to %q after a refused rename", org.Slug)
	}
	if ownerID, ok := origin.OwnerOf(db, "owner-example-com.app.example.com"); !ok || ownerID != 1 {
		t.Errorf("old subdomain after refused rename = (%d, %v), want (1, true)", ownerID, ok)
	}
}
