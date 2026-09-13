package tenantquery

import (
	"context"
	"fmt"
	"strings"

	"github.com/octarq-org/octarq/internal/tenantsql"
	"github.com/octarq-org/octarq/plugin"
)

// ColumnSchema describes a column in a tenant view for describe_tenant_schema output.
type ColumnSchema struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Sensitive   bool   `json:"sensitive,omitempty"`
}

// ViewSchema describes a tenant view for describe_tenant_schema output.
type ViewSchema struct {
	Name    string         `json:"name"`
	Columns []ColumnSchema `json:"columns"`
}

// DescribeTenantSchemaInput is the input for describe_tenant_schema.
type DescribeTenantSchemaInput struct{}

// DescribeTenantSchemaOutput is the output for describe_tenant_schema.
type DescribeTenantSchemaOutput struct {
	Views []ViewSchema `json:"views"`
}

// QueryTenantSQLInput is the input for query_tenant_sql.
type QueryTenantSQLInput struct {
	SQL string `json:"sql"`
}

// QueryTenantSQLOutput is the output for query_tenant_sql.
type QueryTenantSQLOutput struct {
	Rows      []map[string]any `json:"rows"`
	RowCount  int              `json:"rowCount"`
	Truncated bool             `json:"truncated"`
}

func (p *Plugin) registerRoutes(ctx *plugin.Context) {
	_ = plugin.RegisterEndpoint(ctx, plugin.EndpointSpec[DescribeTenantSchemaInput, DescribeTenantSchemaOutput]{
		Name:        "describe_tenant_schema",
		Method:      "GET",
		Path:        "/api/tenant/schema",
		Summary:     "Describe Tenant Schema",
		Description: "Returns the list of available tenant_* views and their columns with data types and sensitivity flags for SQL generation.",
		RiskLevel:   plugin.RiskLevelRead,
		RequireAuth: true,
		ExposeMCP:   true,
		Handler:     p.describeTenantSchema,
	})

	_ = plugin.RegisterEndpoint(ctx, plugin.EndpointSpec[QueryTenantSQLInput, QueryTenantSQLOutput]{
		Name:        "query_tenant_sql",
		Method:      "POST",
		Path:        "/api/tenant/query",
		Summary:     "Query Tenant SQL",
		Description: "Execute a read-only SQL query against tenant_* views with automatic tenant isolation.",
		RiskLevel:   plugin.RiskLevelRead,
		RequireAuth: true,
		ExposeMCP:   true,
		Handler:     p.queryTenantSQL,
	})
}

func (p *Plugin) describeTenantSchema(ctx context.Context, _ DescribeTenantSchemaInput) (*DescribeTenantSchemaOutput, error) {
	reg := p.registry
	if reg == nil {
		reg = tenantsql.DefaultRegistry()
	}
	views := reg.List()
	out := &DescribeTenantSchemaOutput{
		Views: make([]ViewSchema, 0, len(views)),
	}

	for _, v := range views {
		sensitiveMap := make(map[string]bool)
		for _, s := range v.Sensitive {
			sensitiveMap[strings.ToLower(strings.TrimSpace(s))] = true
		}

		cols := make([]ColumnSchema, 0, len(v.Columns))
		for _, col := range v.Columns {
			colNameLower := strings.ToLower(strings.TrimSpace(col.Name))
			isSensitive := sensitiveMap[colNameLower] || plugin.SensitiveColumns[colNameLower]
			cols = append(cols, ColumnSchema{
				Name:        col.Name,
				Type:        col.Type,
				Description: col.Description,
				Sensitive:   isSensitive,
			})
		}

		out.Views = append(out.Views, ViewSchema{
			Name:    v.Name,
			Columns: cols,
		})
	}

	return out, nil
}

func (p *Plugin) queryTenantSQL(ctx context.Context, in QueryTenantSQLInput) (*QueryTenantSQLOutput, error) {
	reg := p.registry
	if reg == nil {
		reg = tenantsql.DefaultRegistry()
	}

	rows, meta, err := tenantsql.Execute(ctx, p.db, reg, in.SQL)
	if err != nil {
		views := reg.List()
		viewNames := make([]string, len(views))
		for i, v := range views {
			viewNames[i] = v.Name
		}
		availMsg := fmt.Sprintf("available views: %s", strings.Join(viewNames, ", "))
		return nil, fmt.Errorf("%w (%s)", err, availMsg)
	}

	if rows == nil {
		rows = []map[string]any{}
	}

	return &QueryTenantSQLOutput{
		Rows:      rows,
		RowCount:  meta.RowCount,
		Truncated: meta.Truncated,
	}, nil
}
