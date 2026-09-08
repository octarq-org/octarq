package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/octarq-org/octarq/internal/models"
)

func TestAuditLogs(t *testing.T) {
	srv, db := newTestHandler(t)
	adminCookies := loginCookies(t, srv)

	// Seed audit logs
	// Create logs with different created_at to test ordering (newest first)
	now := time.Now()

	logs := []models.AuditLog{
		{OrgID: 1, Action: "link.create", TargetType: "link", TargetID: 10, CreatedAt: now.Add(-10 * time.Minute)},
		{OrgID: 1, Action: "member.role", TargetType: "user", TargetID: 20, CreatedAt: now.Add(-5 * time.Minute)},
		{OrgID: 1, Action: "link.update", TargetType: "link", TargetID: 11, CreatedAt: now.Add(-2 * time.Minute)},
		{OrgID: 1, Action: "link.delete", TargetType: "link", TargetID: 12, CreatedAt: now},
		{OrgID: 2, Action: "other.org", TargetType: "other", TargetID: 99, CreatedAt: now.Add(time.Minute)}, // Different org
	}

	for _, l := range logs {
		if err := db.Create(&l).Error; err != nil {
			t.Fatalf("failed to create seed log: %v", err)
		}
	}

	t.Run("Pagination - Limit", func(t *testing.T) {
		rec := do(srv, "GET", "/api/audit?limit=2", adminCookies, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("limit 2: got %d", rec.Code)
		}

		var logs []models.AuditLog
		if err := json.Unmarshal(rec.Body.Bytes(), &logs); err != nil {
			t.Fatalf("failed to unmarshal: %v (body: %s)", err, rec.Body.String())
		}
		if len(logs) != 2 {
			t.Fatalf("expected 2 logs, got %d", len(logs))
		}
		if logs[0].CreatedAt.Before(logs[1].CreatedAt) {
			t.Errorf("unexpected order: logs not newest first")
		}
	})

	t.Run("Pagination - Offset", func(t *testing.T) {
		rec := do(srv, "GET", "/api/audit?limit=2&offset=2", adminCookies, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("offset 2: got %d", rec.Code)
		}

		var logs []models.AuditLog
		if err := json.Unmarshal(rec.Body.Bytes(), &logs); err != nil {
			t.Fatal(err)
		}
		if len(logs) != 2 {
			t.Fatalf("expected 2 logs, got %d", len(logs))
		}
	})

	t.Run("Filter - Action", func(t *testing.T) {
		rec := do(srv, "GET", "/api/audit?action=member.role", adminCookies, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("action filter: got %d", rec.Code)
		}

		var logs []models.AuditLog
		if err := json.Unmarshal(rec.Body.Bytes(), &logs); err != nil {
			t.Fatal(err)
		}
		if len(logs) != 1 {
			t.Fatalf("expected 1 log, got %d", len(logs))
		}
		if logs[0].Action != "member.role" {
			t.Errorf("unexpected action: %s", logs[0].Action)
		}
	})

	t.Run("Filter - TargetType", func(t *testing.T) {
		rec := do(srv, "GET", "/api/audit?targetType=link", adminCookies, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("targetType filter: got %d", rec.Code)
		}

		var logs []models.AuditLog
		if err := json.Unmarshal(rec.Body.Bytes(), &logs); err != nil {
			t.Fatal(err)
		}
		if len(logs) != 3 {
			t.Fatalf("expected 3 logs, got %d", len(logs))
		}
		for _, l := range logs {
			if l.TargetType != "link" {
				t.Errorf("unexpected targetType: %s", l.TargetType)
			}
		}
	})

	t.Run("Filter - Both Action and TargetType", func(t *testing.T) {
		rec := do(srv, "GET", "/api/audit?action=link.create&targetType=link", adminCookies, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("action and targetType filter: got %d", rec.Code)
		}

		var logs []models.AuditLog
		if err := json.Unmarshal(rec.Body.Bytes(), &logs); err != nil {
			t.Fatal(err)
		}
		if len(logs) != 1 {
			t.Fatalf("expected 1 log, got %d", len(logs))
		}
		if logs[0].Action != "link.create" || logs[0].TargetType != "link" {
			t.Errorf("unexpected log: %v", logs[0])
		}
	})
}
