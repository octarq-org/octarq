package origin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type testOrgHost struct {
	ID      uint `gorm:"primaryKey"`
	OrgID   uint `gorm:"column:owner_id"`
	Host    string
	Traffic bool
	Source  string
}

func (testOrgHost) TableName() string { return "org_hosts" }

func openDB(t *testing.T, suffix string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+suffix+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return db
}

func testDB(t *testing.T, rows ...testOrgHost) *gorm.DB {
	t.Helper()
	clearAllCache()
	db := openDB(t, "")
	if len(rows) == 0 {
		return db
	}
	if err := db.AutoMigrate(&testOrgHost{}); err != nil {
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

func acme() []testOrgHost {
	return []testOrgHost{
		{OrgID: 1, Host: "acme.example", Traffic: false, Source: "test"},
		{OrgID: 1, Host: "go.acme.example", Traffic: true, Source: "test"},
		{OrgID: 1, Host: "go.partner.example", Traffic: true, Source: "test"},
		{OrgID: 1, Host: "mail.acme.example", Traffic: true, Source: "test"},
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
		{name: "host outside the apex", host: "go.partner.example", want: true},
		{name: "disabled host-list entry", host: "off.partner.example", want: false},
		{name: "unrelated host", host: "evil.example", want: false},
		{name: "registered name as a prefix of the attacker's", host: "acme.example.evil.com", want: false},
		{name: "registered name as a suffix of the attacker's", host: "notacme.example", want: false},
		{name: "subdomain of someone else's domain when that matches suffix", host: "app.notacme.example", want: false},
		{name: "org 1 owns it, asking for org 1", host: "acme.example", orgID: 1, want: true},
		{name: "org 1 owns it, asking for org 2", host: "acme.example", orgID: 2, want: false},
		{name: "empty host header", host: "", want: false},
		{name: "host is localhost — not an owned domain", host: "localhost", want: false},
		{name: "host is an IP literal", host: "127.0.0.1", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := testDB(t, acme()...)
			r := req(tc.host)
			if gotHost, gotOK := OwnedHost(db, tc.orgID, r); gotOK != tc.want {
				t.Errorf("OwnedHost(org %d, Host: %q) = (%q, %v), want (_, %v)", tc.orgID, tc.host, gotHost, gotOK, tc.want)
			}
		})
	}

	t.Run("nil db", func(t *testing.T) {
		if _, ok := OwnedHost(nil, 0, req("acme.example")); ok {
			t.Error("nil db must report unowned")
		}
	})

	t.Run("nil request", func(t *testing.T) {
		if _, ok := OwnedHost(testDB(t), 0, nil); ok {
			t.Error("nil request must report unowned")
		}
	})

	t.Run("no org_hosts table at all", func(t *testing.T) {
		if _, ok := OwnedHost(testDB(t), 0, req("acme.example")); ok {
			t.Error("a build without org hosts has no owned hosts")
		}
	})
}

func TestOwnerOf(t *testing.T) {
	cases := []struct {
		name    string
		host    string
		wantOrg uint
		wantOK  bool
	}{
		{name: "registered name", host: "acme.example", wantOrg: 1, wantOK: true},
		{name: "subdomain of a registered name", host: "app.acme.example", wantOrg: 1, wantOK: true},
		{name: "deep subdomain", host: "deep.sub.acme.example", wantOrg: 1, wantOK: true},
		{name: "registered link host outside the apex", host: "go.partner.example", wantOrg: 1, wantOK: true},
		{name: "case and port are normalised away", host: "APP.Acme.Example:8443", wantOrg: 1, wantOK: true},
		{name: "disabled host-list entry resolves to nothing", host: "off.partner.example", wantOrg: 0, wantOK: false},
		{name: "unrelated host", host: "evil.example", wantOrg: 0, wantOK: false},
		{name: "empty host", host: "", wantOrg: 0, wantOK: false},
		{name: "localhost is not an owned domain", host: "localhost", wantOrg: 0, wantOK: false},
		{name: "IPv4 literal is not an owned domain", host: "127.0.0.1", wantOrg: 0, wantOK: false},
		{name: "IPv6 literal is not an owned domain", host: "[::1]", wantOrg: 0, wantOK: false},
		{name: "single-label host cannot be owned", host: "internal", wantOrg: 0, wantOK: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := testDB(t, acme()...)
			gotOrg, gotOK := OwnerOf(db, tc.host)
			if gotOK != tc.wantOK || gotOrg != tc.wantOrg {
				t.Errorf("OwnerOf(%q) = (%d, %v), want (%d, %v)", tc.host, gotOrg, gotOK, tc.wantOrg, tc.wantOK)
			}
		})
	}

	t.Run("specificity: subdomain row beats parent row owned by another org", func(t *testing.T) {
		db := testDB(t,
			testOrgHost{OrgID: 1, Host: "acme.example", Source: "test"},
			testOrgHost{OrgID: 2, Host: "sub.acme.example", Source: "test"},
		)
		if org, ok := OwnerOf(db, "sub.acme.example"); !ok || org != 2 {
			t.Errorf("OwnerOf(sub.acme.example) = (%d, %v), want (2, true) — exact match must win", org, ok)
		}
		if org, ok := OwnerOf(db, "deep.sub.acme.example"); !ok || org != 2 {
			t.Errorf("OwnerOf(deep.sub.acme.example) = (%d, %v), want (2, true) — closest parent must win", org, ok)
		}
		if org, ok := OwnerOf(db, "other.acme.example"); !ok || org != 1 {
			t.Errorf("OwnerOf(other.acme.example) = (%d, %v), want (1, true) — fall through to parent", org, ok)
		}
	})

	t.Run("conflict: two distinct orgs match at the same specificity level", func(t *testing.T) {
		db := testDB(t,
			testOrgHost{OrgID: 1, Host: "shared.partner.example", Traffic: true, Source: "test"},
			testOrgHost{OrgID: 2, Host: "shared.partner.example", Traffic: true, Source: "test"},
		)
		if org, ok := OwnerOf(db, "shared.partner.example"); ok || org != 0 {
			t.Errorf("OwnerOf(shared.partner.example) = (%d, %v), want (0, false) on ambiguity", org, ok)
		}
	})

	t.Run("nil db", func(t *testing.T) {
		if _, ok := OwnerOf(nil, "acme.example"); ok {
			t.Error("nil db must report no owner")
		}
	})

	t.Run("no org_hosts table at all", func(t *testing.T) {
		if _, ok := OwnerOf(testDB(t), "acme.example"); ok {
			t.Error("a build without org hosts has no owners")
		}
	})
}

func TestServesTraffic(t *testing.T) {
	cases := []struct {
		name string
		host string
		want bool
	}{
		{name: "registered link host serves traffic", host: "go.acme.example", want: true},
		{name: "registered mail host serves traffic", host: "mail.acme.example", want: true},
		{name: "subdomain of an apex does NOT serve unless listed", host: "app.acme.example", want: false},
		{name: "apex itself does NOT serve when it is ownership only", host: "acme.example", want: false},
		{name: "disabled host does not serve", host: "off.partner.example", want: false},
		{name: "unrelated host does not serve", host: "evil.example", want: false},
		{name: "case and port normalised away", host: "GO.Acme.Example:8443", want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := testDB(t, acme()...)
			if got := ServesTraffic(db, tc.host); got != tc.want {
				t.Errorf("ServesTraffic(%q) = %v, want %v", tc.host, got, tc.want)
			}
		})
	}

	t.Run("apex with traffic enabled serves", func(t *testing.T) {
		db := testDB(t, testOrgHost{
			OrgID:   1,
			Host:    "barelink.example",
			Traffic: true,
			Source:  "test",
		})
		if !ServesTraffic(db, "barelink.example") {
			t.Error("a host marked as serving traffic must serve it")
		}
	})

	t.Run("no org_hosts table at all", func(t *testing.T) {
		if ServesTraffic(testDB(t), "anything.example") {
			t.Error("a build without org hosts has no serving hosts")
		}
	})
}

func TestAnyRegistered(t *testing.T) {
	if AnyRegistered(testDB(t)) {
		t.Error("an instance with no org_hosts table has no whitelist")
	}
	if !AnyRegistered(testDB(t, acme()...)) {
		t.Error("a registered host is a whitelist")
	}
}
