package origin

import (
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
)

func TestSecure(t *testing.T) {
	if Secure(nil, true) {
		t.Error("a nil request is not TLS")
	}
	r := req("acme.example")
	r.Header.Set("X-Forwarded-Proto", "HTTPS")
	if Secure(r, false) {
		t.Error("X-Forwarded-Proto must be ignored without TrustProxy")
	}
	if !Secure(r, true) {
		t.Error("X-Forwarded-Proto is case-insensitive and must be honoured with TrustProxy")
	}
}

// TestResolverOwnerOfCaches pins that the storefront/portal question — the one
// asked on every unauthenticated request — is answered from the cache, both when
// the host is owned and when it is not. An uncached negative answer is a free
// full-table scan for anyone spraying hostnames.
func TestResolverOwnerOfCaches(t *testing.T) {
	clearAllCache()
	db := testDB(t, acme())
	var queries int
	if err := db.Callback().Query().After("gorm:query").Register("test:count", func(tx *gorm.DB) {
		queries++
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}

	rv := NewResolver(db)
	if org, ok := rv.OwnerOf("go.partner.example"); !ok || org != 1 {
		t.Errorf("Resolver.OwnerOf(go.partner.example) = (%d, %v), want (1, true)", org, ok)
	}
	if org, ok := rv.OwnerOf("stranger.example"); ok || org != 0 {
		t.Errorf("Resolver.OwnerOf(stranger.example) = (%d, %v), want (0, false)", org, ok)
	}

	afterFirstPass := queries
	if afterFirstPass == 0 {
		t.Fatal("no queries were counted; the assertion below would be vacuous")
	}
	for i := 0; i < 5; i++ {
		if org, ok := rv.OwnerOf("go.partner.example"); !ok || org != 1 {
			t.Fatalf("cached OwnerOf returned (%d, %v); the cache must preserve the org", org, ok)
		}
		rv.OwnerOf("stranger.example")
		// Port and case must land on the same cache key, not a fresh miss.
		rv.OwnerOf("GO.Partner.Example:8443")
	}
	if queries != afterFirstPass {
		t.Errorf("repeated lookups issued %d further queries; answers must come from the cache", queries-afterFirstPass)
	}
}

// TestResolverCaches pins that the cached forms answer the same as the direct
// ones, and that a repeated question does not re-read the table — the endpoints
// behind these are unauthenticated, so an attacker spraying Host headers would
// otherwise be a free DB-query amplifier.
func TestResolverCaches(t *testing.T) {
	db := testDB(t, acme())
	var queries int
	if err := db.Callback().Query().After("gorm:query").Register("test:count", func(tx *gorm.DB) {
		queries++
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}

	rv := NewResolver(db)
	if !rv.ServesTraffic("go.acme.example") {
		t.Error("Resolver.ServesTraffic disagrees with ServesTraffic")
	}
	if rv.ServesTraffic("app.acme.example") {
		t.Error("Resolver.ServesTraffic reported a non-serving host")
	}
	if _, ok := rv.OwnedHost(0, req("app.acme.example")); !ok {
		t.Error("Resolver.OwnedHost disagrees with OwnedHost")
	}
	if !rv.AnyRegistered() {
		t.Error("Resolver.AnyRegistered disagrees with AnyRegistered")
	}

	afterFirstPass := queries
	if afterFirstPass == 0 {
		t.Fatal("no queries were counted; the assertion below would be vacuous")
	}
	for i := 0; i < 5; i++ {
		rv.ServesTraffic("go.acme.example")
		rv.OwnedHost(0, req("app.acme.example"))
		rv.AnyRegistered()
	}
	if queries != afterFirstPass {
		t.Errorf("repeated lookups issued %d further queries; answers must come from the cache", queries-afterFirstPass)
	}
}

// seedSetting ensures the settings table exists and inserts the given key/value
// row, the way the instance-settings API would. Needed by every reserved-zone
// test: models.BaseDomain reads the base domain from here.
func seedSetting(t *testing.T, db *gorm.DB, key, value string) {
	t.Helper()
	if err := db.AutoMigrate(&models.Setting{}); err != nil {
		t.Fatalf("migrate settings: %v", err)
	}
	if err := db.Create(&models.Setting{Key: key, Value: value}).Error; err != nil {
		t.Fatalf("seed setting %q: %v", key, err)
	}
}

// TestReservedZoneHostListsNeverMatch pins the other half of the reserved-zone
// guard: inside the base zone, another org's link/mail host list must not be
// able to claim a label.
func TestReservedZoneHostListsNeverMatch(t *testing.T) {
	db := testDB(t,
		testDomain{OrgID: 7, Name: "evil.com", LinkHosts: models.HostList{{Host: "victim7x.app.octarq.org", Enabled: true}}},
		testDomain{OrgID: 8, Name: "spam.com", MailHosts: models.HostList{{Host: "victim8y.app.octarq.org", Enabled: true}}},
		testDomain{OrgID: 9, Name: "quiet.com", LinkHosts: models.HostList{{Host: "off.app.octarq.org", Enabled: false}}},
		testDomain{OrgID: 1, Name: "app.octarq.org"},
	)
	seedSetting(t, db, models.BaseDomainSetting, "app.octarq.org")

	for _, host := range []string{"victim7x.app.octarq.org", "victim8y.app.octarq.org"} {
		if org, ok := OwnerOf(db, host); ok || org != 0 {
			t.Errorf("OwnerOf(%q) = (%d, %v), want (0, false)", host, org, ok)
		}
		if owned, ok := OwnedHost(db, 7, req(host)); ok {
			t.Errorf("OwnedHost(org 7, %q) claimed it via a host list — want unowned", owned)
		}
	}

	if host, ok := OwnedHost(db, 7, req("go.evil.com")); !ok || host != "go.evil.com" {
		t.Errorf("OwnedHost(org 7, go.evil.com) = (%q, %v), want owned via the link host list", host, ok)
	}
	if org, ok := OwnerOf(db, "go.evil.com"); !ok || org != 7 {
		t.Errorf("OwnerOf(go.evil.com) = (%d, %v), want (7, true)", org, ok)
	}
	if org, ok := OwnerOf(db, "off.app.octarq.org"); ok || org != 0 {
		t.Errorf("OwnerOf(off.app.octarq.org) = (%d, %v), want (0, false)", org, ok)
	}
}

// TestBaseDomainCacheInvalidation pins the base-domain cache: the reserved zone
// is read once per ttl per database, survives until the setting actually
// changes, and is invalidated explicitly when it does.
func TestBaseDomainCacheInvalidation(t *testing.T) {
	clearAllCache()
	db := testDB(t)
	seedSetting(t, db, models.BaseDomainSetting, "app.example.com")

	if got := cachedBaseDomain(db); got != "app.example.com" {
		t.Fatalf("cachedBaseDomain = %q, want app.example.com", got)
	}
	if err := db.Model(&models.Setting{}).Where("key = ?", models.BaseDomainSetting).Update("value", "other.example.net").Error; err != nil {
		t.Fatalf("update setting: %v", err)
	}
	if got := cachedBaseDomain(db); got != "app.example.com" {
		t.Fatalf("cachedBaseDomain read through the cache = %q, want the cached app.example.com", got)
	}
	ClearBaseDomainCache(db)
	if got := cachedBaseDomain(db); got != "other.example.net" {
		t.Fatalf("cachedBaseDomain after invalidation = %q, want other.example.net", got)
	}

	other := openDB(t, "other")
	seedSetting(t, other, models.BaseDomainSetting, "other.example.com")
	if got := cachedBaseDomain(other); got != "other.example.com" {
		t.Fatalf("other db cachedBaseDomain = %q, want other.example.com", got)
	}
	if got := cachedBaseDomain(db); got != "other.example.net" {
		t.Fatalf("db cachedBaseDomain after other-db ops = %q, want other.example.net", got)
	}
}

// TestReservedZoneStopsApexBubbling pins that unprovisioned subdomains do not bubble up to apex.
func TestReservedZoneStopsApexBubbling(t *testing.T) {
	db := testDB(t,
		testDomain{OrgID: 1, Name: "octarq.org"},
		testDomain{OrgID: 7, Name: "acme7x.app.octarq.org"},
	)
	seedSetting(t, db, models.BaseDomainSetting, "app.octarq.org")

	if org, ok := OwnerOf(db, "nobody.app.octarq.org"); ok || org != 0 {
		t.Errorf("OwnerOf(nobody.app.octarq.org) = (%d, %v), want (0, false)", org, ok)
	}
	if org, ok := OwnerOf(db, "acme7x.app.octarq.org"); !ok || org != 7 {
		t.Errorf("OwnerOf(acme7x.app.octarq.org) = (%d, %v), want (7, true)", org, ok)
	}
}

func TestReservedZoneDoesNotLeakThroughBaseRow(t *testing.T) {
	db := testDB(t,
		testDomain{OrgID: 1, Name: "octarq.org"},
		testDomain{OrgID: 1, Name: "app.octarq.org"},
	)
	seedSetting(t, db, models.BaseDomainSetting, "app.octarq.org")

	if org, ok := OwnerOf(db, "nobody.app.octarq.org"); ok || org != 0 {
		t.Errorf("OwnerOf(nobody.app.octarq.org) = (%d, %v), want (0, false)", org, ok)
	}
	if org, ok := OwnerOf(db, "app.octarq.org"); !ok || org != 1 {
		t.Errorf("OwnerOf(app.octarq.org) = (%d, %v), want (1, true)", org, ok)
	}
}

func TestOwnedHostReservedZoneIsolation(t *testing.T) {
	db := testDB(t,
		testDomain{OrgID: 1, Name: "app.octarq.org"},
		testDomain{OrgID: 7, Name: "acme7x.app.octarq.org"},
	)
	seedSetting(t, db, models.BaseDomainSetting, "app.octarq.org")

	if host, ok := OwnedHost(db, 1, req("acme7x.app.octarq.org")); ok {
		t.Errorf("vendor OwnedHost claimed %q, another tenant's subdomain", host)
	}
	if host, ok := OwnedHost(db, 7, req("acme7x.app.octarq.org")); !ok || host != "acme7x.app.octarq.org" {
		t.Errorf("tenant OwnedHost = (%q, %v), want its own subdomain", host, ok)
	}
}

func TestNoBaseDomainKeepsTodayBehaviour(t *testing.T) {
	db := testDB(t, acme())
	if org, ok := OwnerOf(db, "sub.app.acme.example"); !ok || org != 1 {
		t.Errorf("OwnerOf(sub.app.acme.example) = (%d, %v), want (1, true)", org, ok)
	}
	if org, ok := OwnerOf(db, "evil.example"); ok || org != 0 {
		t.Errorf("OwnerOf(evil.example) = (%d, %v), want (0, false)", org, ok)
	}
}

func TestAbsoluteAfterProvisioningUsesSharedHost(t *testing.T) {
	db := testDB(t, testDomain{OrgID: 7, Name: "acme7x.app.octarq.org"})
	seedSetting(t, db, models.BaseDomainSetting, "app.octarq.org")
	t.Setenv("OCTARQ_SHARED_HOSTS", "app.octarq.org")
	rv := NewResolver(db)

	if got := rv.Absolute(0, req("app.octarq.org"), true); got != "https://app.octarq.org" {
		t.Errorf("Absolute on the dashboard host = %q, want https://app.octarq.org", got)
	}
	if got := rv.Absolute(0, req("acme7x.app.octarq.org"), true); got != "https://acme7x.app.octarq.org" {
		t.Errorf("Absolute on a tenant subdomain = %q, want https://acme7x.app.octarq.org", got)
	}
}

func TestCacheIsNamespacedPerDatabase(t *testing.T) {
	clearAllCache()

	bare := NewResolver(openDB(t, "bare"))
	if got := bare.Absolute(0, req("evil.example"), false); got != "" {
		t.Fatalf("Absolute on a whitelist-free instance with untrusted host = %q, want \"\"", got)
	}
	if got := bare.Absolute(0, req("localhost:8080"), false); got != "http://localhost:8080" {
		t.Fatalf("Absolute on a whitelist-free instance with loopback host = %q, want \"http://localhost:8080\"", got)
	}
	if bare.AnyRegistered() {
		t.Fatal("AnyRegistered on a database with no domains table")
	}

	guardedDB := openDB(t, "guarded")
	if err := guardedDB.AutoMigrate(&testDomain{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	row := acme()
	if err := guardedDB.Create(&row).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	guarded := NewResolver(guardedDB)
	if !guarded.AnyRegistered() {
		t.Error("AnyRegistered read back the other database's answer")
	}
	if got := guarded.Absolute(0, req("evil.example"), false); got != "" {
		t.Errorf("Absolute honoured a forged Host as %q", got)
	}
	if _, ok := guarded.OwnerOf("evil.example"); ok {
		t.Error("OwnerOf claimed an owner for an unregistered host")
	}
}
