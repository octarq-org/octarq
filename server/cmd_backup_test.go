package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/internal/db"
	"gorm.io/gorm"
)

type cliMockS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
}

type cliS3ListResult struct {
	XMLName  xml.Name         `xml:"ListBucketResult"`
	Xmlns    string           `xml:"xmlns,attr"`
	Name     string           `xml:"Name"`
	Contents []cliS3ListEntry `xml:"Contents"`
}

type cliS3ListEntry struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
}

func newCLIMockS3() (*httptest.Server, *cliMockS3) {
	m := &cliMockS3{objects: make(map[string][]byte)}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()

		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		var key string
		if len(parts) > 1 {
			key = strings.Join(parts[1:], "/")
		}

		switch r.Method {
		case http.MethodPut:
			data, _ := io.ReadAll(r.Body)
			m.objects[key] = data
			sum := md5.Sum(data)
			w.Header().Set("ETag", fmt.Sprintf("\"%s\"", hex.EncodeToString(sum[:])))
			w.WriteHeader(http.StatusOK)

		case http.MethodGet:
			if r.URL.Query().Get("list-type") == "2" {
				var entries []cliS3ListEntry
				for k, v := range m.objects {
					sum := md5.Sum(v)
					entries = append(entries, cliS3ListEntry{
						Key:          k,
						LastModified: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
						ETag:         fmt.Sprintf("\"%s\"", hex.EncodeToString(sum[:])),
						Size:         int64(len(v)),
					})
				}
				res := cliS3ListResult{
					Xmlns:    "http://s3.amazonaws.com/doc/2006-03-01/",
					Name:     parts[0],
					Contents: entries,
				}
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				_ = xml.NewEncoder(w).Encode(res)
				return
			}
			data, ok := m.objects[key]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)

		case http.MethodDelete:
			delete(m.objects, key)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	return ts, m
}

func cleanupSafetyBackups() {
	files, _ := os.ReadDir(".")
	for _, f := range files {
		if strings.HasPrefix(f.Name(), "octarq-backup-before-restore-") {
			_ = os.Remove(f.Name())
		}
	}
}

func TestCLIBackupAndRestoreCommands(t *testing.T) {
	defer cleanupSafetyBackups()
	tempDir := t.TempDir()
	sourceDBPath := filepath.Join(tempDir, "octarq_cli_test.db")
	backupOutPath := filepath.Join(tempDir, "cli_backup.db")

	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", sourceDBPath)
	t.Setenv("OCTARQ_SECRET_KEY", "test-secret-key-16-bytes")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "test-admin-password")

	gdb, err := gorm.Open(sqlite.Open(sourceDBPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test sqlite db: %v", err)
	}

	type DummyItem struct {
		ID    uint   `gorm:"primaryKey"`
		Value string `gorm:"size:255"`
	}
	if err := gdb.AutoMigrate(&DummyItem{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	gdb.Create(&DummyItem{Value: "InitialValue"})
	sqlDB, _ := gdb.DB()
	sqlDB.Close()

	exitCode := runBackupCommand([]string{"--out", backupOutPath})
	if exitCode != 0 {
		t.Fatalf("runBackupCommand failed with exit code %d", exitCode)
	}

	if err := db.VerifySQLiteIntegrity(backupOutPath); err != nil {
		t.Fatalf("backup file integrity check failed: %v", err)
	}

	gdb2, err := gorm.Open(sqlite.Open(sourceDBPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db for mutation: %v", err)
	}
	gdb2.Model(&DummyItem{}).Where("id = 1").Update("value", "MutatedValue")
	sqlDB2, _ := gdb2.DB()
	sqlDB2.Close()

	restoreExitCode := runRestoreCommand([]string{"--in", backupOutPath, "--yes"})
	if restoreExitCode != 0 {
		t.Fatalf("runRestoreCommand failed with exit code %d", restoreExitCode)
	}

	gdbRestored, err := gorm.Open(sqlite.Open(sourceDBPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open restored db: %v", err)
	}
	defer func() {
		db, _ := gdbRestored.DB()
		db.Close()
	}()

	var item DummyItem
	if err := gdbRestored.First(&item).Error; err != nil {
		t.Fatalf("failed to find item in restored db: %v", err)
	}
	if item.Value != "InitialValue" {
		t.Errorf("expected value 'InitialValue', got %q", item.Value)
	}
}

func TestCLIRemoteBackupAndRestore(t *testing.T) {
	defer cleanupSafetyBackups()
	ts, mock := newCLIMockS3()
	defer ts.Close()

	tempDir := t.TempDir()
	sourceDBPath := filepath.Join(tempDir, "remote_test.db")
	backupOutPath := filepath.Join(tempDir, "octarq-backup-test.db.enc")

	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", sourceDBPath)
	t.Setenv("OCTARQ_SECRET_KEY", "test-secret-key-32-bytes-secure!")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "test-admin-password")
	t.Setenv("OCTARQ_BACKUP_S3_ENDPOINT", ts.URL)
	t.Setenv("OCTARQ_BACKUP_S3_BUCKET", "cli-test-bucket")
	t.Setenv("OCTARQ_BACKUP_S3_ACCESS_KEY", "cli-access-key")
	t.Setenv("OCTARQ_BACKUP_S3_SECRET_KEY", "cli-secret-key")
	t.Setenv("OCTARQ_BACKUP_S3_REGION", "auto")

	type AppItem struct {
		ID   uint `gorm:"primaryKey"`
		Data string
	}

	gdb, err := gorm.Open(sqlite.Open(sourceDBPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	_ = gdb.AutoMigrate(&AppItem{})
	gdb.Create(&AppItem{Data: "OriginalRemoteData"})
	sdb, _ := gdb.DB()
	sdb.Close()

	// 1. Run backup with --remote
	code := runBackupCommand([]string{"--remote", "--out", backupOutPath})
	if code != 0 {
		t.Fatalf("runBackupCommand --remote failed: code %d", code)
	}

	// Encrypted file must not be valid raw sqlite
	if err := db.VerifySQLiteIntegrity(backupOutPath); err == nil {
		t.Fatal("expected encrypted backup file to fail VerifySQLiteIntegrity")
	}

	// Verify object uploaded to S3
	remoteKey := filepath.Base(backupOutPath)
	if _, ok := mock.objects[remoteKey]; !ok {
		t.Fatalf("expected remote key %q in mock S3", remoteKey)
	}

	// 2. Mutate DB
	gdb2, _ := gorm.Open(sqlite.Open(sourceDBPath), &gorm.Config{})
	gdb2.Model(&AppItem{}).Where("id = 1").Update("data", "MutatedData")
	sdb2, _ := gdb2.DB()
	sdb2.Close()

	// 3. Restore from S3 via latest
	restoreCode := runRestoreCommand([]string{"--from-s3", "latest", "--yes"})
	if restoreCode != 0 {
		t.Fatalf("runRestoreCommand --from-s3 latest failed: code %d", restoreCode)
	}

	// 4. Verify restored DB content
	gdbRestored, err := gorm.Open(sqlite.Open(sourceDBPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open restored db failed: %v", err)
	}
	defer func() {
		db, _ := gdbRestored.DB()
		db.Close()
	}()

	var item AppItem
	if err := gdbRestored.First(&item).Error; err != nil {
		t.Fatalf("find item failed: %v", err)
	}
	if item.Data != "OriginalRemoteData" {
		t.Errorf("expected 'OriginalRemoteData', got %q", item.Data)
	}

	// 5. Mutate again, close db handle, then test restore with specific key
	gdbRestored.Model(&AppItem{}).Where("id = 1").Update("data", "MutatedAgain")
	sqlDBRestored, _ := gdbRestored.DB()
	sqlDBRestored.Close()

	restoreKeySpecific := runRestoreCommand([]string{"--from-s3", remoteKey, "--yes"})
	if restoreKeySpecific != 0 {
		t.Fatalf("restore with specific key failed: %d", restoreKeySpecific)
	}

	gdbFresh, err := gorm.Open(sqlite.Open(sourceDBPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open fresh db after specific key restore failed: %v", err)
	}
	defer func() {
		db, _ := gdbFresh.DB()
		db.Close()
	}()

	var item2 AppItem
	if err := gdbFresh.First(&item2).Error; err != nil {
		t.Fatalf("find item2 failed: %v", err)
	}
	if item2.Data != "OriginalRemoteData" {
		t.Errorf("expected 'OriginalRemoteData', got %q", item2.Data)
	}
}

func TestCLILocalEncryptedBackupAndRestore(t *testing.T) {
	defer cleanupSafetyBackups()
	tempDir := t.TempDir()
	sourceDBPath := filepath.Join(tempDir, "local_enc_test.db")
	backupOutPath := filepath.Join(tempDir, "local_backup.db.enc")

	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", sourceDBPath)
	t.Setenv("OCTARQ_SECRET_KEY", "master-secret-key-for-local-enc")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "test-admin-password")

	type LocalItem struct {
		ID  uint `gorm:"primaryKey"`
		Val string
	}

	gdb, _ := gorm.Open(sqlite.Open(sourceDBPath), &gorm.Config{})
	_ = gdb.AutoMigrate(&LocalItem{})
	gdb.Create(&LocalItem{Val: "LocalEncValue"})
	sdb, _ := gdb.DB()
	sdb.Close()

	// Backup with --encrypt
	code := runBackupCommand([]string{"--out", backupOutPath, "--encrypt"})
	if code != 0 {
		t.Fatalf("runBackupCommand --encrypt failed: %d", code)
	}

	// Verify it is not raw sqlite
	if err := db.VerifySQLiteIntegrity(backupOutPath); err == nil {
		t.Fatal("expected local encrypted backup to fail VerifySQLiteIntegrity")
	}

	// Mutate
	gdb2, _ := gorm.Open(sqlite.Open(sourceDBPath), &gorm.Config{})
	gdb2.Model(&LocalItem{}).Where("id = 1").Update("val", "MutatedLocal")
	sdb2, _ := gdb2.DB()
	sdb2.Close()

	// Restore with --decrypt
	restoreCode := runRestoreCommand([]string{"--in", backupOutPath, "--decrypt", "--yes"})
	if restoreCode != 0 {
		t.Fatalf("runRestoreCommand with --decrypt failed: %d", restoreCode)
	}

	gdbRestored, _ := gorm.Open(sqlite.Open(sourceDBPath), &gorm.Config{})
	defer func() {
		db, _ := gdbRestored.DB()
		db.Close()
	}()

	var item LocalItem
	gdbRestored.First(&item)
	if item.Val != "LocalEncValue" {
		t.Errorf("expected 'LocalEncValue', got %q", item.Val)
	}
}

func TestCLIBackupAndRestoreErrors(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "err_test.db")

	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", dbPath)
	t.Setenv("OCTARQ_SECRET_KEY", "test-secret-key-16-bytes")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "test-admin-password")
	t.Setenv("OCTARQ_BACKUP_S3_BUCKET", "") // unconfigured S3

	// 1. --remote without S3 config should exit 1
	if code := runBackupCommand([]string{"--remote"}); code != 1 {
		t.Errorf("expected backup --remote without S3 config to exit 1, got %d", code)
	}

	// 2. --from-s3 without S3 config should exit 1
	if code := runRestoreCommand([]string{"--from-s3", "latest", "--yes"}); code != 1 {
		t.Errorf("expected restore --from-s3 without S3 config to exit 1, got %d", code)
	}

	// 3. restore without --in or --from-s3 should exit 1
	if code := runRestoreCommand([]string{}); code != 1 {
		t.Errorf("expected restore without target to exit 1, got %d", code)
	}

	// 4. Interactive restore cancelled by user
	inReader := strings.NewReader("no\n")
	if code := runRestoreCommandWithInput([]string{"--in", "dummy.db"}, inReader); code != 1 {
		t.Errorf("expected restore cancelled by user to exit 1, got %d", code)
	}
}
