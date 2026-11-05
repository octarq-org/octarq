package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/db"
	"github.com/octarq-org/octarq/server/internal/models"
)

// Every successful pass records the version this binary writes, so a later
// binary can tell whether it already knows the schema.
func TestPrepareMigrationRecordsTheSchemaVersion(t *testing.T) {
	a := bootApp(t)
	if err := a.prepareMigrationAndCipher(); err != nil {
		t.Fatalf("prepareMigrationAndCipher: %v", err)
	}
	got, err := db.SchemaVersionOf(a.gdb)
	if err != nil {
		t.Fatalf("SchemaVersionOf: %v", err)
	}
	if got != db.SchemaVersion {
		t.Fatalf("recorded version %d, want %d", got, db.SchemaVersion)
	}
}

// A database migrated by a newer release must not serve an older binary: the
// older binary does not know the columns and would fail mid-request instead.
func TestPrepareMigrationRefusesANewerSchema(t *testing.T) {
	a := bootApp(t)
	if err := a.prepareMigrationAndCipher(); err != nil {
		t.Fatalf("first prepare: %v", err)
	}
	if err := a.gdb.Create(&models.SchemaMigration{Version: db.SchemaVersion + 1}).Error; err != nil {
		t.Fatalf("seed newer version: %v", err)
	}

	err := a.prepareMigrationAndCipher()
	if !errors.Is(err, db.ErrSchemaNewer) {
		t.Fatalf("want ErrSchemaNewer, got %v", err)
	}
	if !strings.Contains(err.Error(), "newer octarq release") {
		t.Fatalf("the error must name the recovery step, got %v", err)
	}
}

// AutoMigrate has no downgrade path, so an upgrade of an existing schema writes
// a snapshot first. A database that predates the version table takes this path
// on the first boot with this binary.
func TestPrepareMigrationBacksUpBeforeAnUpgrade(t *testing.T) {
	backupRoot := t.TempDir()
	t.Setenv("OCTARQ_STORAGE_DIR", filepath.Join(backupRoot, "storage"))
	a := bootApp(t)
	if err := a.prepareMigrationAndCipher(); err != nil {
		t.Fatalf("first prepare: %v", err)
	}

	// Simulate a database that predates the version table: the schema exists,
	// no version is recorded.
	if err := a.gdb.Where("1 = 1").Delete(&models.SchemaMigration{}).Error; err != nil {
		t.Fatalf("clear version rows: %v", err)
	}
	if err := a.prepareMigrationAndCipher(); err != nil {
		t.Fatalf("second prepare: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(backupRoot, "backups"))
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	var found int
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "octarq-premigrate-") {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("want one pre-migration backup, found %d in %v", found, entries)
	}

	version, err := db.SchemaVersionOf(a.gdb)
	if err != nil {
		t.Fatalf("SchemaVersionOf: %v", err)
	}
	if version != db.SchemaVersion {
		t.Fatalf("recorded version %d after the upgrade, want %d", version, db.SchemaVersion)
	}
}
