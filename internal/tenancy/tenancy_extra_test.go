package tenancy

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestProvision_NilAndEmptyInputs(t *testing.T) {
	// db nil
	name, ok, err := Provision(nil, 1, "test")
	if name != "" || ok || err != nil {
		t.Errorf("expected empty/false/nil for nil db, got (%q, %v, %v)", name, ok, err)
	}

	// orgID 0
	db := openDB(t)
	name, ok, err = Provision(db, 0, "test")
	if name != "" || ok || err != nil {
		t.Errorf("expected empty/false/nil for orgID 0, got (%q, %v, %v)", name, ok, err)
	}

	// slug empty
	name, ok, err = Provision(db, 1, "")
	if name != "" || ok || err != nil {
		t.Errorf("expected empty/false/nil for empty slug, got (%q, %v, %v)", name, ok, err)
	}
}

func TestProvision_NoDomainsTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nodomains.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	// Migrate only Setting (no domains table)
	if err := db.AutoMigrate(&models.Setting{}); err != nil {
		t.Fatal(err)
	}
	setBase(t, db, "app.example.com")

	name, ok, err := Provision(db, 1, "acme")
	if name != "" || ok || err != nil {
		t.Errorf("expected false/nil when domains table missing, got (%q, %v, %v)", name, ok, err)
	}
}

func TestRetire_NilAndEmptyAndNoTable(t *testing.T) {
	// nil db
	if err := Retire(nil, 1, "test"); err != nil {
		t.Errorf("expected nil for nil db, got %v", err)
	}
	// orgID 0
	db := openDB(t)
	if err := Retire(db, 0, "test"); err != nil {
		t.Errorf("expected nil for orgID 0, got %v", err)
	}
	// slug empty
	if err := Retire(db, 1, ""); err != nil {
		t.Errorf("expected nil for empty slug, got %v", err)
	}

	// No base domain
	if err := Retire(db, 1, "test"); err != nil {
		t.Errorf("expected nil when base domain empty, got %v", err)
	}

	// No domains table
	dbPath := filepath.Join(t.TempDir(), "nodomains_retire.db")
	dbNoDomains, _ := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Discard})
	_ = dbNoDomains.AutoMigrate(&models.Setting{})
	setBase(t, dbNoDomains, "app.example.com")
	if err := Retire(dbNoDomains, 1, "test"); err != nil {
		t.Errorf("expected nil when domains table missing, got %v", err)
	}
}

func TestIsDuplicateKeyVariants(t *testing.T) {
	if !isDuplicateKey(gorm.ErrDuplicatedKey) {
		t.Error("expected true for gorm.ErrDuplicatedKey")
	}
	if !isDuplicateKey(errors.New("ERROR: duplicate key value violates unique constraint \"idx_name\"")) {
		t.Error("expected true for postgres duplicate key error string")
	}
	if !isDuplicateKey(errors.New("UNIQUE constraint failed: domains.name")) {
		t.Error("expected true for sqlite unique constraint error string")
	}
	if isDuplicateKey(errors.New("connection reset by peer")) {
		t.Error("expected false for unrelated error")
	}
}
