// Audit trail for AI database access through the MCP server.
//
// Every query_db_readonly call — successful or rejected — is recorded in led's
// audit_logs table, so an operator can review exactly what an AI client asked
// the database, catching hallucinated or unexpected queries. ActorID is 0 (the
// access is system/AI-initiated, not a dashboard user). This is the OSS-side
// half of the roadmap's "AI 执行审计追踪"; the Pro AI plugin audits its own
// actions separately.
package mcp

import (
	"encoding/json"
	"time"
)

// auditRow mirrors led's audit_logs table (the mcp package avoids importing
// internal/models so it stays a thin tool layer).
type auditRow struct {
	ID         uint `gorm:"primaryKey"`
	OrgID      uint
	ActorID    uint // 0 = system / AI
	Action     string
	TargetType string
	TargetID   uint
	Meta       string
	IP         string
	CreatedAt  time.Time
}

func (auditRow) TableName() string { return "audit_logs" }

// maxAuditSQL bounds how much of the query text is stored, so a huge query
// doesn't bloat the audit log.
const maxAuditSQL = 500

// auditQuery records one query_db_readonly invocation. It never fails the tool
// call — an audit miss must not block a read.
func (s *server) auditQuery(query string, rows int, qErr error) {
	if s.gdb == nil {
		return
	}
	q := query
	if len(q) > maxAuditSQL {
		q = q[:maxAuditSQL]
	}
	meta := map[string]any{"sql": q, "rows": rows}
	if qErr != nil {
		meta["error"] = qErr.Error()
	}
	var metaJSON string
	if b, err := json.Marshal(meta); err == nil {
		metaJSON = string(b)
	}
	_ = s.gdb.Create(&auditRow{
		OrgID:      s.ownerScope(),
		ActorID:    0, // system / AI
		Action:     "ai.mcp.query",
		TargetType: "database",
		Meta:       metaJSON,
		IP:         "mcp",
	}).Error
}
