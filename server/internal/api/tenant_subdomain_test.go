package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/internal/tenancy"
	"github.com/octarq-org/octarq/server/origin"
	"gorm.io/gorm"
)

func setBaseDomain(t *testing.T, db *gorm.DB, base string) {
	t.Helper()
	if err := db.Save(&models.Setting{Key: models.BaseDomainSetting, Value: base}).Error; err != nil {
		t.Fatalf("set base domain: %v", err)
	}
}

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
	db.Model(&models.OrgHost{}).Where("owner_id = ?", org.ID).Count(&n)
	if n != 0 {
		t.Fatalf("creating an org with no base domain wrote %d domains row(s)", n)
	}
}

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
	var host models.OrgHost
	if err := db.Where("host = ?", want).First(&host).Error; err != nil {
		t.Fatalf("no host row %q after org creation: %v", want, err)
	}
	if host.OrgID != org.ID {
		t.Errorf("host row owner = %d, want %d", host.OrgID, org.ID)
	}
	if !host.Traffic {
		t.Error("tenant subdomain must serve traffic")
	}
	if owner, ok := origin.OwnerOf(db, want); !ok || owner != org.ID {
		t.Errorf("OwnerOf(%q) = (%d, %v), want (%d, true)", want, owner, ok, org.ID)
	}
}

func TestRegisterProvisionsTenantSubdomain(t *testing.T) {
	srv, db := newTestHandler(t)
	disableEmailVerification(t, db)
	enableMail(t, db)
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
	var host models.OrgHost
	if err := db.Where("host = ?", want).First(&host).Error; err != nil {
		t.Fatalf("no host row %q after register: %v", want, err)
	}
	if host.OrgID != org.ID {
		t.Errorf("host row owner = %d, want %d", host.OrgID, org.ID)
	}
}

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
	db.Model(&models.OrgHost{}).Where("host = ?", "owner-example-com.app.example.com").Count(&n)
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

func TestRenameCollisionOnSubdomainRollsBack(t *testing.T) {
	db, do, owner, _, _ := orgSlugFixture(t)
	setBaseDomain(t, db, "app.example.com")
	if _, _, err := tenancy.Provision(db, 1, "owner-example-com"); err != nil {
		t.Fatalf("provision org 1: %v", err)
	}
	if err := db.Create(&models.OrgHost{OrgID: 2, Host: "ghost.app.example.com", Source: "dns"}).Error; err != nil {
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
