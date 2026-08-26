package db

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/config"
	"gorm.io/gorm"
)

func TestParsePostgresDSN(t *testing.T) {
	t.Run("URL format", func(t *testing.T) {
		dsn := "postgres://admin:secret123@db.example.com:5433/octarq_prod?sslmode=require"
		cfg, err := ParsePostgresDSN(dsn)
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		if cfg.Host != "db.example.com" {
			t.Errorf("expected host db.example.com, got %q", cfg.Host)
		}
		if cfg.Port != "5433" {
			t.Errorf("expected port 5433, got %q", cfg.Port)
		}
		if cfg.User != "admin" {
			t.Errorf("expected user admin, got %q", cfg.User)
		}
		if cfg.Password != "secret123" {
			t.Errorf("expected password secret123, got %q", cfg.Password)
		}
		if cfg.DBName != "octarq_prod" {
			t.Errorf("expected dbname octarq_prod, got %q", cfg.DBName)
		}
		if cfg.SSLMode != "require" {
			t.Errorf("expected sslmode require, got %q", cfg.SSLMode)
		}
	})

	t.Run("Key-Value format", func(t *testing.T) {
		dsn := "host=localhost port=5432 user=postgres password=mysecret dbname=octarq_db sslmode=disable"
		cfg, err := ParsePostgresDSN(dsn)
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		if cfg.Host != "localhost" {
			t.Errorf("expected host localhost, got %q", cfg.Host)
		}
		if cfg.Port != "5432" {
			t.Errorf("expected port 5432, got %q", cfg.Port)
		}
		if cfg.User != "postgres" {
			t.Errorf("expected user postgres, got %q", cfg.User)
		}
		if cfg.Password != "mysecret" {
			t.Errorf("expected password mysecret, got %q", cfg.Password)
		}
		if cfg.DBName != "octarq_db" {
			t.Errorf("expected dbname octarq_db, got %q", cfg.DBName)
		}
		if cfg.SSLMode != "disable" {
			t.Errorf("expected sslmode disable, got %q", cfg.SSLMode)
		}
	})

	t.Run("Missing dbname error", func(t *testing.T) {
		dsn := "host=localhost user=postgres"
		_, err := ParsePostgresDSN(dsn)
		if err == nil {
			t.Fatal("expected error for missing dbname, got nil")
			return
		}
	})
}

func TestSQLiteBackupAndRestore(t *testing.T) {
	tempDir := t.TempDir()
	sourceDBPath := filepath.Join(tempDir, "source.db")
	backupPath := filepath.Join(tempDir, "backup.db")
	restoredDBPath := filepath.Join(tempDir, "restored.db")

	// 1. Seed a source database with a test table and row
	gdb, err := gorm.Open(sqlite.Open(sourceDBPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open source db: %v", err)
	}

	type TestItem struct {
		ID   uint   `gorm:"primaryKey"`
		Name string `gorm:"size:255"`
	}
	if err := gdb.AutoMigrate(&TestItem{}); err != nil {
		t.Fatalf("failed to migrate test table: %v", err)
	}
	gdb.Create(&TestItem{Name: "BackupTestItem"})

	sqlDB, _ := gdb.DB()
	sqlDB.Close()

	// 2. Perform Backup
	cfg := &config.Config{
		DBDriver: "sqlite",
		DBDSN:    sourceDBPath,
	}

	if err := Backup(cfg, backupPath); err != nil {
		t.Fatalf("Backup failed: %v", err)
	}

	// 3. Verify backup file existence and integrity
	info, err := os.Stat(backupPath)
	if err != nil {
		t.Fatalf("backup file stat failed: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("backup file size is 0 bytes")
	}

	if err := VerifySQLiteIntegrity(backupPath); err != nil {
		t.Fatalf("VerifySQLiteIntegrity failed: %v", err)
	}

	// 4. Perform Restore to a new location
	restoreCfg := &config.Config{
		DBDriver: "sqlite",
		DBDSN:    restoredDBPath,
	}
	if err := Restore(restoreCfg, backupPath); err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	// 5. Verify restored database has the data
	restoredDB, err := gorm.Open(sqlite.Open(restoredDBPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open restored db: %v", err)
	}
	defer func() {
		db, _ := restoredDB.DB()
		db.Close()
	}()

	var item TestItem
	if err := restoredDB.First(&item).Error; err != nil {
		t.Fatalf("failed to query item from restored db: %v", err)
	}
	if item.Name != "BackupTestItem" {
		t.Errorf("expected item name BackupTestItem, got %q", item.Name)
	}
}

func TestVerifySQLiteIntegrity_Errors(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("Non-existent file", func(t *testing.T) {
		err := VerifySQLiteIntegrity(filepath.Join(tempDir, "missing.db"))
		if err == nil {
			t.Fatal("expected error for non-existent file")
			return
		}
	})

	t.Run("Empty file", func(t *testing.T) {
		emptyPath := filepath.Join(tempDir, "empty.db")
		os.WriteFile(emptyPath, []byte{}, 0644)
		err := VerifySQLiteIntegrity(emptyPath)
		if err == nil {
			t.Fatal("expected error for 0-byte file")
			return
		}
	})
}

func TestDefaultBackupFilename(t *testing.T) {
	now := time.Date(2026, 7, 25, 18, 0, 0, 0, time.UTC)
	sqliteName := DefaultBackupFilename("sqlite", now)
	if sqliteName != "octarq-backup-20260725-180000.db" {
		t.Errorf("unexpected sqlite backup filename: %s", sqliteName)
	}

	postgresName := DefaultBackupFilename("postgres", now)
	if postgresName != "octarq-backup-20260725-180000.sql" {
		t.Errorf("unexpected postgres backup filename: %s", postgresName)
	}
}
