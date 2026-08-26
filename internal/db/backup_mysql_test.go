package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/config"
)

func mysqlCfg(dsn string) *config.Config {
	return &config.Config{DBDriver: "mysql", DBDSN: dsn}
}

func TestParseMySQLDSN(t *testing.T) {
	t.Run("Standard Go DSN with TCP host and port", func(t *testing.T) {
		dsn := "root:secret123@tcp(db.example.com:3307)/octarq_prod?charset=utf8mb4&parseTime=True&loc=Local"
		cfg, err := ParseMySQLDSN(dsn)
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		if cfg.Host != "db.example.com" {
			t.Errorf("expected host db.example.com, got %q", cfg.Host)
		}
		if cfg.Port != "3307" {
			t.Errorf("expected port 3307, got %q", cfg.Port)
		}
		if cfg.User != "root" {
			t.Errorf("expected user root, got %q", cfg.User)
		}
		if cfg.Password != "secret123" {
			t.Errorf("expected password secret123, got %q", cfg.Password)
		}
		if cfg.DBName != "octarq_prod" {
			t.Errorf("expected dbname octarq_prod, got %q", cfg.DBName)
		}
		if cfg.Protocol != "tcp" {
			t.Errorf("expected protocol tcp, got %q", cfg.Protocol)
		}
	})

	t.Run("Standard Go DSN with default localhost", func(t *testing.T) {
		dsn := "octarq:mypassword@/octarq_db"
		cfg, err := ParseMySQLDSN(dsn)
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		if cfg.Host != "127.0.0.1" {
			t.Errorf("expected host 127.0.0.1, got %q", cfg.Host)
		}
		if cfg.Port != "3306" {
			t.Errorf("expected port 3306, got %q", cfg.Port)
		}
		if cfg.User != "octarq" {
			t.Errorf("expected user octarq, got %q", cfg.User)
		}
		if cfg.Password != "mypassword" {
			t.Errorf("expected password mypassword, got %q", cfg.Password)
		}
		if cfg.DBName != "octarq_db" {
			t.Errorf("expected dbname octarq_db, got %q", cfg.DBName)
		}
	})

	t.Run("Standard Go DSN with Unix socket", func(t *testing.T) {
		dsn := "octarq:mypass@unix(/var/run/mysqld/mysqld.sock)/octarq_db"
		cfg, err := ParseMySQLDSN(dsn)
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		if cfg.Protocol != "unix" {
			t.Errorf("expected protocol unix, got %q", cfg.Protocol)
		}
		if cfg.Socket != "/var/run/mysqld/mysqld.sock" {
			t.Errorf("expected socket /var/run/mysqld/mysqld.sock, got %q", cfg.Socket)
		}
		if cfg.DBName != "octarq_db" {
			t.Errorf("expected dbname octarq_db, got %q", cfg.DBName)
		}
	})

	t.Run("URL format", func(t *testing.T) {
		dsn := "mysql://admin:pass456@dbhost:3308/octarq_test?charset=utf8mb4"
		cfg, err := ParseMySQLDSN(dsn)
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		if cfg.Host != "dbhost" {
			t.Errorf("expected host dbhost, got %q", cfg.Host)
		}
		if cfg.Port != "3308" {
			t.Errorf("expected port 3308, got %q", cfg.Port)
		}
		if cfg.User != "admin" {
			t.Errorf("expected user admin, got %q", cfg.User)
		}
		if cfg.Password != "pass456" {
			t.Errorf("expected password pass456, got %q", cfg.Password)
		}
		if cfg.DBName != "octarq_test" {
			t.Errorf("expected dbname octarq_test, got %q", cfg.DBName)
		}
	})

	t.Run("Empty DSN error", func(t *testing.T) {
		if _, err := ParseMySQLDSN(""); err == nil {
			t.Fatal("expected error for empty DSN, got nil")
			return
		}
	})

	t.Run("Missing dbname error", func(t *testing.T) {
		if _, err := ParseMySQLDSN("root:secret@tcp(127.0.0.1:3306)/"); err == nil {
			t.Fatal("expected error for missing dbname, got nil")
			return
		}
		if _, err := ParseMySQLDSN("root:secret@tcp(127.0.0.1:3306)"); err == nil {
			t.Fatal("expected error for missing slash and dbname, got nil")
			return
		}
	})
}

func TestBackupMySQLRunsMysqldump(t *testing.T) {
	envFile := filepath.Join(t.TempDir(), "env.txt")
	t.Setenv("MYSQL_PWD_FILE", envFile)
	installFakeTool(t, "mysqldump", `echo "MYSQL_PWD=$MYSQL_PWD" > "$MYSQL_PWD_FILE"; exit 0`)

	dsn := "root:sekrit@tcp(127.0.0.1:3306)/octarq_prod"
	if err := Backup(mysqlCfg(dsn), filepath.Join(t.TempDir(), "out.sql")); err != nil {
		t.Fatalf("Backup(mysql) with stubbed mysqldump: %v", err)
	}
	b, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("read env capture: %v", err)
	}
	if !strings.Contains(string(b), "MYSQL_PWD=sekrit") {
		t.Errorf("MYSQL_PWD never reached mysqldump: %q", string(b))
	}
}

func TestBackupMySQLToolFailure(t *testing.T) {
	installFakeTool(t, "mysqldump", `echo "cannot connect" >&2; exit 1`)
	err := Backup(mysqlCfg("root:pass@tcp(127.0.0.1:3306)/db"), filepath.Join(t.TempDir(), "out.sql"))
	if err == nil || !strings.Contains(err.Error(), "mysqldump failed") {
		t.Fatalf("expected mysqldump failure error, got %v", err)
	}
}

func TestBackupMySQLToolMissing(t *testing.T) {
	t.Setenv("PATH", "/nonexistent-bin")
	err := Backup(mysqlCfg("root:pass@tcp(127.0.0.1:3306)/db"), "out.sql")
	if err == nil || !strings.Contains(err.Error(), "mysqldump") {
		t.Fatalf("expected missing-tool error, got %v", err)
	}
}

func TestRestoreMySQLRunsClient(t *testing.T) {
	envFile := filepath.Join(t.TempDir(), "env.txt")
	t.Setenv("MYSQL_PWD_FILE", envFile)
	installFakeTool(t, "mysql", `echo "MYSQL_PWD=$MYSQL_PWD" > "$MYSQL_PWD_FILE"; exit 0`)

	input := restoreInput(t, "backup.sql", "SELECT 1;")
	dsn := "root:sekrit@tcp(127.0.0.1:3306)/octarq_prod"
	if err := Restore(mysqlCfg(dsn), input); err != nil {
		t.Fatalf("Restore(mysql): %v", err)
	}
	b, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("read env capture: %v", err)
	}
	if !strings.Contains(string(b), "MYSQL_PWD=sekrit") {
		t.Errorf("MYSQL_PWD never reached mysql client: %q", string(b))
	}
}

func TestRestoreMySQLToolFailure(t *testing.T) {
	installFakeTool(t, "mysql", `echo "access denied" >&2; exit 1`)
	input := restoreInput(t, "backup.sql", "SELECT 1;")
	err := Restore(mysqlCfg("root:pass@tcp(127.0.0.1:3306)/db"), input)
	if err == nil || !strings.Contains(err.Error(), "mysql restore failed") {
		t.Fatalf("expected mysql restore failure error, got %v", err)
	}
}

func TestRestoreMySQLToolMissing(t *testing.T) {
	t.Setenv("PATH", "/nonexistent-bin")
	input := restoreInput(t, "backup.sql", "SELECT 1;")
	err := Restore(mysqlCfg("root:pass@tcp(127.0.0.1:3306)/db"), input)
	if err == nil || !strings.Contains(err.Error(), "mysql") {
		t.Fatalf("expected missing-tool error, got %v", err)
	}
}

func TestMySQLBackupAndRestoreErrors(t *testing.T) {
	dir := t.TempDir()

	t.Run("Backup invalid DSN", func(t *testing.T) {
		cfg := &config.Config{
			DBDriver: "mysql",
			DBDSN:    "",
		}
		err := Backup(cfg, filepath.Join(dir, "backup.sql"))
		if err == nil || !strings.Contains(err.Error(), "invalid mysql DSN") {
			t.Fatalf("expected invalid DSN error, got %v", err)
		}
	})

	t.Run("Restore invalid DSN", func(t *testing.T) {
		input := filepath.Join(dir, "input.sql")
		if err := os.WriteFile(input, []byte("CREATE TABLE foo;"), 0644); err != nil {
			t.Fatal(err)
		}
		cfg := &config.Config{
			DBDriver: "mysql",
			DBDSN:    "",
		}
		err := Restore(cfg, input)
		if err == nil || !strings.Contains(err.Error(), "invalid mysql DSN") {
			t.Fatalf("expected invalid DSN error, got %v", err)
		}
	})

	t.Run("Restore empty file", func(t *testing.T) {
		emptyInput := filepath.Join(dir, "empty.sql")
		if err := os.WriteFile(emptyInput, []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
		cfg := &config.Config{
			DBDriver: "mysql",
			DBDSN:    "root:pass@tcp(127.0.0.1:3306)/mydb",
		}
		err := Restore(cfg, emptyInput)
		if err == nil || !strings.Contains(err.Error(), "empty") {
			t.Fatalf("expected empty file error, got %v", err)
		}
	})
}
