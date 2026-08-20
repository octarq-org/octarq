package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/plugins/dns"
	"gorm.io/gorm"
)

// TestRunNewFailure covers the init-failed path: a rejected DB driver makes
// app.New return an error, which run() must map to exit code 1.
func TestRunNewFailure(t *testing.T) {
	t.Setenv("OCTARQ_DB_DRIVER", "invalid-driver")
	if code := run(context.Background()); code != 1 {
		t.Fatalf("run with an invalid DB driver = %d, want 1", code)
	}
}

// TestRunFailureAfterBoot drives the a.Run error branch: with a registered
// domain pre-seeded and a too-short secret key, Run refuses to serve and run()
// must propagate the error.
func TestRunFailureAfterBoot(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "edition_fail.db")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", dbPath)
	t.Setenv("OCTARQ_SECRET_KEY", "short")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "edition-fail-admin-pass")

	seed, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("seed open: %v", err)
	}
	seed.AutoMigrate(&dns.Domain{})
	seed.Create(&dns.Domain{Name: "example.com", ForLink: true})
	sqlDB, _ := seed.DB()
	sqlDB.Close()

	if code := run(context.Background()); code == 0 {
		t.Fatal("run succeeded with a registered domain and short secret key")
	}
}

// TestRunBootsAndShutsDown boots the trimmed dns+links composition against a
// scratch SQLite database with an already-cancelled context, so Run starts,
// migrates, and shuts down promptly instead of blocking the suite.
func TestRunBootsAndShutsDown(t *testing.T) {
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", "file:edition_test?mode=memory&cache=shared")
	t.Setenv("OCTARQ_SECRET_KEY", "edition-nomail-secret-key-32-bytes!!!")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "edition-nomail-admin-pass")
	t.Setenv("OCTARQ_LISTEN", "127.0.0.1:0")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if code := run(ctx); code != 0 {
		t.Fatalf("run = %d, want 0", code)
	}
}
