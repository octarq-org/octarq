package db

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/config"
	"gorm.io/gorm"
)

// DefaultBackupFilename returns a timestamped backup filename for the given driver.
func DefaultBackupFilename(driver string, now time.Time) string {
	ts := now.Format("20060102-150405")
	if driver == "postgres" || driver == "mysql" {
		return fmt.Sprintf("octarq-backup-%s.sql", ts)
	}
	return fmt.Sprintf("octarq-backup-%s.db", ts)
}

// Backup performs an online, non-locking backup for SQLite, Postgres, or MySQL.
func Backup(cfg *config.Config, outputPath string) error {
	if outputPath == "" {
		outputPath = DefaultBackupFilename(cfg.DBDriver, time.Now())
	}

	switch cfg.DBDriver {
	case "sqlite":
		return backupSQLite(cfg.DBDSN, outputPath)
	case "postgres":
		return backupPostgres(cfg.DBDSN, outputPath)
	case "mysql":
		return backupMySQL(cfg.DBDSN, outputPath)
	default:
		return fmt.Errorf("unsupported driver %q", cfg.DBDriver)
	}
}

func backupSQLite(dsn, outputPath string) error {
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("open sqlite db: %w", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return fmt.Errorf("get sql db: %w", err)
	}
	defer sqlDB.Close()

	dir := filepath.Dir(outputPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create backup directory: %w", err)
		}
	}

	// Remove existing output file if present so VACUUM INTO doesn't fail
	_ = os.Remove(outputPath)

	escapedPath := strings.ReplaceAll(outputPath, "'", "''")
	_, err = sqlDB.Exec(fmt.Sprintf("VACUUM INTO '%s'", escapedPath))
	if err != nil {
		return fmt.Errorf("sqlite VACUUM INTO failed: %w", err)
	}
	return nil
}

func backupPostgres(dsn, outputPath string) error {
	pgCfg, err := ParsePostgresDSN(dsn)
	if err != nil {
		return fmt.Errorf("invalid postgres DSN: %w", err)
	}

	pgDumpPath, err := exec.LookPath("pg_dump")
	if err != nil {
		return fmt.Errorf("pg_dump command not found in PATH; install postgresql-client / pg_dump")
	}

	dir := filepath.Dir(outputPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create backup directory: %w", err)
		}
	}

	cmd := exec.Command(pgDumpPath,
		"-h", pgCfg.Host,
		"-p", pgCfg.Port,
		"-U", pgCfg.User,
		"-d", pgCfg.DBName,
		"-F", "p",
		"-f", outputPath,
	)
	env := os.Environ()
	if pgCfg.Password != "" {
		env = append(env, "PGPASSWORD="+pgCfg.Password)
	}
	if pgCfg.SSLMode != "" {
		env = append(env, "PGSSLMODE="+pgCfg.SSLMode)
	}
	cmd.Env = env

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pg_dump failed (%w): %s", err, string(output))
	}
	return nil
}

func backupMySQL(dsn, outputPath string) error {
	myCfg, err := ParseMySQLDSN(dsn)
	if err != nil {
		return fmt.Errorf("invalid mysql DSN: %w", err)
	}

	dumpPath, err := exec.LookPath("mysqldump")
	if err != nil {
		return fmt.Errorf("mysqldump command not found in PATH; install mysql-client / default-mysql-client / mariadb-client")
	}

	dir := filepath.Dir(outputPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create backup directory: %w", err)
		}
	}

	args := []string{
		"--single-transaction",
		"--quick",
		"--routines",
		"--triggers",
	}
	if myCfg.Protocol == "unix" && myCfg.Socket != "" {
		args = append(args, "--socket", myCfg.Socket)
	} else {
		if myCfg.Host != "" {
			args = append(args, "-h", myCfg.Host)
		}
		if myCfg.Port != "" {
			args = append(args, "-P", myCfg.Port)
		}
	}
	if myCfg.User != "" {
		args = append(args, "-u", myCfg.User)
	}
	args = append(args, "--result-file="+outputPath, myCfg.DBName)

	cmd := exec.Command(dumpPath, args...)
	env := os.Environ()
	if myCfg.Password != "" {
		env = append(env, "MYSQL_PWD="+myCfg.Password)
	}
	cmd.Env = env

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mysqldump failed (%w): %s", err, string(output))
	}
	return nil
}

// VerifySQLiteIntegrity opens a SQLite database file and executes PRAGMA integrity_check.
func VerifySQLiteIntegrity(dbPath string) error {
	info, err := os.Stat(dbPath)
	if err != nil {
		return fmt.Errorf("cannot stat database file: %w", err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("database file is empty (0 bytes)")
	}

	gdb, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("open database file: %w", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return fmt.Errorf("get sql db: %w", err)
	}
	defer sqlDB.Close()

	var result string
	row := sqlDB.QueryRow("PRAGMA integrity_check")
	if err := row.Scan(&result); err != nil {
		return fmt.Errorf("integrity check scan error: %w", err)
	}
	if strings.ToLower(result) != "ok" {
		return fmt.Errorf("integrity check returned: %s", result)
	}
	return nil
}
