package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
)

func TestNormalizeHost(t *testing.T) {
	cases := map[string]string{
		"acme.com":          "acme.com",
		"Acme.COM":          "acme.com",
		"acme.com:8443":     "acme.com",
		"acme.com.":         "acme.com",
		"  acme.com  ":      "acme.com",
		"[::1]:8080":        "[::1]",
		"192.168.1.10:8080": "192.168.1.10",
		"":                  "",
	}
	for in, want := range cases {
		if got := normalizeHost(in); got != want {
			t.Errorf("normalizeHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHostCandidatesMostSpecificFirst(t *testing.T) {
	got := hostCandidates("mail.app.acme.com")
	want := []string{"mail.app.acme.com", "app.acme.com", "acme.com"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("hostCandidates = %v, want %v", got, want)
	}
	for _, h := range []string{"localhost", "[::1]", "127.0.0.1", "192.168.1.10", ""} {
		if c := hostCandidates(h); len(c) != 0 {
			t.Errorf("hostCandidates(%q) = %v, want none", h, c)
		}
	}
}

func TestOrgIDForHostPrefersMostSpecificDomain(t *testing.T) {
	h, _, db := newTestHandlerRaw(t)
	const apexOrg, subOrg = uint(5101), uint(5102)

	if err := db.Create(&models.OrgHost{OrgID: apexOrg, Host: "acme.com", Source: "test"}).Error; err != nil {
		t.Fatalf("seed apex: %v", err)
	}
	if err := db.Create(&models.OrgHost{OrgID: subOrg, Host: "app.acme.com", Source: "test"}).Error; err != nil {
		t.Fatalf("seed sub: %v", err)
	}

	cases := map[string]uint{
		"app.acme.com":      subOrg,
		"app.acme.com:8443": subOrg,
		"APP.ACME.COM":      subOrg,
		"acme.com":          apexOrg,
		"mail.acme.com":     apexOrg,
		"someone-else.com":  0,
		"127.0.0.1":         0,
		"localhost":         0,
	}
	for host, want := range cases {
		if got := h.orgIDForHost(host); got != want {
			t.Errorf("orgIDForHost(%q) = %d, want %d", host, got, want)
		}
	}
}

func TestOrgIDForHostRejectsAmbiguousOwners(t *testing.T) {
	h, _, db := newTestHandlerRaw(t)

	var indexes []string
	if err := db.Raw("SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = 'org_hosts' AND sql IS NOT NULL").Scan(&indexes).Error; err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	for _, name := range indexes {
		if err := db.Exec("DROP INDEX IF EXISTS " + name).Error; err != nil {
			t.Fatalf("drop index %s: %v", name, err)
		}
	}
	for _, row := range []models.OrgHost{
		{OrgID: 1, Host: "shared.example", Source: "test"},
		{OrgID: 2, Host: "shared.example", Source: "test"},
	} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("seed ambiguous host: %v", err)
		}
	}

	if got := h.orgIDForHost("shared.example"); got != 0 {
		t.Errorf("orgIDForHost(ambiguous) = %d, want 0", got)
	}
	if got := h.orgIDForHost("app.shared.example"); got != 0 {
		t.Errorf("orgIDForHost(ambiguous parent) = %d, want 0", got)
	}
}

func TestBrandingFallbackChain(t *testing.T) {
	h, _, db := newTestHandlerRaw(t)
	const org = uint(5201)

	if err := db.Save(&models.Setting{Key: keyAppName, Value: "Instance Co"}).Error; err != nil {
		t.Fatalf("seed instance name: %v", err)
	}
	if err := db.Save(&models.Setting{Key: keyBrandColor, Value: "#111111"}).Error; err != nil {
		t.Fatalf("seed instance color: %v", err)
	}

	if got := h.AppNameFor(0); got != "Instance Co" {
		t.Errorf("AppNameFor(0) = %q, want the instance name", got)
	}
	if got := h.AppNameFor(org); got != "Instance Co" {
		t.Errorf("workspace with no branding should inherit the instance name, got %q", got)
	}

	if err := h.SetWorkspaceSetting(org, keyAppName, "Acme Links"); err != nil {
		t.Fatalf("set workspace name: %v", err)
	}
	if got := h.AppNameFor(org); got != "Acme Links" {
		t.Errorf("AppNameFor(org) = %q, want the workspace override", got)
	}
	if got := h.AppNameFor(0); got != "Instance Co" {
		t.Errorf("a workspace override must not leak into the instance default, got %q", got)
	}
	if _, color, _ := h.BrandFor(org); color != "#111111" {
		t.Errorf("unset workspace key should inherit instance color, got %q", color)
	}

	db.Where("key = ?", keyAppName).Delete(&models.Setting{})
	if got := h.AppNameFor(0); got == "" || got == "Instance Co" {
		t.Errorf("AppNameFor(0) with nothing set = %q, want the built-in default", got)
	}
}

func TestAuthConfigBrandsByHost(t *testing.T) {
	srv, db := newTestHandler(t)
	const org = uint(5301)

	if err := db.Create(&models.OrgHost{OrgID: org, Host: "acme.com", Source: "test"}).Error; err != nil {
		t.Fatalf("seed host: %v", err)
	}
	if err := db.Save(&models.Setting{Key: keyAppName, Value: "Octarq Cloud"}).Error; err != nil {
		t.Fatalf("seed instance name: %v", err)
	}
	if err := db.Save(&models.WorkspaceSetting{OrgID: org, Key: keyAppName, Value: "Acme Links"}).Error; err != nil {
		t.Fatalf("seed workspace name: %v", err)
	}

	appNameOn := func(host string) string {
		req := httptest.NewRequest("GET", "/api/auth/config", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("auth config on %s: got %d", host, rec.Code)
		}
		var body struct {
			AppName string `json:"appName"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return body.AppName
	}

	if got := appNameOn("acme.com"); got != "Acme Links" {
		t.Errorf("tenant domain: got %q, want the workspace brand", got)
	}
	if got := appNameOn("app.octarq.org"); got != "Octarq Cloud" {
		t.Errorf("shared host: got %q, want the instance brand", got)
	}
}
