// Tool handlers for the octarq MCP server. Each is a read-only projection over the
// core models, scoped to the operator's tenant. They deliberately omit secret
// fields (passwords, raw bodies) — the model never needs them and the roadmap's
// guardrails forbid leaking them.
package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/plugin"
)

type queryInput struct {
	Query string `json:"query"` // a single read-only SELECT / WITH…SELECT statement
}

type queryOutput struct {
	Columns []string         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
	Count   int              `json:"count"`
	// Truncated is true when the result hit the row cap.
	Truncated bool `json:"truncated"`
}

func (s *server) queryDBReadonly(ctx context.Context, _ *mcp.CallToolRequest, in queryInput) (*mcp.CallToolResult, any, error) {
	cols, rows, err := s.runReadOnlyQuery(ctx, in.Query)
	if err != nil {
		// Audit the rejected/failed attempt too — the audit trail is exactly
		// where a hallucinated or unsafe query should be visible.
		s.auditQuery(in.Query, 0, err)
		// Return the error as tool content (isError) rather than a transport
		// error, so the model can read and adjust its query.
		msg := fmt.Sprintf("query rejected or failed: %v", err)
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: msg}},
		}, queryOutput{}, nil
	}
	s.auditQuery(in.Query, len(rows), nil)
	out := queryOutput{Columns: cols, Rows: rows, Count: len(rows), Truncated: len(rows) >= maxRows}
	return jsonResult(out)
}

type exportInput struct {
	Resource string `json:"resource"` // one of: links, emails, domains, mailboxes
}

func (s *server) exportData(ctx context.Context, _ *mcp.CallToolRequest, in exportInput) (*mcp.CallToolResult, any, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, nil, errNoOrgInContext
	}
	if s.lookup != nil {
		// A resource whose plugin isn't mounted falls through to the
		// unknown-resource reply below; see plugin.MCPExporter.
		if fn, ok := plugin.LookupServiceAs[plugin.MCPExporter](s.lookup, plugin.MCPExportServiceName(in.Resource)); ok {
			res, err := fn(ctx, orgID)
			if err != nil {
				return nil, nil, err
			}
			return jsonResultAny(res)
		}
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: "unknown resource; use one of: links, emails, domains, mailboxes"}},
	}, nil, nil
}

// jsonResultAny is jsonResult for the `any`-typed export handler.
func jsonResultAny(v any) (*mcp.CallToolResult, any, error) {
	r, _, err := jsonResult(v)
	return r, v, err
}
