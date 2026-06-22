package shortlink

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/jungley/led/internal/geo"
	"github.com/jungley/led/internal/models"
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
	past := time.Now().Add(-time.Hour)
	// Create then update: GORM substitutes the column default:true for a
	// zero-value bool at insert time, so force Enabled=false explicitly.
	off := models.Link{Slug: "off", Target: "https://x", Enabled: true}
	s.db.Create(&off)
	s.db.Model(&off).Update("enabled", false)
	s.db.Create(&models.Link{Slug: "old", Target: "https://x", Enabled: true, ExpiresAt: &past})

	if _, ok := s.Lookup("h", "off"); ok {
		t.Error("expected disabled link to be unavailable")
	}
	if _, ok := s.Lookup("h", "old"); ok {
		t.Error("expected expired link to be unavailable")
	}
	if _, ok := s.Lookup("h", "missing"); ok {
		t.Error("expected missing slug to be unavailable")
	}
}
