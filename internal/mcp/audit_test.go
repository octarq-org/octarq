package mcp

import (
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

// TestAuditQueryRecordsBothOutcomes verifies the audit trail records a database
// access whether it succeeded or was rejected, with the system actor and the
// caller's org.
func TestAuditQueryRecordsBothOutcomes(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open(t.TempDir()+"/mcpaudit.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := gdb.AutoMigrate(&models.AuditLog{}); err != nil {
		t.Fatal(err)
	}
	s := &server{gdb: gdb, orgID: 1}

	s.auditQuery("SELECT count(*) AS n FROM links", 1, nil)
	s.auditQuery("DELETE FROM links", 0, errors.New("disallowed keyword"))

	var logs []auditRow
	gdb.Where("action = ?", "ai.mcp.query").Find(&logs)
	if len(logs) != 2 {
		t.Fatalf("expected 2 audit rows, got %d", len(logs))
	}
	for _, l := range logs {
		if l.ActorID != 0 || l.OrgID != 1 || l.TargetType != "database" {
			t.Errorf("audit fields wrong: %+v", l)
		}
	}
}
