package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDBLoggerConfig(t *testing.T) {
	t.Run("development keeps record-not-found logging", func(t *testing.T) {
		cfg := dbLoggerConfig(false)
		if cfg.LogLevel != logger.Warn {
			t.Errorf("LogLevel = %v, want Warn", cfg.LogLevel)
		}
		if cfg.IgnoreRecordNotFoundError {
			t.Error("IgnoreRecordNotFoundError = true, want false in development")
		}
	})

	t.Run("production silences record-not-found, keeps errors and slow queries", func(t *testing.T) {
		cfg := dbLoggerConfig(true)
		if cfg.LogLevel != logger.Warn {
			t.Errorf("LogLevel = %v, want Warn (slow queries stay surfaced)", cfg.LogLevel)
		}
		if !cfg.IgnoreRecordNotFoundError {
			t.Error("IgnoreRecordNotFoundError = false, want true in production")
		}
		if cfg.SlowThreshold != 200*time.Millisecond {
			t.Errorf("SlowThreshold = %v, want 200ms", cfg.SlowThreshold)
		}
	})
}

func TestOpenSQLiteAppliesWALAndSingleConnPool(t *testing.T) {
	dir := t.TempDir()

	openAndCheck := func(t *testing.T, dsn string) {
		t.Helper()
		gdb, err := Open(&config.Config{DBDriver: "sqlite", DBDSN: dsn})
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		sqlDB, err := gdb.DB()
		if err != nil {
			t.Fatalf("DB(): %v", err)
		}
		if got := sqlDB.Stats().MaxOpenConnections; got != 1 {
			t.Fatalf("MaxOpenConnections = %d, want 1", got)
		}
		var mode string
		if err := sqlDB.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
			t.Fatalf("journal_mode: %v", err)
		}
		if strings.ToLower(mode) != "wal" {
			t.Fatalf("journal_mode = %q, want wal", mode)
		}
	}

	t.Run("plain path", func(t *testing.T) {
		openAndCheck(t, filepath.Join(dir, "plain.db"))
	})
	t.Run("existing pragma in DSN still gets WAL", func(t *testing.T) {
		openAndCheck(t, filepath.Join(dir, "pragma.db")+"?_pragma=busy_timeout(5000)")
	})
}

func TestDBOpenAndMigrate(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		DBDriver: "sqlite",
		DBDSN:    "file::memory:?cache=shared",
	}
	gdb, err := Open(cfg)
	if err != nil {
		t.Fatalf("expected no error opening DB, got %v", err)
	}

	badCfg := &config.Config{
		DBDriver: "invalid",
	}
	if _, err := Open(badCfg); err == nil {
		t.Error("expected error for unsupported driver, got nil")
	}

	mysqlCfg := &config.Config{
		DBDriver: "mysql",
		DBDSN:    "user:password@tcp(127.0.0.1:1)/nonexistent?timeout=100ms",
	}
	_, _ = Open(mysqlCfg)

	gdb.AutoMigrate(&models.Setting{}, &models.WorkspaceSetting{}, &models.OrgMember{}, &models.User{}, &models.Session{})
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

	var user models.User
	if err := gdb.Where("id = ?", 42).First(&user).Error; err == nil {
		if !user.IsInstanceAdmin {
			t.Errorf("expected user 42 to be flagged as instance admin")
		}
		if !user.EmailVerified {
			t.Errorf("expected user 42 to have EmailVerified = true")
		}
	}

	var count int64
	gdb.Model(&models.Session{}).Where("token = ?", "unscoped-session-token").Count(&count)
	if count != 0 {
		t.Errorf("expected org 0 session to be deleted, found count %d", count)
	}
}

func findOrgHost(t *testing.T, gdb *gorm.DB, host, source string) *models.OrgHost {
	t.Helper()
	var row models.OrgHost
	if err := gdb.Where("host = ? AND source = ?", host, source).First(&row).Error; err != nil {
		t.Fatalf("read org host %s from %s: %v", host, source, err)
	}
	return &row
}

type domainProbeRow struct {
	ID      uint   `gorm:"primaryKey"`
	OwnerID uint   `gorm:"column:owner_id"`
	Name    string `gorm:"column:name"`
}

func (domainProbeRow) TableName() string { return "domains" }

func TestBackfillOrgHosts(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := gdb.AutoMigrate(&models.Setting{}, &models.Org{}, &models.OrgHost{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	gdb.Create(&models.Setting{Key: models.BaseDomainSetting, Value: "app.example.com"})
	gdb.Create(&models.Org{ID: 1, Name: "Acme", Slug: "acme9x"})
	gdb.Create(&models.Org{ID: 2, Name: "Globex", Slug: "globex"})

	if err := backfillOrgHosts(gdb); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	tenant := findOrgHost(t, gdb, "acme9x.app.example.com", "core:tenancy")
	if tenant.OrgID != 1 || !tenant.Traffic {
		t.Errorf("tenant host = %+v, want org 1 and traffic true", tenant)
	}
	globex := findOrgHost(t, gdb, "globex.app.example.com", "core:tenancy")
	if globex.OrgID != 2 || !globex.Traffic {
		t.Errorf("globex host = %+v, want org 2 and traffic true", globex)
	}

	var baseRows int64
	gdb.Model(&models.OrgHost{}).Where("host = ?", "app.example.com").Count(&baseRows)
	if baseRows != 0 {
		t.Error("the configured base domain apex must never be bound")
	}

	if err := backfillOrgHosts(gdb); err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	var count int64
	gdb.Model(&models.OrgHost{}).Where("host = ? AND source = ?", "acme9x.app.example.com", "core:tenancy").Count(&count)
	if count != 1 {
		t.Errorf("backfill is not idempotent: %d rows for the tenant host", count)
	}
	if err := gdb.Migrator().CreateTable(&domainProbeRow{}); err != nil {
		t.Fatalf("create domains: %v", err)
	}
	gdb.Create(&domainProbeRow{OwnerID: 1, Name: "legacy.example.com"})
	if err := backfillOrgHosts(gdb); err != nil {
		t.Fatalf("backfill with a domains table: %v", err)
	}
	var pluginOwned int64
	gdb.Model(&models.OrgHost{}).Where("host = ?", "legacy.example.com").Count(&pluginOwned)
	if pluginOwned != 0 {
		t.Error("core must not bind hosts from the plugin-owned domains table")
	}
}

func TestBackfillOrgHostsSkipsOrgSlugsWithoutBase(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := gdb.AutoMigrate(&models.Setting{}, &models.Org{}, &models.OrgHost{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	gdb.Create(&models.Org{ID: 1, Name: "Acme", Slug: "acme9x"})

	if err := backfillOrgHosts(gdb); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var slugRows int64
	gdb.Model(&models.OrgHost{}).Where("host = ?", "acme9x.").Count(&slugRows)
	if slugRows != 0 {
		t.Error("org slugs must not be bound without a base domain")
	}
	var total int64
	gdb.Model(&models.OrgHost{}).Count(&total)
	if total != 0 {
		t.Errorf("core bound %d hosts with no base domain configured", total)
	}
}

func TestBackupAndRestoreEdgeCases(t *testing.T) {
	t.Parallel()

	fnPg := DefaultBackupFilename("postgres", time.Now())
	if filepath.Ext(fnPg) != ".sql" {
		t.Errorf("expected .sql extension for postgres backup, got %q", fnPg)
	}

	fnMy := DefaultBackupFilename("mysql", time.Now())
	if filepath.Ext(fnMy) != ".sql" {
		t.Errorf("expected .sql extension for mysql backup, got %q", fnMy)
	}

	fnLite := DefaultBackupFilename("sqlite", time.Now())
	if filepath.Ext(fnLite) != ".db" {
		t.Errorf("expected .db extension for sqlite backup, got %q", fnLite)
	}

	badCfg := &config.Config{DBDriver: "unsupported"}
	if err := Backup(badCfg, "out.db"); err == nil {
		t.Error("expected Backup error for unsupported driver")
	}

	if err := Restore(&config.Config{DBDriver: "sqlite"}, "nonexistent.db"); err == nil {
		t.Error("expected Restore error for non-existent file")
	}

	tmpFile := filepath.Join(t.TempDir(), "empty.db")
	_ = os.WriteFile(tmpFile, []byte{}, 0644)
	if err := VerifySQLiteIntegrity(tmpFile); err == nil {
		t.Error("expected VerifySQLiteIntegrity error for empty file")
	}

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

	gdb, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	gdb.AutoMigrate(&models.Setting{})
	gdb.Create(&models.Setting{Key: "k1", Value: "v1"})
	sqlDB, _ := gdb.DB()
	sqlDB.Close()

	cfg := &config.Config{DBDriver: "sqlite", DBDSN: dbPath}
	if err := Backup(cfg, backupPath); err != nil {
		t.Fatalf("Backup failed: %v", err)
	}

	if err := VerifySQLiteIntegrity(backupPath); err != nil {
		t.Fatalf("VerifySQLiteIntegrity failed on valid backup: %v", err)
	}

	cfgRestore := &config.Config{DBDriver: "sqlite", DBDSN: restorePath}
	if err := Restore(cfgRestore, backupPath); err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	rdb, err := gorm.Open(sqlite.Open(restorePath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open restored db: %v", err)
	}
	var s models.Setting
	if err := rdb.Where("key = ?", "k1").First(&s).Error; err != nil || s.Value != "v1" {
		t.Errorf("restored DB value mismatch: got %+v, err=%v", s, err)
	}
}
