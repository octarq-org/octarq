package tenancy

import (
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/origin"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// The domains table is the dns plugin's; origin reads it. AutoMigrating it
	// here mirrors a build composed with the dns plugin.
	if err := db.AutoMigrate(&models.Setting{}, &orgRow{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Migrator().CreateTable(&domainRow{}); err != nil {
		t.Fatalf("create domains: %v", err)
	}
	return db
}

type orgRow struct {
	ID   uint `gorm:"primaryKey"`
	Slug string
}

func (orgRow) TableName() string { return "orgs" }

func seedOrg(t *testing.T, db *gorm.DB, id uint, slug string) {
	t.Helper()
	if err := db.Create(&orgRow{ID: id, Slug: slug}).Error; err != nil {
		t.Fatalf("seed org: %v", err)
	}
}

func setBase(t *testing.T, db *gorm.DB, base string) {
	t.Helper()
	if err := db.Create(&models.Setting{Key: models.BaseDomainSetting, Value: base}).Error; err != nil {
		t.Fatalf("set base: %v", err)
	}
}

func hasDomainRow(t *testing.T, db *gorm.DB, name string) bool {
	t.Helper()
	var n int64
	if err := db.Model(&domainRow{}).Where("name = ?", name).Count(&n).Error; err != nil {
		t.Fatalf("count domains: %v", err)
	}
	return n > 0
}

// Subdomain is the read side of Provision — what the dashboard shows as the
// org's address. It must agree with what Provision writes.
func TestSubdomain(t *testing.T) {
	db := openDB(t)
	if name, ok := Subdomain(db, "acme7x"); ok || name != "" {
		t.Fatalf("Subdomain with no base = (%q, %v), want (\"\", false)", name, ok)
	}

	setBase(t, db, "App.Example.com.") // normalization is applied
	name, ok := Subdomain(db, "acme7x")
	if !ok || name != "acme7x.app.example.com" {
		t.Fatalf("Subdomain = (%q, %v), want (acme7x.app.example.com, true)", name, ok)
	}
	if name, ok := Subdomain(db, ""); ok || name != "" {
		t.Fatalf("Subdomain with empty slug = (%q, %v), want (\"\", false)", name, ok)
	}
}

// Guard 1: with no base domain configured, Provision creates no Domain row and
// reports the feature as off. Origin behaviour must be exactly today's.
func TestProvisionDisabledWithoutBase(t *testing.T) {
	db := openDB(t)
	seedOrg(t, db, 1, "acme7x")

	name, ok, err := Provision(db, 1, "acme7x")
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if ok {
		t.Fatal("Provision reported a provisioned subdomain with no base configured")
	}
	if name != "" {
		t.Fatalf("Provision returned a name %q with no base configured", name)
	}
	if hasDomainRow(t, db, "acme7x.app.example.com") {
		t.Fatal("Provision wrote a domains row with no base configured")
	}
}

// Guard 2: once the base domain is configured, Provision writes the
// <slug>.<base> row and origin resolves the hostname to the new org.
func TestProvisionWritesResolvableRow(t *testing.T) {
	db := openDB(t)
	setBase(t, db, "app.example.com")
	seedOrg(t, db, 7, "acme7x")

	name, ok, err := Provision(db, 7, "acme7x")
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if !ok {
		t.Fatal("Provision reported no subdomain with a base configured")
	}
	if name != "acme7x.app.example.com" {
		t.Fatalf("Provision returned %q, want acme7x.app.example.com", name)
	}
	if !hasDomainRow(t, db, "acme7x.app.example.com") {
		t.Fatal("Provision did not write the domains row")
	}
	org, ok := origin.OwnerOf(db, "acme7x.app.example.com")
	if !ok || org != 7 {
		t.Errorf("OwnerOf(acme7x.app.example.com) = (%d, %v), want (7, true)", org, ok)
	}
}

// Guard 8: the subdomain row Provision writes must be a usable link host —
// ForLink true plus one enabled LinkHost naming the subdomain itself. Before
// this, Cloud tenants had an empty host dropdown and every link fell into a
// shared host="" namespace (one tenant's slug blocking all others).
func TestProvisionWritesLinkHostRow(t *testing.T) {
	db := openDB(t)
	setBase(t, db, "app.example.com")
	seedOrg(t, db, 7, "acme7x")

	if _, ok, err := Provision(db, 7, "acme7x"); err != nil || !ok {
		t.Fatalf("Provision = (ok=%v, err=%v), want provisioned", ok, err)
	}
	var row domainRow
	if err := db.Where("name = ?", "acme7x.app.example.com").First(&row).Error; err != nil {
		t.Fatalf("read domain row: %v", err)
	}
	if !row.ForLink {
		t.Fatal("tenant subdomain row has for_link = false, want true")
	}
	if len(row.LinkHosts) != 1 || row.LinkHosts[0].Host != "acme7x.app.example.com" || !row.LinkHosts[0].Enabled {
		t.Fatalf("LinkHosts = %+v, want one enabled host acme7x.app.example.com", row.LinkHosts)
	}
	if row.ForMail || len(row.MailHosts) != 0 {
		t.Fatalf("ForMail=%v MailHosts=%+v, want mail untouched (false, empty)", row.ForMail, row.MailHosts)
	}
}

// Guard 7: a hostname collision must surface as an error, never a silent skip.
func TestProvisionCollisionIsError(t *testing.T) {
	db := openDB(t)
	setBase(t, db, "app.example.com")
	seedOrg(t, db, 1, "acme7x")
	seedOrg(t, db, 2, "acme8x")

	if _, ok, err := Provision(db, 1, "acme7x"); err != nil || !ok {
		t.Fatalf("first Provision = (ok=%v, err=%v), want provisioned", ok, err)
	}
	// A different org, same slug — must collide, not silently skip.
	if _, ok, err := Provision(db, 2, "acme7x"); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("second Provision err = %v, want ErrNameTaken (ok=%v)", err, ok)
	}
	// The first org's row is untouched by the failed attempt.
	org, ok := origin.OwnerOf(db, "acme7x.app.example.com")
	if !ok || org != 1 {
		t.Errorf("OwnerOf after collision = (%d, %v), want (1, true)", org, ok)
	}
}

// Guard for trap 4 (rename): Retire removes the old slug's subdomain row so the
// retired address stops resolving to the org.
func TestRetireRemovesSubdomain(t *testing.T) {
	db := openDB(t)
	setBase(t, db, "app.example.com")
	seedOrg(t, db, 3, "oldslug")

	if _, ok, err := Provision(db, 3, "oldslug"); err != nil || !ok {
		t.Fatalf("Provision = (ok=%v, err=%v), want provisioned", ok, err)
	}
	if _, ok := origin.OwnerOf(db, "oldslug.app.example.com"); !ok {
		t.Fatal("OwnerOf before retire")
	}

	if err := Retire(db, 3, "oldslug"); err != nil {
		t.Fatalf("Retire: %v", err)
	}
	if hasDomainRow(t, db, "oldslug.app.example.com") {
		t.Fatal("Retire left the subdomain row behind")
	}
	if org, ok := origin.OwnerOf(db, "oldslug.app.example.com"); ok || org != 0 {
		t.Errorf("OwnerOf after retire = (%d, %v), want (0, false)", org, ok)
	}
}

// Retire must not touch another org's row, even under the same base.
func TestRetireLeavesOtherOrgsAlone(t *testing.T) {
	db := openDB(t)
	setBase(t, db, "app.example.com")
	seedOrg(t, db, 3, "oldslug")
	seedOrg(t, db, 4, "other")

	if _, _, err := Provision(db, 3, "oldslug"); err != nil {
		t.Fatalf("provision org 3: %v", err)
	}
	if _, _, err := Provision(db, 4, "other"); err != nil {
		t.Fatalf("provision org 4: %v", err)
	}
	if err := Retire(db, 3, "oldslug"); err != nil {
		t.Fatalf("Retire: %v", err)
	}
	if org, ok := origin.OwnerOf(db, "other.app.example.com"); !ok || org != 4 {
		t.Errorf("OwnerOf(other.app.example.com) after retire = (%d, %v), want (4, true)", org, ok)
	}
}

// The env var is only a bootstrap fallback: once a settings row exists its
// value governs, so an empty setting disables the feature even with env set.
func TestBaseDomainEnvIsBootstrapFallbackOnly(t *testing.T) {
	db := openDB(t)
	t.Setenv(models.BaseDomainEnv, "env.example.com")

	if got := models.BaseDomain(db); got != "env.example.com" {
		t.Fatalf("BaseDomain with no setting row = %q, want the env fallback", got)
	}

	setBase(t, db, "")
	if got := models.BaseDomain(db); got != "" {
		t.Fatalf("BaseDomain with an explicit empty setting = %q, want \"\" (setting governs once present)", got)
	}
}
