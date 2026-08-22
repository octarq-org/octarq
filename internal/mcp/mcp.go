// Package mcp implements octarq's Model Context Protocol server for the
// `octarq mcp` subcommand and for the networked (HTTP/SSE) transports. It
// exposes read-only tools over the core models, every one of them scoped to a
// single tenant.
//
// There is deliberately no general-purpose raw-SQL tool. A raw SELECT reaches
// arbitrary tables, so no owner_id predicate can be injected into it and no
// content denylist can stand in for one (plugin.ValidateReadOnlyQuery is a
// content filter, not an authorization boundary — it leaves the analytics tables
// queryable, and `emails` has no owner_id column at all). The capability is
// removed from the tool surface rather than gated by a flag, so no call site can
// re-enable it by mistake.
//
// Scope is the open-core surface only. Write tools, and Finance/Infra tools,
// belong to the Pro plugins — don't add them here.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/cache"
	"github.com/octarq-org/octarq/internal/db"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

// errNoOrgInContext refuses a tool call that arrives without a tenant scope.
// Every transport supplies one — the networked ones from the caller's API token,
// the stdio CLI from its entry point — so an absent org means something is wrong,
// and defaulting it to a tenant would hand that tenant's data to whoever asked.
var errNoOrgInContext = errors.New("no workspace in this request")

// version is reported to MCP clients in the server handshake.
const version = "0.1.0"

// stdioOrgID is the tenant the stdio CLI operates as. `octarq mcp` is launched by
// the operator on their own machine against their own database, so there is one
// tenant and it is the bootstrap org.
const stdioOrgID uint = 1

// server bundles the dependencies the tool handlers share.
type server struct {
	gdb    *gorm.DB
	orgID  uint // tenant scope for the tools (defaults to 1 for Stdio CLI, dynamically set via HTTP tokens for remote SSE)
	lookup func(name string) (any, bool)
}

// Run loads configuration, opens the database read-only-style, builds the MCP
// server with every tool registered, and serves over stdio until ctx is
// cancelled or stdin closes. It is the body of the `octarq mcp` subcommand.
func Run(ctx context.Context) error {
	return RunWithPlugins(ctx, nil)
}

// NewNetworkedServerInstance builds an MCP server for a networked transport
// (HTTP/SSE), where the caller is one tenant among many and orgID comes from
// their API token. Only tenant-scoped tools exist on any transport; see the
// package doc for why raw SQL is not one of them.
//
// It also never Mounts the plugins. They are shared instances, already mounted
// at app boot with per-request resolvers; re-Mounting them here — once per
// connection, with this connection's org — would overwrite those resolvers
// process-wide and silently repoint every other tenant's requests at this org.
// Tools get the caller's org from the request context instead, via
// plugin.OrgIDFromContext. lookup is the app's own service registry, which
// Mounting used to (re)build; it is required, not optional, so a caller cannot
// quietly drop it and lose the export tools.
func NewNetworkedServerInstance(gdb *gorm.DB, orgID uint, plugins []plugin.Plugin, lookup func(string) (any, bool)) *mcp.Server {
	return buildServerInstance(gdb, orgID, plugins, lookup, false)
}

// NewServerInstance builds an MCP server for a single operator, scoped to
// orgID, with the given plugins mounted the way app boot mounts them. The
// networked counterpart is NewNetworkedServerInstance. Both expose the same
// tenant-scoped tool set; there is no transport that gets extra reach.
func NewServerInstance(gdb *gorm.DB, orgID uint, plugins []plugin.Plugin) *mcp.Server {
	return buildServerInstance(gdb, orgID, plugins, nil, true)
}

// buildServerInstance is the single chokepoint that constructs an MCP server.
func buildServerInstance(gdb *gorm.DB, orgID uint, plugins []plugin.Plugin, lookup func(string) (any, bool), mountPlugins bool) *mcp.Server {
	lookupFn := lookup
	if mountPlugins {
		reg := plugin.NewRegistry()
		cacheBackend := cache.New("")
		pctx := &plugin.Context{
			DB:      gdb,
			OrgID:   func(_ *http.Request) uint { return orgID },
			Provide: reg.Provide,
			Lookup:  reg.Lookup,
			Cache:   cache.NewScoped(cacheBackend, "mcp"),
			// RequirePerm is intentionally left nil: MCP requests do not carry per-request HTTP user identity.
			// Authorization is handled by the MCP layer itself; plugin callers using HasPerm must tolerate nil.
		}
		for _, p := range plugins {
			pctxCopy := *pctx
			pctxCopy.Cache = cache.NewScoped(cacheBackend, p.Name())
			p.Mount(nil, &pctxCopy)
		}
		if lookupFn == nil {
			lookupFn = reg.Lookup
		}
	}

	s := &server{gdb: gdb, orgID: orgID, lookup: lookupFn}

	impl := &mcp.Implementation{Name: "octarq", Version: version}
	opts := &mcp.ServerOptions{
		Instructions: "octarq is a self-hosted one-person-company backend. These tools " +
			"read/write short links, email, and domains. Everything is scoped to the " +
			"caller's own workspace.",
	}
	srv := mcp.NewServer(impl, opts)
	s.registerTools(srv)

	for _, p := range plugins {
		if mp, ok := p.(plugin.MCPProvider); ok {
			mp.RegisterMCP(srv)
		}
	}
	return srv
}

// RunWithPlugins is identical to Run but lets the caller supply registered
// Pro plugins so they can register their custom MCP write or finance tools.
//
// It is the stdio entry point: the caller is the single local operator, and the
// tools are scoped to the bootstrap org (stdioOrgID).
func RunWithPlugins(ctx context.Context, plugins []plugin.Plugin) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("mcp: load config: %w", err)
	}
	gdb, err := db.Open(cfg)
	if err != nil {
		return fmt.Errorf("mcp: open db: %w", err)
	}

	// Stdio CLI: single local operator, org 1.
	//
	// The org goes on the context, exactly as the networked transports put the
	// caller's org there. That is what lets every tool read its scope from one
	// place and refuse when it is absent: without this, stdio would be the sole
	// caller arriving with no org, and each tool would need a "default to 1"
	// fallback — a fallback that is harmless here and a cross-tenant read over
	// the network. One line at the entry point removes the need for all of them.
	ctx = plugin.WithOrgID(ctx, stdioOrgID)
	srv := NewServerInstance(gdb, stdioOrgID, plugins)
	return srv.Run(ctx, &mcp.StdioTransport{})
}

// registerTools wires every tool onto the server. Every tool registered here
// must be scoped to a single tenant. Do not add a tool that takes caller-supplied
// SQL: an arbitrary SELECT cannot carry an owner_id predicate, which is why the
// former query_db_readonly tool no longer exists on any transport.
func (s *server) registerTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "export_data",
		Description: "Export the operator's data for one resource type (links, emails, domains, mailboxes) as JSON — for backup and data sovereignty.",
	}, s.exportData)
}

// jsonResult marshals v to pretty JSON and wraps it as an MCP text result,
// returning v as the structured output too.
func jsonResult[T any](v T) (*mcp.CallToolResult, any, error) {
	buf, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(buf)}},
	}, v, nil
}
