package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	mcpToolsMu sync.Mutex
	mcpTools   = make(map[string]string)
)

// AddMCPTool registers an MCP tool and panics if the tool name is already registered by another component.
// Note on naming: The MCP protocol specification restricts tool names to ^[a-zA-Z0-9_-]{1,64}$.
// Therefore, we use double-underscore fallback (e.g., octarq_identity__x) instead of dotted names (octarq.identity.x)
// to ensure compatibility with strict MCP clients like Claude Desktop and Cursor.
//
// Invariant (Fail-Closed): If the request context is marked as an Agent service account (IsAgentFromContext),
// the tool name MUST be present in AllowedToolsFromContext. If unauthorized, execution is immediately rejected
// with a 403 FORBIDDEN_AGENT_TOOL_DISALLOWED AgentError and an audit warning log without calling the underlying handler.
func AddMCPTool[In, Out any](srv *mcp.Server, owner string, t *mcp.Tool, h mcp.ToolHandlerFor[In, Out]) {
	mcpToolsMu.Lock()
	if existing, exists := mcpTools[t.Name]; exists && existing != owner {
		mcpToolsMu.Unlock()
		panic(fmt.Sprintf("mcp: tool conflict: %q already registered by %q", t.Name, existing))
	}
	mcpTools[t.Name] = owner
	mcpToolsMu.Unlock()

	wrapped := func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		if IsAgentFromContext(ctx) {
			allowed := AllowedToolsFromContext(ctx)
			if !slices.Contains(allowed, t.Name) {
				slog.Warn("mcp: agent tool disallowed", "tool", t.Name, "allowed", allowed)
				var zero Out
				return nil, zero, NewAgentError(
					403,
					"FORBIDDEN_AGENT_TOOL_DISALLOWED",
					fmt.Sprintf("tool %q is not authorized for this agent token", t.Name),
					"Contact your administrator to add this tool to the agent token's allowed_tools whitelist.",
					false,
				)
			}
		}
		return h(ctx, req, in)
	}

	mcp.AddTool(srv, t, wrapped)
}
