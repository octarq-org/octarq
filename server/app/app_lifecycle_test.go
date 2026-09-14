package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppLifecycleNew(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "app_lifecycle.db")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", dbPath)
	t.Setenv("OCTARQ_SECRET_KEY", "test-secret-key-16-bytes")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "test-admin-pass")

	a, err := New()
	if err != nil {
		t.Fatalf("app.New() failed: %v", err)
	}

	// New() is documented as opening the database but not migrating it. Assert
	// both halves: an absent file would mean db.Open never ran (and "no error"
	// only meant nothing was attempted), while a populated schema would mean
	// migration crept back into New() — Run() owns that, so plugin models
	// registered after New() would be missing from a migration done here.
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("New() returned without opening the database: %v", err)
	}
	var tables int
	if err := a.gdb.Raw(`SELECT count(*) FROM sqlite_master WHERE type='table'`).Scan(&tables).Error; err != nil {
		t.Fatalf("querying schema: %v", err)
	}
	if tables != 0 {
		t.Errorf("New() migrated %d tables; migration belongs to Run(), after plugins register their models", tables)
	}

	if got := a.WithWebFS(os.DirFS(tempDir)); got != a {
		t.Error("WithWebFS must return the same app for chaining")
	}
	if a.webFS == nil {
		t.Error("WithWebFS did not record the override, so the embedded dashboard would still be served")
	}
}
