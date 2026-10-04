package cleanup_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/internal/cleanup"
	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
)

func TestCleanupCharacterizationEdgeCases(t *testing.T) {
	// 1. Nil DB returns (0, nil)
	purged, err := cleanup.PruneAuditLogs(nil, 30)
	if err != nil || purged != 0 {
		t.Fatalf("expected (0, nil) on nil DB, got (%d, %v)", purged, err)
	}

	// 2. Zero or negative days returns (0, nil)
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:cleanup_char_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.AuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	purged, err = cleanup.PruneAuditLogs(db, 0)
	if err != nil || purged != 0 {
		t.Fatalf("expected (0, nil) on days=0, got (%d, %v)", purged, err)
	}

	purged, err = cleanup.PruneAuditLogs(db, -5)
	if err != nil || purged != 0 {
		t.Fatalf("expected (0, nil) on days=-5, got (%d, %v)", purged, err)
	}
}
