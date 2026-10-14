package db

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/octarq-org/octarq/server/config"
)

// Restore restores a database from the specified input file for SQLite, Postgres, or MySQL.
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
	case "mysql":
		return restoreMySQL(cfg.DBDSN, inputPath)
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
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create database directory: %w", err)
		}
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

func restoreMySQL(dsn, inputPath string) error {
	myCfg, err := ParseMySQLDSN(dsn)
	if err != nil {
		return fmt.Errorf("invalid mysql DSN: %w", err)
	}

	mysqlClientPath, err := exec.LookPath("mysql")
	if err != nil {
		return fmt.Errorf("mysql command not found in PATH; please install mysql-client / default-mysql-client / mariadb-client")
	}

	in, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open backup file: %w", err)
	}
	defer in.Close()

	var args []string
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
	args = append(args, myCfg.DBName)

	cmd := exec.Command(mysqlClientPath, args...)
	cmd.Stdin = in
	env := os.Environ()
	if myCfg.Password != "" {
		env = append(env, "MYSQL_PWD="+myCfg.Password)
	}
	cmd.Env = env

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mysql restore failed (%w): %s", err, string(output))
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
