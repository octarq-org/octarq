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
