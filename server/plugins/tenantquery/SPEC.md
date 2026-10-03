# Plugin Specification: Tenant Query (`plugins/tenantquery`)

## 1. Architectural Overview & Role
The `tenantquery` plugin (`plugins/tenantquery`) provides safe, read-only SQL view description and execution capabilities for AI agents and analytics engines, enforcing tenant data isolation across the Octarq backend.

- **Plugin Identifier**: `"tenantquery"` (`plugin.go:26`)
- **Plugin Metadata**:
  - Title: `"Tenant Query"`
  - Description: `"Read-only tenant SQL views and query engine for AI and analytics."`
  - Category: `plugin.CategoryUtilities`
  - Core: `true`
  - Enabled By Default: `true`
- **Implemented Interfaces**:
  - `plugin.Plugin` (`plugin.go:16`)
  - `plugin.Describer` (`plugin.go:17`)
- **Binary Embedding Guarantee**: Co-located at `server/plugins/tenantquery/SPEC.md`. Strictly excluded from binary embedding (Tier 2 engineering specification; zero binary bloat, zero secret leak).

---

## 2. Exact Go Types & Structs

### 2.1 Plugin Struct (`plugins/tenantquery/plugin.go:9-13`)
```go
type Plugin struct {
	db       *gorm.DB
	registry *tenantsql.Registry
}
```

### 2.2 Compile-Time Interface Assertions (`plugins/tenantquery/plugin.go:15-18`)
```go
var (
	_ plugin.Plugin    = (*Plugin)(nil)
	_ plugin.Describer = (*Plugin)(nil)
)
```

### 2.3 Endpoint Data Transfer Objects (`plugins/tenantquery/routes.go:12-44`)
```go
type ColumnSchema struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Sensitive   bool   `json:"sensitive,omitempty"`
}

type ViewSchema struct {
	Name    string         `json:"name"`
	Columns []ColumnSchema `json:"columns"`
}

type DescribeTenantSchemaInput struct{}

type DescribeTenantSchemaOutput struct {
	Views []ViewSchema `json:"views"`
}

type QueryTenantSQLInput struct {
	SQL string `json:"sql"`
}

type QueryTenantSQLOutput struct {
	Rows      []map[string]any `json:"rows"`
	RowCount  int              `json:"rowCount"`
	Truncated bool             `json:"truncated"`
}
```

---

## 3. Endpoints & MCP Tool Registration

Both endpoints are registered via `plugin.RegisterEndpoint` with full Huma OpenAPI documentation and MCP server exposure:

| Endpoint Name | HTTP Method & Path | MCP Tool Name | Risk Level | Auth Required | Expose MCP | Description |
|---|---|---|---|---|---|---|
| `describe_tenant_schema` | `GET /api/tenant/schema` | `octarq_infra__describe_tenant_schema` | `plugin.RiskLevelRead` | `true` | `true` | Enumerates available `tenant_*` views, columns, data types, and sensitivity indicators. |
| `query_tenant_sql` | `POST /api/tenant/query` | `octarq_infra__query_db_readonly` | `plugin.RiskLevelRead` | `true` | `true` | Executes read-only SQL queries scoped to `tenant_*` views with strict tenant isolation. |

---

## 4. Security & Safety Model

### 4.1 SQL AST Validation & Mutation Prevention
- **Strict Read-Only Enforcement**: Only `SELECT` statements are permitted. Statements attempting mutations (`INSERT`, `UPDATE`, `DELETE`, `DROP`, `ALTER`, `TRUNCATE`, `ATTACH`, `DETACH`, `PRAGMA`, etc.) are rejected during AST parsing before reaching the database engine.
- **Allowed Table Whitelist**: Queries may only reference virtual views registered in `tenantsql.Registry` (e.g. `tenant_links`, `tenant_clicks`, `tenant_domains`, `tenant_mailboxes`). Direct access to raw physical tables (such as `users`, `settings`, `sessions`, `api_keys`, or credentials) is strictly rejected with a fail-closed error listing all permitted views.

### 4.2 Multi-Tenant Boundary Isolation
- **Tenant Context Assertion**: The execution layer derives the authenticated organization ID from the request context (`plugin.OrgIDFromContext(ctx)` or session context).
- **Scope Injection**: Queries against registered views automatically inject tenant boundary predicates (`WHERE org_id = ?`) to guarantee cross-tenant query isolation at the database level.
- **Fail-Closed Missing Context**: If a tenant ID cannot be established from the execution context, queries fail closed immediately without executing SQL.

### 4.3 Sensitive Field Protection
- **Sensitivity Flags**: Columns flagged as sensitive either in the view schema or in `plugin.SensitiveColumns` are marked `Sensitive: true` in `describe_tenant_schema`.
- **Result Masking**: Internal credentials, password hashes, and encryption keys are excluded from tenant views by design.

### 4.4 Resource & Denial-of-Service Guards
- **Statement Timeout**: Execution queries are bounded by database context deadlines.
- **Row Limits**: Output row counts are capped; when results exceed the safety ceiling, `Truncated: true` is reported with the actual row count returned.
