package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCoverageLiftBackupDefaultPath(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	_, err = db.Exec("CREATE TABLE sample (id INTEGER PRIMARY KEY);")
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	_ = db.Close()

	cfg := &config.Config{
		DBDriver: "sqlite",
		DBDSN:    dbPath,
	}
	// Backup with empty outputPath exercises default backup filename creation
	err = Backup(cfg, "")
	if err != nil {
		t.Fatalf("backup with default filename failed: %v", err)
	}
	// Default backup filename matches octarq-backup-sqlite-*.db
	matches, _ := filepath.Glob("octarq-backup-sqlite-*.db")
	for _, m := range matches {
		_ = os.Remove(m)
	}
}

func TestCoverageLiftVerifySQLiteIntegrityCorrupt(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "corrupted.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	_, err = db.Exec("CREATE TABLE t(x TEXT);")
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	for i := 0; i < 500; i++ {
		_, _ = db.Exec("INSERT INTO t VALUES(hex(randomblob(500)));")
	}
	_ = db.Close()

	data, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read db: %v", err)
	}
	if len(data) > 4096+100 {
		for i := 4096; i < 4096+100; i++ {
			data[i] = 0xAA
		}
		if err := os.WriteFile(dbPath, data, 0644); err != nil {
			t.Fatalf("write db: %v", err)
		}
	}

	err = VerifySQLiteIntegrity(dbPath)
	if err == nil {
		t.Fatalf("expected integrity check failure, got nil")
	}
	if !strings.Contains(err.Error(), "integrity check returned:") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestCoverageLiftBackupDirCreateError(t *testing.T) {
	tmpDir := t.TempDir()
	fileAsDir := filepath.Join(tmpDir, "file_as_dir")
	if err := os.WriteFile(fileAsDir, []byte("blocker"), 0644); err != nil {
		t.Fatalf("create blocker: %v", err)
	}
	blockedOutput := filepath.Join(fileAsDir, "sub", "backup.sql")

	installFakeTool(t, "pg_dump", `exit 0`)
	pgCfg := &config.Config{
		DBDriver: "postgres",
		DBDSN:    "postgres://user:pass@127.0.0.1:5432/testdb?sslmode=disable",
	}
	err := Backup(pgCfg, blockedOutput)
	if err == nil || !strings.Contains(err.Error(), "create backup directory") {
		t.Fatalf("expected create backup directory error for postgres, got %v", err)
	}

	installFakeTool(t, "mysqldump", `exit 0`)
	myCfg := &config.Config{
		DBDriver: "mysql",
		DBDSN:    "user:pass@tcp(127.0.0.1:3306)/testdb",
	}
	err = Backup(myCfg, blockedOutput)
	if err == nil || !strings.Contains(err.Error(), "create backup directory") {
		t.Fatalf("expected create backup directory error for mysql, got %v", err)
	}
}

func TestCoverageLiftMySQLUnixSocket(t *testing.T) {
	installFakeTool(t, "mysqldump", `exit 0`)
	installFakeTool(t, "mysql", `exit 0`)

	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "out.sql")
	inFile := filepath.Join(tmpDir, "in.sql")
	if err := os.WriteFile(inFile, []byte("SELECT 1;"), 0644); err != nil {
		t.Fatalf("write in.sql: %v", err)
	}

	unixDSN := "user:pass@unix(/tmp/fake.sock)/testdb"
	myCfg := &config.Config{
		DBDriver: "mysql",
		DBDSN:    unixDSN,
	}
	if err := Backup(myCfg, outFile); err != nil {
		t.Fatalf("backup with unix socket failed: %v", err)
	}
	if err := Restore(myCfg, inFile); err != nil {
		t.Fatalf("restore with unix socket failed: %v", err)
	}
}

func TestCoverageLiftRestoreErrors(t *testing.T) {
	// 1. restorePostgres with .dump suffix when pg_restore not in PATH
	t.Setenv("PATH", t.TempDir())
	pgCfg := &config.Config{
		DBDriver: "postgres",
		DBDSN:    "postgres://user:pass@127.0.0.1:5432/testdb",
	}
	tmpDir := t.TempDir()
	dumpFile := filepath.Join(tmpDir, "data.dump")
	if err := os.WriteFile(dumpFile, []byte("dumpcontent"), 0644); err != nil {
		t.Fatalf("create dump file: %v", err)
	}
	err := Restore(pgCfg, dumpFile)
	if err == nil || !strings.Contains(err.Error(), "pg_restore command not found") {
		t.Fatalf("expected pg_restore not found error, got: %v", err)
	}

	// 2. restoreMySQL open error
	installFakeTool(t, "mysql", `exit 0`)
	myCfg := &config.Config{
		DBDriver: "mysql",
		DBDSN:    "user:pass@tcp(127.0.0.1:3306)/testdb",
	}
	err = restoreMySQL(myCfg.DBDSN, filepath.Join(tmpDir, "nonexistent.sql"))
	if err == nil || !strings.Contains(err.Error(), "open backup file") {
		t.Fatalf("expected open backup file error, got: %v", err)
	}

	// 3. restoreSQLite rename failure (target file is an existing directory)
	validDB := filepath.Join(tmpDir, "valid.db")
	db, err := sql.Open("sqlite", validDB)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	_, _ = db.Exec("CREATE TABLE s (id INT);")
	_ = db.Close()

	targetDir := filepath.Join(tmpDir, "target_is_dir")
	if err := os.Mkdir(targetDir, 0755); err != nil {
		t.Fatalf("mkdir targetDir: %v", err)
	}
	err = restoreSQLite(targetDir, validDB)
	if err == nil || !strings.Contains(err.Error(), "replace database file") {
		t.Fatalf("expected replace database file error, got: %v", err)
	}
}

func TestCoverageLiftMigrateAndBackfill(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migrate.db")
	gdb, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err := gdb.AutoMigrate(&models.Setting{}, &models.WorkspaceSetting{}, &tenantDomainRow{}, &models.Org{}); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}

	gdb.Create(&models.Setting{Key: models.BaseDomainSetting, Value: "example.com"})

	// Seed settings and workspace settings to exercise lines 114, 126, 129
	gdb.Create(&models.Setting{Key: "catch_all", Value: "val1"})
	gdb.Create(&models.Setting{Key: "auto_wrap_links", Value: "val2"})
	gdb.Create(&models.WorkspaceSetting{OrgID: 1, Key: "catch_all", Value: "existing_val"})

	// Also seed domains for backfillTenantSubdomainLinkHosts
	// 1. domain with for_link = false and len(LinkHosts) > 0 (exercises line 233)
	// 2. domain with for_link = false, empty LinkHosts, but nonexistent OrgID (exercises line 237)
	gdb.Create(&models.Org{Slug: "myorg"})
	gdb.Create(&tenantDomainRow{
		ID:        101,
		OrgID:     1,
		Name:      "sub1.example.com",
		ForLink:   false,
		LinkHosts: models.HostList{{Host: "sub1.example.com", Enabled: true}},
	})
	gdb.Create(&tenantDomainRow{
		ID:        102,
		OrgID:     9999, // Unknown org
		Name:      "sub2.example.com",
		ForLink:   false,
		LinkHosts: models.HostList{},
	})

	if err := Migrate(gdb); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Check migrated setting
	var ws models.WorkspaceSetting
	if err := gdb.Where("org_id = 1 AND key = ?", "auto_wrap_links").First(&ws).Error; err != nil {
		t.Fatalf("expected migrated auto_wrap_links setting, got: %v", err)
	}
}
