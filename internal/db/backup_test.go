package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/config"
	"gorm.io/gorm"
)

func TestParsePostgresDSN(t *testing.T) {
	tests := []struct {
		name      string
		dsn       string
		want      PostgresConfig
		wantErr   bool
		errString string
	}{
		{
			name: "URL format",
			dsn:  "postgres://admin:secret123@db.example.com:5433/octarq_prod?sslmode=require",
			want: PostgresConfig{
				Host:     "db.example.com",
				Port:     "5433",
				User:     "admin",
				Password: "secret123",
				DBName:   "octarq_prod",
				SSLMode:  "require",
			},
		},
		{
			name: "URL format with postgresql scheme",
			dsn:  "postgresql://u:p@h:1234/db?sslmode=disable",
			want: PostgresConfig{
				Host:     "h",
				Port:     "1234",
				User:     "u",
				Password: "p",
				DBName:   "db",
				SSLMode:  "disable",
			},
		},
		{
			name: "URL format missing host and port",
			dsn:  "postgres://user:pass@/dbname",
			want: PostgresConfig{
				Host:     "localhost",
				Port:     "5432",
				User:     "user",
				Password: "pass",
				DBName:   "dbname",
			},
		},
		{
			name: "Key-Value format",
			dsn:  "host=localhost port=5432 user=postgres password=mysecret dbname=octarq_db sslmode=disable",
			want: PostgresConfig{
				Host:     "localhost",
				Port:     "5432",
				User:     "postgres",
				Password: "mysecret",
				DBName:   "octarq_db",
				SSLMode:  "disable",
			},
		},
		{
			name: "Key-Value format with quotes",
			dsn:  `host='db.h' port='9999' dbname='mydb' password='p@ss'`,
			want: PostgresConfig{
				Host:     "db.h",
				Port:     "9999",
				User:     "",
				Password: "p@ss",
				DBName:   "mydb",
				SSLMode:  "",
			},
		},
		{
			name: "Key-Value format with stray tokens",
			dsn:  "stray-token host=realhost dbname=octarq",
			want: PostgresConfig{
				Host:     "realhost",
				Port:     "5432",
				User:     "",
				Password: "",
				DBName:   "octarq",
				SSLMode:  "",
			},
		},
		{
			name:      "Empty string",
			dsn:       "",
			wantErr:   true,
			errString: "empty DSN",
		},
		{
			name:      "Missing dbname error",
			dsn:       "host=localhost user=postgres",
			wantErr:   true,
			errString: "missing dbname in DSN",
		},
		{
			name:      "Invalid URL",
			dsn:       "postgres://:invalid-url/%zz",
			wantErr:   true,
			errString: "parse postgres URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePostgresDSN(tt.dsn)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParsePostgresDSN() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if !strings.Contains(err.Error(), tt.errString) {
					t.Errorf("ParsePostgresDSN() error = %v, want errString %v", err, tt.errString)
				}
				return
			}
			if got != tt.want {
				t.Errorf("ParsePostgresDSN() = %+v, want %+v", got, tt.want)
			}
		})
	}
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
