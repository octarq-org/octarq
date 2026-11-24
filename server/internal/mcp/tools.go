package mcp

import (
	"context"
	"fmt"
	"reflect"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/plugin"
)

type exportInput struct {
	Resource string `json:"resource" jsonschema:"Resource to export: a plugin-declared resource name, all, or zip_url"`
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
		summary := map[string]int64{}
		for _, p := range s.plugins {
			fn, ok := plugin.LookupServiceAs[plugin.ExportFunc](s.lookup, plugin.ExportServiceName(p.Name()))
			if !ok {
				continue
			}
			for key, v := range fn(orgID) {
				if n, ok := exportItemCount(v); ok {
					summary[key] = n
				}
			}
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
			"summary":     summary,
			"downloadUrl": downloadURL,
			"expiresIn":   "15m",
			"resource":    in.Resource,
		}
		return jsonResultAny(res)
	}
	if s.lookup != nil {
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
		Content: []mcp.Content{&mcp.TextContent{Text: "unknown resource; use a plugin-declared resource name, all, or zip_url"}},
	}, nil, nil
}

func exportItemCount(v any) (int64, bool) {
	if v == nil {
		return 0, false
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return 0, false
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map:
		return int64(rv.Len()), true
	default:
		return 0, false
	}
}

func jsonResultAny(v any) (*mcp.CallToolResult, any, error) {
	r, _, err := jsonResult(v)
	return r, v, err
}
