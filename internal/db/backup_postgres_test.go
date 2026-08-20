package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/config"
	"gorm.io/gorm"
)

// installFakeTool puts an executable shell script named `name` on PATH and
// returns its full path. The script is invoked by the product's exec.Command,
// so it exercises the full command construction, env and output paths without
// needing a real Postgres server (or the postgresql-client tools).
func installFakeTool(t *testing.T, name, script string) string {
	t.Helper()
	bin := t.TempDir()
	path := filepath.Join(bin, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatalf("write fake tool: %v", err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	return path
}

func postgresCfg(dsn string) *config.Config {
	return &config.Config{DBDriver: "postgres", DBDSN: dsn}
}

func TestBackupPostgresRunsPgDump(t *testing.T) {
	envFile := filepath.Join(t.TempDir(), "env.txt")
	t.Setenv("PGPASSWORD_FILE", envFile)
	installFakeTool(t, "pg_dump", `echo "PGPW=$PGPASSWORD" > "$PGPASSWORD_FILE"; exit 0`)

	dsn := "postgres://admin:sekrit@db.internal:5433/proddb?sslmode=require"
	if err := Backup(postgresCfg(dsn), filepath.Join(t.TempDir(), "out.sql")); err != nil {
		t.Fatalf("Backup(postgres) with stubbed pg_dump: %v", err)
	}
	b, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("read env capture: %v", err)
	}
	if !strings.Contains(string(b), "PGPW=sekrit") {
		t.Errorf("PGPASSWORD never reached the tool: %q", string(b))
	}
}

func TestBackupPostgresToolFailure(t *testing.T) {
	installFakeTool(t, "pg_dump", `echo "cannot connect" >&2; exit 1`)
	err := Backup(postgresCfg("postgres://u:p@h/db"), filepath.Join(t.TempDir(), "out.sql"))
	if err == nil || !strings.Contains(err.Error(), "pg_dump failed") {
		t.Fatalf("expected pg_dump failure error, got %v", err)
	}
}

func TestBackupPostgresToolMissing(t *testing.T) {
	t.Setenv("PATH", "/nonexistent-bin")
	err := Backup(postgresCfg("postgres://u:p@h/db"), "out.sql")
	if err == nil || !strings.Contains(err.Error(), "pg_dump") {
		t.Fatalf("expected missing-tool error, got %v", err)
	}
}

func TestBackupPostgresInvalidDSN(t *testing.T) {
	installFakeTool(t, "pg_dump", "exit 0")
	err := Backup(postgresCfg("host=localhost user=x"), "out.sql") // no dbname
	if err == nil || !strings.Contains(err.Error(), "invalid postgres DSN") {
		t.Fatalf("expected invalid-DSN error, got %v", err)
	}
}

func restoreInput(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRestorePostgresPlainSQL(t *testing.T) {
	installFakeTool(t, "psql", "exit 0")
	input := restoreInput(t, "backup.sql", "SELECT 1;")
	if err := Restore(postgresCfg("postgres://u:p@h/db"), input); err != nil {
		t.Fatalf("Restore(postgres/sql): %v", err)
	}
}

func TestRestorePostgresDumpFile(t *testing.T) {
	installFakeTool(t, "pg_restore", "exit 0")
	input := restoreInput(t, "backup.dump", "binary-ish")
	if err := Restore(postgresCfg("postgres://u:p@h/db"), input); err != nil {
		t.Fatalf("Restore(postgres/dump): %v", err)
	}
}

func TestRestorePostgresToolFailure(t *testing.T) {
	installFakeTool(t, "psql", `echo "syntax error" >&2; exit 1`)
	input := restoreInput(t, "backup.sql", "SELECT 1;")
	err := Restore(postgresCfg("postgres://u:p@h/db"), input)
	if err == nil || !strings.Contains(err.Error(), "restore failed") {
		t.Fatalf("expected restore failure error, got %v", err)
	}
}

func TestRestorePostgresToolMissing(t *testing.T) {
	t.Setenv("PATH", "/nonexistent-bin")
	input := restoreInput(t, "backup.sql", "SELECT 1;")
	err := Restore(postgresCfg("postgres://u:p@h/db"), input)
	if err == nil || !strings.Contains(err.Error(), "psql") {
		t.Fatalf("expected missing-psql error, got %v", err)
	}
}

func TestRestorePostgresInvalidDSN(t *testing.T) {
	installFakeTool(t, "psql", "exit 0")
	input := restoreInput(t, "backup.sql", "SELECT 1;")
	err := Restore(postgresCfg("host=localhost user=x"), input)
	if err == nil || !strings.Contains(err.Error(), "invalid postgres DSN") {
		t.Fatalf("expected invalid-DSN error, got %v", err)
	}
}

func TestRestorePostgresResetPasswordEnv(t *testing.T) {
	envFile := filepath.Join(t.TempDir(), "env.txt")
	t.Setenv("PG_ENV_FILE", envFile)
	t.Setenv("PGSSLMODE_INIT", "verified")
	installFakeTool(t, "psql", `
echo "PGPW=$PGPASSWORD" > "$PG_ENV_FILE";
echo "SSLMODE=$PGSSLMODE" >> "$PG_ENV_FILE";
exit 0`)
	input := restoreInput(t, "backup.sql", "SELECT 1;")
	dsn := "host=localhost port=5432 user=admin password=bootstrapped dbname=octarq sslmode=verify-full"
	if err := Restore(postgresCfg(dsn), input); err != nil {
		t.Fatalf("Restore with env capture: %v", err)
	}
	b, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "PGPW=bootstrapped") || !strings.Contains(string(b), "SSLMODE=verify-full") {
		t.Errorf("env not propagated to psql: %q", string(b))
	}
}

func TestParsePostgresDSNPostgresqlSchemeAndQuotes(t *testing.T) {
	cfg, err := ParsePostgresDSN("postgresql://u:p@h:1234/db?sslmode=disable")
	if err != nil {
		t.Fatalf("postgresql:// parse: %v", err)
	}
	if cfg.Port != "1234" || cfg.DBName != "db" {
		t.Errorf("postgresql:// config = %+v", cfg)
	}

	// Key-value DSNs tolerate quoted values (quoting exists to protect
	// values that could otherwise look like keys; whitespace inside a quoted
	// value still cannot survive the whitespace-based split).
	cfg2, err := ParsePostgresDSN(`host='db.h' port='9999' dbname='mydb' password='p@ss'`)
	if err != nil {
		t.Fatalf("quoted KV parse: %v", err)
	}
	if cfg2.Host != "db.h" || cfg2.Port != "9999" || cfg2.Password != "p@ss" {
		t.Errorf("quoted KV config = %+v", cfg2)
	}
}

func TestBackupSQLiteWritesIntoNestedDir(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "src.db")
	gdb, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	type Row struct {
		ID   uint `gorm:"primaryKey"`
		Name string
	}
	gdb.AutoMigrate(&Row{})
	gdb.Create(&Row{Name: "nested"})
	sqlDB, _ := gdb.DB()
	sqlDB.Close()

	// The output path includes a fresh nested directory plus a single quote,
	// exercising both MkdirAll and the VACUUM-path escaping.
	out := filepath.Join(t.TempDir(), "sub/dir", "o'bri'en.db")
	if err := Backup(&config.Config{DBDriver: "sqlite", DBDSN: dbPath}, out); err != nil {
		t.Fatalf("Backup into nested dir: %v", err)
	}
	if err := VerifySQLiteIntegrity(out); err != nil {
		t.Fatalf("backed-up db failed integrity: %v", err)
	}
}

func TestBackupSQLiteErrors(t *testing.T) {
	dir := t.TempDir()

	// MkdirAll fails when a parent path component is a file.
	blocker := filepath.Join(dir, "afile")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := backupSQLite(filepath.Join(dir, "src.db"), filepath.Join(blocker, "nested", "out.db"))
	if err == nil || !strings.Contains(err.Error(), "create backup directory") {
		t.Fatalf("expected mkdir error, got %v", err)
	}

	// VACUUM INTO fails when the destination directory is unwritable.
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o755)

	src := filepath.Join(dir, "src.db")
	gdb, err := gorm.Open(sqlite.Open(src), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	gdb.AutoMigrate(&struct {
		ID uint `gorm:"primaryKey"`
	}{})
	sqlDB, _ := gdb.DB()
	sqlDB.Close()

	err = backupSQLite(src, filepath.Join(locked, "out.db"))
	if err == nil || !strings.Contains(err.Error(), "VACUUM INTO failed") {
		t.Fatalf("expected VACUUM error, got %v", err)
	}
}

func TestRestoreSQLiteDSNWithParams(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.db")
	gdb, err := gorm.Open(sqlite.Open(src), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	type Row struct {
		ID   uint `gorm:"primaryKey"`
		Name string
	}
	gdb.AutoMigrate(&Row{})
	gdb.Create(&Row{Name: "v1"})
	sqlDB, _ := gdb.DB()
	sqlDB.Close()

	backupPath := filepath.Join(t.TempDir(), "backup.db")
	if err := Backup(&config.Config{DBDriver: "sqlite", DBDSN: src}, backupPath); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(t.TempDir(), "restored.db")
	targetDSN := target + "?_pragma=busy_timeout(5000)"
	if err := Restore(&config.Config{DBDriver: "sqlite", DBDSN: targetDSN}, backupPath); err != nil {
		t.Fatalf("Restore with DSN params: %v", err)
	}
	rdb, err := gorm.Open(sqlite.Open(target), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	var r Row
	if err := rdb.First(&r).Error; err != nil || r.Name != "v1" {
		t.Errorf("restored row = %+v, err=%v", r, err)
	}
}

func TestRestoreSQLiteCopyFailure(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.db")
	gdb, err := gorm.Open(sqlite.Open(src), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	gdb.AutoMigrate(&struct {
		ID uint `gorm:"primaryKey"`
	}{})
	sqlDB, _ := gdb.DB()
	sqlDB.Close()

	backupPath := filepath.Join(t.TempDir(), "backup.db")
	if err := Backup(&config.Config{DBDriver: "sqlite", DBDSN: src}, backupPath); err != nil {
		t.Fatal(err)
	}

	// Target dir is a file, so writing the temp copy fails.
	blocker := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = Restore(&config.Config{DBDriver: "sqlite", DBDSN: filepath.Join(blocker, "t.db")}, backupPath)
	if err == nil {
		t.Fatal("expected copy failure, got nil")
	}
}

func TestParsePostgresDSNNoSeparatorParts(t *testing.T) {
	// A field without '=' is skipped entirely, not treated as an error.
	cfg, err := ParsePostgresDSN("stray-token host=realhost dbname=octarq")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Host != "realhost" || cfg.DBName != "octarq" {
		t.Errorf("config = %+v", cfg)
	}
}

func TestRestoreMoreEdgeCases(t *testing.T) {
	dir := t.TempDir()

	// Unsupported driver reaches the default branch when the input exists and
	// is non-empty.
	existing := filepath.Join(dir, "input.sql")
	if err := os.WriteFile(existing, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Restore(&config.Config{DBDriver: "invalid"}, existing)
	if err == nil || !strings.Contains(err.Error(), "unsupported driver") {
		t.Fatalf("expected unsupported-driver error, got %v", err)
	}

	// An existing-but-empty input is rejected before any driver logic.
	emptyFile := filepath.Join(dir, "empty.sql")
	if err := os.WriteFile(emptyFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	err = Restore(&config.Config{DBDriver: "sqlite"}, emptyFile)
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty-input error, got %v", err)
	}

	// backupSQLite with a directory as the source DSN fails at open time.
	direntry := filepath.Join(dir, "src.db")
	if err := os.Mkdir(direntry, 0o755); err != nil {
		t.Fatal(err)
	}
	err = backupSQLite(direntry, filepath.Join(dir, "out.db"))
	if err == nil || !strings.Contains(err.Error(), "open sqlite db") {
		t.Fatalf("expected open error for directory DSN, got %v", err)
	}
}

func TestCopyFileErrors(t *testing.T) {
	if err := copyFile(filepath.Join(t.TempDir(), "missing"), filepath.Join(t.TempDir(), "x")); err == nil {
		t.Error("copyFile of a missing source must error")
	}
	src := filepath.Join(t.TempDir(), "src")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, filepath.Join(blocker, "dest")); err == nil {
		t.Error("copyFile with unwritable destination must error")
	}
}

func TestVerifySQLiteIntegrityOnDirectory(t *testing.T) {
	err := VerifySQLiteIntegrity(t.TempDir())
	if err == nil {
		t.Fatal("VerifySQLiteIntegrity on a directory must error")
	}
}

func TestOpenPostgresConnectsOrFails(t *testing.T) {
	// An unreachable DSN must fail fast with an error, covering the postgres
	// branch of Open.
	_, err := Open(&config.Config{
		DBDriver: "postgres",
		DBDSN:    "postgres://u:p@127.0.0.1:1/nonexistent?sslmode=disable",
	})
	if err == nil {
		t.Error("Open with unreachable postgres must error")
	}
}
