// Package mcp implements led's Model Context Protocol server: the `led mcp`
// subcommand. It turns the existing self-hosted "one-person company" database
// into something an AI assistant (Claude Code, Claude Desktop, Cursor) can read
// directly — "how many SaaS did I pay for this month?", "what mail landed
// today?", "which links got the most clicks?".
//
// It is the OSS引流神器 (lead-magnet) the AI roadmap calls for: the data and the
// Bearer-token API already exist, so MCP is a thin, read-only protocol-translation
// layer over them. Scope here is the open-core surface — short links, email,
// domains — plus a guarded general-purpose read-only SQL tool. Write tools and
// Finance/Infra tools belong to the Pro plugins.
//
// Transport is stdio (the universal local MCP transport): a client launches
// `led mcp` as a subprocess and speaks JSON-RPC over stdin/stdout. The server is
// built on the official MCP Go SDK so we don't hand-roll the protocol.
package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Jungley8/led/config"
	"github.com/Jungley8/led/internal/db"
	"github.com/Jungley8/led/plugin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"
)

// version is reported to MCP clients in the server handshake.
const version = "0.1.0"

// server bundles the dependencies the tool handlers share.
type server struct {
	gdb   *gorm.DB
	orgID uint // tenant scope for the tools (defaults to 1 for Stdio CLI, dynamically set via HTTP tokens for remote SSE)
}

// Run loads configuration, opens the database read-only-style, builds the MCP
// server with every tool registered, and serves over stdio until ctx is
// cancelled or stdin closes. It is the body of the `led mcp` subcommand.
func Run(ctx context.Context) error {
	return RunWithPlugins(ctx, nil)
}

// RunWithPlugins is identical to Run but lets the caller supply registered
// Pro plugins so they can register their custom MCP write or finance tools.
func NewServerInstance(gdb *gorm.DB, orgID uint, plugins []plugin.Plugin) *mcp.Server {
	s := &server{gdb: gdb, orgID: orgID}

	impl := &mcp.Implementation{Name: "led", Version: version}
	opts := &mcp.ServerOptions{
		Instructions: "led is a self-hosted one-person-company backend. These tools " +
			"read/write short links, email, and domains, plus run guarded read-only SQL. " +
			"Everything is scoped to the operator's data.",
	}
	srv := mcp.NewServer(impl, opts)
	s.registerTools(srv)

	// Register any plugin-supplied MCP tools.
	for _, p := range plugins {
		if mp, ok := p.(plugin.MCPProvider); ok {
			mp.RegisterMCP(srv)
		}
	}
	return srv
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

	srv := NewServerInstance(gdb, 1, plugins) // Default to org 1 for Stdio CLI
	return srv.Run(ctx, &mcp.StdioTransport{})
}

// registerTools wires every tool onto the server.
func (s *server) registerTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_links",
		Description: "List short links with their click counts. Optionally filter by host or tag, and limit the count.",
	}, s.listLinks)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_mailboxes",
		Description: "List email mailboxes with their unread counts.",
	}, s.listMailboxes)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_emails",
		Description: "List recently received emails (subject, from, to, date) — optionally for one mailbox. Bodies are not returned; use query_db_readonly for more, or get the full message via the dashboard.",
	}, s.listEmails)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_domains",
		Description: "List managed domains and what each is used for (mail / links).",
	}, s.listDomains)

	mcp.AddTool(srv, &mcp.Tool{
		Name: "query_db_readonly",
		Description: "Run an arbitrary read-only SQL SELECT against led's database and return rows as JSON. " +
			"Use this to compute any metric the dedicated tools don't cover (click trends, spend, mail volume…). " +
			"Only a single SELECT/WITH query is allowed; writes, PRAGMA and ATTACH are rejected; results are row-capped " +
			"and sensitive columns (password hashes, token hashes, encrypted credentials, raw email bodies) are redacted. " +
			"Tables include: links, link_events, mailboxes, emails, domains, provider_accounts, tokens, notification_channels.",
	}, s.queryDBReadonly)

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

// clampLimit bounds a user-supplied limit to a sane window.
func clampLimit(n, def int) int {
	if n <= 0 {
		return def
	}
	if n > maxRows {
		return maxRows
	}
	return n
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

// ownerScope returns a WHERE-ready owner_id value; convenience tools use it to
// stay within the operator's tenant.
func (s *server) ownerScope() uint {
	if s.orgID == 0 {
		return 1
	}
	return s.orgID
}

// trimToTag reports whether a comma-separated tags field contains tag.
func tagsContain(field, tag string) bool {
	if tag == "" {
		return true
	}
	for _, t := range strings.Split(field, ",") {
		if strings.EqualFold(strings.TrimSpace(t), tag) {
			return true
		}
	}
	return false
}
