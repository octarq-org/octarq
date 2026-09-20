package plugin

import (
	"fmt"
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
func AddMCPTool[In, Out any](srv *mcp.Server, owner string, t *mcp.Tool, h mcp.ToolHandlerFor[In, Out]) {
	mcpToolsMu.Lock()
	if existing, exists := mcpTools[t.Name]; exists && existing != owner {
		mcpToolsMu.Unlock()
		panic(fmt.Sprintf("mcp: tool conflict: %q already registered by %q", t.Name, existing))
	}
	mcpTools[t.Name] = owner
	mcpToolsMu.Unlock()

	mcp.AddTool(srv, t, h)
}
