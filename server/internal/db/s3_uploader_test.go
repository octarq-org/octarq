package db_test

import (
	"context"
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
	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/db"
	"gorm.io/gorm"
)

type mockS3Server struct {
	mu           sync.Mutex
	objects      map[string][]byte
	lastModified map[string]time.Time
	badETag      bool
}

type s3ListBucketResult struct {
	XMLName     xml.Name        `xml:"ListBucketResult"`
	Xmlns       string          `xml:"xmlns,attr"`
	Name        string          `xml:"Name"`
	Prefix      string          `xml:"Prefix"`
	KeyCount    int             `xml:"KeyCount"`
	MaxKeys     int             `xml:"MaxKeys"`
	IsTruncated bool            `xml:"IsTruncated"`
	Contents    []s3ObjectEntry `xml:"Contents"`
}

type s3ObjectEntry struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
	StorageClass string `xml:"StorageClass"`
}

func newMockS3Server() (*httptest.Server, *mockS3Server) {
	m := &mockS3Server{
		objects:      make(map[string][]byte),
		lastModified: make(map[string]time.Time),
	}

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
			m.lastModified[key] = time.Now().UTC()

			sum := md5.Sum(data)
			etag := hex.EncodeToString(sum[:])
			if m.badETag {
				etag = "00000000000000000000000000000000"
			}
			w.Header().Set("ETag", fmt.Sprintf("\"%s\"", etag))
			w.WriteHeader(http.StatusOK)

		case http.MethodGet:
			if r.URL.Query().Get("list-type") == "2" {
				// ListObjectsV2
				prefix := r.URL.Query().Get("prefix")
				var entries []s3ObjectEntry
				for k, v := range m.objects {
					if prefix == "" || strings.HasPrefix(k, prefix) {
						sum := md5.Sum(v)
						etag := hex.EncodeToString(sum[:])
						mod := m.lastModified[k]
						if mod.IsZero() {
							mod = time.Now().UTC()
						}
						entries = append(entries, s3ObjectEntry{
							Key:          k,
							LastModified: mod.Format("2006-01-02T15:04:05.000Z"),
							ETag:         fmt.Sprintf("\"%s\"", etag),
							Size:         int64(len(v)),
							StorageClass: "STANDARD",
						})
					}
				}
				res := s3ListBucketResult{
					Xmlns:       "http://s3.amazonaws.com/doc/2006-03-01/",
					Name:        parts[0],
					Prefix:      prefix,
					KeyCount:    len(entries),
					MaxKeys:     1000,
					IsTruncated: false,
					Contents:    entries,
				}
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				_ = xml.NewEncoder(w).Encode(res)
				return
			}

			// GetObject
			data, ok := m.objects[key]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			sum := md5.Sum(data)
			w.Header().Set("ETag", fmt.Sprintf("\"%x\"", sum))
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)

		case http.MethodDelete:
			delete(m.objects, key)
			delete(m.lastModified, key)
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))

	return ts, m
}

func TestS3Backup(t *testing.T) {
	ts, mock := newMockS3Server()
	defer ts.Close()

	tempDir := t.TempDir()
	sourceDBPath := filepath.Join(tempDir, "production.db")
	backupRawPath := filepath.Join(tempDir, "octarq-backup-test.db")
	backupEncPath := filepath.Join(tempDir, "octarq-backup-test.db.enc")
	restoredPath := filepath.Join(tempDir, "restored.db")

	secretKey := "test-secret-key-32-chars-long!!"

	cfg := &config.Config{
		DBDriver:            "sqlite",
		DBDSN:               sourceDBPath,
		SecretKey:           secretKey,
		BackupS3Endpoint:    ts.URL,
		BackupS3Bucket:      "octarq-test-bucket",
		BackupS3AccessKey:   "mock-access-key",
		BackupS3SecretKey:   "mock-secret-key",
		BackupS3Region:      "auto",
		BackupRetentionDays: 30,
	}

	// 1. Populate a realistic test SQLite database with tables, indexes, and rows
	type TenantSecret struct {
		ID        uint   `gorm:"primaryKey"`
		TenantID  string `gorm:"index:idx_tenant_name;size:64"`
		Name      string `gorm:"index:idx_tenant_name;size:128"`
		Value     string `gorm:"size:255"`
		CreatedAt time.Time
	}

	gdb, err := gorm.Open(sqlite.Open(sourceDBPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open source sqlite db: %v", err)
	}

	if err := gdb.AutoMigrate(&TenantSecret{}); err != nil {
		t.Fatalf("auto-migrate failed: %v", err)
	}

	records := []TenantSecret{
		{TenantID: "org-1", Name: "stripe_api_key", Value: "sk_live_12345"},
		{TenantID: "org-1", Name: "smtp_password", Value: "smtp_pass_secret"},
		{TenantID: "org-2", Name: "cloudflare_token", Value: "cf_token_secret"},
	}
	if err := gdb.Create(&records).Error; err != nil {
		t.Fatalf("create test records failed: %v", err)
	}
	sqlDB, _ := gdb.DB()
	sqlDB.Close()

	// Verify original DB integrity
	if err := db.VerifySQLiteIntegrity(sourceDBPath); err != nil {
		t.Fatalf("source db integrity failed: %v", err)
	}

	// 2. Perform native SQLite online backup
	if err := db.Backup(cfg, backupRawPath); err != nil {
		t.Fatalf("db.Backup failed: %v", err)
	}
	if err := db.VerifySQLiteIntegrity(backupRawPath); err != nil {
		t.Fatalf("raw backup file integrity failed: %v", err)
	}

	// 3. Encrypt backup file with AES-256-GCM zero-knowledge encryption
	if err := crypto.EncryptBackup(backupRawPath, backupEncPath, secretKey); err != nil {
		t.Fatalf("crypto.EncryptBackup failed: %v", err)
	}

	// 4. Verification: Local SQLite file encrypted cannot be opened by SQLite driver directly
	// (ensures real ciphertext encryption)
	if err := db.VerifySQLiteIntegrity(backupEncPath); err == nil {
		t.Fatal("CRITICAL: encrypted backup file passed VerifySQLiteIntegrity! It must not be valid SQLite")
	}

	badGDB, err := gorm.Open(sqlite.Open(backupEncPath), &gorm.Config{})
	if err == nil {
		badSQL, _ := badGDB.DB()
		var count int64
		queryErr := badGDB.Table("tenant_secrets").Count(&count).Error
		badSQL.Close()
		if queryErr == nil {
			t.Fatal("CRITICAL: encrypted backup could be read as SQLite table!")
		}
	}

	// 5. Upload encrypted backup to Mock S3
	uploader, err := db.NewS3Uploader(cfg)
	if err != nil {
		t.Fatalf("NewS3Uploader failed: %v", err)
	}

	ctx := context.Background()
	if err := uploader.Upload(ctx, backupEncPath); err != nil {
		t.Fatalf("uploader.Upload failed: %v", err)
	}

	// Verify object exists in Mock S3
	encRemoteKey := filepath.Base(backupEncPath)
	if _, ok := mock.objects[encRemoteKey]; !ok {
		t.Fatalf("expected remote key %q in mock s3 objects", encRemoteKey)
	}

	// 6. Test ListBackups and LatestBackup
	latestKey, err := uploader.LatestBackupKey(ctx)
	if err != nil {
		t.Fatalf("LatestBackupKey failed: %v", err)
	}
	if latestKey != encRemoteKey {
		t.Errorf("latest key = %q, want %q", latestKey, encRemoteKey)
	}

	// 7. Download encrypted backup from S3
	downloadedEncPath := filepath.Join(tempDir, "downloaded.db.enc")
	if err := uploader.Download(ctx, encRemoteKey, downloadedEncPath); err != nil {
		t.Fatalf("uploader.Download failed: %v", err)
	}

	// 8. Decrypt the downloaded backup
	if err := crypto.DecryptBackup(downloadedEncPath, restoredPath, secretKey); err != nil {
		t.Fatalf("crypto.DecryptBackup failed: %v", err)
	}

	// 9. Verify decrypted database integrity and data rows
	if err := db.VerifySQLiteIntegrity(restoredPath); err != nil {
		t.Fatalf("restored db integrity check failed: %v", err)
	}

	restoredGDB, err := gorm.Open(sqlite.Open(restoredPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open restored sqlite db: %v", err)
	}
	defer func() {
		sdb, _ := restoredGDB.DB()
		sdb.Close()
	}()

	var restoredRecords []TenantSecret
	if err := restoredGDB.Order("id asc").Find(&restoredRecords).Error; err != nil {
		t.Fatalf("query restored records failed: %v", err)
	}
	if len(restoredRecords) != 3 {
		t.Fatalf("expected 3 restored records, got %d", len(restoredRecords))
	}
	if restoredRecords[0].Name != "stripe_api_key" || restoredRecords[0].Value != "sk_live_12345" {
		t.Errorf("restored record 0 mismatch: %+v", restoredRecords[0])
	}
	if restoredRecords[2].Name != "cloudflare_token" || restoredRecords[2].Value != "cf_token_secret" {
		t.Errorf("restored record 2 mismatch: %+v", restoredRecords[2])
	}
}

func TestS3Uploader_ChecksumMismatch(t *testing.T) {
	ts, mock := newMockS3Server()
	defer ts.Close()
	mock.badETag = true // deliberately return bogus ETag to trigger mismatch detection

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "octarq-backup-corrupt.db")
	_ = os.WriteFile(filePath, []byte("some backup data"), 0644)

	cfg := &config.Config{
		BackupS3Endpoint:  ts.URL,
		BackupS3Bucket:    "octarq-test-bucket",
		BackupS3AccessKey: "mock-access-key",
		BackupS3SecretKey: "mock-secret-key",
	}

	uploader, err := db.NewS3Uploader(cfg)
	if err != nil {
		t.Fatalf("NewS3Uploader failed: %v", err)
	}

	err = uploader.Upload(context.Background(), filePath)
	if err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("expected error to mention 'checksum mismatch', got: %v", err)
	}
}

func TestS3Uploader_PruneOlderThan(t *testing.T) {
	ts, mock := newMockS3Server()
	defer ts.Close()

	// Seed mock objects: 1 fresh, 2 old
	now := time.Now().UTC()
	oldTime := now.AddDate(0, 0, -40) // 40 days ago

	mock.objects["octarq-backup-old1.db.enc"] = []byte("old1")
	mock.lastModified["octarq-backup-old1.db.enc"] = oldTime

	mock.objects["octarq-backup-old2.db.enc"] = []byte("old2")
	mock.lastModified["octarq-backup-old2.db.enc"] = oldTime

	mock.objects["octarq-backup-fresh.db.enc"] = []byte("fresh")
	mock.lastModified["octarq-backup-fresh.db.enc"] = now

	// A non-backup object that should NOT be touched
	mock.objects["unrelated-file.txt"] = []byte("keep")
	mock.lastModified["unrelated-file.txt"] = oldTime

	cfg := &config.Config{
		BackupS3Endpoint:  ts.URL,
		BackupS3Bucket:    "octarq-test-bucket",
		BackupS3AccessKey: "mock-access-key",
		BackupS3SecretKey: "mock-secret-key",
	}

	uploader, err := db.NewS3Uploader(cfg)
	if err != nil {
		t.Fatalf("NewS3Uploader failed: %v", err)
	}

	ctx := context.Background()
	pruned, err := uploader.PruneOlderThan(ctx, 30) // retain 30 days
	if err != nil {
		t.Fatalf("PruneOlderThan failed: %v", err)
	}
	if pruned != 2 {
		t.Errorf("expected 2 pruned objects, got %d", pruned)
	}

	// Verify old backups are deleted
	if _, exists := mock.objects["octarq-backup-old1.db.enc"]; exists {
		t.Errorf("old1 was not deleted")
	}
	if _, exists := mock.objects["octarq-backup-old2.db.enc"]; exists {
		t.Errorf("old2 was not deleted")
	}

	// Verify fresh backup is preserved
	if _, exists := mock.objects["octarq-backup-fresh.db.enc"]; !exists {
		t.Errorf("fresh was deleted!")
	}
}

func TestS3Uploader_FailClosedMissingConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  *config.Config
	}{
		{"nil config", nil},
		{"empty bucket", &config.Config{BackupS3Bucket: "", BackupS3AccessKey: "k", BackupS3SecretKey: "s"}},
		{"empty access key", &config.Config{BackupS3Bucket: "b", BackupS3AccessKey: "", BackupS3SecretKey: "s"}},
		{"empty secret key", &config.Config{BackupS3Bucket: "b", BackupS3AccessKey: "k", BackupS3SecretKey: ""}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := db.NewS3Uploader(tc.cfg)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
		})
	}
}
