package links

import (
	"testing"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugins/dns"
)

// The abuse-report handler consumes links.resolve through a bare type
// assertion on an `any` (internal/api/abuse.go). Nothing links the two halves
// at compile time: change this signature and the assertion simply stops
// matching, so reports silently file with no target and no workspace and no
// build ever breaks. This pins the exact shape that consumer asserts.
func TestResolveServiceSignatureMatchesConsumer(t *testing.T) {
	_, p, _ := setupOwnershipTestDB(t)

	var svc any = p.resolveSlug
	if _, ok := svc.(func(host, slug string) (target string, orgID uint, ok bool)); !ok {
		t.Fatal("resolveSlug no longer matches the signature internal/api/abuse.go asserts; " +
			"update that assertion in the same change or abuse reports lose attribution silently")
	}
}

// A slug held by two workspaces on different hostnames must attribute to the
// one that actually owns the reported hostname.
func TestResolveSlugAttributesByHost(t *testing.T) {
	db, p, engine := setupOwnershipTestDB(t)
	p.engine = engine

	db.Create(&dns.Domain{OrgID: 1, Name: "victim.test", ForLink: true})
	db.Create(&dns.Domain{OrgID: 2, Name: "other.test", ForLink: true})
	db.Create(&Link{OrgID: 1, Slug: "dup", Host: "victim.test", Target: "https://victim.example", Enabled: true})
	db.Create(&Link{OrgID: 2, Slug: "dup", Host: "other.test", Target: "https://other.example", Enabled: true})

	target, orgID, ok := p.resolveSlug("victim.test", "dup")
	if !ok || orgID != 1 || target != "https://victim.example" {
		t.Fatalf("report on victim.test misattributed: org=%d target=%q ok=%v", orgID, target, ok)
	}

	target, orgID, ok = p.resolveSlug("other.test", "dup")
	if !ok || orgID != 2 || target != "https://other.example" {
		t.Fatalf("report on other.test misattributed: org=%d target=%q ok=%v", orgID, target, ok)
	}
}

// A contested hostname attributes to nobody rather than to a guess.
func TestResolveSlugRefusesContestedHost(t *testing.T) {
	db, p, engine := setupOwnershipTestDB(t)
	p.engine = engine

	db.Create(&dns.Domain{
		OrgID: 2, Name: "attacker.test", ForLink: true,
		LinkHosts: models.HostList{models.Host{Host: "victim.test", Enabled: true}},
	})
	db.Create(&dns.Domain{OrgID: 1, Name: "victim.test", ForLink: true})
	db.Create(&Link{OrgID: 1, Slug: "x", Host: "victim.test", Target: "https://victim.example", Enabled: true})

	if _, orgID, ok := p.resolveSlug("victim.test", "x"); ok {
		t.Fatalf("contested host attributed to org %d instead of refusing", orgID)
	}
}

// A disabled link still attributes: a report about a link that was just turned
// off is exactly the report a moderator needs to see.
func TestResolveSlugAttributesDisabledLink(t *testing.T) {
	db, p, engine := setupOwnershipTestDB(t)
	p.engine = engine

	db.Create(&Link{OrgID: 2, Slug: "off", Host: "", Target: "https://dest.example", Enabled: false})

	target, orgID, ok := p.resolveSlug("shared.example", "off")
	if !ok || orgID != 2 || target != "https://dest.example" {
		t.Fatalf("disabled link lost attribution: org=%d target=%q ok=%v", orgID, target, ok)
	}
}
