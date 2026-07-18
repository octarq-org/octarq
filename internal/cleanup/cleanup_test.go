package cleanup

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calledWithDays := -1
	mockCleanup := func(ctx context.Context, retentionDays int) {
		calledWithDays = retentionDays
	}

	// 1. Check with negative/zero retention days - should not call cleanups
	retentionDays := func() int { return 0 }
	Start(ctx, retentionDays, mockCleanup)

	if calledWithDays != -1 {
		t.Errorf("cleanup should not run when retentionDays is 0, got called with %d", calledWithDays)
	}

	// 2. Check with retention days = 3
	retentionDays = func() int { return 3 }
	Start(ctx, retentionDays, mockCleanup)

	if calledWithDays != 3 {
		t.Errorf("expected cleanup to be called with 3 days, got %d", calledWithDays)
	}
}

func TestStartSessionCleanup(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	expired := models.Session{
		Token:     "exp",
		UserID:    1,
		ExpiresAt: now.Add(-1 * time.Hour),
	}
	db.Create(&expired)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	StartSessionCleanup(ctx, db)

	var count int64
	db.Model(&models.Session{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 sessions after cleanup, got %d", count)
	}
}
