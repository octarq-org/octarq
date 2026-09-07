package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/octarq-org/octarq/internal/cron"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
)

func TestAppCronSetupAndExecution(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "cron_test.db")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", dbPath)
	t.Setenv("OCTARQ_SECRET_KEY", "test-secret-key-16-bytes")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "test-admin-pass")

	a, err := New()
	if err != nil {
		t.Fatalf("app.New() failed: %v", err)
	}

	// Auto-migrate the tables needed for cron jobs
	if err := a.gdb.AutoMigrate(&models.Session{}, &models.Token{}, &models.AuditLog{}); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	cronEngine := cron.NewEngine(nil)
	pctx := &plugin.Context{
		RegisterCron: cronEngine.Register,
		Cron:         cronEngine,
	}

	// Nil safety checks
	a.Setup(nil, nil)
	a.setupBuiltinCron(&plugin.Context{}, nil)

	// Setup built-in cron jobs
	a.Setup(pctx, nil)

	jobs := cronEngine.List()
	if len(jobs) != 2 {
		t.Fatalf("expected 2 built-in jobs, got %d", len(jobs))
	}
	if jobs[0].Name != "log_rotation" || jobs[1].Name != "session_cleanup" {
		t.Fatalf("unexpected jobs in list: %+v", jobs)
	}

	// Test session_cleanup execution
	now := time.Now()
	expiredSession := &models.Session{
		UserID:    1,
		OrgID:     1,
		Token:     "expired-session-token",
		ExpiresAt: now.Add(-time.Hour),
	}
	activeSession := &models.Session{
		UserID:    1,
		OrgID:     1,
		Token:     "active-session-token",
		ExpiresAt: now.Add(time.Hour),
	}
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)
	expiredToken := &models.Token{
		Name:      "expired-token",
		Hash:      "hash-1",
		UserID:    1,
		ExpiresAt: &past,
	}
	activeToken := &models.Token{
		Name:      "active-token",
		Hash:      "hash-2",
		UserID:    1,
		ExpiresAt: &future,
	}
	a.gdb.Create(expiredSession)
	a.gdb.Create(activeSession)
	a.gdb.Create(expiredToken)
	a.gdb.Create(activeToken)

	ctx := context.Background()
	if err := cronEngine.RunJob(ctx, "session_cleanup"); err != nil {
		t.Fatalf("session_cleanup failed: %v", err)
	}

	var sessionCount int64
	a.gdb.Model(&models.Session{}).Count(&sessionCount)
	if sessionCount != 1 {
		t.Fatalf("expected 1 active session remaining, got %d", sessionCount)
	}

	var tokenCount int64
	a.gdb.Model(&models.Token{}).Count(&tokenCount)
	if tokenCount != 1 {
		t.Fatalf("expected 1 active token remaining, got %d", tokenCount)
	}

	// Test log_rotation execution
	oldLog := &models.AuditLog{
		OrgID:     1,
		ActorID:   1,
		Action:    "test.old",
		CreatedAt: now.Add(-100 * 24 * time.Hour),
	}
	recentLog := &models.AuditLog{
		OrgID:     1,
		ActorID:   1,
		Action:    "test.recent",
		CreatedAt: now.Add(-1 * 24 * time.Hour),
	}
	a.gdb.Create(oldLog)
	a.gdb.Create(recentLog)

	// Test log_rotation with nil apiHandler (defaults to no-op)
	if err := cronEngine.RunJob(ctx, "log_rotation"); err != nil {
		t.Fatalf("log_rotation with nil handler failed: %v", err)
	}
}
