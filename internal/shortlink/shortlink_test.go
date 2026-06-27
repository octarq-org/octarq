package shortlink

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/Jungley8/led/internal/geo"
	"github.com/Jungley8/led/internal/models"
	"gorm.io/gorm"
)

func TestStripPort(t *testing.T) {
	cases := map[string]string{
		"go.example.com:8080": "go.example.com",
		"go.example.com":      "go.example.com",
		"127.0.0.1:80":        "127.0.0.1",
	}
	for in, want := range cases {
		if got := stripPort(in); got != want {
			t.Errorf("stripPort(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAnonymizeIP(t *testing.T) {
	cases := []struct{ in, want string }{
		{"1.2.3.4", "1.2.3.0"},
		{"192.168.100.255", "192.168.100.0"},
		{"2001:db8:85a3::8a2e:370:7334", "2001:db8:85a3::"},
		{"not-an-ip", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := anonymizeIP(c.in); got != c.want {
			t.Errorf("anonymizeIP(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestClientIPPrecedence(t *testing.T) {
	// X-Forwarded-For wins, taking the first hop.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.9:1234"
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	r.Header.Set("X-Real-IP", "9.9.9.9")
	if got := clientIP(r); got != "1.2.3.4" {
		t.Errorf("XFF precedence: got %q want 1.2.3.4", got)
	}

	// X-Real-IP next.
	r = httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.9:1234"
	r.Header.Set("X-Real-IP", "9.9.9.9")
	if got := clientIP(r); got != "9.9.9.9" {
		t.Errorf("X-Real-IP precedence: got %q want 9.9.9.9", got)
	}

	// RemoteAddr fallback, port stripped.
	r = httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.9:1234"
	if got := clientIP(r); got != "10.0.0.9" {
		t.Errorf("RemoteAddr fallback: got %q want 10.0.0.9", got)
	}
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Clear any rows left by a prior test sharing the in-memory cache.
	db.Where("1 = 1").Delete(&models.Link{})
	g, _ := geo.Open("")
	return &Service{db: db, geo: g}
}

func TestLookupHostPreference(t *testing.T) {
	s := newTestService(t)
	// A host-agnostic link and a host-specific link share the same slug.
	s.db.Create(&models.Link{Slug: "x", Host: "", Target: "https://any", Enabled: true})
	s.db.Create(&models.Link{Slug: "x", Host: "go.example.com", Target: "https://exact", Enabled: true})

	link, ok := s.Lookup("go.example.com:8080", "x")
	if !ok {
		t.Fatal("expected to find link")
	}
	if link.Target != "https://exact" {
		t.Errorf("host preference: got %q want https://exact", link.Target)
	}

	// A different host falls back to the host-agnostic link.
	link, ok = s.Lookup("other.example.com", "x")
	if !ok || link.Target != "https://any" {
		t.Errorf("fallback: ok=%v target=%q want https://any", ok, link.Target)
	}
}

func TestLookupDisabledAndExpired(t *testing.T) {
	s := newTestService(t)
	// Create then update: GORM substitutes the column default:true for a
	// zero-value bool at insert time, so force Enabled=false explicitly.
	off := models.Link{Slug: "off", Target: "https://x", Enabled: true}
	s.db.Create(&off)
	s.db.Model(&off).Update("enabled", false)

	if _, ok := s.Lookup("h", "off"); ok {
		t.Error("expected disabled link to be unavailable")
	}
	if _, ok := s.Lookup("h", "missing"); ok {
		t.Error("expected missing slug to be unavailable")
	}
}

// Expiry/click-limit are enforced in Handle (so ExpiredURL can be honored),
// not in Lookup.
func TestHandleExpiryAndClickLimit(t *testing.T) {
	s := newTestService(t)
	past := time.Now().Add(-time.Hour)

	// Expired with no ExpiredURL -> 404.
	old := &models.Link{Slug: "old", Target: "https://x", Enabled: true, ExpiresAt: &past}
	s.db.Create(old)
	l, ok := s.Lookup("h", "old")
	if !ok {
		t.Fatal("expired link should still be found by Lookup")
	}
	rec := httptest.NewRecorder()
	s.Handle(rec, httptest.NewRequest("GET", "/old", nil), l)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expired link without ExpiredURL: got %d, want 404", rec.Code)
	}

	// Expired with ExpiredURL -> 302 to that URL.
	s.db.Create(&models.Link{Slug: "exp", Target: "https://x", Enabled: true, ExpiresAt: &past, ExpiredURL: "https://fallback.example"})
	l2, _ := s.Lookup("h", "exp")
	rec2 := httptest.NewRecorder()
	s.Handle(rec2, httptest.NewRequest("GET", "/exp", nil), l2)
	if rec2.Code != http.StatusFound || rec2.Header().Get("Location") != "https://fallback.example" {
		t.Errorf("expired link with ExpiredURL: got %d -> %q", rec2.Code, rec2.Header().Get("Location"))
	}

	// Over click limit -> 404.
	s.db.Create(&models.Link{Slug: "cap", Target: "https://x", Enabled: true, ClickLimit: 5, Clicks: 5})
	l3, _ := s.Lookup("h", "cap")
	rec3 := httptest.NewRecorder()
	s.Handle(rec3, httptest.NewRequest("GET", "/cap", nil), l3)
	if rec3.Code != http.StatusNotFound {
		t.Errorf("over-limit link: got %d, want 404", rec3.Code)
	}
}

func TestLookupLinkHostDisabled(t *testing.T) {
	s := newTestService(t)

	d := models.Domain{
		Name:    "example.com",
		ForLink: true,
		LinkHosts: models.HostList{
			{Host: "disabled.example.com", Enabled: false},
			{Host: "enabled.example.com", Enabled: true},
		},
	}
	s.db.Create(&d)

	if !s.linkHostDisabled("disabled.example.com") {
		t.Error("expected disabled.example.com to be reported as disabled")
	}

	if s.linkHostDisabled("enabled.example.com") {
		t.Error("expected enabled.example.com to NOT be reported as disabled")
	}

	if s.linkHostDisabled("nonexistent.example.com") {
		t.Error("expected nonexistent.example.com to NOT be reported as disabled")
	}
}

func TestHandlePasswordGate(t *testing.T) {
	s := newTestService(t)

	link := &models.Link{
		Slug:     "pwlink",
		Target:   "https://target",
		Enabled:  true,
		Password: "secretpassword",
	}
	s.db.Create(link)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/pwlink", nil)
	s.Handle(rec, req, link)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Protected link") {
		t.Error("expected response to contain password gate html")
	}

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "/pwlink?pw=wrong", nil)
	s.Handle(rec2, req2, link)
	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", rec2.Code)
	}

	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest("GET", "/pwlink?pw=secretpassword", nil)
	s.Handle(rec3, req3, link)
	if rec3.Code != http.StatusFound || rec3.Header().Get("Location") != "https://target" {
		t.Errorf("expected redirect to https://target, got status %d Location %q", rec3.Code, rec3.Header().Get("Location"))
	}
}

func TestHandleRoutingRules(t *testing.T) {
	s := newTestService(t)

	link := &models.Link{
		Slug:    "route",
		Target:  "https://default",
		Enabled: true,
		RoutingRules: []models.RoutingRule{
			{
				Type:   "geo",
				Match:  "CN",
				Target: "https://china",
			},
			{
				Type:   "device",
				Match:  "mobile",
				Target: "https://mobile",
			},
			{
				Type:   "os",
				Match:  "android",
				Target: "https://android",
			},
			{
				Type:   "language",
				Match:  "zh-cn",
				Target: "https://chinese",
			},
		},
	}
	s.db.Create(link)

	if !matchRule(link.RoutingRules[0], "CN", "desktop", "linux", "") {
		t.Error("expected geo CN to match CN")
	}
	if !matchRule(link.RoutingRules[1], "US", "mobile", "ios", "") {
		t.Error("expected device mobile to match mobile")
	}
	if !matchRule(link.RoutingRules[2], "US", "desktop", "android", "") {
		t.Error("expected os android to match android")
	}
	if !matchRule(link.RoutingRules[3], "US", "desktop", "linux", "zh-CN,zh;q=0.9") {
		t.Error("expected language zh-cn to match Accept-Language headers containing zh-cn")
	}
}

func TestIsBot(t *testing.T) {
	botUAs := []string{
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
		"facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_codedec.html)",
		"WhatsApp/2.19.244 A",
		"Slackbot-LinkExpanding 1.0 (+https://api.slack.com/robots)",
	}
	for _, ua := range botUAs {
		if !isBot(ua) {
			t.Errorf("expected %q to be recognized as bot", ua)
		}
	}

	humanUAs := []string{
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 16_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.5 Mobile/15E148 Safari/604.1",
	}
	for _, ua := range humanUAs {
		if isBot(ua) {
			t.Errorf("expected %q to NOT be recognized as bot", ua)
		}
	}
}
