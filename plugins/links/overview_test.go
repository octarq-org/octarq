package links

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	dns "github.com/octarq-org/octarq/plugins/dns"
	"gorm.io/gorm"
)

func newOverviewDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(append(models.AllModels(), &Link{}, &LinkEvent{}, &dns.Domain{}, &dns.ProviderAccount{})...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// TestOverviewBotClicksScopedToOrgLinks guards the click aggregates against the
// regression where botCount bound the org id directly into `link_id IN (?)`
// instead of the org's link subquery — which made bot counts filter on
// link_id == orgID rather than "any link owned by the org".
func TestOverviewBotClicksScopedToOrgLinks(t *testing.T) {
	db := newOverviewDB(t)
	p := &Plugin{db: db}

	const org = uint(7)
	// A link whose id deliberately differs from the org id, so a query that
	// confuses the two would miss these events entirely.
	link := Link{OrgID: org, Host: "go.example.com", Slug: "a", Target: "https://x", Enabled: true}
	if err := db.Create(&link).Error; err != nil {
		t.Fatalf("create link: %v", err)
	}
	if link.ID == org {
		t.Fatalf("test needs link.ID (%d) != org (%d)", link.ID, org)
	}
	now := time.Now()
	// Two bot clicks and one human click, all within the 7-day window.
	db.Create(&LinkEvent{LinkID: link.ID, IsBot: true, CreatedAt: now})
	db.Create(&LinkEvent{LinkID: link.ID, IsBot: true, CreatedAt: now})
	db.Create(&LinkEvent{LinkID: link.ID, IsBot: false, CreatedAt: now})

	out := p.overview(org, true)

	if got := out["botClicks7d"].(int64); got != 2 {
		t.Errorf("botClicks7d = %d, want 2", got)
	}
	if got := out["botClicks30d"].(int64); got != 2 {
		t.Errorf("botClicks30d = %d, want 2", got)
	}
}
