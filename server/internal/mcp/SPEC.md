# Module Specification: Model Context Protocol Provider (`server/internal/mcp`)

## 1. Architectural Overview & Role
The `internal/mcp` package implements Octarq's Model Context Protocol (MCP) provider. It serves LLM agent integrations via two distinct transport lifecycles:
1. **Local CLI Transport (`octarq mcp`)**: Runs over stdio for the single local machine operator, scoped to bootstrap workspace `stdioOrgID = 1`.
2. **Networked Remote Transports (HTTP / SSE)**: Multi-tenant server instances constructed per connection via `NewNetworkedServerInstance`.

### Key Responsibilities & Invariants:
- **Tenant Scope Enforcement**: All tool and resource calls are strictly bound to a tenant org ID via `plugin.OrgIDFromContext(ctx)`. Absent workspace IDs in networked context immediately fail with `errNoOrgInContext` ("no workspace in this request").
- **Zero General-Purpose Raw-SQL Invariant**: General raw SELECT queries are deliberately forbidden. Raw queries cannot safely enforce `owner_id` predicates across join boundaries or tables lacking tenant columns (`emails`).
- **Networked Mount Isolation**: Plugins are mounted once at bootstrap. Networked MCP instances **never re-mount plugins**; re-mounting per connection would overwrite process-wide resolvers and leak cross-tenant context.
- **Resource Subscriptions & Live Updates**: Supports MCP push notifications (`notifications/resources/updated`) for streaming trending links and new incoming mailbox arrivals.
- **Binary Embedding Guarantee**: Co-located at `server/internal/mcp/SPEC.md`. Strictly excluded from binary embedding.

---

## 2. Go API & Interface Contracts

### 2.1 Constructors
```go
// Single-operator stdio construction (mounts plugins locally)
func NewServerInstance(gdb *gorm.DB, orgID uint, plugins []plugin.Plugin) *mcp.Server

// Multi-tenant networked construction (consumes shared registry, does NOT re-mount)
func NewNetworkedServerInstance(gdb *gorm.DB, orgID uint, plugins []plugin.Plugin, lookup func(string) (any, bool)) *mcp.Server

// CLI stdio runner
func Run(ctx context.Context) error
func RunWithPlugins(ctx context.Context, plugins []plugin.Plugin) error
```

### 2.2 Plugin Capability Assertion
Plugins offering custom MCP tools implement `plugin.MCPProvider`:
```go
type MCPProvider interface {
    RegisterMCP(srv *mcp.Server)
}
```

---

## 3. Canonical MCP Resources

| URI Template | Title | Description |
|---|---|---|
| `octarq://metrics/traffic` | Traffic Metrics | Read-only shortlink traffic and redirection statistics for the workspace |
| `octarq://domains/health` | Domains Health | Read-only DNS routing status and domain health overview |
| `octarq://links/trending` | Trending Links | Workspace links ranked by click velocity |
| `octarq://mailboxes/{id}/latest` | Mailbox Latest | Newest MIME message arrival notification resource |

---

## 4. Multi-Tenant Safety & Fail-Closed Guardrails

1. **Resolver Precedence**:
   - Primary: `plugin.OrgIDFromContext(ctx)` (extracted from authenticated API token).
   - Fallback: Instance `server.orgID` (valid only for Stdio operator).
   - Fail-Closed: If resolved `orgID == 0`, tool execution is aborted with `errNoOrgInContext`.
2. **Subscription Isolation**:
   - `subscriptionTracker` maintains thread-safe sets of active watcher URIs.
   - Resource updates are published strictly to subscribed sessions watching exact tenant URIs.
