package tenantsql_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/tenantsql"
	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/plugins/links"
	"github.com/octarq-org/octarq/plugins/mail"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) (*gorm.DB, *tenantsql.Registry) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite memory db: %v", err)
	}

	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})

	// Auto-migrate tables
	if err := db.AutoMigrate(
		&links.Link{},
		&links.LinkEvent{},
		&mail.Mailbox{},
		&mail.Email{},
		&models.AuditLog{},
	); err != nil {
		t.Fatalf("auto-migrate failed: %v", err)
	}

	reg := tenantsql.NewRegistry()
	pctx := &plugin.Context{
		RegisterTenantView: func(v plugin.TenantView) {
			if err := reg.Register(v); err != nil {
				t.Fatalf("register view %s: %v", v.Name, err)
			}
		},
	}

	links.RegisterViews(pctx)
	mail.RegisterViews(pctx)

	return db, reg
}

func TestExecute_CrossTenantIsolation(t *testing.T) {
	db, reg := setupTestDB(t)

	// Seed data for Org 1
	link1 := links.Link{
		OrgID:    1,
		Slug:     "slug-org-1",
		Target:   "https://org1.example.com",
		Password: "supersecretpassword1",
		Title:    "Org 1 Link",
	}
	if err := db.Create(&link1).Error; err != nil {
		t.Fatalf("create link1: %v", err)
	}

	// Seed data for Org 2
	link2 := links.Link{
		OrgID:    2,
		Slug:     "slug-org-2",
		Target:   "https://org2.example.com",
		Password: "supersecretpassword2",
		Title:    "Org 2 Link",
	}
	if err := db.Create(&link2).Error; err != nil {
		t.Fatalf("create link2: %v", err)
	}

	// Query as Org 1
	ctx1 := plugin.WithOrgID(context.Background(), 1)
	ctx1 = auth.WithUserID(ctx1, 101)
	rows1, meta1, err := tenantsql.Execute(ctx1, db, reg, "SELECT id, slug, target, password, title FROM tenant_links")
	if err != nil {
		t.Fatalf("execute query for org 1: %v", err)
	}

	if meta1.RowCount != 1 || len(rows1) != 1 {
		t.Fatalf("expected exactly 1 row for org 1, got %d", len(rows1))
	}
	if rows1[0]["slug"] != "slug-org-1" {
		t.Errorf("expected slug-org-1, got %v", rows1[0]["slug"])
	}
	// Verify sensitive redaction
	if rows1[0]["password"] != plugin.RedactedValue {
		t.Errorf("expected password to be redacted to %q, got %v", plugin.RedactedValue, rows1[0]["password"])
	}

	// Query as Org 2
	ctx2 := plugin.WithOrgID(context.Background(), 2)
	ctx2 = auth.WithUserID(ctx2, 102)
	rows2, meta2, err := tenantsql.Execute(ctx2, db, reg, "SELECT id, slug, target, password, title FROM tenant_links")
	if err != nil {
		t.Fatalf("execute query for org 2: %v", err)
	}

	if meta2.RowCount != 1 || len(rows2) != 1 {
		t.Fatalf("expected exactly 1 row for org 2, got %d", len(rows2))
	}
	if rows2[0]["slug"] != "slug-org-2" {
		t.Errorf("expected slug-org-2, got %v", rows2[0]["slug"])
	}
	if rows2[0]["password"] != plugin.RedactedValue {
		t.Errorf("expected password to be redacted to %q, got %v", plugin.RedactedValue, rows2[0]["password"])
	}

	// Verify AuditLog entries were recorded
	var logs []models.AuditLog
	if err := db.Where("action = ?", "tenant_sql.query").Find(&logs).Error; err != nil {
		t.Fatalf("find audit logs: %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("expected 2 audit logs, got %d", len(logs))
	}
}

func TestExecute_JoinViewsIsolation(t *testing.T) {
	db, reg := setupTestDB(t)

	// Create Link and LinkEvents for Org 1 and Org 2
	l1 := links.Link{OrgID: 1, Slug: "l1", Target: "https://l1.com"}
	db.Create(&l1)
	db.Create(&links.LinkEvent{LinkID: l1.ID, IP: "1.1.1.1", Fingerprint: "fp1", CreatedAt: time.Now()})

	l2 := links.Link{OrgID: 2, Slug: "l2", Target: "https://l2.com"}
	db.Create(&l2)
	db.Create(&links.LinkEvent{LinkID: l2.ID, IP: "2.2.2.2", Fingerprint: "fp2", CreatedAt: time.Now()})

	// Create Mailbox and Email for Org 1 and Org 2
	mb1 := mail.Mailbox{OrgID: 1, Address: "box1@org1.com"}
	db.Create(&mb1)
	db.Create(&mail.Email{MailboxID: mb1.ID, MessageID: "msg-1", Subject: "Secret 1", Text: "Body 1"})

	mb2 := mail.Mailbox{OrgID: 2, Address: "box2@org2.com"}
	db.Create(&mb2)
	db.Create(&mail.Email{MailboxID: mb2.ID, MessageID: "msg-2", Subject: "Secret 2", Text: "Body 2"})

	// Query tenant_link_events as Org 1
	ctx1 := plugin.WithOrgID(context.Background(), 1)
	rows, _, err := tenantsql.Execute(ctx1, db, reg, "SELECT ip, fingerprint FROM tenant_link_events")
	if err != nil {
		t.Fatalf("execute link events query: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 event for Org 1, got %d", len(rows))
	}
	if rows[0]["ip"] != "1.1.1.1" {
		t.Errorf("expected IP 1.1.1.1, got %v", rows[0]["ip"])
	}
	if rows[0]["fingerprint"] != plugin.RedactedValue {
		t.Errorf("expected fingerprint redacted, got %v", rows[0]["fingerprint"])
	}

	// Query tenant_emails as Org 1
	emailRows, _, err := tenantsql.Execute(ctx1, db, reg, "SELECT message_id, subject, text FROM tenant_emails")
	if err != nil {
		t.Fatalf("execute emails query: %v", err)
	}
	if len(emailRows) != 1 {
		t.Fatalf("expected 1 email for Org 1, got %d", len(emailRows))
	}
	if emailRows[0]["message_id"] != "msg-1" {
		t.Errorf("expected message_id msg-1, got %v", emailRows[0]["message_id"])
	}
	if emailRows[0]["subject"] != plugin.RedactedValue {
		t.Errorf("expected subject redacted, got %v", emailRows[0]["subject"])
	}
	if emailRows[0]["text"] != plugin.RedactedValue {
		t.Errorf("expected text redacted, got %v", emailRows[0]["text"])
	}
}

func TestExecute_FailClosedSecurityChecks(t *testing.T) {
	db, reg := setupTestDB(t)

	// 1. Missing Org Context
	ctxNoOrg := context.Background()
	_, _, err := tenantsql.Execute(ctxNoOrg, db, reg, "SELECT * FROM tenant_links")
	if err == nil {
		t.Error("expected error when org context is missing")
	}

	// 2. Direct Base Table Access Rejection
	ctx := plugin.WithOrgID(context.Background(), 1)
	_, _, err = tenantsql.Execute(ctx, db, reg, "SELECT * FROM links")
	if err == nil {
		t.Error("expected error when querying raw base table 'links'")
	}

	// 3. Unknown View Rejection
	_, _, err = tenantsql.Execute(ctx, db, reg, "SELECT * FROM tenant_unknown")
	if err == nil {
		t.Error("expected error when querying unregistered view")
	}

	// 4. Disallowed Statements Rejection
	mutations := []string{
		"INSERT INTO tenant_links (slug) VALUES ('test')",
		"UPDATE tenant_links SET slug = 'foo'",
		"DELETE FROM tenant_links",
		"DROP VIEW tenant_links",
		"CREATE TABLE evil (id int)",
	}
	for _, sqlStmt := range mutations {
		_, _, err = tenantsql.Execute(ctx, db, reg, sqlStmt)
		if err == nil {
			t.Errorf("expected error for mutation SQL: %q", sqlStmt)
		}
	}

	// 5. Disallowed Function Rejection
	_, _, err = tenantsql.Execute(ctx, db, reg, "SELECT load_extension('foo') FROM tenant_links")
	if err == nil {
		t.Error("expected error for disallowed function load_extension")
	}
}

func TestExecute_RowAndByteLimits(t *testing.T) {
	db, reg := setupTestDB(t)

	// Seed 5 links for Org 1
	for i := 0; i < 5; i++ {
		db.Create(&links.Link{
			OrgID:  1,
			Slug:   fmt.Sprintf("slug-%d", i),
			Target: "https://example.com",
		})
	}

	ctx := plugin.WithOrgID(context.Background(), 1)

	// Test MaxRows limit and Truncated flag
	rows, meta, err := tenantsql.Execute(ctx, db, reg, "SELECT slug FROM tenant_links ORDER BY slug ASC", tenantsql.WithMaxRows(3))
	if err != nil {
		t.Fatalf("query with max rows failed: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("expected 3 rows, got %d", len(rows))
	}
	if !meta.Truncated {
		t.Error("expected Truncated to be true when exceeding maxRows")
	}

	// Test MaxBytes limit
	_, _, err = tenantsql.Execute(ctx, db, reg, "SELECT slug FROM tenant_links", tenantsql.WithMaxBytes(20))
	if err == nil {
		t.Error("expected error when query exceeds MaxBytes limit")
	}
}

type fakeDialector struct {
	gorm.Dialector
	name string
}

func (f fakeDialector) Name() string {
	return f.name
}

func TestExecute_DialectEnforcement(t *testing.T) {
	fakeDB := &gorm.DB{
		Config: &gorm.Config{
			Dialector: fakeDialector{name: "mysql"},
		},
	}

	ctx := plugin.WithOrgID(context.Background(), 1)
	reg := tenantsql.NewRegistry()
	_, _, err := tenantsql.Execute(ctx, fakeDB, reg, "SELECT 1")
	if err == nil || err.Error() != "dialect not supported: mysql" {
		t.Fatalf("expected dialect not supported error, got %v", err)
	}
}

func TestExecute_NilDBAndNilRegistry(t *testing.T) {
	ctx := plugin.WithOrgID(context.Background(), 1)
	_, _, err := tenantsql.Execute(ctx, nil, nil, "SELECT 1")
	if err == nil {
		t.Error("expected error for nil DB")
	}

	db, _ := setupTestDB(t)
	// Using DefaultRegistry when reg is nil
	_ = tenantsql.DefaultRegistry().Register(plugin.TenantView{
		Name:       "tenant_default_reg_test",
		Definition: func(orgID uint) string { return "SELECT 1 AS col" },
	})
	rows, _, err := tenantsql.Execute(ctx, db, nil, "SELECT col FROM tenant_default_reg_test")
	if err != nil {
		t.Fatalf("execute with nil reg failed: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(rows))
	}
}

func TestExecute_MaterializationFailure(t *testing.T) {
	db, _ := setupTestDB(t)
	reg := tenantsql.NewRegistry()
	_ = reg.Register(plugin.TenantView{
		Name:       "tenant_broken_view",
		Definition: func(orgID uint) string { return "INVALID SQL SYNTAX HERE" },
	})

	ctx := plugin.WithOrgID(context.Background(), 1)
	_, _, err := tenantsql.Execute(ctx, db, reg, "SELECT * FROM tenant_broken_view")
	if err == nil {
		t.Error("expected error for invalid view materialization")
	}
}

func TestExecute_ContextCancellation(t *testing.T) {
	db, reg := setupTestDB(t)

	db.Create(&links.Link{OrgID: 1, Slug: "cancel-test", Target: "https://example.com"})

	ctx, cancel := context.WithCancel(plugin.WithOrgID(context.Background(), 1))
	cancel() // cancel immediately

	_, _, err := tenantsql.Execute(ctx, db, reg, "SELECT * FROM tenant_links")
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}
