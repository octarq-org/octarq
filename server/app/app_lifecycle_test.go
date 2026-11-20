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
