package tenantquery_test

import (
	"context"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugins/tenantquery"
)

// Compile-time interface checks.
var (
	_ plugin.Plugin    = (*tenantquery.Plugin)(nil)
	_ plugin.Describer = (*tenantquery.Plugin)(nil)
)

func TestTenantQuery_Characterization_Metadata(t *testing.T) {
	p := tenantquery.New()

	if p.Name() != "tenantquery" {
		t.Fatalf("expected name %q, got %q", "tenantquery", p.Name())
	}

	if p.Models() != nil {
		t.Fatalf("expected Models() == nil, got %v", p.Models())
	}

	info := p.Describe()
	if info.Title != "Tenant Query" {
		t.Errorf("expected Title %q, got %q", "Tenant Query", info.Title)
	}
	if info.Category != plugin.CategoryUtilities {
		t.Errorf("expected Category %q, got %q", plugin.CategoryUtilities, info.Category)
	}
	if !info.Core {
		t.Error("expected Core to be true")
	}
	if !info.EnabledByDefault {
		t.Error("expected EnabledByDefault to be true")
	}
	if info.Description == "" {
		t.Error("expected non-empty description")
	}
}

func TestTenantQuery_Characterization_Endpoints(t *testing.T) {
	_, engine := setupTestApp(t)

	var (
		describeEP plugin.Endpoint
		queryEP    plugin.Endpoint
	)

	for _, ep := range engine.Endpoints() {
		switch ep.EndpointName() {
		case "describe_tenant_schema":
			describeEP = ep
		case "query_tenant_sql":
			queryEP = ep
		}
	}

	if describeEP == nil {
		t.Fatal("describe_tenant_schema endpoint was not registered")
	}
	if queryEP == nil {
		t.Fatal("query_tenant_sql endpoint was not registered")
	}

	// Verify describe_tenant_schema endpoint specifications
	if describeEP.EndpointMethod() != "GET" {
		t.Errorf("describe_tenant_schema method = %q, want GET", describeEP.EndpointMethod())
	}
	if describeEP.EndpointPath() != "/api/tenant/schema" {
		t.Errorf("describe_tenant_schema path = %q, want /api/tenant/schema", describeEP.EndpointPath())
	}
	if !describeEP.EndpointExposeMCP() {
		t.Error("describe_tenant_schema ExposeMCP must be true")
	}
	if !describeEP.EndpointRequireAuth() {
		t.Error("describe_tenant_schema RequireAuth must be true")
	}

	// Verify query_tenant_sql endpoint specifications
	if queryEP.EndpointMethod() != "POST" {
		t.Errorf("query_tenant_sql method = %q, want POST", queryEP.EndpointMethod())
	}
	if queryEP.EndpointPath() != "/api/tenant/query" {
		t.Errorf("query_tenant_sql path = %q, want /api/tenant/query", queryEP.EndpointPath())
	}
	if !queryEP.EndpointExposeMCP() {
		t.Error("query_tenant_sql ExposeMCP must be true")
	}
	if !queryEP.EndpointRequireAuth() {
		t.Error("query_tenant_sql RequireAuth must be true")
	}
}

func TestTenantQuery_Characterization_FailClosedOnMissingContext(t *testing.T) {
	_, engine := setupTestApp(t)

	var queryEP plugin.Endpoint
	for _, ep := range engine.Endpoints() {
		if ep.EndpointName() == "query_tenant_sql" {
			queryEP = ep
			break
		}
	}
	if queryEP == nil {
		t.Fatal("query_tenant_sql endpoint not found")
	}

	// Context without OrgID must fail closed
	_, err := queryEP.Execute(context.Background(), tenantquery.QueryTenantSQLInput{
		SQL: "SELECT * FROM tenant_links",
	})
	if err == nil {
		t.Fatal("expected error executing query without tenant context, got nil")
	}
}
