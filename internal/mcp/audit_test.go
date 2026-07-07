package mcp

import (
	"context"
	"testing"

	"github.com/octarq-org/led/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestQueryAudited verifies query_db_readonly writes an audit row for both
// successful and rejected queries.
func TestQueryAudited(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open("file:mcpaudit?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := gdb.AutoMigrate(&models.Link{}, &models.AuditLog{}); err != nil {
		t.Fatal(err)
	}
	s := &server{gdb: gdb, orgID: 1}

	// Successful query.
	if _, _, err := s.queryDBReadonly(context.Background(), nil, queryInput{Query: "SELECT count(*) AS n FROM links"}); err != nil {
		t.Fatal(err)
	}
	// Rejected query.
	if _, _, err := s.queryDBReadonly(context.Background(), nil, queryInput{Query: "DELETE FROM links"}); err != nil {
		t.Fatal(err)
	}

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
