package shortlink

import (
	"net/http"
	"net/http/httptest"
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
