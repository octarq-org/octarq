package cleanup

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/api"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:cleanup_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
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

func TestAuditLogRetentionDays(t *testing.T) {
	t.Run("defaults to DefaultRetentionDays when unset", func(t *testing.T) {
		db := testDB(t)
		if got := auditLogRetentionDays(db); got != api.DefaultRetentionDays {
			t.Errorf("expected default %d, got %d", api.DefaultRetentionDays, got)
		}
	})

	t.Run("reads the instance setting", func(t *testing.T) {
		db := testDB(t)
		db.Create(&models.Setting{Key: settingKeyDataRetentionDays, Value: "45"})
		if got := auditLogRetentionDays(db); got != 45 {
			t.Errorf("expected 45, got %d", got)
		}
	})

	t.Run("falls back on an invalid value", func(t *testing.T) {
		db := testDB(t)
		db.Create(&models.Setting{Key: settingKeyDataRetentionDays, Value: "not-a-number"})
		if got := auditLogRetentionDays(db); got != api.DefaultRetentionDays {
			t.Errorf("expected default %d, got %d", api.DefaultRetentionDays, got)
		}
	})
}

func TestPruneAuditLogs(t *testing.T) {
	t.Run("default window removes old rows, keeps recent", func(t *testing.T) {
		db := testDB(t)
		now := time.Now()
		old := &models.AuditLog{OrgID: 1, ActorID: 1, Action: "test.old", CreatedAt: now.Add(-120 * 24 * time.Hour)}
		recent := &models.AuditLog{OrgID: 1, ActorID: 1, Action: "test.recent", CreatedAt: now.Add(-1 * 24 * time.Hour)}
		db.Create(old)
		db.Create(recent)
		pruneAuditLogs(db)
		if err := db.First(&models.AuditLog{}, old.ID).Error; err == nil {
			t.Errorf("expected old audit log %d to be pruned", old.ID)
		}
		if err := db.First(&models.AuditLog{}, recent.ID).Error; err != nil {
			t.Errorf("expected recent audit log %d to survive: %v", recent.ID, err)
		}
	})

	t.Run("instance setting overrides the default", func(t *testing.T) {
		db := testDB(t)
		now := time.Now()
		gap := &models.AuditLog{OrgID: 1, ActorID: 1, Action: "test.gap", CreatedAt: now.Add(-75 * 24 * time.Hour)}
		recent := &models.AuditLog{OrgID: 1, ActorID: 1, Action: "test.recent", CreatedAt: now.Add(-1 * 24 * time.Hour)}
		db.Create(gap)
		db.Create(recent)
		db.Create(&models.Setting{Key: settingKeyDataRetentionDays, Value: "60"})
		pruneAuditLogs(db)
		if err := db.First(&models.AuditLog{}, gap.ID).Error; err == nil {
			t.Errorf("expected audit log %d (75d old) to be pruned under the 60d instance setting", gap.ID)
		}
		if err := db.First(&models.AuditLog{}, recent.ID).Error; err != nil {
			t.Errorf("expected recent audit log %d to survive: %v", recent.ID, err)
		}
	})

	t.Run("retention of 0 disables pruning", func(t *testing.T) {
		db := testDB(t)
		now := time.Now()
		old := &models.AuditLog{OrgID: 1, ActorID: 1, Action: "test.old", CreatedAt: now.Add(-120 * 24 * time.Hour)}
		recent := &models.AuditLog{OrgID: 1, ActorID: 1, Action: "test.recent", CreatedAt: now.Add(-1 * 24 * time.Hour)}
		db.Create(old)
		db.Create(recent)
		db.Create(&models.Setting{Key: settingKeyDataRetentionDays, Value: "0"})
		pruneAuditLogs(db)
		if err := db.First(&models.AuditLog{}, old.ID).Error; err != nil {
			t.Errorf("expected audit log %d to survive with pruning disabled", old.ID)
		}
		if err := db.First(&models.AuditLog{}, recent.ID).Error; err != nil {
			t.Errorf("expected recent audit log %d to survive: %v", recent.ID, err)
		}
	})
}

func TestStartSessionCleanupPrunesAuditLogs(t *testing.T) {
	db := testDB(t)
	db.Create(&models.Setting{Key: settingKeyDataRetentionDays, Value: "30"})
	now := time.Now()
	old := &models.AuditLog{OrgID: 1, ActorID: 1, Action: "test.old", CreatedAt: now.Add(-90 * 24 * time.Hour)}
	recent := &models.AuditLog{OrgID: 1, ActorID: 1, Action: "test.recent", CreatedAt: now.Add(-1 * 24 * time.Hour)}
	db.Create(old)
	db.Create(recent)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	StartSessionCleanup(ctx, db)

	if err := db.First(&models.AuditLog{}, old.ID).Error; err == nil {
		t.Errorf("expected old audit log %d to be pruned", old.ID)
	}
	if err := db.First(&models.AuditLog{}, recent.ID).Error; err != nil {
		t.Errorf("expected recent audit log %d to survive: %v", recent.ID, err)
	}
}
