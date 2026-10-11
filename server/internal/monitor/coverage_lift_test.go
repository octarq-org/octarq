package monitor

import (
	"context"
	"database/sql"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

func TestCoverageLift_RuntimeProvider_ZeroStartTime(t *testing.T) {
	p := &RuntimeProvider{}
	res := p.Check(context.Background())
	if res.Status != plugin.HealthOK {
		t.Fatalf("expected HealthOK, got %v", res.Status)
	}
	if res.Metrics["uptime_seconds"] != int64(0) {
		t.Fatalf("expected uptime_seconds to be 0 for zero startTime, got %v", res.Metrics["uptime_seconds"])
	}
}

func TestCoverageLift_DiskProvider_DefaultStatFnAndStatfsError(t *testing.T) {
	p := &DiskProvider{
		dataDir: "/nonexistent-path-for-test-statfs-12345",
	}
	res := p.Check(context.Background())
	if res.Status != plugin.HealthError {
		t.Fatalf("expected HealthError for nonexistent path, got %v", res.Status)
	}
}

func TestCoverageLift_DBProvider_LazyInitAndDBError(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite DB: %v", err)
	}

	// 1. Lazy init when sqlDB is nil on DBProvider
	pLazy := &DBProvider{db: gdb}
	res := pLazy.Check(context.Background())
	if res.Status != plugin.HealthOK {
		t.Fatalf("expected HealthOK after lazy init, got %v", res.Status)
	}
	if pLazy.sqlDB == nil {
		t.Fatal("expected sqlDB to be populated after Check")
	}

	// 2. Error when db.DB() fails (Statement.ConnPool is a *sql.Tx).
	// Clone via Session so no lock-bearing gorm structs are copied (copylocks).
	badGdb := gdb.Session(&gorm.Session{NewDB: true})
	badGdb.Statement.ConnPool = &sql.Tx{}
	pBroken := &DBProvider{db: badGdb}
	resErr := pBroken.Check(context.Background())
	if resErr.Status != plugin.HealthError {
		t.Fatalf("expected HealthError when db.DB() fails, got %v", resErr.Status)
	}
}
