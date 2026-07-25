package db

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/config"
	"gorm.io/gorm"
)

// PostgresConfig holds parsed connection parameters from a Postgres DSN.
type PostgresConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// ParsePostgresDSN parses a Postgres DSN string in either URL format
// (postgres://user:pass@host:port/dbname?sslmode=...) or key-value format
// (host=localhost port=5432 user=postgres password=secret dbname=octarq).
func ParsePostgresDSN(dsn string) (PostgresConfig, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return PostgresConfig{}, fmt.Errorf("empty DSN")
	}

	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return PostgresConfig{}, fmt.Errorf("parse postgres URL: %w", err)
		}
		host := u.Hostname()
		if host == "" {
			host = "localhost"
		}
		port := u.Port()
		if port == "" {
			port = "5432"
		}
		pass, _ := u.User.Password()
		dbname := strings.TrimPrefix(u.Path, "/")
		sslmode := u.Query().Get("sslmode")

		return PostgresConfig{
			Host:     host,
			Port:     port,
			User:     u.User.Username(),
			Password: pass,
			DBName:   dbname,
			SSLMode:  sslmode,
		}, nil
	}

	// Key-value format: host=localhost port=5432 user=postgres password=secret dbname=octarq
	cfg := PostgresConfig{Host: "localhost", Port: "5432"}
	parts := strings.Fields(dsn)
	for _, part := range parts {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		v = strings.Trim(v, "'\"")
		switch k {
		case "host":
			cfg.Host = v
		case "port":
			cfg.Port = v
		case "user":
			cfg.User = v
		case "password":
			cfg.Password = v
		case "dbname":
			cfg.DBName = v
		case "sslmode":
			cfg.SSLMode = v
		}
	}
	if cfg.DBName == "" {
		return PostgresConfig{}, fmt.Errorf("missing dbname in DSN")
	}
	return cfg, nil
}

// DefaultBackupFilename returns a timestamped backup filename for the given driver.
func DefaultBackupFilename(driver string, now time.Time) string {
	ts := now.Format("20060102-150405")
	if driver == "postgres" {
		return fmt.Sprintf("octarq-backup-%s.sql", ts)
	}
	return fmt.Sprintf("octarq-backup-%s.db", ts)
}

// Backup performs an online, non-locking backup for SQLite or Postgres.
func Backup(cfg *config.Config, outputPath string) error {
	if outputPath == "" {
		outputPath = DefaultBackupFilename(cfg.DBDriver, time.Now())
	}

	switch cfg.DBDriver {
	case "sqlite":
		return backupSQLite(cfg.DBDSN, outputPath)
	case "postgres":
		return backupPostgres(cfg.DBDSN, outputPath)
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
		return fmt.Errorf("pg_dump command not found in PATH; please install postgresql-client / pg_dump")
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

// Restore restores a database from the specified input file for SQLite or Postgres.
func Restore(cfg *config.Config, inputPath string) error {
	info, err := os.Stat(inputPath)
	if err != nil {
		return fmt.Errorf("cannot find input backup file: %w", err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("input backup file is empty (0 bytes)")
	}

	switch cfg.DBDriver {
	case "sqlite":
		return restoreSQLite(cfg.DBDSN, inputPath)
	case "postgres":
		return restorePostgres(cfg.DBDSN, inputPath)
	default:
		return fmt.Errorf("unsupported driver %q", cfg.DBDriver)
	}
}

func restoreSQLite(targetDSN, inputPath string) error {
	if err := VerifySQLiteIntegrity(inputPath); err != nil {
		return fmt.Errorf("invalid input backup database: %w", err)
	}

	targetFile := targetDSN
	if idx := strings.Index(targetFile, "?"); idx != -1 {
		targetFile = targetFile[:idx]
	}

	dir := filepath.Dir(targetFile)
	if dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0755)
	}

	tmpTarget := targetFile + ".tmp." + fmt.Sprintf("%d", time.Now().UnixNano())
	if err := copyFile(inputPath, tmpTarget); err != nil {
		return fmt.Errorf("copy backup to temp target: %w", err)
	}

	if err := os.Rename(tmpTarget, targetFile); err != nil {
		if copyErr := copyFile(tmpTarget, targetFile); copyErr != nil {
			_ = os.Remove(tmpTarget)
			return fmt.Errorf("replace database file: %w", copyErr)
		}
		_ = os.Remove(tmpTarget)
	}
	return nil
}

func restorePostgres(dsn, inputPath string) error {
	pgCfg, err := ParsePostgresDSN(dsn)
	if err != nil {
		return fmt.Errorf("invalid postgres DSN: %w", err)
	}

	isDump := strings.HasSuffix(inputPath, ".dump") || strings.HasSuffix(inputPath, ".tar")

	var toolName string
	var cmd *exec.Cmd

	if isDump {
		toolName = "pg_restore"
		pgRestorePath, err := exec.LookPath(toolName)
		if err != nil {
			return fmt.Errorf("%s command not found in PATH; please install postgresql-client", toolName)
		}
		cmd = exec.Command(pgRestorePath,
			"-h", pgCfg.Host,
			"-p", pgCfg.Port,
			"-U", pgCfg.User,
			"-d", pgCfg.DBName,
			"--clean",
			inputPath,
		)
	} else {
		toolName = "psql"
		psqlPath, err := exec.LookPath(toolName)
		if err != nil {
			return fmt.Errorf("%s command not found in PATH; please install postgresql-client", toolName)
		}
		cmd = exec.Command(psqlPath,
			"-h", pgCfg.Host,
			"-p", pgCfg.Port,
			"-U", pgCfg.User,
			"-d", pgCfg.DBName,
			"-f", inputPath,
		)
	}

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
		return fmt.Errorf("%s restore failed (%w): %s", toolName, err, string(output))
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
