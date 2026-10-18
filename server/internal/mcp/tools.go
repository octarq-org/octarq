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
	"fmt"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/plugin"
)

type exportInput struct {
	Resource string `json:"resource" jsonschema:"Resource type to export: links, emails, domains, mailboxes, all, or zip_url"`
}

func (s *server) exportData(ctx context.Context, _ *mcp.CallToolRequest, in exportInput) (*mcp.CallToolResult, any, error) {
	if auth.IsAgentFromContext(ctx) {
		allowed := auth.AllowedToolsFromContext(ctx)
		if !slices.Contains(allowed, "octarq_infra__export_data") {
			return nil, nil, plugin.NewAgentError(
				403,
				"FORBIDDEN_AGENT_TOOL_DISALLOWED",
				fmt.Sprintf("tool %q is not authorized for this agent token", "octarq_infra__export_data"),
				"Contact your administrator to add this tool to the agent token's allowed_tools whitelist.",
				false,
			)
		}
	}
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, nil, errNoOrgInContext
	}
	if in.Resource == "all" || in.Resource == "zip_url" {
		var linksCount, domainsCount, mailboxesCount, emailsCount int64
		if s.gdb != nil {
			s.gdb.Table("links").Where("owner_id = ?", orgID).Count(&linksCount)
			s.gdb.Table("domains").Where("owner_id = ?", orgID).Count(&domainsCount)
			s.gdb.Table("mailboxes").Where("owner_id = ?", orgID).Count(&mailboxesCount)
			mailboxIDs := s.gdb.Table("mailboxes").Where("owner_id = ?", orgID).Select("id")
			s.gdb.Table("emails").Where("mailbox_id IN (?)", mailboxIDs).Count(&emailsCount)
		}
		var downloadURL string
		if s.lookup != nil {
			if fn, ok := plugin.LookupServiceAs[plugin.ExportSignedURLFunc](s.lookup, plugin.ServiceExportSignedURL); ok {
				downloadURL, _ = fn(orgID)
			}
		}
		if downloadURL == "" {
			downloadURL = fmt.Sprintf("/api/account/export?org=%d", orgID)
		}
		res := map[string]any{
			"summary": map[string]int64{
				"links":     linksCount,
				"domains":   domainsCount,
				"mailboxes": mailboxesCount,
				"emails":    emailsCount,
			},
			"downloadUrl": downloadURL,
			"expiresIn":   "15m",
			"resource":    in.Resource,
		}
		return jsonResultAny(res)
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
		Content: []mcp.Content{&mcp.TextContent{Text: "unknown resource; use one of: links, emails, domains, mailboxes, all, zip_url"}},
	}, nil, nil
}

// jsonResultAny is jsonResult for the `any`-typed export handler.
func jsonResultAny(v any) (*mcp.CallToolResult, any, error) {
	r, _, err := jsonResult(v)
	return r, v, err
}
