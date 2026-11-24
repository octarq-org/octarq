package tenancy

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/origin"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tenancy.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Setting{}, &orgRow{}, &models.OrgHost{}); err != nil {
		t.Fatalf("migrate: %v", err)
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

func hasHostRow(t *testing.T, db *gorm.DB, host string) bool {
	t.Helper()
	var n int64
	if err := db.Model(&models.OrgHost{}).Where("host = ?", host).Count(&n).Error; err != nil {
		t.Fatalf("count hosts: %v", err)
	}
	return n > 0
}

func TestSubdomain(t *testing.T) {
	db := openDB(t)
	if name, ok := Subdomain(db, "acme7x"); ok || name != "" {
		t.Fatalf("Subdomain with no base = (%q, %v), want (\"\", false)", name, ok)
	}

	setBase(t, db, "App.Example.com.")
	name, ok := Subdomain(db, "acme7x")
	if !ok || name != "acme7x.app.example.com" {
		t.Fatalf("Subdomain = (%q, %v), want (acme7x.app.example.com, true)", name, ok)
	}
	if name, ok := Subdomain(db, ""); ok || name != "" {
		t.Fatalf("Subdomain with empty slug = (%q, %v), want (\"\", false)", name, ok)
	}
}

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
	if hasHostRow(t, db, "acme7x.app.example.com") {
		t.Fatal("Provision wrote a host row with no base configured")
	}
}

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
	if !hasHostRow(t, db, "acme7x.app.example.com") {
		t.Fatal("Provision did not write the host row")
	}
	org, ok := origin.OwnerOf(db, "acme7x.app.example.com")
	if !ok || org != 7 {
		t.Errorf("OwnerOf(acme7x.app.example.com) = (%d, %v), want (7, true)", org, ok)
	}
}

func TestProvisionWritesTrafficHostRow(t *testing.T) {
	db := openDB(t)
	setBase(t, db, "app.example.com")
	seedOrg(t, db, 7, "acme7x")

	if _, ok, err := Provision(db, 7, "acme7x"); err != nil || !ok {
		t.Fatalf("Provision = (ok=%v, err=%v), want provisioned", ok, err)
	}
	var row models.OrgHost
	if err := db.Where("host = ?", "acme7x.app.example.com").First(&row).Error; err != nil {
		t.Fatalf("read host row: %v", err)
	}
	if !row.Traffic {
		t.Fatal("tenant subdomain row has traffic = false, want true")
	}
	if row.Source != "core:tenancy" {
		t.Fatalf("Source = %q, want core:tenancy", row.Source)
	}
}

func TestProvisionCollisionIsError(t *testing.T) {
	db := openDB(t)
	setBase(t, db, "app.example.com")
	seedOrg(t, db, 1, "acme7x")
	seedOrg(t, db, 2, "acme8x")

	if _, ok, err := Provision(db, 1, "acme7x"); err != nil || !ok {
		t.Fatalf("first Provision = (ok=%v, err=%v), want provisioned", ok, err)
	}
	if _, ok, err := Provision(db, 2, "acme7x"); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("second Provision err = %v, want ErrNameTaken (ok=%v)", err, ok)
	}
	org, ok := origin.OwnerOf(db, "acme7x.app.example.com")
	if !ok || org != 1 {
		t.Errorf("OwnerOf after collision = (%d, %v), want (1, true)", org, ok)
	}
}

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
	if hasHostRow(t, db, "oldslug.app.example.com") {
		t.Fatal("Retire left the subdomain row behind")
	}
	if org, ok := origin.OwnerOf(db, "oldslug.app.example.com"); ok || org != 0 {
		t.Errorf("OwnerOf after retire = (%d, %v), want (0, false)", org, ok)
	}
}

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
