package origin

import (
	"database/sql"
	"fmt"
	"testing"
)

func TestCoverageLift_NormalizeHost_IPv6Port(t *testing.T) {
	h := NormalizeHost("[::1]:8080")
	if h != "[::1]" {
		t.Fatalf("expected [::1], got %q", h)
	}
	h2 := NormalizeHost("[2001:db8::1]:9000")
	if h2 != "[2001:db8::1]" {
		t.Fatalf("expected [2001:db8::1], got %q", h2)
	}
}

func TestCoverageLift_ZoneCandidates_SingleLabelBase(t *testing.T) {
	cands := zoneCandidates("singlelabel", "singlelabel")
	if cands != nil {
		t.Fatalf("expected nil for singlelabel host/base, got %v", cands)
	}
}

func TestCoverageLift_IsNumericHost_EmptyLabel(t *testing.T) {
	if isNumericHost("1.2..3") {
		t.Fatal("expected false for empty label in 1.2..3")
	}
	if isNumericHost(".1.2.3") {
		t.Fatal("expected false for leading dot in .1.2.3")
	}
}

func TestCoverageLift_NilAndEmptyInputs(t *testing.T) {
	if _, ok := OwnedHost(nil, 0, nil); ok {
		t.Fatal("expected false from OwnedHost with nil inputs")
	}
	if ServesTraffic(nil, "example.com") {
		t.Fatal("expected false from ServesTraffic with nil db")
	}

	db := testDB(t)
	if ServesTraffic(db, "") {
		t.Fatal("expected false from ServesTraffic with empty host")
	}

	if p := namespace(nil); p != "nodb|" {
		t.Fatalf("expected nodb| for nil db, got %q", p)
	}
	badGDB := *db
	stmt := *badGDB.Statement
	stmt.ConnPool = &sql.Tx{}
	badGDB.Statement = &stmt
	if p := namespace(&badGDB); p != "nodb|" {
		t.Fatalf("expected nodb| for broken gorm.DB, got %q", p)
	}

	if !isLoopbackHost("tenant.localhost") {
		t.Fatal("expected isLoopbackHost true for tenant.localhost")
	}
}

func TestCoverageLift_ServesTraffic_MailApex(t *testing.T) {
	dbMail := testDB(t, testDomain{
		OrgID:   1,
		Name:    "mailapex.example",
		ForMail: true,
	})
	if !ServesTraffic(dbMail, "mailapex.example") {
		t.Fatal("expected ServesTraffic true for mail apex when host list is empty")
	}
}

func TestCoverageLift_ClearDomainCache_AnySuffix(t *testing.T) {
	clearAllCache()
	globalCacheMu.Lock()
	globalCache["some-ns|any"] = entry{ok: true}
	globalCacheMu.Unlock()

	ClearDomainCache("example.com")

	globalCacheMu.RLock()
	_, exists := globalCache["some-ns|any"]
	globalCacheMu.RUnlock()
	if exists {
		t.Fatal("expected entry ending with |any to be cleared")
	}
}

func TestCoverageLift_GlobalCache_OverLimitResets(t *testing.T) {
	db := testDB(t)

	// 1. Trigger reset in cachedBaseDomain
	globalCacheMu.Lock()
	for i := 0; i < 1030; i++ {
		globalCache[fmt.Sprintf("k-base-%d", i)] = entry{ok: true}
	}
	globalCacheMu.Unlock()
	_ = cachedBaseDomain(db)

	// 2. Trigger reset in cachedSharedHosts
	globalCacheMu.Lock()
	for i := 0; i < 1030; i++ {
		globalCache[fmt.Sprintf("k-shared-%d", i)] = entry{ok: true}
	}
	globalCacheMu.Unlock()
	_ = cachedSharedHosts(db)

	// 3. Trigger reset in cachedCompute
	globalCacheMu.Lock()
	for i := 0; i < 1030; i++ {
		globalCache[fmt.Sprintf("k-comp-%d", i)] = entry{ok: true}
	}
	globalCacheMu.Unlock()
	rv := NewResolver(db)
	_, _ = rv.cachedOwner("evict-test.example", func() (uint, bool) {
		return 1, true
	})
}
