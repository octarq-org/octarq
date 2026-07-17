package dns

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openMemDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	db.AutoMigrate(models.AllModels()...)
	return db
}

func newVerifyHarness(t *testing.T) (*Plugin, http.Handler, *gorm.DB) {
	db := openMemDB(t) // sqlite :memory:, AutoMigrate models.AllModels()
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("t", "1.0"))
	p := New()
	reg := plugin.NewRegistry()
	p.Mount(nil, &plugin.Context{
		Huma: api, DB: db,
		OrgID:   func(*http.Request) uint { return 1 }, // fixed org, no session needed
		Audit:   func(*http.Request, string, string, uint, map[string]any) {},
		Encrypt: func(b []byte) (string, error) { return string(b), nil },
		Decrypt: func(s string) ([]byte, error) { return []byte(s), nil },
		Provide: reg.Provide, Lookup: reg.Lookup,
	})
	return p, mux, db
}

func do(srv http.Handler, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func TestVerifyDNS(t *testing.T) {
	p, srv, db := newVerifyHarness(t)
	const orgID = uint(1)

	dom := models.Domain{
		OrgID: orgID,
		Name:  "mytestdomain.com",
	}
	if err := db.Create(&dom).Error; err != nil {
		t.Fatalf("failed to create domain: %v", err)
	}

	// 1. Stub healthy records
	p.lookupTXT = func(name string) ([]string, error) {
		switch name {
		case "mytestdomain.com":
			return []string{"v=spf1 include:_spf.google.com ~all"}, nil
		case "_dmarc.mytestdomain.com":
			return []string{"v=DMARC1; p=reject; pct=100"}, nil
		case "default._domainkey.mytestdomain.com":
			return []string{"v=DKIM1; k=rsa; p=MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A"}, nil
		default:
			return nil, fmt.Errorf("dns lookup failed")
		}
	}

	rec := do(srv, "GET", fmt.Sprintf("/api/domains/%d/verify-dns", dom.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("verify-dns request failed: got status %d, body %s", rec.Code, rec.Body.String())
	}

	var res struct {
		Spf struct {
			Set     bool   `json:"set"`
			Healthy bool   `json:"healthy"`
			Value   string `json:"value"`
		} `json:"spf"`
		Dmarc struct {
			Set     bool   `json:"set"`
			Healthy bool   `json:"healthy"`
			Value   string `json:"value"`
		} `json:"dmarc"`
		Dkim struct {
			Set      bool   `json:"set"`
			Healthy  bool   `json:"healthy"`
			Value    string `json:"value"`
			Selector string `json:"selector"`
		} `json:"dkim"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !res.Spf.Set || !res.Spf.Healthy || res.Spf.Value != "v=spf1 include:_spf.google.com ~all" {
		t.Errorf("SPF healthy check failed: %+v", res.Spf)
	}
	if !res.Dmarc.Set || !res.Dmarc.Healthy || res.Dmarc.Value != "v=DMARC1; p=reject; pct=100" {
		t.Errorf("DMARC healthy check failed: %+v", res.Dmarc)
	}
	if !res.Dkim.Set || !res.Dkim.Healthy || res.Dkim.Selector != "default" || !strings.Contains(res.Dkim.Value, "v=DKIM1") {
		t.Errorf("DKIM healthy check failed: %+v", res.Dkim)
	}

	// 2. Stub unhealthy records (missing/unhealthy)
	p.lookupTXT = func(name string) ([]string, error) {
		switch name {
		case "mytestdomain.com":
			// spf without v=spf1 prefix
			return []string{"spf1 ~all"}, nil
		case "_dmarc.mytestdomain.com":
			// dmarc without p=
			return []string{"v=DMARC1; pct=100"}, nil
		default:
			return nil, fmt.Errorf("dns lookup failed")
		}
	}

	recUnhealthy := do(srv, "GET", fmt.Sprintf("/api/domains/%d/verify-dns", dom.ID))
	if recUnhealthy.Code != http.StatusOK {
		t.Fatalf("verify-dns unhealthy request failed: got status %d", recUnhealthy.Code)
	}

	var resUnhealthy struct {
		Spf struct {
			Set     bool `json:"set"`
			Healthy bool `json:"healthy"`
		} `json:"spf"`
		Dmarc struct {
			Set     bool `json:"set"`
			Healthy bool `json:"healthy"`
		} `json:"dmarc"`
		Dkim struct {
			Set     bool `json:"set"`
			Healthy bool `json:"healthy"`
		} `json:"dkim"`
	}

	if err := json.Unmarshal(recUnhealthy.Body.Bytes(), &resUnhealthy); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resUnhealthy.Spf.Set || resUnhealthy.Spf.Healthy {
		t.Errorf("SPF should not be set or healthy: %+v", resUnhealthy.Spf)
	}
	if !resUnhealthy.Dmarc.Set || resUnhealthy.Dmarc.Healthy {
		t.Errorf("DMARC should be set but not healthy: %+v", resUnhealthy.Dmarc)
	}
	if resUnhealthy.Dkim.Set || resUnhealthy.Dkim.Healthy {
		t.Errorf("DKIM should not be set or healthy: %+v", resUnhealthy.Dkim)
	}
}

func TestVerifyDNSMailHosts(t *testing.T) {
	p, srv, db := newVerifyHarness(t)
	const orgID = uint(1)

	dom := models.Domain{
		OrgID:   orgID,
		Name:    "mytestdomain.com",
		ForMail: true,
		MailHosts: models.HostList{
			{Host: "mytestdomain.com", Enabled: true},
			{Host: "mail.mytestdomain.com", Enabled: true},
			{Host: "old.mytestdomain.com", Enabled: false}, // disabled — must be skipped
		},
	}
	if err := db.Create(&dom).Error; err != nil {
		t.Fatalf("failed to create domain: %v", err)
	}

	// Apex is healthy; the subdomain has SPF only.
	p.lookupTXT = func(name string) ([]string, error) {
		switch name {
		case "mytestdomain.com":
			return []string{"v=spf1 include:_spf.google.com ~all"}, nil
		case "_dmarc.mytestdomain.com":
			return []string{"v=DMARC1; p=reject"}, nil
		case "default._domainkey.mytestdomain.com":
			return []string{"v=DKIM1; k=rsa; p=MIIBIjANBg"}, nil
		case "mail.mytestdomain.com":
			return []string{"v=spf1 -all"}, nil
		case "old.mytestdomain.com":
			t.Errorf("disabled host old.mytestdomain.com should not be probed")
			return nil, fmt.Errorf("should not happen")
		default:
			return nil, fmt.Errorf("dns lookup failed")
		}
	}

	rec := do(srv, "GET", fmt.Sprintf("/api/domains/%d/verify-dns", dom.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("verify-dns failed: status %d, body %s", rec.Code, rec.Body.String())
	}

	var res struct {
		Spf   struct{ Set, Healthy bool } `json:"spf"`
		Hosts []struct {
			Host  string                      `json:"host"`
			Spf   struct{ Set, Healthy bool } `json:"spf"`
			Dmarc struct{ Set, Healthy bool } `json:"dmarc"`
		} `json:"hosts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(res.Hosts) != 2 {
		t.Fatalf("expected 2 enabled mail hosts, got %d: %+v", len(res.Hosts), res.Hosts)
	}
	// Top-level mirrors the apex.
	if !res.Spf.Set || !res.Spf.Healthy {
		t.Errorf("top-level SPF should mirror healthy apex: %+v", res.Spf)
	}

	byHost := map[string]struct{ SPFHealthy, DMARCSet bool }{}
	for _, hh := range res.Hosts {
		byHost[hh.Host] = struct{ SPFHealthy, DMARCSet bool }{hh.Spf.Healthy, hh.Dmarc.Set}
	}
	if apex := byHost["mytestdomain.com"]; !apex.SPFHealthy || !apex.DMARCSet {
		t.Errorf("apex host wrong: %+v", apex)
	}
	if sub := byHost["mail.mytestdomain.com"]; !sub.SPFHealthy || sub.DMARCSet {
		t.Errorf("subdomain host should have SPF but no DMARC: %+v", sub)
	}
}

func TestVerifyDNSLinkHosts(t *testing.T) {
	p, srv, db := newVerifyHarness(t)
	const orgID = uint(1)

	dom := models.Domain{
		OrgID:   orgID,
		Name:    "mytestdomain.com",
		ForLink: true,
		LinkHosts: models.HostList{
			{Host: "go.mytestdomain.com", Enabled: true},   // CNAME into zone → healthy
			{Host: "s.mytestdomain.com", Enabled: true},    // proxied/A-only → set, unverified
			{Host: "dead.mytestdomain.com", Enabled: true}, // NXDOMAIN → not set
			{Host: "off.mytestdomain.com", Enabled: false}, // disabled → skipped
		},
	}
	if err := db.Create(&dom).Error; err != nil {
		t.Fatalf("failed to create domain: %v", err)
	}

	p.lookupTXT = func(string) ([]string, error) { return nil, fmt.Errorf("no txt") }
	p.lookupCNAME = func(name string) (string, error) {
		switch name {
		case "go.mytestdomain.com":
			return "mytestdomain.com.", nil
		case "s.mytestdomain.com":
			return "s.mytestdomain.com.", nil // flattened/A-only: resolves to self
		case "off.mytestdomain.com":
			t.Errorf("disabled link host should not be probed")
			return "", fmt.Errorf("should not happen")
		default:
			return "", fmt.Errorf("no such host")
		}
	}

	rec := do(srv, "GET", fmt.Sprintf("/api/domains/%d/verify-dns", dom.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("verify-dns failed: status %d, body %s", rec.Code, rec.Body.String())
	}

	var res struct {
		Links []struct {
			Host    string `json:"host"`
			Set     bool   `json:"set"`
			Healthy bool   `json:"healthy"`
			CNAME   string `json:"cname"`
		} `json:"links"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(res.Links) != 3 {
		t.Fatalf("expected 3 enabled link hosts, got %d: %+v", len(res.Links), res.Links)
	}
	byHost := map[string]struct {
		Set, Healthy bool
		CNAME        string
	}{}
	for _, l := range res.Links {
		byHost[l.Host] = struct {
			Set, Healthy bool
			CNAME        string
		}{l.Set, l.Healthy, l.CNAME}
	}
	if g := byHost["go.mytestdomain.com"]; !g.Set || !g.Healthy || g.CNAME != "mytestdomain.com" {
		t.Errorf("go host should be healthy CNAME into zone: %+v", g)
	}
	if s := byHost["s.mytestdomain.com"]; !s.Set || s.Healthy {
		t.Errorf("s host should resolve but be unverified: %+v", s)
	}
	if d := byHost["dead.mytestdomain.com"]; d.Set || d.Healthy {
		t.Errorf("dead host should not resolve: %+v", d)
	}
}
