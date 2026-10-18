package cron_test

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/cron"
	"gorm.io/gorm"
)

type mockCronRegistrar struct {
	jobs map[string]string
}

func (m *mockCronRegistrar) Register(name string, spec string, handler func(ctx context.Context) error) error {
	m.jobs[name] = spec
	return nil
}

type cronMockS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
}

type s3ListResult struct {
	XMLName  xml.Name      `xml:"ListBucketResult"`
	Xmlns    string        `xml:"xmlns,attr"`
	Name     string        `xml:"Name"`
	Contents []s3ListEntry `xml:"Contents"`
}

type s3ListEntry struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
}

func newCronMockS3() (*httptest.Server, *cronMockS3) {
	m := &cronMockS3{objects: make(map[string][]byte)}
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
				var entries []s3ListEntry
				for k, v := range m.objects {
					sum := md5.Sum(v)
					entries = append(entries, s3ListEntry{
						Key:          k,
						LastModified: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
						ETag:         fmt.Sprintf("\"%s\"", hex.EncodeToString(sum[:])),
						Size:         int64(len(v)),
					})
				}
				res := s3ListResult{
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

func TestRegisterBackupJob_UnconfiguredSkipsSilently(t *testing.T) {
	registrar := &mockCronRegistrar{jobs: make(map[string]string)}
	cfg := &config.Config{
		BackupS3Bucket: "", // unconfigured
	}

	err := cron.RegisterBackupJob(registrar, cfg)
	if err != nil {
		t.Fatalf("expected nil error on unconfigured backup, got: %v", err)
	}
	if len(registrar.jobs) != 0 {
		t.Errorf("expected 0 registered jobs, got %d", len(registrar.jobs))
	}
}

func TestRegisterBackupJob_ConfiguredRegistersJob(t *testing.T) {
	registrar := &mockCronRegistrar{jobs: make(map[string]string)}
	cfg := &config.Config{
		BackupS3Bucket:    "my-bucket",
		BackupS3AccessKey: "access-key",
		BackupS3SecretKey: "secret-key",
		BackupCron:        "0 4 * * *",
	}

	err := cron.RegisterBackupJob(registrar, cfg)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	spec, ok := registrar.jobs[cron.BackupJobName]
	if !ok {
		t.Fatalf("expected job %q to be registered", cron.BackupJobName)
	}
	if spec != "0 4 * * *" {
		t.Errorf("registered spec = %q, want '0 4 * * *'", spec)
	}
}

func TestExecuteBackupWorkflow_EndToEnd(t *testing.T) {
	ts, mock := newCronMockS3()
	defer ts.Close()

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "cron_test.db")

	// Create test DB
	gdb, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	type CronItem struct {
		ID   uint `gorm:"primaryKey"`
		Data string
	}
	if err := gdb.AutoMigrate(&CronItem{}); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	if err := gdb.Create(&CronItem{Data: "cron-data-123"}).Error; err != nil {
		t.Fatalf("create failed: %v", err)
	}
	sdb, _ := gdb.DB()
	sdb.Close()

	cfg := &config.Config{
		DBDriver:            "sqlite",
		DBDSN:               dbPath,
		SecretKey:           "master-secret-key-32-bytes-long!",
		BackupS3Endpoint:    ts.URL,
		BackupS3Bucket:      "octarq-cron-backups",
		BackupS3AccessKey:   "key",
		BackupS3SecretKey:   "secret",
		BackupS3Region:      "auto",
		BackupRetentionDays: 30,
	}

	ctx := context.Background()
	if err := cron.ExecuteBackupWorkflow(ctx, cfg); err != nil {
		t.Fatalf("ExecuteBackupWorkflow failed: %v", err)
	}

	// Verify object uploaded to S3
	if len(mock.objects) != 1 {
		t.Fatalf("expected 1 object in mock S3, got %d", len(mock.objects))
	}
	for key := range mock.objects {
		if !strings.HasPrefix(key, "octarq-backup-") || !strings.HasSuffix(key, ".db.enc") {
			t.Errorf("unexpected object key name: %s", key)
		}
	}
}

func TestExecuteBackupWorkflow_UnconfiguredFailsClosed(t *testing.T) {
	cfg := &config.Config{
		DBDriver:       "sqlite",
		BackupS3Bucket: "",
	}
	err := cron.ExecuteBackupWorkflow(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected ExecuteBackupWorkflow to fail when unconfigured")
	}
}
