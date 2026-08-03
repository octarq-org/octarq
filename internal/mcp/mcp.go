// Package mcp implements octarq's Model Context Protocol server for the
// `octarq mcp` subcommand. It exposes short link, email, and domain read-only
// tools and a guarded read-only SQL tool over stdio transport.
//
// Scope is the open-core surface only. Write tools, and Finance/Infra tools,
// belong to the Pro plugins — don't add them here.
package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/config"
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
	gdb   *gorm.DB
	orgID uint // tenant scope for the tools (defaults to 1 for Stdio CLI, dynamically set via HTTP tokens for remote SSE)
	// rawSQLEnabled records whether the general-purpose query_db_readonly tool
	// was actually registered on this instance. It exists so the networked-
	// transport invariant (raw SQL is never exposed over HTTP/SSE) is testable.
	rawSQLEnabled bool
	lookup        func(name string) (any, bool)
}

// Run loads configuration, opens the database read-only-style, builds the MCP
// server with every tool registered, and serves over stdio until ctx is
// cancelled or stdin closes. It is the body of the `octarq mcp` subcommand.
func Run(ctx context.Context) error {
	return RunWithPlugins(ctx, nil)
}

// RunWithPlugins is identical to Run but lets the caller supply registered
// Pro plugins so they can register their custom MCP write or finance tools.
//
// allowRawSQL gates the general-purpose query_db_readonly tool. It must be true
// ONLY for the single-operator stdio transport (`octarq mcp`), where the caller has
// local access to the whole database anyway. Over the HTTP/SSE transports the
// caller is one tenant among many (orgID comes from their API token), and raw
// SQL cannot be safely scoped to a single owner_id — so it is never registered
// there. The tenant-scoped convenience tools remain available on every transport.
// NewNetworkedServerInstance builds an MCP server for a networked transport
// (HTTP/SSE), where the caller is one tenant among many (orgID comes from their
// API token) and raw SQL cannot be safely scoped to a single owner_id. It hard-
// wires allowRawSQL=false so the raw-SQL tool can NEVER be exposed over the
// network: the invariant is enforced in code, not by convention at the call
// site. All networked callers MUST use this constructor.
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
	srv, _ := buildServerInstance(gdb, orgID, plugins, false, lookup, false)
	return srv
}

func NewServerInstance(gdb *gorm.DB, orgID uint, plugins []plugin.Plugin, allowRawSQL bool) *mcp.Server {
	srv, _ := buildServerInstance(gdb, orgID, plugins, allowRawSQL, nil, true)
	return srv
}

// buildServerInstance is the single chokepoint that constructs an MCP server. It
// returns the internal *server so the raw-SQL invariant can be asserted in
// tests. rawSQLEnabled on the returned *server reflects what was actually wired.
func buildServerInstance(gdb *gorm.DB, orgID uint, plugins []plugin.Plugin, allowRawSQL bool, lookup func(string) (any, bool), mountPlugins bool) (*mcp.Server, *server) {
	lookupFn := lookup
	if mountPlugins {
		reg := plugin.NewRegistry()
		pctx := &plugin.Context{
			DB:      gdb,
			OrgID:   func(_ *http.Request) uint { return orgID },
			Provide: reg.Provide,
			Lookup:  reg.Lookup,
		}
		for _, p := range plugins {
			p.Mount(nil, pctx)
		}
		if lookupFn == nil {
			lookupFn = reg.Lookup
		}
	}

	s := &server{gdb: gdb, orgID: orgID, lookup: lookupFn}

	impl := &mcp.Implementation{Name: "octarq", Version: version}
	opts := &mcp.ServerOptions{
		Instructions: "octarq is a self-hosted one-person-company backend. These tools " +
			"read/write short links, email, and domains, plus run guarded read-only SQL. " +
			"Everything is scoped to the operator's data.",
	}
	srv := mcp.NewServer(impl, opts)
	s.registerTools(srv, allowRawSQL)

	// Register any plugin-supplied MCP tools.
	for _, p := range plugins {
		if mp, ok := p.(plugin.MCPProvider); ok {
			mp.RegisterMCP(srv)
		}
	}
	return srv, s
}

func RunWithPlugins(ctx context.Context, plugins []plugin.Plugin) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("mcp: load config: %w", err)
	}
	gdb, err := db.Open(cfg)
	if err != nil {
		return fmt.Errorf("mcp: open db: %w", err)
	}

	// Stdio CLI: single local operator, org 1, and raw SQL is allowed (the caller
	// already has full local DB access).
	//
	// The org goes on the context, exactly as the networked transports put the
	// caller's org there. That is what lets every tool read its scope from one
	// place and refuse when it is absent: without this, stdio would be the sole
	// caller arriving with no org, and each tool would need a "default to 1"
	// fallback — a fallback that is harmless here and a cross-tenant read over
	// the network. One line at the entry point removes the need for all of them.
	ctx = plugin.WithOrgID(ctx, stdioOrgID)
	srv := NewServerInstance(gdb, stdioOrgID, plugins, true)
	return srv.Run(ctx, &mcp.StdioTransport{})
}

// registerTools wires every tool onto the server. The general-purpose raw-SQL
// tool is registered only when allowRawSQL is set (stdio transport); see
// NewServerInstance for why it is withheld from the multi-tenant HTTP transports.
func (s *server) registerTools(srv *mcp.Server, allowRawSQL bool) {
	// The general-purpose raw-SQL tool is registered ONLY on the single-operator
	// stdio transport (allowRawSQL). Over a networked transport the caller is one
	// tenant among many and raw SQL cannot be scoped to a single owner_id, so it
	// is withheld. This is a hard invariant enforced here in code; the networked
	// constructor (NewNetworkedServerInstance) can never reach this branch.
	if allowRawSQL {
		mcp.AddTool(srv, &mcp.Tool{
			Name: "query_db_readonly",
			Description: "Run an arbitrary read-only SQL SELECT against octarq's database and return rows as JSON. " +
				"Use this to compute any metric the dedicated tools don't cover (click trends, spend, mail volume…). " +
				"Only a single SELECT/WITH query is allowed; writes, PRAGMA and ATTACH are rejected; results are row-capped " +
				"and sensitive columns (password hashes, token hashes, encrypted credentials, raw email bodies) are redacted. " +
				"Tables include: links, link_events, mailboxes, emails, domains, provider_accounts, tokens, notification_channels.",
		}, s.queryDBReadonly)
		s.rawSQLEnabled = true
	}

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "export_data",
		Description: "Export the operator's data for one resource type (links, emails, domains, mailboxes) as JSON — for backup and data sovereignty.",
	}, s.exportData)
}

// --- shared helpers ---

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

// runReadOnlyQuery validates and executes a SELECT, returning the result rows as
// generic maps with sensitive columns redacted. It runs inside a read-only
// transaction so the database connection rejects any (defensively impossible)
// write the validator missed.
func (s *server) runReadOnlyQuery(ctx context.Context, query string) (cols []string, rows []map[string]any, err error) {
	safe, err := validateReadOnlyQuery(query)
	if err != nil {
		return nil, nil, err
	}

	err = s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		sqlRows, qerr := tx.Raw(safe).Rows()
		if qerr != nil {
			return qerr
		}
		defer sqlRows.Close()

		cols, qerr = sqlRows.Columns()
		if qerr != nil {
			return qerr
		}
		for sqlRows.Next() {
			if len(rows) >= maxRows {
				break
			}
			holders := make([]any, len(cols))
			for i := range holders {
				holders[i] = new(any)
			}
			if scanErr := sqlRows.Scan(holders...); scanErr != nil {
				return scanErr
			}
			row := make(map[string]any, len(cols))
			for i, c := range cols {
				row[c] = normalizeSQLValue(*(holders[i].(*any)))
			}
			redactRow(cols, row)
			rows = append(rows, row)
		}
		return sqlRows.Err()
	}, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, nil, err
	}
	return cols, rows, nil
}

// normalizeSQLValue turns driver byte slices into strings so JSON output is
// readable rather than base64.
func normalizeSQLValue(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}
