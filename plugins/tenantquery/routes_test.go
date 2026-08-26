package tenantquery_test

import (
	"context"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/endpoint"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/tenantsql"
	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/plugins/links"
	"github.com/octarq-org/octarq/plugins/mail"
	"github.com/octarq-org/octarq/plugins/tenantquery"
	"gorm.io/gorm"
)

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
		&links.Link{},
		&links.LinkEvent{},
		&mail.Mailbox{},
		&mail.Email{},
		&models.AuditLog{},
	); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}

	engine := endpoint.NewEngine()
	pctx := &plugin.Context{
		DB:               db,
		RegisterEndpoint: engine.Register,
		RegisterTenantView: func(v plugin.TenantView) {
			_ = tenantsql.DefaultRegistry().Register(v)
		},
	}

	links.RegisterViews(pctx)
	mail.RegisterViews(pctx)

	tq := tenantquery.New()
	tq.Mount(nil, pctx)

	return db, engine
}

func TestTenantQuery_PluginInfo(t *testing.T) {
	p := tenantquery.New()
	if p.Name() != "tenantquery" {
		t.Errorf("Name() = %q, want tenantquery", p.Name())
	}
	if p.Models() != nil {
		t.Errorf("Models() = %v, want nil", p.Models())
	}
	desc := p.Describe()
	if !desc.Core {
		t.Error("expected Core to be true")
	}
	if !desc.EnabledByDefault {
		t.Error("expected EnabledByDefault to be true")
	}
}

func TestTenantQuery_DescribeTenantSchema(t *testing.T) {
	_, engine := setupTestApp(t)

	var describeEP plugin.Endpoint
	for _, ep := range engine.Endpoints() {
		if ep.EndpointName() == "describe_tenant_schema" {
			describeEP = ep
			break
		}
	}

	if describeEP == nil {
		t.Fatal("describe_tenant_schema endpoint not registered")
	}

	if !describeEP.EndpointExposeMCP() {
		t.Error("expected ExposeMCP = true")
	}
	if !describeEP.EndpointRequireAuth() {
		t.Error("expected RequireAuth = true")
	}

	ctx := plugin.WithOrgID(context.Background(), 1)
	res, err := describeEP.Execute(ctx, tenantquery.DescribeTenantSchemaInput{})
	if err != nil {
		t.Fatalf("describe handler failed: %v", err)
	}

	out, ok := res.(*tenantquery.DescribeTenantSchemaOutput)
	if !ok || out == nil {
		t.Fatalf("unexpected output type: %T", res)
	}

	if len(out.Views) < 3 {
		t.Fatalf("expected at least 3 views (tenant_links, tenant_link_events, tenant_emails), got %d", len(out.Views))
	}

	// Verify sensitive column marking
	var foundLinks, foundEmails bool
	for _, v := range out.Views {
		if v.Name == "tenant_links" {
			foundLinks = true
			for _, col := range v.Columns {
				if col.Name == "password" && !col.Sensitive {
					t.Error("expected password column to have Sensitive: true")
				}
			}
		}
		if v.Name == "tenant_emails" {
			foundEmails = true
			for _, col := range v.Columns {
				if (col.Name == "text" || col.Name == "html" || col.Name == "subject") && !col.Sensitive {
					t.Errorf("expected %s column to have Sensitive: true", col.Name)
				}
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
	db.Create(&links.Link{
		OrgID:    1,
		Slug:     "test-route-slug",
		Target:   "https://route.example.com",
		Password: "route-password-123",
	})

	var queryEP plugin.Endpoint
	for _, ep := range engine.Endpoints() {
		if ep.EndpointName() == "query_tenant_sql" {
			queryEP = ep
			break
		}
	}

	if queryEP == nil {
		t.Fatal("query_tenant_sql endpoint not registered")
	}

	if !queryEP.EndpointExposeMCP() {
		t.Error("expected ExposeMCP = true")
	}
	if !queryEP.EndpointRequireAuth() {
		t.Error("expected RequireAuth = true")
	}

	// Valid query
	ctx := plugin.WithOrgID(context.Background(), 1)
	res, err := queryEP.Execute(ctx, tenantquery.QueryTenantSQLInput{
		SQL: "SELECT slug, password FROM tenant_links",
	})
	if err != nil {
		t.Fatalf("query handler failed: %v", err)
	}

	out, ok := res.(*tenantquery.QueryTenantSQLOutput)
	if !ok || out == nil {
		t.Fatalf("unexpected output type: %T", res)
	}

	if out.RowCount != 1 || len(out.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", out.RowCount)
	}
	if out.Rows[0]["slug"] != "test-route-slug" {
		t.Errorf("got slug %v, want test-route-slug", out.Rows[0]["slug"])
	}
	if out.Rows[0]["password"] != plugin.RedactedValue {
		t.Errorf("got password %v, want %q", out.Rows[0]["password"], plugin.RedactedValue)
	}

	// Zero rows query
	resZero, err := queryEP.Execute(ctx, tenantquery.QueryTenantSQLInput{
		SQL: "SELECT slug FROM tenant_links WHERE slug = 'nonexistent'",
	})
	if err != nil {
		t.Fatalf("zero rows query failed: %v", err)
	}
	outZero := resZero.(*tenantquery.QueryTenantSQLOutput)
	if outZero.RowCount != 0 || len(outZero.Rows) != 0 {
		t.Errorf("expected 0 rows, got %d", outZero.RowCount)
	}

	// Invalid query should include self-correction guidance listing available views
	_, err = queryEP.Execute(ctx, tenantquery.QueryTenantSQLInput{
		SQL: "SELECT * FROM links",
	})
	if err == nil {
		t.Fatal("expected error on invalid SQL query")
	}
	if !strings.Contains(err.Error(), "available views:") {
		t.Errorf("expected error message to contain self-correction 'available views:', got: %v", err)
	}
	if !strings.Contains(err.Error(), "tenant_links") {
		t.Errorf("expected error message to mention 'tenant_links', got: %v", err)
	}
}
