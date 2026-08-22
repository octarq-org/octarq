package origin

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
)

func TestSecureExtra(t *testing.T) {
	// nil request
	if Secure(nil, true) {
		t.Error("Secure(nil) should be false")
	}

	// TLS request
	tlsReq := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
	tlsReq.TLS = &tls.ConnectionState{}
	if !Secure(tlsReq, false) {
		t.Error("TLS request should be Secure even if trustProxy=false")
	}

	// Plain HTTP without trustProxy
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	if Secure(req, false) {
		t.Error("X-Forwarded-Proto should be ignored when trustProxy=false")
	}

	// Plain HTTP with trustProxy and comma-separated header
	req.Header.Set("X-Forwarded-Proto", "https, http")
	if !Secure(req, true) {
		t.Error("X-Forwarded-Proto 'https, http' should be Secure when trustProxy=true")
	}

	// Plain HTTP with trustProxy and http scheme
	req.Header.Set("X-Forwarded-Proto", "http")
	if Secure(req, true) {
		t.Error("X-Forwarded-Proto 'http' should not be Secure")
	}
}

func TestCandidatesEdgeCases(t *testing.T) {
	if c := Candidates(""); c != nil {
		t.Errorf("Candidates(\"\") expected nil, got %v", c)
	}
	if c := Candidates("[::1]"); c != nil {
		t.Errorf("Candidates(\"[::1]\") expected nil, got %v", c)
	}
	if c := Candidates("127.0.0.1"); c != nil {
		t.Errorf("Candidates(\"127.0.0.1\") expected nil, got %v", c)
	}
	if c := Candidates("localhost"); c != nil {
		t.Errorf("Candidates(\"localhost\") expected nil, got %v", c)
	}

	c := Candidates("a.b.c.example.com")
	wantLen := 4 // a.b.c.example.com, b.c.example.com, c.example.com, example.com
	if len(c) != wantLen {
		t.Errorf("Candidates(\"a.b.c.example.com\") length = %d, want %d", len(c), wantLen)
	}
}

func TestOwnerOfAmbiguityRejection(t *testing.T) {
	// Two orgs claim the exact same hostname. OwnerOf MUST reject rather than guess.
	db := testDB(t,
		testDomain{
			OrgID:   1,
			Name:    "ambiguous.example.com",
			ForLink: true,
		},
		testDomain{
			OrgID:   2,
			Name:    "ambiguous.example.com",
			ForLink: true,
		},
	)

	rv := NewResolver(db)
	orgID, ok := rv.OwnerOf("ambiguous.example.com")
	if ok || orgID != 0 {
		t.Fatalf("OwnerOf on ambiguous host must reject, got orgID=%d ok=%v", orgID, ok)
	}

	// Also test direct package-level OwnerOf
	orgID, ok = OwnerOf(db, "ambiguous.example.com")
	if ok || orgID != 0 {
		t.Fatalf("direct OwnerOf on ambiguous host must reject, got orgID=%d ok=%v", orgID, ok)
	}
}

func TestSharedHost(t *testing.T) {
	t.Setenv("OCTARQ_SHARED_HOSTS", "shared.example.com, portal.example.org:8080")

	// Nil request
	if _, ok := SharedHost(nil); ok {
		t.Error("SharedHost(nil) should be false")
	}

	// Empty host
	rEmpty := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	rEmpty.Host = ""
	if _, ok := SharedHost(rEmpty); ok {
		t.Error("SharedHost with empty host should be false")
	}

	// Matching shared host
	r1 := httptest.NewRequest(http.MethodGet, "http://shared.example.com/", nil)
	if h, ok := SharedHost(r1); !ok || h != "shared.example.com" {
		t.Errorf("SharedHost(shared.example.com) = %q, %v", h, ok)
	}

	// Matching shared host with port
	r2 := httptest.NewRequest(http.MethodGet, "http://portal.example.org:8080/", nil)
	if h, ok := SharedHost(r2); !ok || h != "portal.example.org" {
		t.Errorf("SharedHost(portal.example.org:8080) = %q, %v", h, ok)
	}

	// Non-matching host
	r3 := httptest.NewRequest(http.MethodGet, "http://other.example.com/", nil)
	if _, ok := SharedHost(r3); ok {
		t.Error("SharedHost(other.example.com) should be false")
	}
}

func TestClearDomainCache(t *testing.T) {
	db := testDB(t, testDomain{
		OrgID:   1,
		Name:    "cached.example.com",
		ForLink: true,
	})

	rv := NewResolver(db)
	if !rv.ServesTraffic("cached.example.com") {
		t.Fatal("expected ServesTraffic to be true")
	}

	// ClearDomainCache empty host does nothing
	ClearDomainCache("")

	// ClearDomainCache for specific domain removes cache
	ClearDomainCache("cached.example.com")
	ClearDomainCache("sub.cached.example.com")

	// Query again through resolver
	if !rv.ServesTraffic("cached.example.com") {
		t.Fatal("expected ServesTraffic to still work after cache clear and re-query")
	}
}

func TestClearBaseDomainCacheAndResolver(t *testing.T) {
	db := testDB(t)
	// Migrate Setting table for BaseDomain
	if err := db.AutoMigrate(&models.Setting{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	s := models.Setting{Key: models.BaseDomainSetting, Value: "tenants.example.com"}
	if err := db.Create(&s).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}

	rv := NewResolver(db)
	base := cachedBaseDomain(db)
	if base != "tenants.example.com" {
		t.Fatalf("expected base domain tenants.example.com, got %q", base)
	}

	// Update setting and clear cache
	db.Model(&s).Update("value", "new-tenants.example.com")
	ClearBaseDomainCache(db)

	base2 := cachedBaseDomain(db)
	if base2 != "new-tenants.example.com" {
		t.Fatalf("expected updated base domain new-tenants.example.com, got %q", base2)
	}

	// Test Resolver methods with nil or empty
	if _, ok := rv.OwnedHost(1, nil); ok {
		t.Error("OwnedHost(nil request) should be false")
	}
	rEmpty := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	rEmpty.Host = ""
	if _, ok := rv.OwnedHost(1, rEmpty); ok {
		t.Error("OwnedHost(empty host) should be false")
	}
	if _, ok := rv.OwnerOf(""); ok {
		t.Error("OwnerOf(\"\") should be false")
	}
	if rv.ServesTraffic("") {
		t.Error("ServesTraffic(\"\") should be false")
	}
	if _, ok := rv.SharedHost(nil); ok {
		t.Error("SharedHost(nil) should be false")
	}
	if _, ok := rv.SharedHost(rEmpty); ok {
		t.Error("SharedHost(empty host) should be false")
	}
	if rv.Absolute(1, nil, false) != "" {
		t.Error("Absolute(nil request) should be empty")
	}
}

func TestNamespaceNilDB(t *testing.T) {
	ns := namespace(nil)
	if ns != "nodb|" {
		t.Errorf("namespace(nil) = %q, want nodb|", ns)
	}
}

func TestClearSharedHostsCacheAndResolver(t *testing.T) {
	db := testDB(t)
	if err := db.AutoMigrate(&models.Setting{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	s := models.Setting{Key: models.SharedHostsSetting, Value: "shared1.example.com"}
	if err := db.Create(&s).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}

	rv := NewResolver(db)
	r1 := httptest.NewRequest(http.MethodGet, "http://shared1.example.com/", nil)
	if h, ok := rv.SharedHost(r1); !ok || h != "shared1.example.com" {
		t.Fatalf("expected SharedHost to match shared1.example.com, got %q %v", h, ok)
	}
	if !HasSharedHosts(db) {
		t.Fatal("expected HasSharedHosts(db) to be true")
	}

	// Update setting in DB and clear cache
	db.Model(&s).Update("value", "shared2.example.com")
	ClearSharedHostsCache(db)

	if h, ok := rv.SharedHost(r1); ok {
		t.Fatalf("expected old shared host to fail, got %q", h)
	}
	r2 := httptest.NewRequest(http.MethodGet, "http://shared2.example.com/", nil)
	if h, ok := rv.SharedHost(r2); !ok || h != "shared2.example.com" {
		t.Fatalf("expected new shared host to match shared2.example.com, got %q %v", h, ok)
	}
}
