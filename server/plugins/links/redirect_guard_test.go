package links

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugins/dns"
	"gorm.io/gorm"
)

// newGuardDB opens a per-test in-memory database. The other links tests share
// one anonymous shared-cache DB (they clear rows defensively); these guards
// assert exact ownership states, so they need storage that no other test
// touches.
func newGuardDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(append(models.AllModels(), &Link{}, &LinkEvent{}, &dns.Domain{}, &dns.ProviderAccount{})...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func seedOrgs(t *testing.T, db *gorm.DB, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		if err := db.Create(&models.Org{
			Name: fmt.Sprintf("org-%d", i), Slug: fmt.Sprintf("org-%d", i), InboundToken: "tok",
		}).Error; err != nil {
			t.Fatalf("seed org %d: %v", i, err)
		}
	}
}

// Guard 1: a host that no domain registers — the shared dashboard host
// app.octarq.org, a bare instance hostname, an IP — must still serve
// host-agnostic links (host = ""). This is the branch mail click tracking
// runs on: plugins/mail's wrapLinksInEmail creates Host:"" links and serves
// them from the shared host when the org has no custom link domain. Refusing
// here would break Cloud click tracking for every tenant without a domain.
func TestLookupServesHostAgnosticOnUnregisteredHost(t *testing.T) {
	db := newGuardDB(t)
	seedOrgs(t, db, 2)
	link := &Link{OrgID: 2, Slug: "promo", Host: "", Target: "https://org2-internal.example", Enabled: true}
	if err := db.Create(link).Error; err != nil {
		t.Fatalf("seed link: %v", err)
	}

	eng := NewEngine(db, mockCtx())
	got, ok := eng.Lookup("app.octarq.org", "promo")
	if !ok || got.ID != link.ID {
		t.Fatalf("host-agnostic link on an unregistered host: ok=%v, want found", ok)
	}
}

// Guard 2: a Link row that claims an exact host no domain registers is an
// unauthorized claim — the write side only lets an org put links on hosts it
// owns, so such a row can only be a dirty insert. It must not be served, and
// it must not shadow the host-agnostic fallback either.
func TestLookupRefusesUnregisteredHostClaim(t *testing.T) {
	db := newGuardDB(t)
	seedOrgs(t, db, 2)
	if err := db.Create(&Link{OrgID: 1, Slug: "promo", Host: "claimed.example", Target: "https://dirty.example", Enabled: true}).Error; err != nil {
		t.Fatalf("seed link: %v", err)
	}

	eng := NewEngine(db, mockCtx())
	if _, ok := eng.Lookup("claimed.example", "promo"); ok {
		t.Fatal("exact-host link on an unregistered host resolved — unauthorized host claim served")
	}
}

// Guard 3: when two orgs claim the same host — one by registering it as a
// domain name, the other by listing it in their own domain's LinkHosts — the
// redirect must refuse to serve the host rather than pick a side. Both
// insertion orders are tested because the original bug was a first-match
// scan: with only the attacker-first order the test would go red under the
// bug, but with only the victim-first order it would stay green and prove
// nothing.
func TestLookupRefusesContestedHost(t *testing.T) {
	for _, order := range []string{"victim-first", "attacker-first"} {
		t.Run(order, func(t *testing.T) {
			db := newGuardDB(t)
			seedOrgs(t, db, 2)

			victim := dns.Domain{OrgID: 1, Name: "victim.test", ForLink: true}
			attacker := dns.Domain{
				OrgID: 2, Name: "attacker.test", ForLink: true,
				LinkHosts: models.HostList{{Host: "victim.test", Enabled: true}},
			}
			if order == "victim-first" {
				if err := db.Create(&victim).Error; err != nil {
					t.Fatalf("seed victim domain: %v", err)
				}
				if err := db.Create(&attacker).Error; err != nil {
					t.Fatalf("seed attacker domain: %v", err)
				}
			} else {
				if err := db.Create(&attacker).Error; err != nil {
					t.Fatalf("seed attacker domain: %v", err)
				}
				if err := db.Create(&victim).Error; err != nil {
					t.Fatalf("seed victim domain: %v", err)
				}
			}

			// The victim's own link on their apex and the attacker's
			// host-agnostic link share the slug; the host-agnostic one is what
			// the original bug served on the victim's host.
			if err := db.Create(&Link{OrgID: 1, Slug: "promo", Host: "victim.test", Target: "https://legit.example", Enabled: true}).Error; err != nil {
				t.Fatalf("seed victim link: %v", err)
			}
			if err := db.Create(&Link{OrgID: 2, Slug: "promo", Host: "", Target: "https://phish.example", Enabled: true}).Error; err != nil {
				t.Fatalf("seed attacker link: %v", err)
			}

			eng := NewEngine(db, mockCtx())
			got, ok := eng.Lookup("victim.test", "promo")
			if ok {
				t.Fatalf("contested host resolved to org %d target %q — must refuse, not pick a side", got.OrgID, got.Target)
			}
		})
	}
}

// Guard 4: a host that one org owns resolves only that org's links. Another
// org's host-agnostic link must not appear on it — the owner_id filter is the
// tenant boundary on the redirect path.
func TestLookupScopesOwnedHostToOwner(t *testing.T) {
	db := newGuardDB(t)
	seedOrgs(t, db, 2)
	if err := db.Create(&dns.Domain{OrgID: 1, Name: "victim.test", ForLink: true}).Error; err != nil {
		t.Fatalf("seed victim domain: %v", err)
	}
	legit := &Link{OrgID: 1, Slug: "x", Host: "victim.test", Target: "https://legit.example", Enabled: true}
	if err := db.Create(legit).Error; err != nil {
		t.Fatalf("seed victim link: %v", err)
	}
	if err := db.Create(&Link{OrgID: 2, Slug: "y", Host: "", Target: "https://org2.example", Enabled: true}).Error; err != nil {
		t.Fatalf("seed org2 link: %v", err)
	}

	eng := NewEngine(db, mockCtx())

	got, ok := eng.Lookup("victim.test", "x")
	if !ok || got.ID != legit.ID {
		t.Fatalf("owner's own link on owned host: ok=%v, want found", ok)
	}

	// Org 2's host-agnostic link must not be served on org 1's host.
	if _, ok := eng.Lookup("victim.test", "y"); ok {
		t.Fatal("another org's host-agnostic link resolved on a host owned by a different org")
	}
}
