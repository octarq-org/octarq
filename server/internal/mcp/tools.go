// Tool handlers for the octarq MCP server. Each one is a read-only projection
// over the core models and resolves its tenant from the request context
// (plugin.OrgIDFromContext), refusing the call when no workspace is present —
// that is what makes the "tenant-scoped" claim true rather than aspirational.
// They deliberately omit secret fields (passwords, raw bodies).
//
// A handler that cannot be scoped to one tenant does not belong here. The former
// query_db_readonly handler ran caller-supplied SQL with no owner_id predicate
// and was removed for exactly that reason; see the package doc in mcp.go.
package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/plugin"
)

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
