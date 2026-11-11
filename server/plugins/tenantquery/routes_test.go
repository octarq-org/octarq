package tenantquery_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/internal/endpoint"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/internal/tenantsql"
	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugins/tenantquery"
	"gorm.io/gorm"
)

type queryTestLink struct {
	ID        uint `gorm:"primaryKey"`
	OrgID     uint `gorm:"column:owner_id"`
	Slug      string
	Target    string
	Password  string
	Title     string
	Tags      string
	Clicks    int64
	Archived  bool
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (queryTestLink) TableName() string { return "links" }

type queryTestEmail struct {
	ID        uint `gorm:"primaryKey"`
	MailboxID uint
	Subject   string
	Text      string
}

func (queryTestEmail) TableName() string { return "emails" }

func registerQueryTestViews(pctx *plugin.Context) {
	pctx.RegisterTenantView(plugin.TenantView{
		Name: "tenant_links",
		Columns: []plugin.TenantColumn{
			{Name: "id", Type: "integer", Description: "Short link unique identifier"},
			{Name: "owner_id", Type: "integer", Description: "Owning organization/workspace ID"},
			{Name: "slug", Type: "text", Description: "Short link URL slug path"},
			{Name: "target", Type: "text", Description: "Destination redirect URL"},
			{Name: "password", Type: "text", Description: "Link password protection (redacted)"},
		},
		Sensitive: []string{"password"},
		Definition: func(orgID uint) string {
			return fmt.Sprintf("SELECT id, owner_id, slug, target, password FROM links WHERE owner_id = %d", orgID)
		},
	})

	pctx.RegisterTenantView(plugin.TenantView{
		Name: "tenant_emails",
		Columns: []plugin.TenantColumn{
			{Name: "id", Type: "integer", Description: "Email message unique identifier"},
			{Name: "mailbox_id", Type: "integer", Description: "Associated mailbox identifier"},
			{Name: "subject", Type: "text", Description: "Email subject line (redacted)"},
			{Name: "text", Type: "text", Description: "Plain text email body content (redacted)"},
		},
		Sensitive: []string{"subject", "text"},
		Definition: func(orgID uint) string {
			return fmt.Sprintf("SELECT id, mailbox_id, subject, text FROM emails WHERE mailbox_id = %d", orgID)
		},
	})
}

func setupTestApp(t *testing.T) (*gorm.DB, *endpoint.Engine) {
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

	if err := db.AutoMigrate(
		&queryTestLink{},
		&queryTestEmail{},
		&models.AuditLog{},
	); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}

	engine := endpoint.NewEngine()
	pctx := &plugin.Context{
		DB:               db,
		RegisterEndpoint: engine.Register,
		RegisterTenantView: func(v plugin.TenantView) {
			if err := tenantsql.DefaultRegistry().Register(v); err != nil && !strings.Contains(err.Error(), "already registered") {
				t.Fatalf("failed to register tenant view %s: %v", v.Name, err)
			}
		},
	}

	registerQueryTestViews(pctx)

	tq := tenantquery.New()
	tq.Mount(nil, pctx)

	return db, engine
}

func TestTenantQuery_PluginInfo(t *testing.T) {
	p := tenantquery.New()
	if p.Name() != "tenantquery" {
		t.Errorf("Name() = %q, want tenantquery", p.Name())
	}
	desc := p.Describe()
	if desc.Title == "" {
		t.Error("expected non-empty Title")
	}
	if desc.Icon == "" {
		t.Error("expected non-empty Icon")
	}
	if p.Models() != nil {
		t.Error("expected Models() to be nil")
	}
}

func findEndpoint(t *testing.T, engine *endpoint.Engine, name string) plugin.Endpoint {
	t.Helper()
	for _, ep := range engine.Endpoints() {
		if ep.EndpointName() == name {
			return ep
		}
	}
	t.Fatalf("endpoint %q not registered", name)
	return nil
}

func TestTenantQuery_DescribeViews(t *testing.T) {
	_, engine := setupTestApp(t)

	descEP := findEndpoint(t, engine, "describe_tenant_schema")
	res, err := descEP.Execute(context.Background(), tenantquery.DescribeTenantSchemaInput{})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	out, ok := res.(*tenantquery.DescribeTenantSchemaOutput)
	if !ok {
		t.Fatalf("expected *tenantquery.DescribeTenantSchemaOutput, got %T", res)
	}

	if len(out.Views) == 0 {
		t.Fatal("expected at least one view described")
	}

	var foundLinks, foundEmails bool
	for _, v := range out.Views {
		if v.Name == "tenant_links" {
			foundLinks = true
			if len(v.Columns) == 0 {
				t.Error("tenant_links columns should not be empty")
			}
			for _, c := range v.Columns {
				if c.Name == "password" && !c.Sensitive {
					t.Error("tenant_links.password column should be marked sensitive")
				}
			}
		}
		if v.Name == "tenant_emails" {
			foundEmails = true
			if len(v.Columns) == 0 {
				t.Error("tenant_emails columns should not be empty")
			}
		}
	}

	if !foundLinks || !foundEmails {
		t.Errorf("foundLinks=%v, foundEmails=%v", foundLinks, foundEmails)
	}
}

func TestTenantQuery_QueryTenantSQL(t *testing.T) {
	db, engine := setupTestApp(t)

	// Seed link
	db.Create(&queryTestLink{
		OrgID:    1,
		Slug:     "test-route-slug",
		Target:   "https://route.example.com",
		Password: "route-password-123",
	})

	queryEP := findEndpoint(t, engine, "query_tenant_sql")
	ctx := plugin.WithOrgID(context.Background(), 1)

	// Test 1: Successful valid query
	res, err := queryEP.Execute(ctx, tenantquery.QueryTenantSQLInput{
		SQL: "SELECT slug, target FROM tenant_links",
	})
	if err != nil {
		t.Fatalf("valid query failed: %v", err)
	}

	qRes, ok := res.(*tenantquery.QueryTenantSQLOutput)
	if !ok {
		t.Fatalf("expected *tenantquery.QueryTenantSQLOutput, got %T", res)
	}

	if len(qRes.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(qRes.Rows))
	}

	if qRes.Rows[0]["slug"] != "test-route-slug" {
		t.Errorf("expected slug 'test-route-slug', got %v", qRes.Rows[0]["slug"])
	}

	// Test 2: Sensitive column masking
	res, err = queryEP.Execute(ctx, tenantquery.QueryTenantSQLInput{
		SQL: "SELECT password FROM tenant_links",
	})
	if err != nil {
		t.Fatalf("query with password column failed: %v", err)
	}
	qRes, ok = res.(*tenantquery.QueryTenantSQLOutput)
	if !ok {
		t.Fatalf("expected *tenantquery.QueryTenantSQLOutput, got %T", res)
	}
	if len(qRes.Rows) > 0 && qRes.Rows[0]["password"] != plugin.RedactedValue {
		t.Errorf("expected password to be redacted, got %v", qRes.Rows[0]["password"])
	}

	// Test 3: Disallowed mutation query
	_, err = queryEP.Execute(ctx, tenantquery.QueryTenantSQLInput{
		SQL: "DROP TABLE links",
	})
	if err == nil {
		t.Error("expected error for non-SELECT query, got nil")
	}

	// Test 4: Missing query parameter
	_, err = queryEP.Execute(ctx, tenantquery.QueryTenantSQLInput{})
	if err == nil {
		t.Error("expected error for missing query param, got nil")
	}
}
