package api

// Per-workspace branding resolved from the request Host.
//
// The login screen has to be branded before anyone authenticates, so the only
// tenant signal available is the hostname. Host is client-controlled, which is
// exactly why these tests pin that it selects presentation and nothing else.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugins/dns"
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
	// Things that are nobody's branded domain.
	for _, h := range []string{"localhost", "[::1]", "127.0.0.1", "192.168.1.10", ""} {
		if c := hostCandidates(h); len(c) != 0 {
			t.Errorf("hostCandidates(%q) = %v, want none", h, c)
		}
	}
}

func TestOrgIDForHostPrefersMostSpecificDomain(t *testing.T) {
	h, _, db := newTestHandlerRaw(t)
	const apexOrg, subOrg = uint(5101), uint(5102)

	if err := db.Create(&dns.Domain{OrgID: apexOrg, Name: "acme.com"}).Error; err != nil {
		t.Fatalf("seed apex: %v", err)
	}
	if err := db.Create(&dns.Domain{OrgID: subOrg, Name: "app.acme.com"}).Error; err != nil {
		t.Fatalf("seed sub: %v", err)
	}

	cases := map[string]uint{
		"app.acme.com":      subOrg,  // exact match wins over the apex
		"app.acme.com:8443": subOrg,  // port is irrelevant
		"APP.ACME.COM":      subOrg,  // case is irrelevant
		"acme.com":          apexOrg, // apex itself
		"mail.acme.com":     apexOrg, // unregistered subdomain falls up to the apex
		"someone-else.com":  0,       // unknown host is unbranded
		"127.0.0.1":         0,       // an IP is nobody's domain
		"localhost":         0,
	}
	for host, want := range cases {
		if got := h.orgIDForHost(host); got != want {
			t.Errorf("orgIDForHost(%q) = %d, want %d", host, got, want)
		}
	}
}

func TestBrandingFallbackChain(t *testing.T) {
	h, _, db := newTestHandlerRaw(t)
	const org = uint(5201)

	// Instance defaults only.
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

	// Workspace overrides the name but not the color.
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

	// No instance value either → built-in default.
	db.Where("key = ?", keyAppName).Delete(&models.Setting{})
	if got := h.AppNameFor(0); got == "" || got == "Instance Co" {
		t.Errorf("AppNameFor(0) with nothing set = %q, want the built-in default", got)
	}
}

// The pre-auth config endpoint is the whole point of host resolution: it must
// serve the tenant's brand on the tenant's domain and the instance brand on the
// shared host.
func TestAuthConfigBrandsByHost(t *testing.T) {
	srv, db := newTestHandler(t)
	const org = uint(5301)

	if err := db.Create(&dns.Domain{OrgID: org, Name: "acme.com"}).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
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
