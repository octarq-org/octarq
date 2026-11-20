package origin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCharacterization_ResolverContracts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:origin_char_test?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Discard,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	rv := NewResolver(db)
	if rv == nil {
		t.Fatal("NewResolver returned nil")
	}

	if rv.AnyRegistered() {
		t.Fatal("AnyRegistered should be false on empty database")
	}

	req := httptest.NewRequest(http.MethodGet, "http://unregistered.example.com/test", nil)
	req.Host = "unregistered.example.com"
	if _, ok := rv.OwnedHost(1, req); ok {
		t.Fatal("unregistered host must not be owned")
	}

	abs := rv.Absolute(1, req, false)
	if abs != "" {
		t.Fatalf("expected empty origin for untrusted host, got %q", abs)
	}
}

func TestCharacterization_FailClosedSecurity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:origin_security_test?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Discard,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	rv := NewResolver(db)

	hostileHosts := []string{
		"evil.com",
		"attacker.org",
		"localhost:evil.com",
		"127.0.0.1.attacker.com",
		"sub.localhost.attacker.com",
		"attacker.com#localhost",
		"attacker.com?localhost",
		"attacker.com/localhost",
		"attacker.com@localhost",
		"<script>",
		"bad host name with spaces",
	}

	for _, host := range hostileHosts {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = host

		if _, ok := rv.OwnedHost(0, req); ok {
			t.Errorf("hostile host %q must not be OwnedHost", host)
		}

		if org, ok := rv.OwnerOf(host); ok || org != 0 {
			t.Errorf("hostile host %q must not have an owner, got org=%d ok=%v", host, org, ok)
		}

		if abs := rv.Absolute(0, req, false); abs != "" {
			t.Errorf("hostile host %q must return empty absolute origin, got %q", host, abs)
		}
	}
}

func TestCharacterization_LoopbackDevelopmentPermitted(t *testing.T) {
	clearAllCache()
	db, err := gorm.Open(sqlite.Open("file:origin_loopback_test?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Discard,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	rv := NewResolver(db)

	loopbacks := []string{
		"localhost",
		"localhost:8080",
		"127.0.0.1",
		"127.0.0.1:3000",
		"[::1]",
		"[::1]:8443",
		"tenant.localhost",
		"tenant.localhost:5173",
	}

	for _, lb := range loopbacks {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = lb

		absHTTP := rv.Absolute(0, req, false)
		if absHTTP != "http://"+lb {
			t.Errorf("expected http://%s, got %q", lb, absHTTP)
		}

		absHTTPS := rv.Absolute(0, req, true)
		if absHTTPS != "https://"+lb {
			t.Errorf("expected https://%s, got %q", lb, absHTTPS)
		}
	}
}

func TestCharacterization_RegisteredDomainLocksOutLoopback(t *testing.T) {
	clearAllCache()
	db, err := gorm.Open(sqlite.Open("file:origin_lockout_test?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Discard,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domainRow{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&domainRow{
		OrgID: 1,
		Name:  "prod.example.com",
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	rv := NewResolver(db)

	reqProd := httptest.NewRequest(http.MethodGet, "/", nil)
	reqProd.Host = "prod.example.com"
	if abs := rv.Absolute(1, reqProd, true); abs != "https://prod.example.com" {
		t.Fatalf("expected https://prod.example.com, got %q", abs)
	}

	reqLB := httptest.NewRequest(http.MethodGet, "/", nil)
	reqLB.Host = "localhost:8080"
	if abs := rv.Absolute(0, reqLB, false); abs != "" {
		t.Fatalf("expected loopback to be rejected once domains exist, got %q", abs)
	}
}

func TestCharacterization_OwnerResolutionSpecificity(t *testing.T) {
	clearAllCache()
	db, err := gorm.Open(sqlite.Open("file:origin_spec_test?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Discard,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domainRow{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	_ = db.Create(&domainRow{OrgID: 1, Name: "example.com"})
	_ = db.Create(&domainRow{OrgID: 2, Name: "sub.example.com"})

	rv := NewResolver(db)

	if org, ok := rv.OwnerOf("sub.example.com"); !ok || org != 2 {
		t.Fatalf("expected org 2 for sub.example.com, got org=%d ok=%v", org, ok)
	}
	if org, ok := rv.OwnerOf("other.example.com"); !ok || org != 1 {
		t.Fatalf("expected org 1 for other.example.com, got org=%d ok=%v", org, ok)
	}
	if org, ok := rv.OwnerOf("example.com"); !ok || org != 1 {
		t.Fatalf("expected org 1 for example.com, got org=%d ok=%v", org, ok)
	}
}
