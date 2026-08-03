package db

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

func TestDBOpenAndMigrate(t *testing.T) {
	t.Parallel()

	// 1. Open SQLite (valid)
	cfg := &config.Config{
		DBDriver: "sqlite",
		DBDSN:    "file::memory:?cache=shared",
	}
	gdb, err := Open(cfg)
	if err != nil {
		t.Fatalf("expected no error opening DB, got %v", err)
	}

	// 2. Open unsupported driver
	badCfg := &config.Config{
		DBDriver: "invalid",
	}
	if _, err := Open(badCfg); err == nil {
		t.Error("expected error for unsupported driver, got nil")
	}

	// 3. Migrate with legacy data
	// Seed legacy setting
	gdb.AutoMigrate(&models.Setting{}, &models.WorkspaceSetting{}, &models.OrgMember{}, &models.User{}, &models.Session{})
	gdb.Create(&models.Setting{Key: "catch_all", Value: "true"})
	gdb.Create(&models.OrgMember{OrgID: 1, UserID: 42, Role: "owner"})
	gdb.Create(&models.Session{UserID: 42, OrgID: 0, Token: "unscoped-session-token"})

	type ExtraModel struct {
		ID   uint   `gorm:"primaryKey"`
		Name string `gorm:"size:255"`
	}
	err = Migrate(gdb, &ExtraModel{})
	if err != nil {
		t.Errorf("expected no error migrating DB, got %v", err)
	}

	// Verify setting was migrated to WorkspaceSetting
	var ws models.WorkspaceSetting
	if err := gdb.Where("org_id = ? AND key = ?", 1, "catch_all").First(&ws).Error; err != nil {
		t.Errorf("expected catch_all setting migrated to workspace 1, got error: %v", err)
	}

	// Verify user 42 is flagged as instance admin
	var user models.User
	if err := gdb.Where("id = ?", 42).First(&user).Error; err == nil {
		if !user.IsInstanceAdmin {
			t.Errorf("expected user 42 to be flagged as instance admin")
		}
	}

	// Verify orgID 0 session was cleaned up
	var count int64
	gdb.Model(&models.Session{}).Where("token = ?", "unscoped-session-token").Count(&count)
	if count != 0 {
		t.Errorf("expected org 0 session to be deleted, found count %d", count)
	}
}

func TestParsePostgresDSNEdgeCases(t *testing.T) {
	t.Parallel()

	// Empty
	if _, err := ParsePostgresDSN(""); err == nil {
		t.Error("expected error for empty DSN")
	}

	// URL format
	cfg, err := ParsePostgresDSN("postgres://usr:pwd@dbhost:5433/octarq_db?sslmode=disable")
	if err != nil {
		t.Fatalf("unexpected error parsing URL DSN: %v", err)
	}
	if cfg.Host != "dbhost" || cfg.Port != "5433" || cfg.User != "usr" || cfg.Password != "pwd" || cfg.DBName != "octarq_db" || cfg.SSLMode != "disable" {
		t.Errorf("parsed URL config mismatch: %+v", cfg)
	}

	// Key-value format
	cfg2, err := ParsePostgresDSN("host=localhost port=5432 user=myuser password=mypass dbname=mydb sslmode=require")
	if err != nil {
		t.Fatalf("unexpected error parsing KV DSN: %v", err)
	}
	if cfg2.Host != "localhost" || cfg2.User != "myuser" || cfg2.DBName != "mydb" || cfg2.SSLMode != "require" {
		t.Errorf("parsed KV config mismatch: %+v", cfg2)
	}

	// Missing dbname
	if _, err := ParsePostgresDSN("host=localhost port=5432"); err == nil {
		t.Error("expected error for KV DSN missing dbname")
	}
}

func TestBackupAndRestoreEdgeCases(t *testing.T) {
	t.Parallel()

	fnPg := DefaultBackupFilename("postgres", time.Now())
	if filepath.Ext(fnPg) != ".sql" {
		t.Errorf("expected .sql extension for postgres backup, got %q", fnPg)
	}

	fnLite := DefaultBackupFilename("sqlite", time.Now())
	if filepath.Ext(fnLite) != ".db" {
		t.Errorf("expected .db extension for sqlite backup, got %q", fnLite)
	}

	// Backup unsupported driver
	badCfg := &config.Config{DBDriver: "mysql"}
	if err := Backup(badCfg, "out.db"); err == nil {
		t.Error("expected Backup error for unsupported driver")
	}

	// Restore non-existent file
	if err := Restore(&config.Config{DBDriver: "sqlite"}, "nonexistent.db"); err == nil {
		t.Error("expected Restore error for non-existent file")
	}

	// Verify empty file
	tmpFile := filepath.Join(t.TempDir(), "empty.db")
	_ = os.WriteFile(tmpFile, []byte{}, 0644)
	if err := VerifySQLiteIntegrity(tmpFile); err == nil {
		t.Error("expected VerifySQLiteIntegrity error for empty file")
	}

	// Restore corrupted file
	corruptFile := filepath.Join(t.TempDir(), "corrupt.db")
	_ = os.WriteFile(corruptFile, []byte("not a sqlite database file"), 0644)
	if err := Restore(&config.Config{DBDriver: "sqlite", DBDSN: tmpFile}, corruptFile); err == nil {
		t.Error("expected restore error for corrupt backup file")
	}
}

func TestBackupAndRestoreSQLiteSuccess(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "original.db")
	backupPath := filepath.Join(dir, "backup.db")
	restorePath := filepath.Join(dir, "restored.db")

	// Create and populate original DB
	gdb, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	gdb.AutoMigrate(&models.Setting{})
	gdb.Create(&models.Setting{Key: "k1", Value: "v1"})
	sqlDB, _ := gdb.DB()
	sqlDB.Close()

	// Backup
	cfg := &config.Config{DBDriver: "sqlite", DBDSN: dbPath}
	if err := Backup(cfg, backupPath); err != nil {
		t.Fatalf("Backup failed: %v", err)
	}

	if err := VerifySQLiteIntegrity(backupPath); err != nil {
		t.Fatalf("VerifySQLiteIntegrity failed on valid backup: %v", err)
	}

	// Restore to target
	cfgRestore := &config.Config{DBDriver: "sqlite", DBDSN: restorePath}
	if err := Restore(cfgRestore, backupPath); err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	// Verify restored DB contents
	rdb, err := gorm.Open(sqlite.Open(restorePath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open restored db: %v", err)
	}
	var s models.Setting
	if err := rdb.Where("key = ?", "k1").First(&s).Error; err != nil || s.Value != "v1" {
		t.Errorf("restored DB value mismatch: got %+v, err=%v", s, err)
	}
}
