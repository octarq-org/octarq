package monitor

import (
	"context"
	"database/sql"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

func TestDBProvider_NilDB(t *testing.T) {
	p := NewDBProvider(nil)

	if p.Name() != "database" {
		t.Errorf("Name() = %q, want 'database'", p.Name())
	}

	meta := p.Metadata()
	if meta.Category != plugin.CategoryDatabase {
		t.Errorf("Category = %q, want %q", meta.Category, plugin.CategoryDatabase)
	}
	if meta.Unit != plugin.UnitCount {
		t.Errorf("Unit = %q, want %q", meta.Unit, plugin.UnitCount)
	}

	res := p.Check(context.Background())
	if res.Status != plugin.HealthError {
		t.Errorf("expected HealthError for nil db, got %q", res.Status)
	}
	if res.Message != "database handle is nil" {
		t.Errorf("expected 'database handle is nil', got %q", res.Message)
	}

	// Also test NewDBProviderWithSQLDB with nil
	p2 := NewDBProviderWithSQLDB(nil)
	res2 := p2.Check(context.Background())
	if res2.Status != plugin.HealthError {
		t.Errorf("expected HealthError for nil sqlDB, got %q", res2.Status)
	}
}

func TestDBProvider_HealthyAndSlowQueries(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite DB: %v", err)
	}

	p := NewDBProvider(gdb)

	ctx := context.Background()
	res := p.Check(ctx)
	if res.Status != plugin.HealthOK {
		t.Errorf("expected HealthOK, got %q (msg: %s)", res.Status, res.Message)
	}
	if res.Message != "ok" {
		t.Errorf("expected Message 'ok', got %q", res.Message)
	}

	// Check metrics
	metrics := res.Metrics
	for _, k := range []string{"open_connections", "in_use", "idle", "wait_count", "slow_queries"} {
		if _, ok := metrics[k]; !ok {
			t.Errorf("missing metric %q in result", k)
		}
	}
	if sq, ok := metrics["slow_queries"].(uint64); !ok || sq != 0 {
		t.Errorf("expected slow_queries=0, got %v", metrics["slow_queries"])
	}

	// Record slow query
	p.RecordSlowQuery()
	if p.SlowQueries() != 1 {
		t.Errorf("SlowQueries() = %d, want 1", p.SlowQueries())
	}
	p.AddSlowQueries(3)
	if p.SlowQueries() != 4 {
		t.Errorf("SlowQueries() = %d, want 4", p.SlowQueries())
	}

	resWarn := p.Check(ctx)
	if resWarn.Status != plugin.HealthWarn {
		t.Errorf("expected HealthWarn when slow queries present, got %q", resWarn.Status)
	}
	if resWarn.Message != "4 slow queries" {
		t.Errorf("expected Message '4 slow queries', got %q", resWarn.Message)
	}
	if sq, ok := resWarn.Metrics["slow_queries"].(uint64); !ok || sq != 4 {
		t.Errorf("expected slow_queries=4, got %v", resWarn.Metrics["slow_queries"])
	}

	// Reset slow queries
	p.ResetSlowQueries()
	if p.SlowQueries() != 0 {
		t.Errorf("SlowQueries() after reset = %d, want 0", p.SlowQueries())
	}
	resReset := p.Check(ctx)
	if resReset.Status != plugin.HealthOK {
		t.Errorf("expected HealthOK after reset, got %q", resReset.Status)
	}
}

func TestDBProvider_PingFailed(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite DB: %v", err)
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatalf("failed to get sql.DB: %v", err)
	}

	p := NewDBProviderWithSQLDB(sqlDB)

	// Close database to force ping error
	_ = sqlDB.Close()

	res := p.Check(context.Background())
	if res.Status != plugin.HealthError {
		t.Errorf("expected HealthError on closed DB ping, got %q", res.Status)
	}
}

func TestDBProvider_GDBExtractFails(t *testing.T) {
	// Create DBProvider with gdb whose sql.DB cannot be extracted or ping fails with canceled context
	gdb, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite DB: %v", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatalf("failed to get sql.DB: %v", err)
	}
	_ = sqlDB.Close()

	p := NewDBProvider(gdb)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // canceled context
	res := p.Check(ctx)
	if res.Status != plugin.HealthError {
		t.Errorf("expected HealthError with canceled context, got %q", res.Status)
	}
}

type fakeBadDB struct {
	*sql.DB
}
