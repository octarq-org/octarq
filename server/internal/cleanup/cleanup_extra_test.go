package cleanup

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

func TestCleanup_NegativeDaysAndEmptySessions(t *testing.T) {
	// 1. Start with negative retention days
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	called := false
	Start(ctx, func() int { return -5 }, func(ctx context.Context, retentionDays int) {
		called = true
	})
	if called {
		t.Error("expected cleanup not to be called for negative retention days")
	}

	// 2. pruneAuditLogs with negative days
	pruneAuditLogs(nil, -1) // should return immediately without panicking on nil DB

	// 3. StartSessionCleanup with 0 expired sessions
	dbPath := filepath.Join(t.TempDir(), "cleanup_extra.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Session{}, &models.AuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	StartSessionCleanup(ctx2, db, func() int { return 30 })

	// 4. StartSessionCleanup on DB where tables don't exist (triggers error branches)
	errDBPath := filepath.Join(t.TempDir(), "cleanup_empty.db")
	errDB, err := gorm.Open(sqlite.Open(errDBPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	ctx3, cancel3 := context.WithCancel(context.Background())
	cancel3()
	StartSessionCleanup(ctx3, errDB, func() int { return 30 })
}
