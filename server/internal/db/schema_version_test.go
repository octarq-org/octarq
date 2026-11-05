package db

import (
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openVersionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestSchemaVersionRoundTrip(t *testing.T) {
	db := openVersionTestDB(t)

	// A schema without a recorded version reads as 0, the state of a database
	// that predates this table.
	if v, err := SchemaVersionOf(db); err != nil || v != 0 {
		t.Fatalf("fresh database: version %d, err %v; want 0", v, err)
	}

	if err := RecordSchemaVersion(db, 1); err != nil {
		t.Fatalf("RecordSchemaVersion: %v", err)
	}
	if err := RecordSchemaVersion(db, 1); err != nil {
		t.Fatalf("recording the same version twice must be a no-op, got %v", err)
	}
	if v, err := SchemaVersionOf(db); err != nil || v != 1 {
		t.Fatalf("version %d, err %v; want 1", v, err)
	}

	if err := RecordSchemaVersion(db, 2); err != nil {
		t.Fatalf("RecordSchemaVersion: %v", err)
	}
	if v, _ := SchemaVersionOf(db); v != 2 {
		t.Fatalf("version %d, want the highest recorded 2", v)
	}
}

func TestCheckSchemaVersion(t *testing.T) {
	db := openVersionTestDB(t)

	upgrade, err := CheckSchemaVersion(db)
	if err != nil || !upgrade {
		t.Fatalf("unversioned database: upgrade %v, err %v; want true", upgrade, err)
	}

	if err := RecordSchemaVersion(db, SchemaVersion); err != nil {
		t.Fatalf("RecordSchemaVersion: %v", err)
	}
	upgrade, err = CheckSchemaVersion(db)
	if err != nil || upgrade {
		t.Fatalf("current database: upgrade %v, err %v; want no upgrade", upgrade, err)
	}

	if err := RecordSchemaVersion(db, SchemaVersion+1); err != nil {
		t.Fatalf("RecordSchemaVersion: %v", err)
	}
	if _, err := CheckSchemaVersion(db); !errors.Is(err, ErrSchemaNewer) {
		t.Fatalf("newer database: want ErrSchemaNewer, got %v", err)
	}
}
