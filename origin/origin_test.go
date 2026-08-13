package origin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// testDomain mirrors the dns plugin's domains table. The plugin cannot be
// imported here (it depends on core, not the other way round), which is the same
// reason domainRow exists in the package under test.
type testDomain struct {
	ID        uint `gorm:"primaryKey"`
	OrgID     uint `gorm:"column:owner_id"`
	Name      string
	ForLink   bool
	ForMail   bool
	LinkHosts models.HostList
	MailHosts models.HostList
}

func (testDomain) TableName() string { return "domains" }

// openDB opens an empty in-memory database. Callers that want more than one
// within a single test pass distinct suffixes: the DSN is the database's
// identity under cache=shared, so reusing it would hand back the same storage.
// Unlike testDB this leaves the resolver cache alone.
func openDB(t *testing.T, suffix string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+suffix+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return db
}

// testDB returns a database seeded with rows. With no rows the domains table is
// not created at all, which is what an instance composed without the dns plugin
// looks like.
func testDB(t *testing.T, rows ...testDomain) *gorm.DB {
	t.Helper()
	clearAllCache()
	db := openDB(t, "")
	if len(rows) == 0 {
		return db
	}
	if err := db.AutoMigrate(&testDomain{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return db
}

func req(host string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/auth/forgot", nil)
	r.Host = host
	return r
}

// acme is registered by org 1; evil.example is registered by nobody.
func acme() testDomain {
	return testDomain{
		OrgID:   1,
		Name:    "acme.example",
		ForLink: true,
		ForMail: true,
		// The partner.example entries deliberately sit outside the apex, so
		// they exercise host-list matching on its own: a name under
		// acme.example would be owned by the apex rule regardless.
		LinkHosts: models.HostList{{Host: "go.acme.example", Enabled: true}, {Host: "go.partner.example", Enabled: true}, {Host: "off.partner.example", Enabled: false}},
		MailHosts: models.HostList{{Host: "mail.acme.example", Enabled: true}},
	}
}

func TestOwnedHost(t *testing.T) {
	cases := []struct {
		name  string
		host  string
		orgID uint
		want  bool
	}{
		{name: "registered name", host: "acme.example", want: true},
		{name: "subdomain of a registered name", host: "app.acme.example", want: true},
		{name: "registered link host", host: "go.acme.example", want: true},
		{name: "registered mail host", host: "mail.acme.example", want: true},
		{name: "case and port are normalised away", host: "APP.Acme.Example:8443", want: true},
		{name: "host-list entry outside the apex", host: "go.partner.example", want: true},
		{name: "disabled host-list entry", host: "off.partner.example", want: false},
		{name: "unrelated host", host: "evil.example", want: false},
		{name: "registered name as a prefix of the attacker's", host: "acme.example.evil.com", want: false},
		{name: "registered name as a suffix of the attacker's", host: "notacme.example", want: false},
		{name: "another org may not claim it", host: "acme.example", orgID: 2, want: false},
		{name: "the owning org may", host: "acme.example", orgID: 1, want: true},
		{name: "ip literal", host: "203.0.113.9", want: false},
		{name: "localhost", host: "localhost:8080", want: false},
		{name: "empty", host: "", want: false},
	}
	db := testDB(t, acme())
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host, ok := OwnedHost(db, tc.orgID, req(tc.host))
			if ok != tc.want {
				t.Fatalf("OwnedHost(org=%d, host=%q) = %v, want %v", tc.orgID, tc.host, ok, tc.want)
			}
			if ok && host != NormalizeHost(tc.host) {
				t.Errorf("returned host %q, want the normalised request host %q", host, NormalizeHost(tc.host))
			}
		})
	}
}

func TestOwnerOf(t *testing.T) {
	db := testDB(t,
		testDomain{
			OrgID:   1,
			Name:    "acme.example",
			ForLink: true,
			ForMail: true,
			LinkHosts: models.HostList{
				{Host: "go.partner.example", Enabled: true},
				{Host: "off.partner.example", Enabled: false},
			},
			MailHosts: models.HostList{
				{Host: "mail.partner.example", Enabled: true},
			},
		},
		testDomain{
			OrgID: 3,
			Name:  "acme.com",
		},
		testDomain{
			OrgID: 7,
			Name:  "shop.acme.com",
		},
	)

	cases := []struct {
		name    string
		host    string
		wantOrg uint
		wantOK  bool
	}{
		{name: "apex name hit", host: "acme.example", wantOrg: 1, wantOK: true},
		{name: "link_hosts enabled item hit", host: "go.partner.example", wantOrg: 1, wantOK: true},
		{name: "mail_hosts hit", host: "mail.partner.example", wantOrg: 1, wantOK: true},
		{name: "disabled item does not hit", host: "off.partner.example", wantOrg: 0, wantOK: false},
		{name: "subdomain fallback to parent domain org", host: "sub.app.acme.example", wantOrg: 1, wantOK: true},
		{name: "most specific first", host: "shop.acme.com", wantOrg: 7, wantOK: true},
		{name: "unknown host", host: "evil.example", wantOrg: 0, wantOK: false},
		{name: "localhost", host: "localhost:8080", wantOrg: 0, wantOK: false},
		{name: "ip literal", host: "203.0.113.9:80", wantOrg: 0, wantOK: false},
		{name: "case, port, and trailing dot normalization", host: "Shop.Acme.COM.:8443", wantOrg: 7, wantOK: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			org, ok := OwnerOf(db, tc.host)
			if org != tc.wantOrg || ok != tc.wantOK {
				t.Errorf("OwnerOf(%q) = (%d, %v), want (%d, %v)", tc.host, org, ok, tc.wantOrg, tc.wantOK)
			}
		})
	}

	t.Run("nil db and missing domains table", func(t *testing.T) {
		if org, ok := OwnerOf(nil, "shop.acme.com"); ok || org != 0 {
			t.Errorf("OwnerOf(nil, host) = (%d, %v), want (0, false)", org, ok)
		}
		if org, ok := OwnerOf(testDB(t), "shop.acme.com"); ok || org != 0 {
			t.Errorf("OwnerOf(emptyDB, host) = (%d, %v), want (0, false)", org, ok)
		}
	})
}

func TestOwnerOfAmbiguous(t *testing.T) {
	t.Run("one org in name, another org in link_hosts", func(t *testing.T) {
		db := testDB(t,
			testDomain{
				OrgID: 1,
				Name:  "shop.acme.com",
			},
			testDomain{
				OrgID: 2,
				LinkHosts: models.HostList{
					{Host: "shop.acme.com", Enabled: true},
				},
			},
		)
		org, ok := OwnerOf(db, "shop.acme.com")
		if ok || org != 0 {
			t.Fatalf("OwnerOf(shop.acme.com) = (%d, %v), want (0, false) due to ambiguity", org, ok)
		}
	})

	t.Run("two orgs both have it in link_hosts", func(t *testing.T) {
		db := testDB(t,
			testDomain{
				OrgID: 10,
				LinkHosts: models.HostList{
					{Host: "shared.example.com", Enabled: true},
				},
			},
			testDomain{
				OrgID: 20,
				LinkHosts: models.HostList{
					{Host: "shared.example.com", Enabled: true},
				},
			},
		)
		org, ok := OwnerOf(db, "shared.example.com")
		if ok || org != 0 {
			t.Fatalf("OwnerOf(shared.example.com) = (%d, %v), want (0, false) due to ambiguity", org, ok)
		}
	})
}

func TestAbsolute(t *testing.T) {
	t.Run("registered host wins and is https", func(t *testing.T) {
		rv := NewResolver(testDB(t, acme()))
		if got := rv.Absolute(0, req("app.acme.example"), false); got != "https://app.acme.example" {
			t.Errorf("Absolute = %q, want https://app.acme.example", got)
		}
	})

	t.Run("unrecognised host yields nothing once a domain is registered", func(t *testing.T) {
		rv := NewResolver(testDB(t, acme()))
		if got := rv.Absolute(0, req("evil.com"), true); got != "" {
			t.Errorf("Absolute = %q, want \"\" — a forged host must never become an origin", got)
		}
	})

	t.Run("shared host guard tests", func(t *testing.T) {
		// 1. 未声明共享 host + 实例已注册其他域名 + 请求来自入口 host
		t.Run("undeclared shared host yields nothing", func(t *testing.T) {
			rv := NewResolver(testDB(t, acme()))
			t.Setenv("OCTARQ_SHARED_HOSTS", "")
			if got := rv.Absolute(0, req("app.octarq.org"), false); got != "" {
				t.Errorf("Absolute = %q, want \"\" — an undeclared shared host must be rejected to prevent Host forgery", got)
			}
		})

		// 2. 已声明共享 host → 返回该 host
		t.Run("declared shared host is honoured", func(t *testing.T) {
			rv := NewResolver(testDB(t, acme()))
			t.Setenv("OCTARQ_SHARED_HOSTS", "app.octarq.org, api.octarq.org")
			if got := rv.Absolute(0, req("app.octarq.org"), true); got != "https://app.octarq.org" {
				t.Errorf("Absolute = %q, want https://app.octarq.org", got)
			}
		})

		// 3. 已声明共享 host，但租户拥有自己的域名且请求来自该自有域名 → 返回自有域名，不是共享 host
		t.Run("owned domain takes precedence over shared host", func(t *testing.T) {
			rv := NewResolver(testDB(t, acme()))
			t.Setenv("OCTARQ_SHARED_HOSTS", "app.octarq.org")
			if got := rv.Absolute(1, req("acme.example"), true); got != "https://acme.example" {
				t.Errorf("Absolute = %q, want https://acme.example", got)
			}
		})

		// 4. 已声明共享 host，请求来自一个既非共享也非自有的域名 → 仍然 ErrNoOrigin
		t.Run("forged host is rejected even with shared host declared", func(t *testing.T) {
			rv := NewResolver(testDB(t, acme()))
			t.Setenv("OCTARQ_SHARED_HOSTS", "app.octarq.org")
			if got := rv.Absolute(0, req("evil.com"), true); got != "" {
				t.Errorf("Absolute = %q, want \"\" — a forged host must never become an origin even if a shared host exists", got)
			}
		})
	})

	t.Run("no registered domain falls back to the request host", func(t *testing.T) {
		rv := NewResolver(testDB(t))
		if got := rv.Absolute(0, req("localhost:8080"), false); got != "http://localhost:8080" {
			t.Errorf("Absolute = %q, want http://localhost:8080", got)
		}
		if got := rv.Absolute(0, req("localhost:8080"), true); got != "https://localhost:8080" {
			t.Errorf("Absolute over TLS = %q, want https://localhost:8080", got)
		}
	})

	t.Run("a host that cannot be part of a URL is refused even in fallback", func(t *testing.T) {
		rv := NewResolver(testDB(t))
		if got := rv.Absolute(0, req("evil.com/path?x=1"), false); got != "" {
			t.Errorf("Absolute = %q, want \"\" — the authority must not carry a path or query", got)
		}
	})
}

func TestServesTraffic(t *testing.T) {
	cases := []struct {
		name string
		host string
		want bool
	}{
		{name: "link host", host: "go.acme.example", want: true},
		{name: "mail host", host: "mail.acme.example", want: true},
		{name: "disabled link host", host: "off.partner.example", want: false},
		{name: "a subdomain of a serving domain still serves the dashboard", host: "app.acme.example", want: false},
		{name: "unregistered host", host: "dash.example.net", want: false},
	}
	db := testDB(t, acme())
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ServesTraffic(db, tc.host); got != tc.want {
				t.Errorf("ServesTraffic(%q) = %v, want %v", tc.host, got, tc.want)
			}
		})
	}

	t.Run("apex with the toggle on and no host list", func(t *testing.T) {
		db := testDB(t, testDomain{OrgID: 1, Name: "short.example", ForLink: true})
		if !ServesTraffic(db, "short.example") {
			t.Error("an apex with for_link set and no host list serves links there, so it must not serve the dashboard")
		}
	})

	t.Run("no domains table at all", func(t *testing.T) {
		if ServesTraffic(testDB(t), "anything.example") {
			t.Error("a build without the dns plugin has no serving hosts")
		}
	})
}

func TestAnyRegistered(t *testing.T) {
	if AnyRegistered(testDB(t)) {
		t.Error("an instance with no domains table has no whitelist")
	}
	if !AnyRegistered(testDB(t, acme())) {
		t.Error("a registered domain is a whitelist")
	}
}

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

// TestResolverCaches pins that the cached forms answer the same as the direct
// ones, and that a repeated question does not re-read the table — the endpoints
// behind these are unauthenticated, so an attacker spraying Host headers would
// otherwise be a free DB-query amplifier.
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

// TestReservedZoneStopsApexBubbling pins trap 1: with a tenant-subdomain base
// configured, an unprovisioned label under it must resolve to NO org — in
// particular it must not bubble up through the operator's registered apex
// (octarq.org) into the vendor org. That leak is the first step of one tenant
// reading another's data.
func TestReservedZoneStopsApexBubbling(t *testing.T) {
	db := testDB(t,
		testDomain{OrgID: 1, Name: "octarq.org"},
		testDomain{OrgID: 7, Name: "acme7x.app.octarq.org"},
	)
	seedSetting(t, db, models.BaseDomainSetting, "app.octarq.org")

	if org, ok := OwnerOf(db, "nobody.app.octarq.org"); ok || org != 0 {
		t.Errorf("OwnerOf(nobody.app.octarq.org) = (%d, %v), want (0, false) — must not bubble to the vendor apex", org, ok)
	}
	if org, ok := OwnerOf(db, "acme7x.app.octarq.org"); !ok || org != 7 {
		t.Errorf("OwnerOf(acme7x.app.octarq.org) = (%d, %v), want (7, true) — a provisioned tenant still resolves", org, ok)
	}
}

// TestReservedZoneDoesNotLeakThroughBaseRow pins the sharper edge of the same
// rule: even when the base itself is a registered row (the operator's dashboard
// host), a deeper unprovisioned host must not resolve through it.
func TestReservedZoneDoesNotLeakThroughBaseRow(t *testing.T) {
	db := testDB(t,
		testDomain{OrgID: 1, Name: "octarq.org"},
		testDomain{OrgID: 1, Name: "app.octarq.org"},
	)
	seedSetting(t, db, models.BaseDomainSetting, "app.octarq.org")

	if org, ok := OwnerOf(db, "nobody.app.octarq.org"); ok || org != 0 {
		t.Errorf("OwnerOf(nobody.app.octarq.org) = (%d, %v), want (0, false) — must not match the base row", org, ok)
	}
	if org, ok := OwnerOf(db, "app.octarq.org"); !ok || org != 1 {
		t.Errorf("OwnerOf(app.octarq.org) = (%d, %v), want (1, true) — the dashboard host still resolves", org, ok)
	}
}

// TestOwnedHostReservedZoneIsolation pins the same reservation for OwnedHost:
// the vendor must not claim a tenant's subdomain through its base row, while
// the owning tenant still may.
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

// TestNoBaseDomainKeepsTodayBehaviour pins that without a configured base the
// parent-bubbling semantics are exactly what they always were: a subdomain of a
// registered apex is owned, and an unknown host resolves to nothing.
func TestNoBaseDomainKeepsTodayBehaviour(t *testing.T) {
	db := testDB(t, acme())
	if org, ok := OwnerOf(db, "sub.app.acme.example"); !ok || org != 1 {
		t.Errorf("OwnerOf(sub.app.acme.example) = (%d, %v), want (1, true)", org, ok)
	}
	if org, ok := OwnerOf(db, "evil.example"); ok || org != 0 {
		t.Errorf("OwnerOf(evil.example) = (%d, %v), want (0, false)", org, ok)
	}
}

// TestAbsoluteAfterProvisioningUsesSharedHost pins trap 2: provisioning the
// first tenant subdomain creates a whitelist, which closes origin's
// no-whitelist fallback. The dashboard host must still build correct absolute
// URLs — through OCTARQ_SHARED_HOSTS, the documented deployment path.
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

// TestCacheIsNamespacedPerDatabase pins that one Resolver's answers never reach
// a Resolver built over a different database.
//
// The cache is package-global so that the several Resolvers a process builds
// over the ONE production database invalidate together. Under test that global
// meets many databases, and the entry that leaks worst is "any" — "this
// instance has registered no domains", the flag that switches on the fallback
// honouring the request Host verbatim. A case that registers nothing would
// otherwise hand that verdict to the next case, whose registered domains are
// the only thing standing between a forged Host and a password-reset link
// pointed at the attacker.
//
// This deliberately does NOT go through testDB: that fixture calls
// clearAllCache() on every use, which is what keeps the rest of this file
// honest but would here reset the very state under test. The two databases are
// opened directly, and nothing clears the cache between them.
func TestCacheIsNamespacedPerDatabase(t *testing.T) {
	clearAllCache()

	// A whitelist-free instance: no domains table at all, so the fallback is on
	// and the request Host is used as-is.
	bare := NewResolver(openDB(t, "bare"))
	if got := bare.Absolute(0, req("evil.example"), false); got != "http://evil.example" {
		t.Fatalf("Absolute on a whitelist-free instance = %q, want the request host; the assertion below would be vacuous", got)
	}
	if bare.AnyRegistered() {
		t.Fatal("AnyRegistered on a database with no domains table")
	}

	// A second database, registering a domain. Same process, same global cache.
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
		t.Error("AnyRegistered read back the other database's answer: a registered whitelist was reported as absent")
	}
	if got := guarded.Absolute(0, req("evil.example"), false); got != "" {
		t.Errorf("Absolute honoured a forged Host as %q; a whitelist exists and evil.example is not on it", got)
	}
	if _, ok := guarded.OwnerOf("evil.example"); ok {
		t.Error("OwnerOf claimed an owner for an unregistered host")
	}
}
