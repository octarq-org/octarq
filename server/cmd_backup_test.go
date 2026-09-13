package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/db"
	"gorm.io/gorm"
)

func TestCLIBackupAndRestoreCommands(t *testing.T) {
	tempDir := t.TempDir()
	sourceDBPath := filepath.Join(tempDir, "octarq_cli_test.db")
	backupOutPath := filepath.Join(tempDir, "cli_backup.db")

	// Set environment to use the temporary SQLite database
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", sourceDBPath)
	t.Setenv("OCTARQ_SECRET_KEY", "test-secret-key-16-bytes")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "test-admin-password")

	gdb, err := gorm.Open(sqlite.Open(sourceDBPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test sqlite db: %v", err)
	}

	type DummyItem struct {
		ID    uint   `gorm:"primaryKey"`
		Value string `gorm:"size:255"`
	}
	if err := gdb.AutoMigrate(&DummyItem{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	gdb.Create(&DummyItem{Value: "InitialValue"})
	sqlDB, _ := gdb.DB()
	sqlDB.Close()

	exitCode := runBackupCommand([]string{"--out", backupOutPath})
	if exitCode != 0 {
		t.Fatalf("runBackupCommand failed with exit code %d", exitCode)
	}

	if err := db.VerifySQLiteIntegrity(backupOutPath); err != nil {
		t.Fatalf("backup file integrity check failed: %v", err)
	}

	gdb2, err := gorm.Open(sqlite.Open(sourceDBPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db for mutation: %v", err)
	}
	gdb2.Model(&DummyItem{}).Where("id = 1").Update("value", "MutatedValue")
	sqlDB2, _ := gdb2.DB()
	sqlDB2.Close()

	restoreExitCode := runRestoreCommand([]string{"--in", backupOutPath, "--yes"})
	if restoreExitCode != 0 {
		t.Fatalf("runRestoreCommand failed with exit code %d", restoreExitCode)
	}

	gdbRestored, err := gorm.Open(sqlite.Open(sourceDBPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open restored db: %v", err)
	}
	defer func() {
		db, _ := gdbRestored.DB()
		db.Close()
	}()

	var item DummyItem
	if err := gdbRestored.First(&item).Error; err != nil {
		t.Fatalf("failed to find item in restored db: %v", err)
	}
	if item.Value != "InitialValue" {
		t.Errorf("expected value 'InitialValue', got %q", item.Value)
	}

	files, _ := os.ReadDir(".")
	for _, f := range files {
		if strings.HasPrefix(f.Name(), "octarq-backup-before-restore-") {
			_ = os.Remove(f.Name())
		}
	}
}
